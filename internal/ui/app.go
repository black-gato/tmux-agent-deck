package ui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg struct{}

type Model struct {
	conn           *sql.DB
	tmuxC          tmux.ClientIface
	poller         *state.Poller
	groups         []db.Group
	sessions       []db.Session
	items          []ListItem
	cursor         int
	width          int
	height         int
	mode           string // "", "new-session", "new-group", "rename", "move"
	dialog         dialogState
	err            error
	PendingAttach  string // tmux session name to attach after TUI exits
	viewFull       bool
	panes          []tmux.Pane
	output         string
	activePaneIdx  int
	waitingSince   map[string]time.Time
	overdueWaiting int
}

func NewModel(conn *sql.DB, tc tmux.ClientIface, poller *state.Poller) *Model {
	return &Model{conn: conn, tmuxC: tc, poller: poller}
}

func (m *Model) Reload() error {
	now := time.Now()
	if m.poller != nil {
		now = m.poller.Now()
	}
	groups, err := db.ListGroups(m.conn)
	if err != nil {
		return err
	}
	sessions, err := db.ListSessions(m.conn)
	if err != nil {
		return err
	}
	m.groups = groups
	m.sessions = sessions
	m.items = BuildTree(groups, sessions)
	m.waitingSince = nil
	m.overdueWaiting = 0
	if m.poller != nil {
		m.waitingSince = m.poller.WaitingSinceSnapshot()
		for i := range m.items {
			if m.items[i].Kind != "session" || m.items[i].Session.Status != tmux.StatusWaiting {
				continue
			}
			since, ok := m.waitingSince[m.items[i].Session.ID]
			if !ok {
				continue
			}
			m.items[i].WaitLabel = formatElapsed(now.Sub(since))
			if now.Sub(since) > 30*time.Second {
				m.overdueWaiting++
			}
		}
	}
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
	m.panes = nil
	m.output = ""
	m.activePaneIdx = 0
	if m.tmuxC != nil && m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
		s := m.items[m.cursor].Session
		if s.TmuxSession != "" {
			if panes, err := m.tmuxC.ListPanes(s.TmuxSession); err == nil {
				m.panes = panes
			}
			if out, err := m.tmuxC.CapturePaneOutput(s.TmuxSession); err == nil {
				m.output = out
			}
		}
	}
	return nil
}

func (m *Model) Items() []ListItem   { return m.items }
func (m *Model) Cursor() int         { return m.cursor }
func (m *Model) Mode() string        { return m.mode }
func (m *Model) Panes() []tmux.Pane  { return m.panes }
func (m *Model) Output() string      { return m.output }
func (m *Model) ViewFull() bool      { return m.viewFull }
func (m *Model) ActivePaneIdx() int  { return m.activePaneIdx }
func (m *Model) OverdueWaiting() int { return m.overdueWaiting }

func (m *Model) Init() tea.Cmd {
	if err := m.Reload(); err != nil {
		m.err = err
	}
	return tick()
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if err := m.Reload(); err != nil {
			m.err = err
		}
		return m, tick()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.mode != "" {
			return m.updateDialog(msg)
		}
		return m.updateNavigation(msg)
	}
	return m, nil
}

func (m *Model) updateNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := actionForKey(msg)
	switch action {
	case "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "toggle":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "group" {
			g := m.items[m.cursor].Group
			db.SetGroupExpanded(m.conn, g.Path, !g.Expanded)
			m.Reload()
		}
	case "attach":
		if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Kind == "session" {
				tmuxName, err := m.ensureStarted(item.Session)
				if err != nil {
					m.err = err
					break
				}
				m.PendingAttach = tmuxName
				if m.poller != nil {
					m.poller.Stop()
				}
				return m, tea.Quit
			}
		}
	case "new-session":
		m.mode = "new-session"
		m.dialog = newDialogState("Session title:")
	case "new-group":
		m.mode = "new-group"
		m.dialog = newDialogState("Group path (e.g. work/frontend):")
	case "rename":
		if m.cursor < len(m.items) {
			m.mode = "rename"
			m.dialog = newDialogState("New name:")
		}
	case "move":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "move"
			m.dialog = newDialogState("Move to group path:")
		}
	case "delete":
		if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Kind == "session" {
				db.DeleteSession(m.conn, item.Session.ID)
			} else if item.Kind == "group" && item.Group.Path != defaultGroupPath {
				db.DeleteGroup(m.conn, item.Group.Path)
			}
			m.Reload()
		}
	case "toggle-full":
		m.viewFull = !m.viewFull
	case "edit-notes":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "edit-notes"
			m.dialog = dialogState{prompt: "", value: m.items[m.cursor].Session.Notes}
		}
	case "cycle-pane":
		if len(m.panes) > 0 {
			m.activePaneIdx = (m.activePaneIdx + 1) % len(m.panes)
		}
	case "send-pane":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "send-pane"
			m.dialog = newDialogState("Send:")
		}
	case "fork-session":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "fork-session"
			m.dialog = newDialogState("Fork title:")
		}
	case "broadcast":
		if m.cursor < len(m.items) {
			m.mode = "broadcast"
			m.dialog = dialogState{scopeLabels: [2]string{"this group", "all sub-groups"}}
		}
	case "quit":
		if m.poller != nil {
			m.poller.Stop()
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) View() string {
	if m.err != nil {
		return "error: " + m.err.Error()
	}

	leftW := int(float64(m.width) * 0.35)
	if leftW < 10 {
		leftW = 10
	}
	rightW := m.width - leftW - 1
	if rightW < 10 {
		rightW = 10
	}
	contentH := m.height - 3
	if contentH < 1 {
		contentH = 1
	}

	header := m.renderAppHeader()
	footer := renderFooter()

	if m.viewFull {
		sep := strings.Repeat("─", m.width)
		detail := m.RenderDetailPanel(m.width, contentH)
		return header + "\n" + sep + "\n" + detail + "\n" + footer
	}

	leftContent := RenderList(m.items, m.cursor, leftW, contentH)
	var rightContent string
	if m.mode != "" && m.mode != "edit-notes" {
		rightContent = m.renderDialog()
	} else {
		rightContent = m.RenderDetailPanel(rightW, contentH)
	}

	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	sep := strings.Repeat("─", leftW) + "┬" + strings.Repeat("─", rightW)

	var bodyLines []string
	for i := 0; i < contentH; i++ {
		var left, right string
		if i < len(leftLines) {
			left = leftLines[i]
		}
		if i < len(rightLines) {
			right = rightLines[i]
		}
		bodyLines = append(bodyLines, padRight(left, leftW)+"│"+right)
	}

	return header + "\n" + sep + "\n" + strings.Join(bodyLines, "\n") + "\n" + footer
}

func (m *Model) renderAppHeader() string {
	var running, waiting, idle, errs int
	for _, s := range m.sessions {
		switch s.Status {
		case "running":
			running++
		case "waiting":
			waiting++
		case "idle":
			idle++
		case "error":
			errs++
		}
	}
	header := fmt.Sprintf(" Agent Deck  ● %d running  ○ %d waiting  ◐ %d idle  ✕ %d error", running, waiting, idle, errs)
	if m.overdueWaiting > 0 {
		header += fmt.Sprintf("  !%d", m.overdueWaiting)
	}
	return header
}

func renderFooter() string {
	return " Enter Attach  x Send  f Fork  b Broadcast  v Output  e Notes  n New  d Delete  q Quit"
}

func tick() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Second)
		return tickMsg{}
	}
}

func (m *Model) RenderDetailPanel(w, h int) string {
	if m.cursor >= len(m.items) || m.items[m.cursor].Kind != "session" {
		return ""
	}
	s := m.items[m.cursor].Session

	var lines []string

	lines = append(lines, sectionHeader("SESSION", w))

	sym := statusSymbol[s.Status]
	if sym == "" {
		sym = "—"
	}
	statusText := sym
	if s.Status == tmux.StatusWaiting {
		if since, ok := m.waitingSince[s.ID]; ok {
			now := time.Now()
			if m.poller != nil {
				now = m.poller.Now()
			}
			statusText = fmt.Sprintf("%s %s", sym, formatElapsed(now.Sub(since)))
		}
	}
	lines = append(lines, fmt.Sprintf(" %s  %s", s.Title, statusText))
	lines = append(lines, fmt.Sprintf(" group: %s", s.GroupPath))
	lines = append(lines, " "+renderPaneList(m.panes, m.activePaneIdx))

	const sessionHeaderLines = 4
	const notesLines = 5
	outputH := h - sessionHeaderLines - notesLines - 1
	if outputH < 0 {
		outputH = 0
	}
	lines = append(lines, sectionHeader("OUTPUT", w))
	outputTail := tailLines(m.output, outputH)
	for _, ol := range outputTail {
		lines = append(lines, " "+truncate(ol, w-1))
	}
	for i := len(outputTail); i < outputH; i++ {
		lines = append(lines, "")
	}

	lines = append(lines, sectionHeader("NOTES", w))
	var noteText string
	if s.Notes != "" {
		noteText = s.Notes
	} else {
		noteText = "No notes"
	}
	noteRunes := []rune(noteText)
	for row := 0; row < 3; row++ {
		start := row * (w - 1)
		if start >= len(noteRunes) {
			lines = append(lines, "")
			continue
		}
		end := start + (w - 1)
		if end > len(noteRunes) {
			end = len(noteRunes)
		}
		lines = append(lines, " "+string(noteRunes[start:end]))
	}
	if m.mode == "edit-notes" {
		lines = append(lines, " > "+m.dialog.value)
	} else {
		lines = append(lines, " e edit")
	}

	return strings.Join(lines, "\n")
}

func sectionHeader(title string, width int) string {
	dashes := width - len([]rune(title)) - 2
	if dashes < 0 {
		dashes = 0
	}
	return title + " " + strings.Repeat("─", dashes)
}

func renderPaneList(panes []tmux.Pane, activeIdx int) string {
	if len(panes) == 0 {
		return ""
	}
	var parts []string
	for i, p := range panes {
		entry := fmt.Sprintf("[%d] %s", p.Index, p.Command)
		if i == activeIdx {
			parts = append(parts, selectedStyle.Render(entry))
		} else {
			parts = append(parts, dimStyle.Render(entry))
		}
	}
	return strings.Join(parts, "  ")
}

func tailLines(output string, n int) []string {
	if n <= 0 || output == "" {
		return nil
	}
	all := strings.Split(output, "\n")
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	if len(all) <= n {
		return all
	}
	return all[len(all)-n:]
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%dh", int(d/time.Hour))
}

func padRight(s string, width int) string {
	visual := lipgloss.Width(s)
	if visual >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visual)
}

// ensureStarted returns the tmux session name for s, spawning one if needed.
func (m *Model) ensureStarted(s *db.Session) (string, error) {
	if s.TmuxSession != "" {
		exists, err := m.tmuxC.SessionExists(s.TmuxSession)
		if err == nil && exists {
			return s.TmuxSession, nil
		}
	}
	tmuxName := fmt.Sprintf("ad-%s", s.ID[:8])
	if err := m.tmuxC.NewSession(tmuxName, s.ProjectPath, s.Tool); err != nil {
		return "", fmt.Errorf("start session: %w", err)
	}
	_ = db.UpdateSessionTmuxName(m.conn, s.ID, tmuxName)
	_ = db.UpdateSessionStatus(m.conn, s.ID, "waiting")
	return tmuxName, nil
}

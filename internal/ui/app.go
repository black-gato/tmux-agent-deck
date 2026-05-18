package ui

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg struct{}

var (
	headerWaitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // amber
	headerErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	headerOverdueStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

type Model struct {
	conn                 *sql.DB
	tmuxC                tmux.ClientIface
	poller               *state.Poller
	groups               []db.Group
	sessions             []db.Session
	items                []ListItem
	cursor               int
	width                int
	height               int
	mode                 string // "", "new-session", "new-group", "rename", "move"
	dialog               dialogState
	err                  error
	PendingAttach        string // tmux session name to attach after TUI exits
	PendingStartupScript string
	viewFull             bool
	panes                []tmux.Pane
	output               string
	activePaneIdx        int
	waitingSince         map[string]time.Time
	overdueWaiting       int
	selected             map[string]bool
	showArchived         bool
	searchQuery          string
	contextPct           map[string]*int
	navigateToGroup      string
}

func NewModel(conn *sql.DB, tc tmux.ClientIface, poller *state.Poller) *Model {
	return &Model{
		conn:     conn,
		tmuxC:    tc,
		poller:   poller,
		selected: make(map[string]bool),
	}
}

func (m *Model) currentGroupPath() string {
	if m.cursor >= len(m.items) {
		return defaultGroupPath
	}
	item := m.items[m.cursor]
	switch item.Kind {
	case "group":
		return item.Group.Path
	case "session":
		return item.Session.GroupPath
	default:
		return defaultGroupPath
	}
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
	visibleSessions := m.visibleSessions(sessions)
	m.items = BuildTree(m.visibleGroups(groups, visibleSessions), visibleSessions)
	m.pruneSelection()
	for i := range m.items {
		if m.items[i].Kind == "session" && m.selected[m.items[i].Session.ID] {
			m.items[i].Selected = true
		}
		if m.items[i].Kind == "session" && m.isConductorSession(m.items[i].Session) {
			m.items[i].IsConductor = true
		}
	}
	m.waitingSince = nil
	m.overdueWaiting = 0
	m.contextPct = nil
	if m.poller != nil {
		m.waitingSince = m.poller.WaitingSinceSnapshot()
		m.contextPct = m.poller.ContextPctSnapshot()
		for i := range m.items {
			if m.items[i].Kind != "session" {
				continue
			}
			id := m.items[i].Session.ID
			m.items[i].ContextPct = m.contextPct[id]
			if m.items[i].Session.Status != tmux.StatusWaiting {
				continue
			}
			since, ok := m.waitingSince[id]
			if !ok {
				continue
			}
			m.items[i].WaitLabel = formatElapsed(now.Sub(since))
			if now.Sub(since) > 30*time.Second {
				m.overdueWaiting++
				m.items[i].WaitOverdue = true
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
func (m *Model) SelectedCount() int  { return len(m.selected) }

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
		if m.mode == "help" {
			return m.updateHelp(msg)
		}
		if m.mode != "" {
			return m.updateDialog(msg)
		}
		return m.updateNavigation(msg)
	}
	return m, nil
}

func (m *Model) updateNavigation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc && m.searchQuery != "" {
		m.searchQuery = ""
		if err := m.Reload(); err != nil {
			m.err = err
		}
		return m, nil
	}
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
		} else if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			id := m.items[m.cursor].Session.ID
			if m.selected[id] {
				delete(m.selected, id)
			} else {
				m.selected[id] = true
			}
			m.items[m.cursor].Selected = m.selected[id]
		}
	case "attach":
		if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Kind == "session" {
				tmuxName, isNew, err := m.ensureStarted(item.Session)
				if err != nil {
					m.err = err
					break
				}
				m.PendingAttach = tmuxName
				if isNew && item.Session.StartupScript != "" {
					m.PendingStartupScript = item.Session.StartupScript
				}
				if m.poller != nil {
					m.poller.Stop()
				}
				return m, tea.Quit
			}
		}
	case "new-session":
		defaultPath := "."
		if wd, err := os.Getwd(); err == nil {
			defaultPath = wd
		}
		defaultTool := "claude"
		cursorGroupPath := m.currentGroupPath()
		for _, g := range m.groups {
			if g.Path == cursorGroupPath {
				if g.DefaultPath != "" {
					defaultPath = g.DefaultPath
				}
				if g.DefaultTool != "" {
					defaultTool = g.DefaultTool
				}
				break
			}
		}
		toolOptions := []string{"claude", "claude-dangerous", "aider", "cursor", "bash", "custom"}
		toolIdx := 0
		for i, t := range toolOptions {
			if t == defaultTool {
				toolIdx = i
				break
			}
		}
		m.mode = "new-session"
		m.dialog = dialogState{
			prompt:      "Session title:",
			toolOptions: toolOptions,
			toolIdx:     toolIdx,
			savedPath:   defaultPath,
		}
	case "new-group":
		m.mode = "new-group"
		m.dialog = newDialogState("Group path (e.g. work/frontend):")
	case "rename":
		if m.cursor < len(m.items) {
			m.mode = "rename"
			m.dialog = newDialogState("New name:")
		}
	case "move":
		if len(m.selected) > 0 || (m.cursor < len(m.items) && m.items[m.cursor].Kind == "session") {
			m.mode = "move"
			m.dialog = newDialogState("Move to group path:")
		}
	case "delete":
		if len(m.selected) > 0 {
			if err := m.deleteSelectedSessions(); err != nil {
				m.err = err
			}
			if err := m.Reload(); err != nil {
				m.err = err
			}
		} else if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Kind == "session" {
				if err := m.deleteSession(item.Session); err != nil {
					m.err = err
					break
				}
			} else if item.Kind == "group" && item.Group.Path != defaultGroupPath {
				db.DeleteGroup(m.conn, item.Group.Path)
			}
			m.Reload()
		}
	case "toggle-full":
		m.viewFull = !m.viewFull
	case "help":
		m.mode = "help"
	case "edit-notes":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "edit-notes"
			m.dialog = dialogState{prompt: "", value: m.items[m.cursor].Session.Notes}
		}
	case "set-conductor":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			session := m.items[m.cursor].Session
			if err := db.SetGroupConductor(m.conn, session.GroupPath, session.ID); err != nil {
				m.err = err
				break
			}
			if err := m.Reload(); err != nil {
				m.err = err
			}
		}
	case "escalate-conductor":
		if err := m.escalateSelectedSession(); err != nil {
			m.err = err
		}
	case "edit-tags":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			m.mode = "edit-tags"
			m.dialog = newDialogState("Tags:")
			m.dialog.value = m.items[m.cursor].Session.Tags
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
	case "archive":
		if err := m.toggleArchivedSelection(); err != nil {
			m.err = err
			break
		}
		if err := m.Reload(); err != nil {
			m.err = err
		}
	case "toggle-archived":
		m.showArchived = !m.showArchived
		if err := m.Reload(); err != nil {
			m.err = err
		}
	case "filter":
		m.mode = "filter"
		m.dialog = newDialogState("Filter:")
		m.dialog.value = m.searchQuery
	}
	if m.viewFull && m.mode != "" {
		m.viewFull = false
	}
	return m, nil
}

func (m *Model) View() string {
	if m.err != nil {
		return "error: " + m.err.Error() + "\n\nPress q or ctrl+c to quit"
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
	footer := m.renderFooter()

	if m.mode == "help" {
		return header + "\n" + strings.Repeat("─", m.width) + "\n" + m.renderHelpOverlay(m.width, contentH) + "\n" + footer
	}

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
	waitingStr := fmt.Sprintf("%d waiting", waiting)
	errorStr := fmt.Sprintf("%d error", errs)
	if waiting > 0 {
		waitingStr = headerWaitingStyle.Render(waitingStr)
	}
	if errs > 0 {
		errorStr = headerErrorStyle.Render(errorStr)
	}
	header := fmt.Sprintf(" Agent Deck  ● %d running  ○ %s  ◐ %d idle  ✕ %s", running, waitingStr, idle, errorStr)
	if m.overdueWaiting > 0 {
		header += "  " + headerOverdueStyle.Render(fmt.Sprintf("!%d", m.overdueWaiting))
	}
	return header
}

func (m *Model) renderFooter() string {
	footer := " Enter Attach  x Send  f Fork  b Broadcast  v Output  e Notes  n New  d Delete  q Quit"
	footer += "  a Archive  A Archived  c Conductor  t Tags  / Filter  ? Help"
	if len(m.selected) > 0 {
		return fmt.Sprintf("[%d selected] %s", len(m.selected), footer)
	}
	return footer
}

func (m *Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ""
		return m, nil
	}
	switch actionForKey(msg) {
	case "help", "quit":
		m.mode = ""
		return m, nil
	}
	return m, nil
}

func (m *Model) renderHelpOverlay(width, height int) string {
	lines := []string{"KEYBOARD SHORTCUTS", ""}
	currentSection := ""
	for _, binding := range KeyBindings {
		if binding.Section != currentSection {
			if currentSection != "" {
				lines = append(lines, "")
			}
			currentSection = binding.Section
			lines = append(lines, strings.ToUpper(currentSection))
		}
		lines = append(lines, fmt.Sprintf(" %-8s %s", binding.Key, binding.Description))
	}
	lines = append(lines, "", "Press ? or q to close")
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	if width <= 0 {
		return strings.Join(lines, "\n")
	}
	for i := range lines {
		lines[i] = truncate(lines[i], width)
	}
	return strings.Join(lines, "\n")
}

func tick() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Second)
		return tickMsg{}
	}
}

func (m *Model) RenderDetailPanel(w, h int) string {
	if len(m.sessions) == 0 {
		return m.renderEmptyState(w, h)
	}
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
	lines = append(lines, fmt.Sprintf(" conductor: %t", m.isConductorSession(s)))
	lines = append(lines, fmt.Sprintf(" tags: %s", s.Tags))
	toolLine := " tool: " + s.Tool
	if s.ToolFlags != "" {
		toolLine += "  flags: " + s.ToolFlags
	}
	lines = append(lines, toolLine)
	if pct, ok := m.contextPct[s.ID]; ok && pct != nil {
		lines = append(lines, fmt.Sprintf(" context: %s", RenderContextBar(*pct)))
	}
	lines = append(lines, " "+renderPaneList(m.panes, m.activePaneIdx))

	const sessionHeaderLines = 7
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

func (m *Model) renderEmptyState(w, h int) string {
	lines := []string{
		"WELCOME",
		"",
		"Press n to create your first session",
		"Then attach to launch your agent in tmux.",
	}
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if w <= 0 {
		return strings.Join(lines, "\n")
	}
	for i := range lines {
		lines[i] = truncate(lines[i], w)
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

func contextLines(output string, n int) []string {
	if n <= 0 || output == "" {
		return nil
	}
	all := strings.Split(output, "\n")
	lines := make([]string, 0, n)
	for i := len(all) - 1; i >= 0 && len(lines) < n; i-- {
		line := strings.TrimSpace(all[i])
		if !isContextLine(line) {
			continue
		}
		lines = append(lines, line)
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}

func isContextLine(line string) bool {
	if line == "" || line == ">" || line == "❯" {
		return false
	}
	if strings.Contains(line, "-- INSERT --") {
		return false
	}
	if strings.Contains(line, "ctx:") && strings.Contains(line, "@") {
		return false
	}
	return strings.Trim(line, "─━═- ") != ""
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

func (m *Model) pruneSelection() {
	if len(m.selected) == 0 {
		return
	}
	valid := make(map[string]bool, len(m.sessions))
	for _, session := range m.sessions {
		valid[session.ID] = true
	}
	for id := range m.selected {
		if !valid[id] {
			delete(m.selected, id)
		}
	}
}

func (m *Model) selectedSessionIDs() []string {
	ids := make([]string, 0, len(m.selected))
	for _, item := range m.items {
		if item.Kind != "session" || !m.selected[item.Session.ID] {
			continue
		}
		ids = append(ids, item.Session.ID)
	}
	return ids
}

func (m *Model) clearSelection() {
	for id := range m.selected {
		delete(m.selected, id)
	}
}

func (m *Model) deleteSelectedSessions() error {
	ids := m.selectedSessionIDs()
	if len(ids) == 0 {
		return nil
	}
	for _, item := range m.items {
		if item.Kind != "session" || !m.selected[item.Session.ID] {
			continue
		}
		if item.Session.TmuxSession != "" && m.tmuxC != nil {
			if err := m.tmuxC.KillSession(item.Session.TmuxSession); err != nil {
				return err
			}
		}
	}
	if err := db.DeleteSessions(m.conn, ids); err != nil {
		return err
	}
	m.clearSelection()
	return nil
}

func (m *Model) deleteSession(session *db.Session) error {
	if session.TmuxSession != "" && m.tmuxC != nil {
		if err := m.tmuxC.KillSession(session.TmuxSession); err != nil {
			return err
		}
	}
	return db.DeleteSession(m.conn, session.ID)
}

func (m *Model) visibleGroups(groups []db.Group, sessions []db.Session) []db.Group {
	if strings.TrimSpace(m.searchQuery) != "" {
		matchingGroups := make(map[string]bool)
		for _, session := range sessions {
			groupPath := session.GroupPath
			for {
				matchingGroups[groupPath] = true
				idx := strings.LastIndex(groupPath, "/")
				if idx == -1 {
					break
				}
				groupPath = groupPath[:idx]
			}
		}
		filtered := make([]db.Group, 0, len(groups))
		for _, group := range groups {
			if !matchingGroups[group.Path] {
				continue
			}
			if group.Path == "archived" && !m.showArchived {
				continue
			}
			filtered = append(filtered, group)
		}
		return filtered
	}
	if m.showArchived {
		return groups
	}
	filtered := make([]db.Group, 0, len(groups))
	for _, group := range groups {
		if group.Path == "archived" {
			continue
		}
		filtered = append(filtered, group)
	}
	return filtered
}

func (m *Model) visibleSessions(sessions []db.Session) []db.Session {
	filtered := make([]db.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Archived && !m.showArchived {
			continue
		}
		if !matchesSearchQuery(session, m.searchQuery) {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered
}

func (m *Model) isConductorSession(session *db.Session) bool {
	for _, group := range m.groups {
		if group.Path == session.GroupPath {
			return group.ConductorSessionID == session.ID
		}
	}
	return false
}

func (m *Model) escalateSelectedSession() error {
	if m.cursor >= len(m.items) || m.items[m.cursor].Kind != "session" {
		return nil
	}
	session := m.items[m.cursor].Session
	if session.Status != tmux.StatusWaiting {
		return nil
	}
	conductor, err := db.GetGroupConductorSession(m.conn, session.GroupPath)
	if err != nil {
		return fmt.Errorf("resolve conductor for %q: %w", session.GroupPath, err)
	}
	if conductor.ID == session.ID {
		return fmt.Errorf("session %q is already the conductor", session.Title)
	}
	if conductorUnavailable(conductor) {
		return fmt.Errorf("conductor %q is not running", conductor.Title)
	}
	if m.tmuxC == nil {
		return fmt.Errorf("tmux client unavailable")
	}
	if err := m.tmuxC.SendKeys(conductor.TmuxSession, 0, m.escalationMessage(session)); err != nil {
		return err
	}
	return m.tmuxC.SendRawKeys(conductor.TmuxSession, 0, "Enter")
}

func conductorUnavailable(conductor db.Session) bool {
	return conductor.TmuxSession == "" || conductor.Status == tmux.StatusStopped || conductor.Status == tmux.StatusError
}

func (m *Model) escalationMessage(session *db.Session) string {
	out := ""
	if m.output != "" {
		out = m.output
	}
	return state.EscalationMessage(*session, out)
}

func (m *Model) toggleArchivedSelection() error {
	if len(m.selected) > 0 {
		for _, item := range m.items {
			if item.Kind != "session" || !m.selected[item.Session.ID] {
				continue
			}
			if err := m.toggleArchivedSession(item.Session); err != nil {
				return err
			}
		}
		m.clearSelection()
		return nil
	}
	if m.cursor >= len(m.items) || m.items[m.cursor].Kind != "session" {
		return nil
	}
	return m.toggleArchivedSession(m.items[m.cursor].Session)
}

func (m *Model) toggleArchivedSession(session *db.Session) error {
	if session.TmuxSession != "" && !session.Archived && m.tmuxC != nil {
		if err := m.tmuxC.KillSession(session.TmuxSession); err != nil {
			return err
		}
	}
	return db.SetSessionArchived(m.conn, session.ID, !session.Archived)
}

func matchesSearchQuery(session db.Session, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	if strings.HasPrefix(query, "#") {
		prefix := strings.TrimPrefix(strings.ToLower(query), "#")
		for _, tag := range strings.Fields(strings.ToLower(session.Tags)) {
			if strings.HasPrefix(tag, prefix) {
				return true
			}
		}
		return false
	}
	return strings.Contains(strings.ToLower(session.Title), strings.ToLower(query))
}

// ensureStarted returns the tmux session name for s, spawning one if needed.
// The bool return indicates whether a new tmux session was created.
func (m *Model) ensureStarted(s *db.Session) (string, bool, error) {
	if s.TmuxSession != "" {
		exists, err := m.tmuxC.SessionExists(s.TmuxSession)
		if err == nil && exists {
			return s.TmuxSession, false, nil
		}
	}
	tmuxName := fmt.Sprintf("tma-%s-%s", slugifySessionTitle(s.Title), sessionIDSuffix(s.ID))
	if err := m.tmuxC.NewSession(tmuxName, s.ProjectPath, s.Tool, s.ToolFlags); err != nil {
		return "", false, fmt.Errorf("start session: %w", err)
	}
	_ = db.UpdateSessionTmuxName(m.conn, s.ID, tmuxName)
	_ = db.UpdateSessionStatus(m.conn, s.ID, "waiting")
	return tmuxName, true, nil
}

func sessionIDSuffix(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func slugifySessionTitle(title string) string {
	const maxLen = 40
	title = strings.ToLower(title)
	var b strings.Builder
	lastDash := false
	for _, r := range title {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			if b.Len() >= maxLen {
				break
			}
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if b.Len() == 0 || lastDash {
			continue
		}
		if b.Len() >= maxLen {
			break
		}
		b.WriteByte('-')
		lastDash = true
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "session"
	}
	return slug
}

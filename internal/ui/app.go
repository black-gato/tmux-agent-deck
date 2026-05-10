package ui

import (
	"database/sql"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

type tickMsg struct{}

type Model struct {
	conn          *sql.DB
	tmuxC         tmux.ClientIface
	poller        *state.Poller
	groups        []db.Group
	sessions      []db.Session
	items         []ListItem
	cursor        int
	width         int
	height        int
	mode          string // "", "new-session", "new-group", "rename", "move"
	dialog        dialogState
	err           error
	PendingAttach string // tmux session name to attach after TUI exits
	viewFull      bool
	panes         []tmux.Pane
	output        string
}

func NewModel(conn *sql.DB, tc tmux.ClientIface, poller *state.Poller) *Model {
	return &Model{conn: conn, tmuxC: tc, poller: poller}
}

func (m *Model) Reload() error {
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
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
	m.panes = nil
	m.output = ""
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

func (m *Model) Items() []ListItem  { return m.items }
func (m *Model) Cursor() int        { return m.cursor }
func (m *Model) Mode() string       { return m.mode }
func (m *Model) Panes() []tmux.Pane { return m.panes }
func (m *Model) Output() string     { return m.output }
func (m *Model) ViewFull() bool     { return m.viewFull }

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
		return m, nil
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
	if m.mode != "" {
		return m.renderDialog()
	}
	return RenderList(m.items, m.cursor, m.width, m.height)
}

func tick() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Second)
		return tickMsg{}
	}
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

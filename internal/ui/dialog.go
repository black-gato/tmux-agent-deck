package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
)

const defaultGroupPath = "my-sessions"

type dialogState struct {
	prompt      string
	value       string
	scope       bool
	scopeLabels [2]string
}

func newDialogState(prompt string) dialogState {
	return dialogState{prompt: prompt}
}

func interceptCtrl(msg tea.KeyMsg) (string, bool) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return "C-c", true
	case tea.KeyCtrlD:
		return "C-d", true
	case tea.KeyCtrlZ:
		return "C-z", true
	case tea.KeyCtrlL:
		return "C-l", true
	case tea.KeyCtrlU:
		return "C-u", true
	}
	return "", false
}

func (m *Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == "send-pane" || m.mode == "broadcast" {
		if key, ok := interceptCtrl(msg); ok {
			m.dialog.value += key
			return m, nil
		}
		if msg.Type == tea.KeyTab && m.mode == "broadcast" {
			m.dialog.scope = !m.dialog.scope
			return m, nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ""
	case tea.KeyEnter:
		m.commitDialog()
		m.mode = ""
		if err := m.Reload(); err != nil {
			m.err = err
		}
	case tea.KeyBackspace:
		if len(m.dialog.value) > 0 {
			m.dialog.value = m.dialog.value[:len(m.dialog.value)-1]
		}
	default:
		if len(msg.Runes) > 0 {
			m.dialog.value += string(msg.Runes)
		}
	}
	return m, nil
}

func (m *Model) renderDialog() string {
	if m.mode == "edit-notes" {
		return "> " + m.dialog.value
	}
	if m.mode == "broadcast" {
		label0 := m.dialog.scopeLabels[0]
		label1 := m.dialog.scopeLabels[1]
		if !m.dialog.scope {
			label0 = "→ " + label0
			label1 = dimStyle.Render(label1)
		} else {
			label0 = dimStyle.Render(label0)
			label1 = "→ " + label1
		}
		return fmt.Sprintf("Broadcast [%s / %s]:\n> %s", label0, label1, m.dialog.value)
	}
	return m.dialog.prompt + "\n> " + m.dialog.value
}

func (m *Model) commitDialog() {
	switch m.mode {
	case "new-session":
		val := strings.TrimSpace(m.dialog.value)
		if val == "" {
			return
		}
		groupPath := defaultGroupPath
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "group" {
			groupPath = m.items[m.cursor].Group.Path
		}
		if err := db.CreateSession(m.conn, db.Session{
			ID:          uuid.New().String(),
			Title:       val,
			GroupPath:   groupPath,
			ProjectPath: ".",
			Tool:        "claude",
			Status:      "stopped",
			CreatedAt:   time.Now().Unix(),
		}); err != nil {
			m.err = err
		}
	case "new-group":
		val := strings.TrimSpace(m.dialog.value)
		if val == "" {
			return
		}
		parts := strings.Split(val, "/")
		name := parts[len(parts)-1]
		if err := db.CreateGroup(m.conn, db.Group{
			Path:        val,
			Name:        name,
			DefaultTool: "claude",
			Expanded:    true,
		}); err != nil {
			m.err = err
		}
	case "rename":
		val := strings.TrimSpace(m.dialog.value)
		if val == "" {
			return
		}
		if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			var err error
			if item.Kind == "session" {
				err = db.RenameSession(m.conn, item.Session.ID, val)
			} else if item.Kind == "group" {
				err = db.RenameGroup(m.conn, item.Group.Path, val)
			}
			if err != nil {
				m.err = err
			}
		}
	case "move":
		val := strings.TrimSpace(m.dialog.value)
		if val == "" {
			return
		}
		if len(m.selected) > 0 {
			if err := db.MoveSessions(m.conn, m.selectedSessionIDs(), val); err != nil {
				m.err = err
				return
			}
			m.clearSelection()
			return
		}
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			if err := db.MoveSession(m.conn, m.items[m.cursor].Session.ID, val); err != nil {
				m.err = err
			}
		}
	case "edit-notes":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			s := m.items[m.cursor].Session
			if err := db.UpdateSessionNotes(m.conn, s.ID, m.dialog.value); err != nil {
				m.err = err
			}
		}
	case "send-pane":
		if m.dialog.value == "" {
			return
		}
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			s := m.items[m.cursor].Session
			if s.TmuxSession == "" {
				return
			}
			if err := m.tmuxC.SendKeys(s.TmuxSession, m.activePaneIdx, m.dialog.value); err != nil {
				m.err = err
			}
		}
	case "fork-session":
		val := strings.TrimSpace(m.dialog.value)
		if val == "" {
			return
		}
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			s := m.items[m.cursor].Session
			if err := db.CreateSession(m.conn, db.Session{
				ID:          uuid.New().String(),
				Title:       val,
				GroupPath:   s.GroupPath,
				ProjectPath: s.ProjectPath,
				Tool:        s.Tool,
				Status:      "stopped",
				CreatedAt:   time.Now().Unix(),
			}); err != nil {
				m.err = err
			}
		}
	case "broadcast":
		if m.dialog.value == "" {
			return
		}
		if m.cursor >= len(m.items) {
			return
		}
		item := m.items[m.cursor]
		var groupPath string
		if item.Kind == "group" {
			groupPath = item.Group.Path
		} else if item.Kind == "session" {
			groupPath = item.Session.GroupPath
		} else {
			return
		}
		for _, s := range m.sessions {
			if s.Status != "running" || s.TmuxSession == "" {
				continue
			}
			inScope := s.GroupPath == groupPath
			if !inScope && m.dialog.scope {
				inScope = strings.HasPrefix(s.GroupPath, groupPath+"/")
			}
			if !inScope {
				continue
			}
			if err := m.tmuxC.SendKeys(s.TmuxSession, 0, m.dialog.value); err != nil {
				m.err = err
			}
		}
	}
}

package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/black-gato/tmux-agent-deck/internal/db"
)

const defaultGroupPath = "my-sessions"

type dialogState struct {
	prompt string
	value  string
}

func newDialogState(prompt string) dialogState {
	return dialogState{prompt: prompt}
}

func (m *Model) updateDialog(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}
}

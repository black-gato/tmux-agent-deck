package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/black-gato/tmux-agent-deck/internal/db"
)

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
		m.Reload()
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
	return m.dialog.prompt + "\n> " + m.dialog.value
}

func (m *Model) commitDialog() {
	val := strings.TrimSpace(m.dialog.value)
	if val == "" {
		return
	}
	switch m.mode {
	case "new-session":
		groupPath := "my-sessions"
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "group" {
			groupPath = m.items[m.cursor].Group.Path
		}
		db.CreateSession(m.conn, db.Session{
			ID:          uuid.New().String(),
			Title:       val,
			GroupPath:   groupPath,
			ProjectPath: ".",
			Tool:        "claude",
			Status:      "stopped",
			CreatedAt:   time.Now().Unix(),
		})
	case "new-group":
		parts := strings.Split(val, "/")
		name := parts[len(parts)-1]
		db.CreateGroup(m.conn, db.Group{
			Path:        val,
			Name:        name,
			DefaultTool: "claude",
			Expanded:    true,
		})
	case "rename":
		if m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Kind == "session" {
				db.RenameSession(m.conn, item.Session.ID, val)
			} else if item.Kind == "group" {
				db.RenameGroup(m.conn, item.Group.Path, val)
			}
		}
	case "move":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			db.MoveSession(m.conn, m.items[m.cursor].Session.ID, val)
		}
	}
}

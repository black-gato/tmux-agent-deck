package ui

import (
	"errors"
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
	ctrlKeys    []string
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
			m.dialog.ctrlKeys = append(m.dialog.ctrlKeys, key)
			return m, nil
		}
		if msg.Type == tea.KeyTab && m.mode == "broadcast" {
			m.dialog.scope = !m.dialog.scope
			return m, nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
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
			if errors.Is(err, db.ErrGroupExists) {
				m.err = fmt.Errorf("group %q already exists", val)
			} else {
				m.err = err
			}
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
	case "edit-tags":
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			s := m.items[m.cursor].Session
			if err := db.UpdateSessionTags(m.conn, s.ID, strings.TrimSpace(m.dialog.value)); err != nil {
				m.err = err
			}
		}
	case "send-pane":
		if m.dialog.value == "" && len(m.dialog.ctrlKeys) == 0 {
			return
		}
		if len(m.selected) > 0 {
			for _, item := range m.items {
				if item.Kind != "session" || !m.selected[item.Session.ID] {
					continue
				}
				s := item.Session
				if s.Status != "running" || s.TmuxSession == "" {
					continue
				}
				if err := m.sendToPane(s.TmuxSession, 0); err != nil {
					m.err = err
					return
				}
			}
			m.clearSelection()
			return
		}
		if m.cursor < len(m.items) && m.items[m.cursor].Kind == "session" {
			s := m.items[m.cursor].Session
			if s.TmuxSession == "" {
				return
			}
			if err := m.sendToPane(s.TmuxSession, m.activePaneIdx); err != nil {
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
		if m.dialog.value == "" && len(m.dialog.ctrlKeys) == 0 {
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
			if err := m.sendToPane(s.TmuxSession, 0); err != nil {
				m.err = err
			}
		}
	case "filter":
		m.searchQuery = strings.TrimSpace(m.dialog.value)
	}
}

func (m *Model) sendToPane(session string, paneIndex int) error {
	if m.dialog.value != "" {
		if err := m.tmuxC.SendKeys(session, paneIndex, m.dialog.value); err != nil {
			return err
		}
	}
	for _, key := range m.dialog.ctrlKeys {
		if err := m.tmuxC.SendRawKeys(session, paneIndex, key); err != nil {
			return err
		}
	}
	return nil
}

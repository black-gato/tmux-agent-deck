package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	// stub — implemented in Task 10
	_ = strings.TrimSpace(m.dialog.value)
}

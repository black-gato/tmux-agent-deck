package ui

import tea "github.com/charmbracelet/bubbletea"

var keyTypeMap = map[tea.KeyType]string{
	tea.KeyUp:    "up",
	tea.KeyDown:  "down",
	tea.KeyEnter: "attach",
	tea.KeySpace: "toggle",
	tea.KeyTab:   "cycle-pane",
}

var runeMap = map[rune]string{
	'j': "down",
	'k': "up",
	'n': "new-session",
	'g': "new-group",
	'm': "move",
	'r': "rename",
	'd': "delete",
	'a': "archive",
	'A': "toggle-archived",
	'q': "quit",
	'v': "toggle-full",
	'e': "edit-notes",
	'c': "set-conductor",
	't': "edit-tags",
	'x': "send-pane",
	'f': "fork-session",
	'b': "broadcast",
	'C': "escalate-conductor",
	'/': "search",
}

func actionForKey(msg tea.KeyMsg) string {
	if action, ok := keyTypeMap[msg.Type]; ok {
		return action
	}
	if len(msg.Runes) == 1 {
		if action, ok := runeMap[msg.Runes[0]]; ok {
			return action
		}
	}
	return ""
}

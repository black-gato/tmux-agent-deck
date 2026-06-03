package ui

import tea "github.com/charmbracelet/bubbletea"

type KeyBinding struct {
	Section     string
	Rune        rune
	Key         string
	Description string
	Action      string
}

var KeyBindings = []KeyBinding{
	{Section: "Navigation", Rune: 'j', Key: "j / ↓", Description: "Move down", Action: "down"},
	{Section: "Navigation", Rune: 'k', Key: "k / ↑", Description: "Move up", Action: "up"},
	{Section: "Navigation", Key: "Enter", Description: "Attach or start", Action: "attach"},
	{Section: "Navigation", Key: "Space", Description: "Toggle group or select session", Action: "toggle"},
	{Section: "Navigation", Key: "Tab", Description: "Cycle pane", Action: "cycle-pane"},
	{Section: "Editing", Rune: 'n', Key: "n", Description: "New session", Action: "new-session"},
	{Section: "Editing", Rune: 'g', Key: "g", Description: "New group", Action: "new-group"},
	{Section: "Editing", Rune: 'm', Key: "m", Description: "Move", Action: "move"},
	{Section: "Editing", Rune: 'r', Key: "r", Description: "Rename", Action: "rename"},
	{Section: "Editing", Rune: 'd', Key: "d", Description: "Delete", Action: "delete"},
	{Section: "Editing", Rune: 'e', Key: "e", Description: "Edit notes", Action: "edit-notes"},
	{Section: "Editing", Rune: 't', Key: "t", Description: "Edit tags", Action: "edit-tags"},
	{Section: "Editing", Rune: 'I', Key: "I", Description: "Import tmux session", Action: "import"},
	{Section: "Workflow", Rune: 'x', Key: "x", Description: "Send keys", Action: "send-pane"},
	{Section: "Workflow", Rune: 'f', Key: "f", Description: "Fork session", Action: "fork-session"},
	{Section: "Workflow", Rune: 'b', Key: "b", Description: "Broadcast", Action: "broadcast"},
	{Section: "Workflow", Rune: 'c', Key: "c", Description: "Toggle conductor", Action: "set-conductor"},
	{Section: "Workflow", Rune: 'C', Key: "C", Description: "Escalate to conductor", Action: "escalate-conductor"},
	{Section: "Workflow", Rune: 'M', Key: "M", Description: "Set/clear meta-conductor", Action: "set-meta-conductor"},
	{Section: "View", Rune: 'v', Key: "v", Description: "Toggle full output", Action: "toggle-full"},
	{Section: "View", Rune: 'a', Key: "a", Description: "Archive", Action: "archive"},
	{Section: "View", Rune: 'A', Key: "A", Description: "Toggle archived", Action: "toggle-archived"},
	{Section: "View", Rune: '/', Key: "/", Description: "Filter", Action: "filter"},
	{Section: "View", Rune: '?', Key: "?", Description: "Help", Action: "help"},
	{Section: "View", Rune: 'q', Key: "q", Description: "Quit", Action: "quit"},
}

var keyTypeMap = map[tea.KeyType]string{
	tea.KeyUp:    "up",
	tea.KeyDown:  "down",
	tea.KeyEnter: "attach",
	tea.KeySpace: "toggle",
	tea.KeyTab:   "cycle-pane",
	tea.KeyCtrlC: "quit",
}

var runeMap = func() map[rune]string {
	m := make(map[rune]string, len(KeyBindings))
	for _, binding := range KeyBindings {
		if binding.Rune == 0 {
			continue
		}
		m[binding.Rune] = binding.Action
	}
	return m
}()

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

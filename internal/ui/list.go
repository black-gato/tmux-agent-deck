package ui

import (
	"github.com/black-gato/tmux-agent-deck/internal/db"
)

type ListItem struct {
	Kind    string // "group" or "session"
	Group   *db.Group
	Session *db.Session
	Depth   int
}

func BuildTree(groups []db.Group, sessions []db.Session) []ListItem {
	var items []ListItem
	for i := range groups {
		g := groups[i]
		items = append(items, ListItem{Kind: "group", Group: &g, Depth: 0})
		if !g.Expanded {
			continue
		}
		for j := range sessions {
			if sessions[j].GroupPath == g.Path {
				s := sessions[j]
				items = append(items, ListItem{Kind: "session", Session: &s, Depth: 1})
			}
		}
	}
	return items
}

func RenderList(items []ListItem, cursor, width, height int) string {
	return "loading..." // stub — implemented in Task 9
}

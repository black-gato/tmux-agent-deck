package ui

import (
	"fmt"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/charmbracelet/lipgloss"
)

type ListItem struct {
	Kind    string // "group" or "session"
	Group   *db.Group
	Session *db.Session
	Depth   int
}

var statusSymbol = map[string]string{
	"running": "●",
	"waiting": "○",
	"idle":    "◐",
	"error":   "✕",
	"stopped": "—",
}

var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	groupStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
)

// BuildTree returns a flattened, ordered list of groups and their sessions.
// Top-level groups are those with no "/" in their path. Nested groups are
// appended recursively directly after their parent when the parent is expanded.
func BuildTree(groups []db.Group, sessions []db.Session) []ListItem {
	sessionsByGroup := make(map[string][]db.Session)
	for _, s := range sessions {
		sessionsByGroup[s.GroupPath] = append(sessionsByGroup[s.GroupPath], s)
	}

	var items []ListItem
	for _, g := range groups {
		if strings.Contains(g.Path, "/") {
			continue // skip; appended recursively by parent
		}
		items = append(items, appendGroupItems(g, groups, sessionsByGroup, 0)...)
	}
	return items
}

func appendGroupItems(g db.Group, allGroups []db.Group, sessionsByGroup map[string][]db.Session, depth int) []ListItem {
	gc := g
	items := []ListItem{{Kind: "group", Group: &gc, Depth: depth}}
	if !g.Expanded {
		return items
	}
	for _, s := range sessionsByGroup[g.Path] {
		sc := s
		items = append(items, ListItem{Kind: "session", Session: &sc, Depth: depth + 1})
	}
	for _, child := range allGroups {
		// direct child: has exactly one more path segment than g
		prefix := g.Path + "/"
		if !strings.HasPrefix(child.Path, prefix) {
			continue
		}
		remainder := child.Path[len(prefix):]
		if strings.Contains(remainder, "/") {
			continue // grandchild or deeper — handled recursively
		}
		items = append(items, appendGroupItems(child, allGroups, sessionsByGroup, depth+1)...)
	}
	return items
}

func RenderList(items []ListItem, cursor, width, height int) string {
	var sb strings.Builder
	sb.WriteString("tmux-agent-deck\n\n")
	for i, item := range items {
		indent := strings.Repeat("  ", item.Depth)
		var line string
		if item.Kind == "group" {
			arrow := "▼"
			if !item.Group.Expanded {
				arrow = "►"
			}
			line = groupStyle.Render(fmt.Sprintf("%s%s %s", indent, arrow, item.Group.Name))
		} else {
			sym := statusSymbol[item.Session.Status]
			if sym == "" {
				sym = "—"
			}
			toolStr := dimStyle.Render(item.Session.Tool)
			line = fmt.Sprintf("%s%s  %-20s %s", indent, sym, item.Session.Title, toolStr)
		}
		if i == cursor {
			line = selectedStyle.Render("> " + strings.TrimLeft(line, " "))
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\n[n]ew  [g]roup  [m]ove  [r]ename  [d]elete  [q]uit")
	return sb.String()
}

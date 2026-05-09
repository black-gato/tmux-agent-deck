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

	// Viewport: show a window of items centered around cursor
	start := 0
	end := len(items)
	if height > 4 {
		viewHeight := height - 4 // reserve header (2) and footer (2) lines
		if viewHeight > 0 && len(items) > viewHeight {
			start = cursor - viewHeight/2
			if start < 0 {
				start = 0
			}
			end = start + viewHeight
			if end > len(items) {
				end = len(items)
				start = end - viewHeight
				if start < 0 {
					start = 0
				}
			}
		}
	}

	for i := start; i < end; i++ {
		item := items[i]
		indent := strings.Repeat("  ", item.Depth)
		selected := i == cursor

		var line string
		if item.Kind == "group" {
			arrow := "▼"
			if !item.Group.Expanded {
				arrow = "►"
			}
			raw := fmt.Sprintf("%s%s %s", indent, arrow, item.Group.Name)
			if selected {
				line = selectedStyle.Render(raw)
			} else {
				line = groupStyle.Render(raw)
			}
		} else {
			sym := statusSymbol[item.Session.Status]
			if sym == "" {
				sym = "—"
			}
			if selected {
				raw := fmt.Sprintf("%s%s  %s  %s", indent, sym, item.Session.Title, item.Session.Tool)
				line = selectedStyle.Render(raw)
			} else {
				toolStr := dimStyle.Render(item.Session.Tool)
				line = fmt.Sprintf("%s%s  %-20s %s", indent, sym, item.Session.Title, toolStr)
			}
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n[n]ew  [g]roup  [m]ove  [r]ename  [d]elete  [q]uit")
	return sb.String()
}

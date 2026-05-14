package ui

import (
	"fmt"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/charmbracelet/lipgloss"
)

type ListItem struct {
	Kind        string // "group" or "session"
	Group       *db.Group
	Session     *db.Session
	Depth       int
	WaitLabel   string
	WaitOverdue bool
	ContextPct  *int
	Selected    bool
	IsConductor bool
}

var statusSymbol = map[string]string{
	"running": "●",
	"waiting": "○",
	"idle":    "◐",
	"error":   "✕",
	"stopped": "—",
}

var (
	selectedStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	groupStyle       = lipgloss.NewStyle().Bold(true)
	dimStyle         = lipgloss.NewStyle().Faint(true)
	overdueWaitStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // amber
)

func RenderContextBar(pct int) string {
	filled := (pct * 4) / 100
	var bar strings.Builder
	for i := 0; i < 4; i++ {
		if i < filled {
			bar.WriteRune('▓')
		} else {
			bar.WriteRune('░')
		}
	}
	return fmt.Sprintf("%s %d%%", bar.String(), pct)
}

func BuildTree(groups []db.Group, sessions []db.Session) []ListItem {
	sessionsByGroup := make(map[string][]db.Session)
	for _, s := range sessions {
		sessionsByGroup[s.GroupPath] = append(sessionsByGroup[s.GroupPath], s)
	}

	var items []ListItem
	for _, g := range groups {
		if strings.Contains(g.Path, "/") {
			continue
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
		prefix := g.Path + "/"
		if !strings.HasPrefix(child.Path, prefix) {
			continue
		}
		remainder := child.Path[len(prefix):]
		if strings.Contains(remainder, "/") {
			continue
		}
		items = append(items, appendGroupItems(child, allGroups, sessionsByGroup, depth+1)...)
	}
	return items
}

func RenderList(items []ListItem, cursor, width, height int) string {
	var sb strings.Builder
	sb.WriteString("SESSIONS\n")
	sb.WriteString(strings.Repeat("─", width) + "\n")

	start := 0
	end := len(items)
	if height > 4 {
		viewHeight := height - 4
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
			nameMax := width - len([]rune(indent)) - 2
			if nameMax < 1 {
				nameMax = 1
			}
			raw := fmt.Sprintf("%s%s %s", indent, arrow, truncate(item.Group.Name, nameMax))
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
			mark := " "
			if item.Selected {
				mark = "*"
			}
			if item.IsConductor {
				mark = "C"
			}
			prefixLen := len([]rune(indent)) + 1 + 1 + 2 // mark + sym(1) + spaces(2)
			if item.WaitLabel != "" {
				prefixLen += len([]rune(item.WaitLabel)) + 1
			}
			if item.ContextPct != nil {
				prefixLen += len([]rune(RenderContextBar(*item.ContextPct))) + 1
			}
			titleMax := width - prefixLen
			if titleMax < 1 {
				titleMax = 1
			}
			title := truncate(item.Session.Title, titleMax)

			waitStr := item.WaitLabel
			if item.WaitOverdue && !selected {
				waitStr = overdueWaitStyle.Render(waitStr)
			}

			var raw string
			switch {
			case item.WaitLabel != "" && item.ContextPct != nil:
				raw = fmt.Sprintf("%s%s%s %s %s %s", indent, mark, sym, waitStr, RenderContextBar(*item.ContextPct), title)
			case item.WaitLabel != "":
				raw = fmt.Sprintf("%s%s%s %s %s", indent, mark, sym, waitStr, title)
			case item.ContextPct != nil:
				raw = fmt.Sprintf("%s%s%s %s %s", indent, mark, sym, RenderContextBar(*item.ContextPct), title)
			default:
				raw = fmt.Sprintf("%s%s%s  %s", indent, mark, sym, title)
			}
			if selected {
				line = selectedStyle.Render(raw)
			} else {
				line = raw
			}
		}
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

package ui_test

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
)

func TestBuildTreeFlattensGroups(t *testing.T) {
	groups := []db.Group{
		{Path: "my-sessions", Name: "my-sessions", Expanded: true},
		{Path: "work", Name: "work", Expanded: true},
	}
	sessions := []db.Session{
		{ID: "s1", Title: "app", GroupPath: "my-sessions", Status: "stopped"},
		{ID: "s2", Title: "api", GroupPath: "work", Status: "running"},
	}

	items := ui.BuildTree(groups, sessions)
	// my-sessions header, s1, work header, s2
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}
	if items[0].Kind != "group" || items[0].Group.Path != "my-sessions" {
		t.Errorf("items[0] should be my-sessions group")
	}
	if items[1].Kind != "session" || items[1].Session.Title != "app" {
		t.Errorf("items[1] should be session 'app'")
	}
}

func TestBuildTreeCollapsedGroupHidesChildren(t *testing.T) {
	groups := []db.Group{
		{Path: "work", Name: "work", Expanded: false},
	}
	sessions := []db.Session{
		{ID: "s1", Title: "api", GroupPath: "work", Status: "running"},
	}

	items := ui.BuildTree(groups, sessions)
	if len(items) != 1 {
		t.Errorf("expected 1 item (collapsed group header), got %d", len(items))
	}
}

func TestBuildTreeNestedGroups(t *testing.T) {
	groups := []db.Group{
		{Path: "work", Name: "work", Expanded: true},
		{Path: "work/frontend", Name: "frontend", Expanded: true},
	}
	sessions := []db.Session{
		{ID: "s1", Title: "app", GroupPath: "work/frontend", Status: "stopped"},
	}

	items := ui.BuildTree(groups, sessions)
	// work header, frontend header (depth 1), session (depth 2)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items[1].Depth != 1 {
		t.Errorf("nested group depth: got %d want 1", items[1].Depth)
	}
	if items[2].Depth != 2 {
		t.Errorf("session depth: got %d want 2", items[2].Depth)
	}
}

func TestRenderListContainsSessionTitle(t *testing.T) {
	groups := []db.Group{{Path: "my-sessions", Name: "my-sessions", Expanded: true}}
	sessions := []db.Session{{ID: "s1", Title: "my-app", GroupPath: "my-sessions", Status: "running"}}
	items := ui.BuildTree(groups, sessions)

	output := ui.RenderList(items, 1, 80, 24) // cursor on session
	if !strings.Contains(output, "my-app") {
		t.Errorf("render missing session title: %q", output)
	}
}

func TestRenderListContainsStatusSymbol(t *testing.T) {
	groups := []db.Group{{Path: "g", Name: "g", Expanded: true}}
	for status, symbol := range map[string]string{
		"running": "●",
		"waiting": "○",
		"idle":    "◐",
		"error":   "✕",
		"stopped": "—",
	} {
		sessions := []db.Session{{ID: "s1", Title: "x", GroupPath: "g", Status: status}}
		items := ui.BuildTree(groups, sessions)
		output := ui.RenderList(items, 0, 80, 24)
		if !strings.Contains(output, symbol) {
			t.Errorf("status %q: missing symbol %q in output", status, symbol)
		}
	}
}

func TestRenderListTruncatesLongTitleToWidth(t *testing.T) {
	groups := []db.Group{{Path: "g", Name: "g", Expanded: true}}
	sessions := []db.Session{{ID: "s1", Title: "very-long-session-title-that-should-be-cut", GroupPath: "g", Status: "running"}}
	items := ui.BuildTree(groups, sessions)

	output := ui.RenderList(items, 1, 20, 24)
	for _, line := range strings.Split(output, "\n") {
		visible := stripANSI(line)
		if len([]rune(visible)) > 20 {
			t.Errorf("line exceeds width 20: %q (len %d)", visible, len([]rune(visible)))
		}
	}
}

func stripANSI(s string) string {
	var result []rune
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			i += 2
			for i < len(runes) && runes[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		result = append(result, runes[i])
		i++
	}
	return string(result)
}

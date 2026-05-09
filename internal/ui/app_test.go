package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestModelInitializesWithGroups(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	if err := m.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(m.Items()) == 0 {
		t.Errorf("expected at least one item (my-sessions group)")
	}
}

func TestModelNavigateDown(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	before := m.Cursor()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	after := m.Cursor()
	if before == after && len(m.Items()) > 1 {
		t.Errorf("cursor did not advance on 'j'")
	}
}

func TestModelNavigateUp(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	// Move to last item, then back up
	for i := 0; i < len(m.Items()); i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	before := m.Cursor()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	after := m.Cursor()
	if after >= before {
		t.Errorf("cursor did not decrease on 'k': before=%d after=%d", before, after)
	}
}

func TestModelOpenNewSessionDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.Mode() != "new-session" {
		t.Errorf("expected mode new-session, got %q", m.Mode())
	}
}

func TestModelEscClosesDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.Mode() != "" {
		t.Errorf("expected mode empty after Esc, got %q", m.Mode())
	}
}

func TestNewSessionDialogCreatesSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	// open new-session dialog
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// type title
	for _, r := range "my-app" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// confirm
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "my-app" {
		t.Errorf("title: got %q want my-app", sessions[0].Title)
	}
	if sessions[0].GroupPath != "my-sessions" {
		t.Errorf("group: got %q want my-sessions", sessions[0].GroupPath)
	}
}

func TestNewGroupDialogCreatesGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	for _, r := range "work/frontend" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	groups, err := db.ListGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		if g.Path == "work/frontend" {
			found = true
		}
	}
	if !found {
		t.Errorf("group work/frontend not created")
	}
}

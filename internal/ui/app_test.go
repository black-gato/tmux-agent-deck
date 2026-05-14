package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/notify"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
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

func TestEnterOnUnstartedSessionAutoStarts(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	m := ui.NewModel(conn, fake, nil)

	db.CreateSession(conn, db.Session{
		ID:          "abc12345-0000-0000-0000-000000000000",
		Title:       "my-app",
		GroupPath:   "my-sessions",
		ProjectPath: "/tmp",
		Tool:        "claude",
		Status:      "stopped",
		CreatedAt:   1000,
	})
	m.Reload()

	// Navigate to the session (index 1: group is 0, session is 1)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.NewSessionCalls) != 1 {
		t.Fatalf("expected 1 NewSession call, got %d", len(fake.NewSessionCalls))
	}
	if m.PendingAttach == "" {
		t.Errorf("PendingAttach should be set after Enter on unstarted session")
	}
	// DB should be updated with tmux name
	s, _ := db.GetSession(conn, "abc12345-0000-0000-0000-000000000000")
	if s.TmuxSession == "" {
		t.Errorf("TmuxSession should be persisted after auto-start")
	}
}

func TestReloadFetchesPanesForSelectedSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-abc12345"] = "> "
	fake.Panes["ad-abc12345"] = []tmux.Pane{
		{Index: 0, Command: "claude"},
		{Index: 1, Command: "bash"},
	}

	db.CreateSession(conn, db.Session{
		ID:          "abc12345-0000-0000-0000-000000000000",
		Title:       "my-app",
		GroupPath:   "my-sessions",
		TmuxSession: "ad-abc12345",
		ProjectPath: "/tmp",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   1000,
	})

	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	// cursor=0 is the group; move to session at index 1
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	panes := m.Panes()
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}
	if panes[0].Command != "claude" {
		t.Errorf("pane[0].Command: got %q want claude", panes[0].Command)
	}
}

func TestReloadFetchesOutputForSelectedSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-abc12345"] = "Running tests...\n✓ 12 pass\n> "

	db.CreateSession(conn, db.Session{
		ID:          "abc12345-0000-0000-0000-000000000000",
		Title:       "my-app",
		GroupPath:   "my-sessions",
		TmuxSession: "ad-abc12345",
		ProjectPath: "/tmp",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   1000,
	})

	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	if !strings.Contains(m.Output(), "12 pass") {
		t.Errorf("output missing captured pane output, got: %q", m.Output())
	}
}

func TestVTogglesFullScreen(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	if m.ViewFull() {
		t.Fatal("viewFull should start false")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !m.ViewFull() {
		t.Error("viewFull should be true after v")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if m.ViewFull() {
		t.Error("viewFull should be false after second v")
	}
}

func TestEOnSessionOpensEditNotes(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	// cursor 0 = group, move to session
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.Mode() != "edit-notes" {
		t.Errorf("expected mode edit-notes, got %q", m.Mode())
	}
}

func TestEOnGroupHasNoEffect(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	// cursor 0 = group
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.Mode() != "" {
		t.Errorf("e on group should not change mode, got %q", m.Mode())
	}
}

func TestEditNotesEnterSaves(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // select session
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}) // open edit-notes

	for _, r := range "my note" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // save

	if m.Mode() != "" {
		t.Errorf("mode should clear after Enter, got %q", m.Mode())
	}
	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Notes != "my note" {
		t.Errorf("notes: got %q want my note", s.Notes)
	}
}

func TestEditNotesEscDiscards(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	for _, r := range "discard me" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Notes != "" {
		t.Errorf("notes should not be saved on Esc, got %q", s.Notes)
	}
}

func TestDetailPanelShowsSessionTitle(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "some output\n> "

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-feature", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	panel := m.RenderDetailPanel(60, 20)
	if !strings.Contains(panel, "my-feature") {
		t.Errorf("detail panel missing session title, got:\n%s", panel)
	}
}

func TestDetailPanelShowsNotes(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-feature", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "waiting", CreatedAt: 1000,
	})
	db.UpdateSessionNotes(conn, "s1", "check divergences first")

	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	panel := m.RenderDetailPanel(60, 20)
	if !strings.Contains(panel, "check divergences first") {
		t.Errorf("detail panel missing notes, got:\n%s", panel)
	}
}

func TestDetailPanelShowsPaneList(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "
	fake.Panes["ad-s1"] = []tmux.Pane{{Index: 0, Command: "claude"}, {Index: 1, Command: "bash"}}

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-feature", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	panel := m.RenderDetailPanel(60, 20)
	if !strings.Contains(panel, "[0] claude") {
		t.Errorf("detail panel missing pane list, got:\n%s", panel)
	}
	if !strings.Contains(panel, "[1] bash") {
		t.Errorf("detail panel missing pane [1], got:\n%s", panel)
	}
}

func TestViewRendersSplitLayout(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := m.View()
	if !strings.Contains(view, "│") {
		t.Errorf("split layout view should contain │ divider, got:\n%s", view)
	}
	if !strings.Contains(view, "SESSIONS") {
		t.Errorf("split layout view should contain SESSIONS header, got:\n%s", view)
	}
}

func TestViewShowsEmptyStateOnboardingWhenNoSessionsExist(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := m.View()
	if !strings.Contains(view, "Press n to create your first session") {
		t.Fatalf("expected onboarding hint, got:\n%s", view)
	}
}

func TestViewDoesNotShowEmptyStateOnboardingWhenSessionExists(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-feature", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := m.View()
	if strings.Contains(view, "Press n to create your first session") {
		t.Fatalf("did not expect onboarding hint once sessions exist, got:\n%s", view)
	}
}

func TestViewFullScreenHidesLeftColumn(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "output\n> "
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-feature", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m.Reload()

	view := m.View()
	if strings.Contains(view, "SESSIONS") {
		t.Errorf("full-screen view should not show SESSIONS column, got:\n%s", view)
	}
}

func TestEnterOnRunningSessionAttachesWithoutRestart(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-existing"] = "> "
	m := ui.NewModel(conn, fake, nil)

	db.CreateSession(conn, db.Session{
		ID:          "abc12345-0000-0000-0000-000000000001",
		Title:       "running-app",
		GroupPath:   "my-sessions",
		TmuxSession: "ad-existing",
		ProjectPath: "/tmp",
		Tool:        "claude",
		Status:      "waiting",
		CreatedAt:   1000,
	})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.NewSessionCalls) != 0 {
		t.Errorf("should not spawn new session when one already exists, got %d calls", len(fake.NewSessionCalls))
	}
	if m.PendingAttach != "ad-existing" {
		t.Errorf("PendingAttach: got %q want ad-existing", m.PendingAttach)
	}
}

func TestSendPaneCallsSendKeys(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "
	fake.Panes["ad-s1"] = []tmux.Pane{{Index: 0, Command: "claude"}}

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Session != "ad-s1" {
		t.Errorf("session: got %q want ad-s1", fake.SentKeys[0].Session)
	}
	if fake.SentKeys[0].Keys != "hello" {
		t.Errorf("keys: got %q want hello", fake.SentKeys[0].Keys)
	}
	if fake.SentKeys[0].PaneIndex != 0 {
		t.Errorf("pane: got %d want 0", fake.SentKeys[0].PaneIndex)
	}
}

func TestSendPaneCtrlCharSent(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Keys != "C-c" {
		t.Errorf("keys: got %q want C-c", fake.SentKeys[0].Keys)
	}
}

func TestSpaceOnSessionTogglesSelection(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.SelectedCount() != 1 {
		t.Fatalf("selected count: got %d want 1", m.SelectedCount())
	}
	if !m.Items()[1].Selected {
		t.Fatal("selected session should be marked on the list item")
	}

	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count: got %d want 0", m.SelectedCount())
	}
	if m.Items()[1].Selected {
		t.Fatal("selected session should unmark after second toggle")
	}
}

func TestSpaceOnGroupKeepsSelectionEmpty(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count: got %d want 0", m.SelectedCount())
	}
}

func TestReloadPrunesSelectionForDeletedSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if err := db.DeleteSession(conn, "s1"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := m.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count after reload: got %d want 0", m.SelectedCount())
	}
}

func TestViewShowsSelectedCountInFooter(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})

	view := m.View()
	if !strings.Contains(view, "[1 selected]") {
		t.Fatalf("footer missing selected count: %q", view)
	}
}

func TestDeleteKillsSelectedSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "first", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "second", GroupPath: "my-sessions", TmuxSession: "ad-s2",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if len(fake.KillCalls) != 2 {
		t.Fatalf("expected 2 kill calls, got %d", len(fake.KillCalls))
	}
	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count: got %d want 0", m.SelectedCount())
	}
}

func TestMoveSelectedSessionsToPromptedGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "first", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "second", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	for _, r := range "work" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, id := range []string{"s1", "s2"} {
		s, err := db.GetSession(conn, id)
		if err != nil {
			t.Fatal(err)
		}
		if s.GroupPath != "work" {
			t.Fatalf("%s group_path: got %q want work", id, s.GroupPath)
		}
	}
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count: got %d want 0", m.SelectedCount())
	}
}

func TestSendPaneSendsToSelectedRunningSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "first", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1002,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "second", GroupPath: "my-sessions", TmuxSession: "ad-s2",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s3", Title: "third", GroupPath: "my-sessions", TmuxSession: "ad-s3",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	for _, call := range fake.SentKeys {
		if call.Keys != "hello" {
			t.Fatalf("keys: got %q want hello", call.Keys)
		}
		if call.PaneIndex != 0 {
			t.Fatalf("pane index: got %d want 0", call.PaneIndex)
		}
	}
	if m.SelectedCount() != 0 {
		t.Fatalf("selected count: got %d want 0", m.SelectedCount())
	}
}

func TestArchiveHidesSessionFromDefaultList(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Archived {
		t.Fatal("session should be archived")
	}
	if s.Status != "stopped" {
		t.Fatalf("status: got %q want stopped", s.Status)
	}
	if len(fake.KillCalls) != 1 || fake.KillCalls[0] != "ad-s1" {
		t.Fatalf("kill calls: got %#v", fake.KillCalls)
	}
	for _, item := range m.Items() {
		if item.Kind == "session" && item.Session.Title == "my-app" {
			t.Fatalf("archived session should be hidden from default items: %#v", m.Items())
		}
	}
}

func TestToggleArchivedShowsArchivedSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "archived",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000, Archived: true,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	for _, item := range m.Items() {
		if item.Kind == "session" && item.Session.Title == "my-app" {
			t.Fatal("archived session should start hidden")
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	found := false
	for _, item := range m.Items() {
		if item.Kind == "session" && item.Session.Title == "my-app" {
			found = true
		}
	}
	if !found {
		t.Fatalf("archived session should appear after A: %#v", m.Items())
	}
}

func TestArchiveRestoresSessionFromArchivedView(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "archived",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000, Archived: true,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Archived {
		t.Fatal("session should be restored")
	}
	if s.GroupPath != "my-sessions" {
		t.Fatalf("group_path: got %q want my-sessions", s.GroupPath)
	}
}

func TestEditTagsSavesSessionTags(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	for _, r := range "backend api" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Tags != "backend api" {
		t.Fatalf("tags: got %q want backend api", s.Tags)
	}
}

func TestSlashOpensFilterDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	if m.Mode() != "filter" {
		t.Fatalf("mode: got %q want filter", m.Mode())
	}
	if !strings.Contains(m.View(), "Filter:") {
		t.Fatalf("view missing filter prompt: %q", m.View())
	}
}

func TestSearchFiltersSessionsByTagPrefix(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "api", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1001, Tags: "backend",
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "frontend", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000, Tags: "ui",
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "#back" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	foundAPI := false
	foundFrontend := false
	for _, item := range m.Items() {
		if item.Kind != "session" {
			continue
		}
		if item.Session.Title == "api" {
			foundAPI = true
		}
		if item.Session.Title == "frontend" {
			foundFrontend = true
		}
	}
	if !foundAPI {
		t.Fatal("expected tagged session to remain visible")
	}
	if foundFrontend {
		t.Fatal("expected non-matching tagged session to be filtered out")
	}
}

func TestSearchFiltersSessionsByTitle(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "api-server", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "frontend", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "api" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	foundAPI := false
	foundFrontend := false
	for _, item := range m.Items() {
		if item.Kind != "session" {
			continue
		}
		if item.Session.Title == "api-server" {
			foundAPI = true
		}
		if item.Session.Title == "frontend" {
			foundFrontend = true
		}
	}
	if !foundAPI {
		t.Fatal("expected matching title to remain visible")
	}
	if foundFrontend {
		t.Fatal("expected non-matching title to be filtered out")
	}
}

func TestFilterCollapsesGroupsWithoutMatches(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "frontend", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "backend", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "front" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, item := range m.Items() {
		if item.Kind == "group" && item.Group.Path == "work" {
			t.Fatalf("non-matching group should be hidden: %#v", m.Items())
		}
	}
}

func TestEscClearsAppliedFilter(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "api-server", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "frontend", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "api" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	foundAPI := false
	foundFrontend := false
	for _, item := range m.Items() {
		if item.Kind != "session" {
			continue
		}
		if item.Session.Title == "api-server" {
			foundAPI = true
		}
		if item.Session.Title == "frontend" {
			foundFrontend = true
		}
	}
	if !foundAPI || !foundFrontend {
		t.Fatalf("expected all sessions after clearing filter, got %#v", m.Items())
	}
}

func TestQuestionMarkOpensHelpOverlay(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if m.Mode() != "help" {
		t.Fatalf("mode: got %q want help", m.Mode())
	}
	view := m.View()
	if !strings.Contains(view, "KEYBOARD SHORTCUTS") {
		t.Fatalf("help overlay missing title: %q", view)
	}
	if !strings.Contains(view, "?") || !strings.Contains(view, "Help") {
		t.Fatalf("help overlay missing key binding table: %q", view)
	}
}

func TestQuestionMarkDismissesHelpOverlayAndPreservesCursor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	before := m.Cursor()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if m.Mode() != "" {
		t.Fatalf("mode: got %q want empty", m.Mode())
	}
	if m.Cursor() != before {
		t.Fatalf("cursor: got %d want %d", m.Cursor(), before)
	}
}

func TestQDismissesHelpOverlayWithoutQuitting(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if model.(*ui.Model).Mode() != "" {
		t.Fatalf("mode: got %q want empty", model.(*ui.Model).Mode())
	}
	if cmd != nil {
		t.Fatal("q in help overlay should dismiss overlay, not quit")
	}
}

func TestCOnSessionSetsGroupConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "first", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "second", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	moveCursorToSession(t, m, "first")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	group, err := db.GetGroup(conn, "work")
	if err != nil {
		t.Fatal(err)
	}
	if group.ConductorSessionID != "s1" {
		t.Fatalf("conductor_session_id: got %q want %q", group.ConductorSessionID, "s1")
	}
}

func TestCReplacesPriorGroupConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true, ConductorSessionID: "s1"})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "first", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "second", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	moveCursorToSession(t, m, "second")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	group, err := db.GetGroup(conn, "work")
	if err != nil {
		t.Fatal(err)
	}
	if group.ConductorSessionID != "s2" {
		t.Fatalf("conductor_session_id: got %q want %q", group.ConductorSessionID, "s2")
	}
}

func TestDetailPanelShowsConductorState(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true, ConductorSessionID: "s1"})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "lead", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	moveCursorToSession(t, m, "lead")

	panel := m.RenderDetailPanel(60, 20)
	if !strings.Contains(panel, "conductor: true") {
		t.Fatalf("detail panel missing conductor state: %q", panel)
	}
}

func TestCEscalatesWaitingSessionToConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-worker"] = "Investigating flaky test\nNeed decision on retry policy\n> "
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true, ConductorSessionID: "lead"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "lead", Title: "conductor", GroupPath: "work", TmuxSession: "ad-lead",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "ad-worker",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000, Notes: "blocked on retry choice",
	}); err != nil {
		t.Fatal(err)
	}
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	moveCursorToSession(t, m, "worker")
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	call := fake.SentKeys[0]
	if call.Session != "ad-lead" {
		t.Fatalf("session: got %q want ad-lead", call.Session)
	}
	if !strings.Contains(call.Keys, "Escalation from worker") {
		t.Fatalf("message missing title: %q", call.Keys)
	}
	if !strings.Contains(call.Keys, "blocked on retry choice") {
		t.Fatalf("message missing notes: %q", call.Keys)
	}
	if !strings.Contains(call.Keys, "Need decision on retry policy") {
		t.Fatalf("message missing output context: %q", call.Keys)
	}
}

func TestCErrorsWhenConductorIsMissing(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	m := ui.NewModel(conn, testutil.NewFakeTmuxClient(), nil)
	m.Reload()
	moveCursorToSession(t, m, "worker")

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})

	if !strings.Contains(m.View(), "error: resolve conductor") {
		t.Fatalf("expected error view, got %q", m.View())
	}
}

func moveCursorToSession(t *testing.T, m *ui.Model, title string) {
	t.Helper()
	for idx, item := range m.Items() {
		if item.Kind == "session" && item.Session.Title == title {
			for m.Cursor() < idx {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			}
			for m.Cursor() > idx {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
			}
			return
		}
	}
	t.Fatalf("session %q not found in items: %#v", title, m.Items())
}

func TestReloadAnnotatesWaitingSessionsWithElapsedTime(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "

	base := time.Unix(1_700_000_000, 0)
	current := base
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "waiting", CreatedAt: base.Unix(),
	})

	poller := state.NewWithClock(conn, fake, notify.New(notify.Config{}), func() time.Time { return current })
	poller.PollOnce()
	current = current.Add(65 * time.Second)

	m := ui.NewModel(conn, fake, poller)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	if got := m.Items()[1].WaitLabel; got != "1m" {
		t.Fatalf("wait label: got %q want 1m", got)
	}
}

func TestViewShowsErrorCountAndOverdueWaitingBadge(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-wait"] = "> "

	base := time.Unix(1_700_000_000, 0)
	current := base
	db.CreateSession(conn, db.Session{
		ID: "wait-1", Title: "waiting-app", GroupPath: "my-sessions",
		TmuxSession: "ad-wait", ProjectPath: "/p", Tool: "claude",
		Status: "waiting", CreatedAt: base.Unix(),
	})
	db.CreateSession(conn, db.Session{
		ID: "err-1", Title: "broken-app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude",
		Status: "error", CreatedAt: base.Unix(),
	})

	poller := state.NewWithClock(conn, fake, notify.New(notify.Config{}), func() time.Time { return current })
	poller.PollOnce()
	current = current.Add(31 * time.Second)

	m := ui.NewModel(conn, fake, poller)
	m.Reload()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	view := m.View()
	if !strings.Contains(view, "✕ 1 error") {
		t.Fatalf("view missing error count, got:\n%s", view)
	}
	if !strings.Contains(view, "!1") {
		t.Fatalf("view missing overdue waiting badge, got:\n%s", view)
	}
}

func TestSendPaneNoOpWithoutTmuxSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "my-app", GroupPath: "my-sessions",
		TmuxSession: "", ProjectPath: "/p", Tool: "claude",
		Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 0 {
		t.Errorf("expected no SendKeys calls, got %d", len(fake.SentKeys))
	}
}

func TestForkSessionClonesFields(t *testing.T) {
	conn := testutil.OpenTestDB(t)

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "original", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/my/project", Tool: "aider",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	for _, r := range "forked" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	var forked *db.Session
	for i := range sessions {
		if sessions[i].Title == "forked" {
			forked = &sessions[i]
		}
	}
	if forked == nil {
		t.Fatal("forked session not found in DB")
	}
	if forked.ProjectPath != "/my/project" {
		t.Errorf("ProjectPath: got %q want /my/project", forked.ProjectPath)
	}
	if forked.Tool != "aider" {
		t.Errorf("Tool: got %q want aider", forked.Tool)
	}
	if forked.GroupPath != "my-sessions" {
		t.Errorf("GroupPath: got %q want my-sessions", forked.GroupPath)
	}
	if forked.Status != "stopped" {
		t.Errorf("Status: got %q want stopped", forked.Status)
	}
}

func TestBroadcastDirectGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateGroup(conn, db.Group{Path: "my-sessions/sub", Name: "sub", DefaultTool: "claude", Expanded: true})

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	fake.Sessions["ad-s1"] = "> "
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "b", GroupPath: "my-sessions",
		TmuxSession: "ad-s2", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1001,
	})
	fake.Sessions["ad-s2"] = "> "
	db.CreateSession(conn, db.Session{
		ID: "s3", Title: "c", GroupPath: "my-sessions/sub",
		TmuxSession: "ad-s3", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1002,
	})
	fake.Sessions["ad-s3"] = "> "

	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	// cursor=0 is the "my-sessions" group

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	// scope=false by default (this group only)
	for _, r := range "ping" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 2 {
		t.Fatalf("expected 2 SendKeys calls (direct group only), got %d", len(fake.SentKeys))
	}
	for _, sk := range fake.SentKeys {
		if sk.Session == "ad-s3" {
			t.Errorf("sub-group session ad-s3 should not receive direct-group broadcast")
		}
	}
}

func TestBroadcastIncludesSubGroups(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateGroup(conn, db.Group{Path: "my-sessions/sub", Name: "sub", DefaultTool: "claude", Expanded: true})

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	fake.Sessions["ad-s1"] = "> "
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "b", GroupPath: "my-sessions/sub",
		TmuxSession: "ad-s2", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1001,
	})
	fake.Sessions["ad-s2"] = "> "

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // toggle to include sub-groups
	for _, r := range "ping" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 2 {
		t.Fatalf("expected 2 SendKeys calls (group + sub-group), got %d", len(fake.SentKeys))
	}
	sent := map[string]bool{}
	for _, sk := range fake.SentKeys {
		sent[sk.Session] = true
	}
	if !sent["ad-s1"] {
		t.Error("ad-s1 (direct group) should receive broadcast")
	}
	if !sent["ad-s2"] {
		t.Error("ad-s2 (sub-group) should receive broadcast")
	}
}

func TestBroadcastSkipsNonRunning(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	fake.Sessions["ad-s1"] = "> "
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "b", GroupPath: "my-sessions",
		TmuxSession: "ad-s2", ProjectPath: "/p", Tool: "claude",
		Status: "stopped", CreatedAt: 1001,
	})

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	for _, r := range "ping" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call (running only), got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Session != "ad-s1" {
		t.Errorf("expected ad-s1, got %q", fake.SentKeys[0].Session)
	}
}

func TestCyclePaneAdvancesIndex(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "
	fake.Panes["ad-s1"] = []tmux.Pane{
		{Index: 0, Command: "claude"},
		{Index: 1, Command: "bash"},
		{Index: 2, Command: "nvim"},
	}
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()

	if m.ActivePaneIdx() != 0 {
		t.Fatalf("expected 0 initially, got %d", m.ActivePaneIdx())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePaneIdx() != 1 {
		t.Errorf("expected 1 after first Tab, got %d", m.ActivePaneIdx())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePaneIdx() != 2 {
		t.Errorf("expected 2 after second Tab, got %d", m.ActivePaneIdx())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePaneIdx() != 0 {
		t.Errorf("expected wrap to 0 after third Tab, got %d", m.ActivePaneIdx())
	}
}

func TestCtrlCQuitsInNavigationMode(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestCtrlCCancelsNonSendDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.Mode() != "new-session" {
		t.Fatal("expected new-session mode")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if m.Mode() != "" {
		t.Fatalf("expected mode empty after ctrl+c, got %q", m.Mode())
	}
	if strings.Contains(m.View(), "error:") {
		t.Fatalf("expected no error after ctrl+c cancel, got: %q", m.View())
	}
}

func TestCtrlCInSendPaneDialogAppendsToBuffer(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "output"
	if err := db.CreateSession(conn, db.Session{
		ID: "s1", Title: "agent", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	moveCursorToSession(t, m, "agent")

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.Mode() != "send-pane" {
		t.Fatal("expected send-pane mode")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.Mode() != "send-pane" {
		t.Fatalf("ctrl+c should stay in send-pane mode, got %q", m.Mode())
	}
	// confirm — the sent keys should contain "C-c"
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(fake.SentKeys) == 0 {
		t.Fatal("expected SendKeys call")
	}
	if !strings.Contains(fake.SentKeys[0].Keys, "C-c") {
		t.Fatalf("expected C-c in sent keys, got %q", fake.SentKeys[0].Keys)
	}
}

func TestErrorViewShowsQuitHint(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	// force m.err via Init() on a closed DB (Init sets m.err on Reload failure)
	conn.Close()
	m.Init()

	view := m.View()
	if !strings.Contains(view, "error:") {
		t.Fatalf("expected error view, got %q", view)
	}
	if !strings.Contains(view, "ctrl+c") {
		t.Fatalf("expected quit hint in error view, got %q", view)
	}
}

func TestNewGroupDuplicatePathShowsFriendlyError(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true}); err != nil {
		t.Fatal(err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	for _, r := range "work" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()
	if strings.Contains(view, "UNIQUE constraint") {
		t.Fatalf("expected friendly error, got raw SQL: %q", view)
	}
	if !strings.Contains(view, "already exists") {
		t.Fatalf("expected 'already exists' in error view, got: %q", view)
	}
}

func TestCOnNonWaitingSessionIsNoOp(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateSession(conn, db.Session{
		ID: "s1", Title: "running-session", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	m := ui.NewModel(conn, testutil.NewFakeTmuxClient(), nil)
	m.Reload()
	moveCursorToSession(t, m, "running-session")

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})

	if strings.Contains(m.View(), "error:") {
		t.Fatalf("expected no error for C on non-waiting session, got: %q", m.View())
	}
	if m.Mode() != "" {
		t.Fatalf("expected empty mode, got %q", m.Mode())
	}
}

func TestCyclePaneResetsOnReload(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-s1"] = "> "
	fake.Panes["ad-s1"] = []tmux.Pane{
		{Index: 0, Command: "claude"},
		{Index: 1, Command: "bash"},
	}
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.ActivePaneIdx() != 1 {
		t.Fatalf("expected 1 after Tab, got %d", m.ActivePaneIdx())
	}
	m.Reload()
	if m.ActivePaneIdx() != 0 {
		t.Errorf("expected reset to 0 after Reload, got %d", m.ActivePaneIdx())
	}
}

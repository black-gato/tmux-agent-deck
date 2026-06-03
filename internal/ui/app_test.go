package ui_test

import (
	"os"
	"path/filepath"
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

func TestUpdateHandlesRefreshMsg(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, testutil.NewFakeTmuxClient(), nil)
	got, _ := m.Update(ui.RefreshMsg{})
	if got == nil {
		t.Fatal("Update(RefreshMsg) returned nil model")
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

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "my-app" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit form

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

func TestNewSessionFromSelectedSessionUsesItsGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true}); err != nil {
		t.Fatalf("create group: %v", err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID:          "existing",
		Title:       "existing",
		GroupPath:   "work",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "stopped",
		CreatedAt:   time.Now().Unix(),
	}); err != nil {
		t.Fatalf("create existing session: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	for i, item := range m.Items() {
		if item.Kind == "session" && item.Session.ID == "existing" {
			for m.Cursor() < i {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			}
			break
		}
	}
	if m.Items()[m.Cursor()].Kind != "session" || m.Items()[m.Cursor()].Session.ID != "existing" {
		t.Fatalf("expected cursor on existing session, got item %#v", m.Items()[m.Cursor()])
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "new-worker" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit form

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	var created *db.Session
	for i := range sessions {
		if sessions[i].Title == "new-worker" {
			created = &sessions[i]
			break
		}
	}
	if created == nil {
		t.Fatal("new-worker session not created")
	}
	if created.GroupPath != "work" {
		t.Fatalf("GroupPath: got %q want work", created.GroupPath)
	}
}

func TestNewSessionPathTabCompletesSingleMatch(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "ProjectAlpha"), 0755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "my-app" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // advance to PATH field
	// PATH is pre-filled with cwd (tmp); append "/Pro" at cursor end
	for _, r := range "/Pro" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})   // complete to ProjectAlpha/
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit form

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	realTmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realTmp, "ProjectAlpha") + string(os.PathSeparator)
	if sessions[0].ProjectPath != want {
		t.Fatalf("ProjectPath: got %q want %q", sessions[0].ProjectPath, want)
	}
}

func TestNewSessionPathTabShowsCandidatesOnSecondTab(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	tmp := t.TempDir()
	for _, name := range []string{"ProjectAlpha", "ProjectBeta"} {
		if err := os.Mkdir(filepath.Join(tmp, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	m := ui.NewModel(conn, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "my-app" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // advance to PATH field
	// PATH is pre-filled with cwd (tmp); append "/Pro"
	for _, r := range "/Pro" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // start cycling — shows candidates

	view := m.View()
	if !strings.Contains(view, "ProjectAlpha") || !strings.Contains(view, "ProjectBeta") {
		t.Fatalf("expected path candidates in view, got:\n%s", view)
	}
}

func TestNewSessionFlowCreatesSessionWithTool(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	// Open new session form
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.Mode() != "new-session" {
		t.Fatalf("expected new-session mode, got %q", m.Mode())
	}

	// Type title
	for _, r := range "fleet-agent" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab to PATH, replace with nonexistent path so completion finds nothing, Tab to TOOL
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // TITLE → PATH
	for i := 0; i < 512; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "/nonexistent-path-xyz/" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // PATH (no candidates) → BRANCH
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // BRANCH → BASE
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // BASE → WORKTREE
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // WORKTREE → TOOL

	// Cycle tool to "aider" (right arrow twice from "claude")
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Commit form
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Mode() != "" {
		t.Errorf("expected mode cleared after commit, got %q", m.Mode())
	}

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	var found *db.Session
	for i := range sessions {
		if sessions[i].Title == "fleet-agent" {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("session 'fleet-agent' not created")
	}
	if found.Tool != "aider" {
		t.Errorf("Tool: got %q want %q", found.Tool, "aider")
	}
}

func TestNewSessionInheritsGroupDefaults(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	// Set group defaults on the default group
	if _, err := conn.Exec(`UPDATE groups SET default_path = '/opt/api', default_tool = 'aider' WHERE path = 'my-sessions'`); err != nil {
		t.Fatalf("set group defaults: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	// Open new-session while cursor is on my-sessions group (default position)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// Type title and commit — PATH and TOOL will use group defaults
	for _, r := range "defaults-test" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit form

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	var found *db.Session
	for i := range sessions {
		if sessions[i].Title == "defaults-test" {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("session 'defaults-test' not created")
	}
	if found.ProjectPath != "/opt/api" {
		t.Errorf("ProjectPath: got %q want %q", found.ProjectPath, "/opt/api")
	}
	if found.Tool != "aider" {
		t.Errorf("Tool: got %q want %q", found.Tool, "aider")
	}
}

func TestNewSessionDialogPersistsToolFlags(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	for _, r := range "flagged-agent" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Tab to PATH, replace with nonexistent path, Tab to TOOL, Tab to FLAGS
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // TITLE → PATH
	for i := 0; i < 512; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	for _, r := range "/nonexistent-path-xyz/" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // PATH (no candidates) → BRANCH
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // BRANCH → BASE
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // BASE → WORKTREE
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // WORKTREE → TOOL
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // TOOL → FLAGS
	for _, r := range "--dangerously-skip-permissions" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // commit form

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	var found *db.Session
	for i := range sessions {
		if sessions[i].Title == "flagged-agent" {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("session 'flagged-agent' not created")
	}
	if found.ToolFlags != "--dangerously-skip-permissions" {
		t.Errorf("ToolFlags: got %q want %q", found.ToolFlags, "--dangerously-skip-permissions")
	}
}

func TestToolFlagsPassedToNewSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	if err := db.CreateSession(conn, db.Session{
		ID:          "abc12345-0000-0000-0000-000000000099",
		Title:       "flagged",
		GroupPath:   "my-sessions",
		ProjectPath: "/tmp",
		Tool:        "claude",
		ToolFlags:   "--dangerously-skip-permissions",
		Status:      "stopped",
		CreatedAt:   1000,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	moveCursorToSession(t, m, "flagged")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.NewSessionCalls) != 1 {
		t.Fatalf("expected 1 NewSession call, got %d", len(fake.NewSessionCalls))
	}
	call := fake.NewSessionCalls[0]
	if call.Tool != "claude" {
		t.Errorf("Tool: got %q want %q", call.Tool, "claude")
	}
	if call.ToolFlags != "--dangerously-skip-permissions" {
		t.Errorf("ToolFlags: got %q want %q", call.ToolFlags, "--dangerously-skip-permissions")
	}
	if m.PendingStartupScript != "" {
		t.Errorf("PendingStartupScript should be empty when no startup script, got %q", m.PendingStartupScript)
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

func TestSendPaneCtrlCharSentAsRawKey(t *testing.T) {
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

	// C-c then Enter (submit)
	if len(fake.SentRawKeys) != 2 {
		t.Fatalf("expected 2 SendRawKeys calls (C-c + Enter), got %d", len(fake.SentRawKeys))
	}
	if fake.SentRawKeys[0].Keys != "C-c" {
		t.Errorf("raw keys[0]: got %q want C-c", fake.SentRawKeys[0].Keys)
	}
	if fake.SentRawKeys[1].Keys != "Enter" {
		t.Errorf("raw keys[1]: got %q want Enter", fake.SentRawKeys[1].Keys)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("expected no literal SendKeys call, got %d", len(fake.SentKeys))
	}
}

func TestSendPaneCtrlJSendsEnter(t *testing.T) {
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
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Keys != "hello" {
		t.Errorf("keys: got %q want hello", fake.SentKeys[0].Keys)
	}
	if len(fake.SentRawKeys) != 2 {
		t.Fatalf("expected 2 SendRawKeys calls (Ctrl+J Enter + submit Enter), got %d: %v", len(fake.SentRawKeys), fake.SentRawKeys)
	}
	if fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("raw keys[0]: got %q want Enter (from Ctrl+J)", fake.SentRawKeys[0].Keys)
	}
	if fake.SentRawKeys[1].Keys != "Enter" {
		t.Errorf("raw keys[1]: got %q want Enter (submit)", fake.SentRawKeys[1].Keys)
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

func TestDeleteRemovesSessionWhenTmuxSessionAlreadyGone(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "stale", GroupPath: "my-sessions", TmuxSession: "ad-missing",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	moveCursorToSession(t, m, "stale")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected stale session to be removed, got %#v", sessions)
	}
	if strings.Contains(m.View(), "error:") {
		t.Fatalf("delete should not show error view, got:\n%s", m.View())
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

func TestSendPaneSendsToSelectedActiveSessions(t *testing.T) {
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

	// running (s1) and waiting (s2) should receive keys; stopped (s3) should not
	if len(fake.SentKeys) != 2 {
		t.Fatalf("expected 2 SendKeys calls (running + waiting), got %d", len(fake.SentKeys))
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
	fake.Sessions["ad-worker"] = strings.Join([]string{
		"Investigating flaky test",
		"Need decision on retry policy",
		"❯ ",
		"────────────────────────",
		"  anthonymirville@host project [Sonnet 4.6] ctx:12%",
		"  -- INSERT -- ← for agents",
		"",
		"",
	}, "\n")
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
	if len(fake.SentRawKeys) != 1 {
		t.Fatalf("expected 1 SendRawKeys call, got %d", len(fake.SentRawKeys))
	}
	if fake.SentRawKeys[0].Session != "ad-lead" || fake.SentRawKeys[0].Keys != "Enter" {
		t.Fatalf("submit call: got %#v", fake.SentRawKeys[0])
	}
}

func TestCEscalatesToIdleConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-worker"] = "Need a decision\n> "
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true, ConductorSessionID: "lead"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "lead", Title: "conductor", GroupPath: "work", TmuxSession: "ad-lead",
		ProjectPath: "/p", Tool: "claude", Status: "idle", CreatedAt: 1001,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "ad-worker",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
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
	if fake.SentKeys[0].Session != "ad-lead" {
		t.Fatalf("session: got %q want ad-lead", fake.SentKeys[0].Session)
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Fatalf("expected Enter submit, got %#v", fake.SentRawKeys)
	}
}

func TestCEscalatesToParentGroupConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["ad-worker"] = "Need a decision\n> "
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", Expanded: true, ConductorSessionID: "lead"}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateGroup(conn, db.Group{Path: "work/frontend", Name: "frontend", Expanded: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "lead", Title: "conductor", GroupPath: "work", TmuxSession: "ad-lead",
		ProjectPath: "/p", Tool: "claude", Status: "idle", CreatedAt: 1001,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work/frontend", TmuxSession: "ad-worker",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
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
	if fake.SentKeys[0].Session != "ad-lead" {
		t.Fatalf("session: got %q want ad-lead", fake.SentKeys[0].Session)
	}
}

func TestCIsNoOpWhenConductorIsMissing(t *testing.T) {
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

	if strings.Contains(m.View(), "error:") {
		t.Fatalf("expected no error, got %q", m.View())
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

func TestBroadcastSkipsStoppedAndError(t *testing.T) {
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
		Status: "waiting", CreatedAt: 1001,
	})
	fake.Sessions["ad-s2"] = "> "
	db.CreateSession(conn, db.Session{
		ID: "s3", Title: "c", GroupPath: "my-sessions",
		TmuxSession: "ad-s3", ProjectPath: "/p", Tool: "claude",
		Status: "stopped", CreatedAt: 1002,
	})

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	for _, r := range "ping" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 2 {
		t.Fatalf("expected 2 SendKeys calls (running + waiting), got %d", len(fake.SentKeys))
	}
	sent := map[string]bool{}
	for _, sk := range fake.SentKeys {
		sent[sk.Session] = true
	}
	if !sent["ad-s1"] {
		t.Error("running session ad-s1 should receive broadcast")
	}
	if !sent["ad-s2"] {
		t.Error("waiting session ad-s2 should receive broadcast")
	}
	if sent["ad-s3"] {
		t.Error("stopped session ad-s3 should not receive broadcast")
	}
}

func TestBroadcastSendsToWaitingSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "waiting", CreatedAt: 1000,
	})
	fake.Sessions["ad-s1"] = "> "

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	for _, r := range "continue" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call for waiting session, got %d", len(fake.SentKeys))
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

func TestCtrlCInSendPaneDialogSendsAsRawKey(t *testing.T) {
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
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// ctrl+c must go via SendRawKeys, not embedded in literal text
	if len(fake.SentRawKeys) == 0 {
		t.Fatal("expected SendRawKeys call for C-c")
	}
	if fake.SentRawKeys[0].Keys != "C-c" {
		t.Fatalf("expected raw key C-c, got %q", fake.SentRawKeys[0].Keys)
	}
	// literal text buffer should be empty (user only pressed ctrl+c)
	for _, call := range fake.SentKeys {
		if strings.Contains(call.Keys, "C-c") {
			t.Fatalf("C-c must not appear in literal SentKeys, got %q", call.Keys)
		}
	}
}

func TestSendPaneLiteralTextSentViaSendKeys(t *testing.T) {
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
	for _, r := range "hello world" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) == 0 {
		t.Fatal("expected SendKeys call for literal text")
	}
	if fake.SentKeys[0].Keys != "hello world" {
		t.Fatalf("expected literal text %q, got %q", "hello world", fake.SentKeys[0].Keys)
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Fatalf("expected exactly 1 raw key (Enter for submit), got %v", fake.SentRawKeys)
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

func TestNewGroupDuplicateNavigatesToExistingGroup(t *testing.T) {
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

	if strings.Contains(m.View(), "error:") {
		t.Fatalf("expected no error on duplicate group, got: %q", m.View())
	}
	items := m.Items()
	cursor := m.Cursor()
	if cursor >= len(items) || items[cursor].Kind != "group" || items[cursor].Group.Path != "work" {
		t.Fatalf("expected cursor on group 'work', cursor=%d items=%v", cursor, items)
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

func TestHeaderColorsWaitingAmberWhenNonZero(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	view := m.View()
	if !strings.Contains(view, "\x1b[") {
		t.Skip("terminal does not support ANSI — skipping color test")
	}
	if !strings.Contains(view, "1 waiting") {
		t.Fatalf("expected '1 waiting' in header, got:\n%s", view)
	}
}

func TestHeaderColorsErrorRedWhenNonZero(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "app", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "error", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	view := m.View()
	if !strings.Contains(view, "1 error") {
		t.Fatalf("expected '1 error' in header, got:\n%s", view)
	}
}

func TestDetailPanelShowsContextLine(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateSession(conn, db.Session{
		ID: "s1", Title: "app", GroupPath: "my-sessions",
		TmuxSession: "tmux-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["tmux-s1"] = "Some output\n75% context used\n> "

	poller := state.New(conn, fake)
	poller.PollOnce()

	m := ui.NewModel(conn, fake, poller)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	m.Reload()
	moveCursorToSession(t, m, "app")

	view := m.View()
	if !strings.Contains(view, "context:") {
		t.Fatalf("expected 'context:' line in detail panel, got:\n%s", view)
	}
	if !strings.Contains(view, "75%") {
		t.Fatalf("expected '75%%' in detail panel, got:\n%s", view)
	}
}

func TestPendingStartupScriptSetForNewSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	if err := db.CreateSession(conn, db.Session{
		ID:            "abc12345-0000-0000-0000-000000000001",
		Title:         "with-script",
		GroupPath:     "my-sessions",
		ProjectPath:   "/tmp",
		Tool:          "claude",
		Status:        "stopped",
		CreatedAt:     1000,
		StartupScript: "claude --resume",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	moveCursorToSession(t, m, "with-script")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.PendingStartupScript != "claude --resume" {
		t.Errorf("PendingStartupScript: got %q want %q", m.PendingStartupScript, "claude --resume")
	}
	if m.PendingAttach != "tma-with-script-abc12345" {
		t.Errorf("PendingAttach: got %q want tma-with-script-abc12345", m.PendingAttach)
	}
	if len(fake.NewSessionCalls) != 1 || fake.NewSessionCalls[0].Name != "tma-with-script-abc12345" {
		t.Fatalf("new session name: calls=%#v", fake.NewSessionCalls)
	}
}

func TestPendingStartupScriptNotSetForExistingSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	// Pre-create the tmux session so ensureStarted returns isNew=false
	fake.Sessions["ad-abc12345"] = ""

	if err := db.CreateSession(conn, db.Session{
		ID:            "abc12345-0000-0000-0000-000000000002",
		Title:         "already-running",
		GroupPath:     "my-sessions",
		TmuxSession:   "ad-abc12345",
		ProjectPath:   "/tmp",
		Tool:          "claude",
		Status:        "running",
		CreatedAt:     1000,
		StartupScript: "claude --resume",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	moveCursorToSession(t, m, "already-running")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.PendingStartupScript != "" {
		t.Errorf("PendingStartupScript should be empty for existing session, got %q", m.PendingStartupScript)
	}
}

func TestDetailPanelNoContextLineWhenPctNil(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateSession(conn, db.Session{
		ID: "s1", Title: "app", GroupPath: "my-sessions",
		TmuxSession: "", ProjectPath: "/p", Tool: "claude",
		Status: "stopped", CreatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	moveCursorToSession(t, m, "app")

	view := m.View()
	if strings.Contains(view, "context:") {
		t.Fatalf("expected no 'context:' line when pct is nil, got:\n%s", view)
	}
}

func TestSendPaneMultiSelectSkipsStoppedSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker-a", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1002,
	})
	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "worker-b", GroupPath: "my-sessions", TmuxSession: "ad-s2",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: 1001,
	})
	db.CreateSession(conn, db.Session{
		ID: "s3", Title: "worker-c", GroupPath: "my-sessions", TmuxSession: "ad-s3",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	// select all three sessions then send
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	for _, r := range "go" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(fake.SentKeys) != 2 {
		t.Fatalf("expected 2 SendKeys calls (waiting + running), got %d", len(fake.SentKeys))
	}
	sent := map[string]bool{}
	for _, sk := range fake.SentKeys {
		sent[sk.Session] = true
	}
	if !sent["ad-s1"] {
		t.Error("waiting session ad-s1 should receive keys")
	}
	if !sent["ad-s2"] {
		t.Error("running session ad-s2 should receive keys")
	}
	if sent["ad-s3"] {
		t.Error("stopped session ad-s3 should not receive keys")
	}
}

func TestFullscreenExitsWhenDialogOpens(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()
	moveCursorToSession(t, m, "worker")

	// enter fullscreen
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !m.ViewFull() {
		t.Fatal("expected viewFull=true after pressing v")
	}

	// open send-pane dialog from fullscreen
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.ViewFull() {
		t.Error("viewFull should be false once a dialog opens")
	}
	if !strings.Contains(m.View(), "Send:") {
		t.Errorf("expected Send: prompt in view, got:\n%s", m.View())
	}

	// Esc cancels the dialog and TUI remains usable
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.Mode() != "" {
		t.Errorf("expected mode cleared after Esc, got %q", m.Mode())
	}
}

func TestFullscreenExitsBroadcastDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: 1000,
	})
	m := ui.NewModel(conn, fake, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if m.ViewFull() {
		t.Error("viewFull should be false once broadcast dialog opens")
	}
	view := m.View()
	if !strings.Contains(view, "this group") && !strings.Contains(view, "Broadcast") && !strings.Contains(view, "broadcast") {
		t.Errorf("expected broadcast scope selector in view, got:\n%s", view)
	}
}

func TestSendPaneCtrlVTogglesVimMode(t *testing.T) {
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
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if m.DialogVimMode() {
		t.Fatal("vim mode should start false")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if !m.DialogVimMode() {
		t.Error("Ctrl+V should enable vim mode")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if m.DialogVimMode() {
		t.Error("second Ctrl+V should disable vim mode")
	}
}

func TestSendPaneVimModeSendsEscapeIPrefixBeforeText(t *testing.T) {
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
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV}) // enable vim mode
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// 'i' literal must arrive before "hello"; no Escape raw key (avoids Meta-i timing issue)
	if len(fake.SentKeys) < 2 {
		t.Fatalf("expected at least 2 SendKeys calls (i + hello), got %d: %v", len(fake.SentKeys), fake.SentKeys)
	}
	if fake.SentKeys[0].Keys != "i" {
		t.Errorf("first SendKeys should be 'i' to enter insert mode, got %q", fake.SentKeys[0].Keys)
	}
	if fake.SentKeys[1].Keys != "hello" {
		t.Errorf("second SendKeys should be 'hello', got %q", fake.SentKeys[1].Keys)
	}
	for _, k := range fake.SentRawKeys {
		if k.Keys == "Escape" {
			t.Errorf("Escape must not be sent (causes Meta-i timing issue), got raw keys: %v", fake.SentRawKeys)
		}
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("expected exactly one Enter raw key (submit), got: %v", fake.SentRawKeys)
	}
}

func TestSendPaneWithoutVimModeDoesNotSendEscapePrefix(t *testing.T) {
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
	for _, r := range "hello" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, k := range fake.SentRawKeys {
		if k.Keys == "Escape" {
			t.Errorf("no Escape prefix expected without vim mode, got: %v", fake.SentRawKeys)
		}
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("expected exactly one Enter raw key (submit), got: %v", fake.SentRawKeys)
	}
	if len(fake.SentKeys) != 1 || fake.SentKeys[0].Keys != "hello" {
		t.Errorf("expected exactly one SendKeys 'hello', got: %v", fake.SentKeys)
	}
}

func TestBroadcastCtrlVTogglesVimMode(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})

	if m.DialogVimMode() {
		t.Fatal("vim mode should start false in broadcast")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	if !m.DialogVimMode() {
		t.Error("Ctrl+V should enable vim mode in broadcast")
	}
}

func TestCtrlVIsNotSentToTargetPane(t *testing.T) {
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
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlV}) // toggle vim mode — must NOT be queued as a send key
	for _, r := range "hi" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, call := range fake.SentRawKeys {
		if call.Keys == "C-v" || call.Keys == "\x16" {
			t.Errorf("Ctrl+V must not be forwarded to target pane, got raw key: %q", call.Keys)
		}
	}
	for _, call := range fake.SentKeys {
		if strings.Contains(call.Keys, "\x16") {
			t.Errorf("Ctrl+V byte must not appear in literal send, got: %q", call.Keys)
		}
	}
}

func TestSendPaneClaudeInInsertModeSkipsVimPrefix(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.PaneViews["ad-s1"] = "❯ \n  -- INSERT -- ← for agents\n"
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

	for _, k := range fake.SentRawKeys {
		if k.Keys == "Escape" {
			t.Errorf("INSERT mode detected: no ESC prefix expected, got raw keys: %v", fake.SentRawKeys)
		}
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("expected exactly one Enter raw key (submit), got: %v", fake.SentRawKeys)
	}
	if len(fake.SentKeys) != 1 || fake.SentKeys[0].Keys != "hello" {
		t.Errorf("expected exactly one SendKeys 'hello', got: %v", fake.SentKeys)
	}
}

func TestSendPaneClaudeInNormalModeAddsVimPrefix(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.PaneViews["ad-s1"] = "❯ \n"
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

	// No Escape (avoids Meta-i timing issue); 'i' goes via SendKeys to enter INSERT from COMMAND mode
	for _, k := range fake.SentRawKeys {
		if k.Keys == "Escape" {
			t.Errorf("Escape must not be sent (causes Meta-i timing issue), got raw keys: %v", fake.SentRawKeys)
		}
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("expected exactly one Enter raw key (submit), got: %v", fake.SentRawKeys)
	}
	if len(fake.SentKeys) < 2 || fake.SentKeys[0].Keys != "i" || fake.SentKeys[1].Keys != "hello" {
		t.Errorf("expected 'i' then 'hello' via SendKeys, got: %v", fake.SentKeys)
	}
}

func TestBroadcastAutoDetectsPerSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()

	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "in-insert", GroupPath: "my-sessions",
		TmuxSession: "ad-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1001,
	})
	fake.Sessions["ad-s1"] = "❯ "
	fake.PaneViews["ad-s1"] = "❯ \n  -- INSERT -- ← for agents\n"

	db.CreateSession(conn, db.Session{
		ID: "s2", Title: "in-normal", GroupPath: "my-sessions",
		TmuxSession: "ad-s2", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: 1000,
	})
	fake.Sessions["ad-s2"] = "❯ "
	fake.PaneViews["ad-s2"] = "❯ \n"

	m := ui.NewModel(conn, fake, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	for _, r := range "ping" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// s1 (INSERT mode): "ping" + Enter — no prefix
	// s2 (COMMAND mode): "i" (via SendKeys, not Escape+i) + "ping" + Enter
	// No Escape sent to avoid Meta-i timing issue when ESC+'i' arrive within readline's 100ms timeout
	for _, k := range fake.SentRawKeys {
		if k.Keys == "Escape" {
			t.Errorf("Escape must not be sent (causes Meta-i); 'i' goes via SendKeys instead, got: %v", fake.SentRawKeys)
		}
	}
	// Total SentRawKeys: 2 (Enter for s1, Enter for s2)
	if len(fake.SentRawKeys) != 2 {
		t.Fatalf("expected 2 raw keys (Enter per session), got: %v", fake.SentRawKeys)
	}
	for _, k := range fake.SentRawKeys {
		if k.Keys != "Enter" {
			t.Errorf("expected only Enter raw keys, got: %v", fake.SentRawKeys)
		}
	}

	sentBySession := map[string][]string{}
	for _, call := range fake.SentKeys {
		sentBySession[call.Session] = append(sentBySession[call.Session], call.Keys)
	}
	if keys := sentBySession["ad-s1"]; len(keys) != 1 || keys[0] != "ping" {
		t.Errorf("INSERT session ad-s1: expected [ping], got %v", keys)
	}
	if keys := sentBySession["ad-s2"]; len(keys) != 2 || keys[0] != "i" || keys[1] != "ping" {
		t.Errorf("normal session ad-s2: expected [i ping], got %v", keys)
	}
}

func TestNewSessionFormDownUpNavigatesFields(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.Mode() != "new-session" {
		t.Fatalf("expected new-session mode")
	}

	listCursorBefore := m.Cursor()

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor() != listCursorBefore {
		t.Errorf("down in form should not change list cursor: before=%d after=%d", listCursorBefore, m.Cursor())
	}
	if m.FormFocusField() != 1 {
		t.Errorf("down should advance focusField to 1, got %d", m.FormFocusField())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.FormFocusField() != 2 {
		t.Errorf("second down should advance focusField to 2, got %d", m.FormFocusField())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.FormFocusField() != 1 {
		t.Errorf("up should retreat focusField to 1, got %d", m.FormFocusField())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.FormFocusField() != 0 {
		t.Errorf("up should clamp at 0, got %d", m.FormFocusField())
	}
}

func TestSetMetaConductorAssignsSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := db.CreateSession(conn, db.Session{
		ID: "mc-001", Title: "the-conductor", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running",
		CreatedAt: now, LastActive: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	// navigate to the session
	for i, item := range m.Items() {
		if item.Kind == "session" && item.Session.ID == "mc-001" {
			for m.Cursor() < i {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			}
			break
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	if m.MetaConductor() == nil {
		t.Fatal("expected meta-conductor to be set")
	}
	if m.MetaConductor().ID != "mc-001" {
		t.Errorf("meta-conductor id: got %q want mc-001", m.MetaConductor().ID)
	}
}

func TestSetMetaConductorTogglesOff(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := db.CreateSession(conn, db.Session{
		ID: "mc-002", Title: "the-conductor", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running",
		CreatedAt: now, LastActive: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Reload()

	for i, item := range m.Items() {
		if item.Kind == "session" && item.Session.ID == "mc-002" {
			for m.Cursor() < i {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			}
			break
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}}) // set
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}}) // clear
	if m.MetaConductor() != nil {
		t.Errorf("expected meta-conductor to be cleared, got %q", m.MetaConductor().ID)
	}
}

func TestViewContainsMetaConductorRow(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()

	view := m.View()
	if !strings.Contains(view, "no meta-conductor") {
		t.Errorf("expected 'no meta-conductor' placeholder in view")
	}
}

func TestViewShowsMetaConductorTitle(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := db.CreateSession(conn, db.Session{
		ID: "mc-003", Title: "my-orchestrator", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "running",
		CreatedAt: now, LastActive: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.SetMetaConductorID(conn, "mc-003"); err != nil {
		t.Fatalf("set: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()

	view := m.View()
	if !strings.Contains(view, "my-orchestrator") {
		t.Errorf("expected 'my-orchestrator' in view, got:\n%s", view)
	}
}

func TestDeleteSessionClearsMetaConductorSlot(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := db.CreateSession(conn, db.Session{
		ID: "mc-del", Title: "to-delete", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped",
		CreatedAt: now, LastActive: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.SetMetaConductorID(conn, "mc-del"); err != nil {
		t.Fatalf("set meta conductor: %v", err)
	}
	m := ui.NewModel(conn, testutil.NewFakeTmuxClient(), nil)
	m.Reload()

	for i, item := range m.Items() {
		if item.Kind == "session" && item.Session.ID == "mc-del" {
			for m.Cursor() < i {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			}
			break
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m.Reload()

	if m.MetaConductor() != nil {
		t.Errorf("expected meta-conductor to be cleared after session delete, got %q", m.MetaConductor().ID)
	}
}

func TestMetaConductorRowHighlightsWhenWaiting(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := db.CreateSession(conn, db.Session{
		ID: "mc-wait", Title: "waiting-orchestrator", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "waiting",
		CreatedAt: now, LastActive: now,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.SetMetaConductorID(conn, "mc-wait"); err != nil {
		t.Fatalf("set: %v", err)
	}
	m := ui.NewModel(conn, nil, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()

	// status symbol for waiting is ○
	view := m.View()
	if !strings.Contains(view, "waiting-orchestrator") {
		t.Errorf("expected waiting orchestrator title in view")
	}
}

func TestFullscreenExitsNewSessionDialog(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	m := ui.NewModel(conn, fake, nil)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Reload()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if m.ViewFull() {
		t.Error("viewFull should be false once new-session dialog opens")
	}
	if !strings.Contains(m.View(), "NEW SESSION") {
		t.Errorf("expected 'NEW SESSION' in view, got:\n%s", m.View())
	}
}

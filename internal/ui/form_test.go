package ui_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func openModel(t *testing.T) (*ui.Model, *sql.DB) {
	t.Helper()
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, nil, nil)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	return m, conn
}

func sendKey(m *ui.Model, msg tea.KeyMsg) *ui.Model {
	next, _ := m.Update(msg)
	return next.(*ui.Model)
}

func key(t tea.KeyType) tea.KeyMsg   { return tea.KeyMsg{Type: t} }
func rune_(r rune) tea.KeyMsg        { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestNewSessionModeOpens(t *testing.T) {
	m, _ := openModel(t)
	m = sendKey(m, rune_('n'))
	if m.Mode() != "new-session" {
		t.Fatalf("expected new-session mode, got %q", m.Mode())
	}
}

func TestEscCancelsForm(t *testing.T) {
	m, _ := openModel(t)
	m = sendKey(m, rune_('n'))
	m = sendKey(m, key(tea.KeyEsc))
	if m.Mode() != "" {
		t.Fatalf("expected mode cleared, got %q", m.Mode())
	}
}

func TestTextFieldInsertAndCursor(t *testing.T) {
	m, conn := openModel(t)
	m = sendKey(m, rune_('n'))

	// type "ab", move left, insert 'x' → title becomes "axb"
	m = sendKey(m, rune_('a'))
	m = sendKey(m, rune_('b'))
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, rune_('x'))
	m = sendKey(m, key(tea.KeyEnter))

	if m.Mode() != "" {
		t.Fatalf("expected mode cleared after submit, got %q", m.Mode())
	}
	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Title != "axb" {
		t.Errorf("expected title %q, got %q", "axb", sessions[0].Title)
	}
}

func TestBackspace(t *testing.T) {
	m, conn := openModel(t)
	m = sendKey(m, rune_('n'))
	m = sendKey(m, rune_('a'))
	m = sendKey(m, rune_('b'))
	m = sendKey(m, key(tea.KeyBackspace))
	m = sendKey(m, key(tea.KeyEnter))

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Title != "a" {
		t.Errorf("expected title %q, got %q", "a", sessions[0].Title)
	}
}

func TestEnterWithEmptyTitleDoesNotCreate(t *testing.T) {
	m, conn := openModel(t)
	m = sendKey(m, rune_('n'))
	m = sendKey(m, key(tea.KeyEnter)) // empty title → no session created
	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestTabAdvancesField(t *testing.T) {
	m, conn := openModel(t)
	m = sendKey(m, rune_('n'))
	m = sendKey(m, rune_('x')) // type title "x"
	m = sendKey(m, key(tea.KeyTab)) // advance to PATH field

	// Enter should submit now (we're on PATH, but Enter submits from any field)
	m = sendKey(m, key(tea.KeyEnter))

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Title != "x" {
		t.Errorf("expected title %q, got %q", "x", sessions[0].Title)
	}
}

func TestToolSelectorCycles(t *testing.T) {
	m, conn := openModel(t)
	m = sendKey(m, rune_('n'))
	m = sendKey(m, rune_('t')) // title "t"
	m = sendKey(m, key(tea.KeyTab)) // TITLE → PATH

	// Replace PATH with a nonexistent path so Tab finds no candidates and advances to TOOL
	for i := 0; i < 512; i++ {
		m = sendKey(m, key(tea.KeyBackspace))
	}
	for _, r := range []rune("/nonexistent-path-that-has-no-matches-xyz/") {
		m = sendKey(m, rune_(r))
	}
	m = sendKey(m, key(tea.KeyTab)) // PATH (no candidates) → TOOL

	// Right arrow on TOOL cycles forward: claude → claude-dangerous
	m = sendKey(m, key(tea.KeyRight))
	m = sendKey(m, key(tea.KeyEnter))

	sessions, err := db.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Tool != "claude-dangerous" {
		t.Errorf("expected tool %q, got %q", "claude-dangerous", sessions[0].Tool)
	}
}

func TestPathCompletionCycling(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := os.Mkdir(filepath.Join(tmp, name), 0755); err != nil {
			t.Fatal(err)
		}
	}

	m, _ := openModel(t)
	m = sendKey(m, rune_('n'))
	// Tab from TITLE → PATH
	m = sendKey(m, key(tea.KeyTab))

	// Clear the pre-filled path by backspacing then type tmp+"/"
	// The pre-filled path is the cwd; backspace all characters
	// We'll just type the tmp path on top using Right to get to end first
	// Actually the cursor is already at the end of the pre-filled path.
	// Typing overwrites nothing — insertRune inserts at cursor, so typing
	// tmp+"/" would append to the existing path. We need to clear first.
	// Press Backspace many times to clear (up to 512 chars max cwd length)
	for i := 0; i < 512; i++ {
		m = sendKey(m, key(tea.KeyBackspace))
	}
	for _, r := range []rune(tmp + "/") {
		m = sendKey(m, rune_(r))
	}

	// Tab 3 times → cycle alpha/, beta/, gamma/
	m = sendKey(m, key(tea.KeyTab))
	m = sendKey(m, key(tea.KeyTab))
	m = sendKey(m, key(tea.KeyTab))
	// 4th Tab → exhausted → advance focus to TOOL
	m = sendKey(m, key(tea.KeyTab))

	// Still in new-session mode (haven't submitted)
	if m.Mode() != "new-session" {
		t.Errorf("expected still in new-session mode, got %s", m.Mode())
	}
}

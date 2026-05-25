package ui_test

import (
	"database/sql"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func openModelWithTmux(t *testing.T, fake *testutil.FakeTmuxClient) (*ui.Model, *sql.DB) {
	t.Helper()
	conn := testutil.OpenTestDB(t)
	m := ui.NewModel(conn, fake, nil)
	if err := m.Reload(); err != nil {
		t.Fatal(err)
	}
	return m, conn
}

func TestImportPickerOpensOnCapitalI(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["live-foo"] = ""
	fake.Sessions["live-bar"] = ""
	fake.SessionInfos["live-foo"] = tmux.SessionInfo{Name: "live-foo", CurrentPath: "/tmp/foo"}
	fake.SessionInfos["live-bar"] = tmux.SessionInfo{Name: "live-bar", CurrentPath: "/tmp/bar"}

	m, _ := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))

	if m.Mode() != "import-picker" {
		t.Fatalf("expected import-picker mode, got %q", m.Mode())
	}
	cands := m.ImportCandidates()
	if len(cands) != 2 {
		t.Errorf("expected 2 candidates, got %v", cands)
	}
}

func TestImportPickerNoUntrackedOpensEmptyAndDismisses(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	m, _ := openModelWithTmux(t, fake)

	m = sendKey(m, rune_('I'))
	if m.Mode() != "import-picker" {
		t.Fatalf("expected import-picker even when empty, got %q", m.Mode())
	}
	if len(m.ImportCandidates()) != 0 {
		t.Errorf("expected 0 candidates, got %v", m.ImportCandidates())
	}
	m = sendKey(m, key(tea.KeyEsc))
	if m.Mode() != "" {
		t.Errorf("expected mode cleared after esc, got %q", m.Mode())
	}
}

func TestImportPickerEnterOpensForm(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["pickme"] = ""
	fake.SessionInfos["pickme"] = tmux.SessionInfo{Name: "pickme", CurrentPath: "/srv"}

	m, _ := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))
	m = sendKey(m, key(tea.KeyEnter))

	if m.Mode() != "import-form" {
		t.Fatalf("expected import-form mode, got %q", m.Mode())
	}
	if m.ImportFormTitle() != "pickme" {
		t.Errorf("expected title prefilled, got %q", m.ImportFormTitle())
	}
}

func TestImportFormCommitInsertsRow(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["adopt-me"] = ""
	fake.SessionInfos["adopt-me"] = tmux.SessionInfo{Name: "adopt-me", CurrentPath: "/work"}

	m, conn := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))
	m = sendKey(m, key(tea.KeyEnter)) // pick first
	m = sendKey(m, key(tea.KeyEnter)) // commit form with defaults

	got, err := db.GetSessionByTmuxName(conn, "adopt-me")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Title != "adopt-me" {
		t.Errorf("title: %q", got.Title)
	}
	if got.ProjectPath != "/work" {
		t.Errorf("project: %q", got.ProjectPath)
	}
	if m.Mode() != "" {
		t.Errorf("expected mode cleared after commit, got %q", m.Mode())
	}
}

func TestImportFormCursorLeftRight(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["mysess"] = ""
	fake.SessionInfos["mysess"] = tmux.SessionInfo{Name: "mysess", CurrentPath: "/tmp"}

	m, _ := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))
	m = sendKey(m, key(tea.KeyEnter)) // enter form; title pre-filled as "mysess"

	// "mysess" has 6 runes; cursor starts at 6 (end)
	// left×2 → cursor=4; insert 'X' → "myseXss"
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, rune_('X'))
	if m.ImportFormTitle() != "myseXss" {
		t.Errorf("got %q want %q", m.ImportFormTitle(), "myseXss")
	}
}

func TestImportFormCursorBackspaceAtMiddle(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["abc"] = ""
	fake.SessionInfos["abc"] = tmux.SessionInfo{Name: "abc", CurrentPath: "/tmp"}

	m, _ := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))
	m = sendKey(m, key(tea.KeyEnter)) // title = "abc", cursor at end (3)

	// left once → cursor=2; backspace removes 'b' → "ac"
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, key(tea.KeyBackspace))
	if m.ImportFormTitle() != "ac" {
		t.Errorf("got %q want %q", m.ImportFormTitle(), "ac")
	}
}

func TestImportFormCursorClampsAtBoundaries(t *testing.T) {
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["z"] = ""
	fake.SessionInfos["z"] = tmux.SessionInfo{Name: "z", CurrentPath: "/tmp"}

	m, _ := openModelWithTmux(t, fake)
	m = sendKey(m, rune_('I'))
	m = sendKey(m, key(tea.KeyEnter)) // title = "z"

	// left past start, right past end — no panic, value unchanged
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, key(tea.KeyLeft))
	m = sendKey(m, key(tea.KeyRight))
	m = sendKey(m, key(tea.KeyRight))
	m = sendKey(m, key(tea.KeyRight))
	if m.ImportFormTitle() != "z" {
		t.Errorf("value changed unexpectedly: %q", m.ImportFormTitle())
	}
}

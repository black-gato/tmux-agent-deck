package cmd_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/black-gato/tmux-agent-deck/cmd"
	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

func TestAddAndList(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"add", "--title", "test-app", "--project", "/tmp"}, &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	out.Reset()
	if err := cmd.RunWith([]string{"list"}, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("test-app")) {
		t.Errorf("list output missing 'test-app': %s", out.String())
	}
}

func TestGroupCreateAndMove(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"group", "create", "work"}, &out); err != nil {
		t.Fatalf("group create: %v", err)
	}
	if err := cmd.RunWith([]string{"add", "--title", "my-app", "--project", "/tmp"}, &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := cmd.RunWith([]string{"group", "move", "my-app", "work"}, &out); err != nil {
		t.Fatalf("group move: %v", err)
	}

	out.Reset()
	if err := cmd.RunWith([]string{"list"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("my-app")) {
		t.Errorf("session missing from list after move: %s", out.String())
	}
}

func TestListAcceptsNotifyFlags(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"--notify", "--notify-style", "digest", "--notify-quiet", "cooldown=10m,hours=22:00-07:00", "list"}, &out); err != nil {
		t.Fatalf("list with notify flags: %v", err)
	}
}

func TestHelpDocumentsNotifyFlags(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"--help"}, &out); err != nil {
		t.Fatalf("help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"waiting    alert per session",
		"conductor  alert names the group conductor",
		"digest     one combined alert",
		"cooldown=5m",
		"hours=22:00-08:00",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestGroupCreateDuplicateShowsFriendlyError(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"group", "create", "work"}, &out); err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := cmd.RunWith([]string{"group", "create", "work"}, &out)
	if err == nil {
		t.Fatal("expected error on duplicate group, got nil")
	}
	if bytes.Contains([]byte(err.Error()), []byte("UNIQUE constraint")) {
		t.Fatalf("expected friendly error, got raw SQL: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("already exists")) {
		t.Fatalf("expected 'already exists' in error, got: %v", err)
	}
}

func TestRemoveSessionWithSlashTitle(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"add", "--title", "////", "--project", "/tmp"}, &out); err != nil {
		t.Fatalf("add slash-title session: %v", err)
	}

	out.Reset()
	if err := cmd.RunWith([]string{"remove", "////"}, &out); err != nil {
		t.Fatalf("remove slash-title session: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`Removed session "////"`)) {
		t.Fatalf("unexpected remove output: %q", out.String())
	}

	out.Reset()
	if err := cmd.RunWith([]string{"list"}, &out); err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("////")) {
		t.Fatalf("slash-title session still present after remove: %q", out.String())
	}
}

func TestListRejectsInvalidPollFlag(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	err := cmd.RunWith([]string{"--poll", "not-a-duration", "list"}, &out)
	if err == nil {
		t.Fatal("expected invalid poll flag error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("invalid argument")) {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestImportCommandList(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.CreateSession(conn, db.Session{
		ID: "id-a", Title: "alpha-deck", GroupPath: "my-sessions",
		TmuxSession: "alpha", Tool: "claude", Status: "running", CreatedAt: 1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	conn.Close()

	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["alpha"] = ""
	fake.Sessions["beta"] = ""

	var out bytes.Buffer
	if err := cmd.RunWithContextAndClient(context.Background(),
		[]string{"import", "--list"}, &out, fake); err != nil {
		t.Fatalf("import --list: %v", err)
	}
	got := out.String()
	if !bytes.Contains([]byte(got), []byte("beta")) {
		t.Errorf("expected 'beta' in output: %q", got)
	}
	if bytes.Contains([]byte(got), []byte("alpha")) {
		t.Errorf("did not expect 'alpha' in output: %q", got)
	}
}

func TestImportCommandSingle(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["gamma"] = ""
	fake.SessionInfos["gamma"] = tmux.SessionInfo{Name: "gamma", CurrentPath: "/srv/code"}

	var out bytes.Buffer
	if err := cmd.RunWithContextAndClient(context.Background(),
		[]string{"import", "gamma", "--title", "Gamma"}, &out, fake); err != nil {
		t.Fatalf("import: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	s, err := db.GetSessionByTmuxName(conn, "gamma")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if s.Title != "Gamma" || s.ProjectPath != "/srv/code" || s.Status != "unknown" {
		t.Errorf("session row wrong: %+v", s)
	}
}

func TestImportCommandAll(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["one"] = ""
	fake.Sessions["two"] = ""

	var out bytes.Buffer
	if err := cmd.RunWithContextAndClient(context.Background(),
		[]string{"import", "--all"}, &out, fake); err != nil {
		t.Fatalf("import --all: %v", err)
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()
	for _, name := range []string{"one", "two"} {
		if _, err := db.GetSessionByTmuxName(conn, name); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

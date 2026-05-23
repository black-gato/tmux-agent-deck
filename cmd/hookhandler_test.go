package cmd_test

import (
	"strings"
	"testing"

	cmd "github.com/black-gato/tmux-agent-deck/cmd"
	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestHookHandler_StopSendsMessage(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	g := db.Group{Path: "grp", Name: "grp", DefaultTool: "claude"}
	_ = db.CreateGroup(conn, g)
	conductor := db.Session{ID: "c1", Title: "conductor", GroupPath: "grp", TmuxSession: "tmux-cond", Tool: "claude", Status: "running", CreatedAt: 1}
	worker := db.Session{ID: "w1", Title: "worker-a", GroupPath: "grp", TmuxSession: "tmux-work", Tool: "claude", Status: "running", CreatedAt: 2}
	_ = db.CreateSession(conn, conductor)
	_ = db.CreateSession(conn, worker)
	_ = db.SetGroupConductor(conn, "grp", "c1")

	fake := testutil.NewFakeTmuxClient()
	r := strings.NewReader(`{"hook_event_name":"Stop"}`)
	err := cmd.RunHookHandlerWith(r, conn, cmd.HookHandlerDeps{
		ResolveSession: func() (string, error) { return "tmux-work", nil },
		Sender:         fake,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Keys != "[deck] worker-a | Stop | task complete" {
		t.Errorf("unexpected message: %q", fake.SentKeys[0].Keys)
	}
	if fake.SentKeys[0].Session != "tmux-cond" {
		t.Errorf("sent to wrong session: %q", fake.SentKeys[0].Session)
	}
	if len(fake.SentRawKeys) != 1 || fake.SentRawKeys[0].Keys != "Enter" {
		t.Errorf("expected Enter raw key submit")
	}
}

func TestHookHandler_PermissionRequestIncludesTool(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	g := db.Group{Path: "grp", Name: "grp", DefaultTool: "claude"}
	_ = db.CreateGroup(conn, g)
	conductor := db.Session{ID: "c1", Title: "conductor", GroupPath: "grp", TmuxSession: "tmux-cond", Tool: "claude", Status: "running", CreatedAt: 1}
	worker := db.Session{ID: "w1", Title: "worker-a", GroupPath: "grp", TmuxSession: "tmux-work", Tool: "claude", Status: "running", CreatedAt: 2}
	_ = db.CreateSession(conn, conductor)
	_ = db.CreateSession(conn, worker)
	_ = db.SetGroupConductor(conn, "grp", "c1")

	fake := testutil.NewFakeTmuxClient()
	r := strings.NewReader(`{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`)
	_ = cmd.RunHookHandlerWith(r, conn, cmd.HookHandlerDeps{
		ResolveSession: func() (string, error) { return "tmux-work", nil },
		Sender:         fake,
	})
	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(fake.SentKeys))
	}
	if fake.SentKeys[0].Keys != "[deck] worker-a | PermissionRequest | tool: Bash" {
		t.Errorf("unexpected message: %q", fake.SentKeys[0].Keys)
	}
}

func TestHookHandler_NoConductorSilentExit(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	g := db.Group{Path: "grp", Name: "grp", DefaultTool: "claude"}
	_ = db.CreateGroup(conn, g)
	worker := db.Session{ID: "w1", Title: "worker-a", GroupPath: "grp", TmuxSession: "tmux-work", Tool: "claude", Status: "running", CreatedAt: 1}
	_ = db.CreateSession(conn, worker)
	// no conductor set

	fake := testutil.NewFakeTmuxClient()
	r := strings.NewReader(`{"hook_event_name":"Stop"}`)
	err := cmd.RunHookHandlerWith(r, conn, cmd.HookHandlerDeps{
		ResolveSession: func() (string, error) { return "tmux-work", nil },
		Sender:         fake,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("expected no SendKeys, got %d", len(fake.SentKeys))
	}
}

func TestHookHandler_UnknownTmuxSessionSilentExit(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	r := strings.NewReader(`{"hook_event_name":"Stop"}`)
	err := cmd.RunHookHandlerWith(r, conn, cmd.HookHandlerDeps{
		ResolveSession: func() (string, error) { return "not-tracked", nil },
		Sender:         fake,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.SentKeys) != 0 {
		t.Errorf("expected no SendKeys, got %d", len(fake.SentKeys))
	}
}

func TestHookHandler_NotificationTruncatesLongMessage(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	g := db.Group{Path: "grp", Name: "grp", DefaultTool: "claude"}
	_ = db.CreateGroup(conn, g)
	conductor := db.Session{ID: "c1", Title: "conductor", GroupPath: "grp", TmuxSession: "tmux-cond", Tool: "claude", Status: "running", CreatedAt: 1}
	worker := db.Session{ID: "w1", Title: "worker-a", GroupPath: "grp", TmuxSession: "tmux-work", Tool: "claude", Status: "running", CreatedAt: 2}
	_ = db.CreateSession(conn, conductor)
	_ = db.CreateSession(conn, worker)
	_ = db.SetGroupConductor(conn, "grp", "c1")

	longMsg := strings.Repeat("x", 80)
	fake := testutil.NewFakeTmuxClient()
	r := strings.NewReader(`{"hook_event_name":"Notification","message":"` + longMsg + `"}`)
	_ = cmd.RunHookHandlerWith(r, conn, cmd.HookHandlerDeps{
		ResolveSession: func() (string, error) { return "tmux-work", nil },
		Sender:         fake,
	})
	if len(fake.SentKeys) != 1 {
		t.Fatalf("expected 1 SendKeys, got %d", len(fake.SentKeys))
	}
	// message context must be truncated to 60 chars
	expected := "[deck] worker-a | Notification | " + strings.Repeat("x", 60)
	if fake.SentKeys[0].Keys != expected {
		t.Errorf("unexpected message: %q", fake.SentKeys[0].Keys)
	}
}

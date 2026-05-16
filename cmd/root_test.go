package cmd_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/cmd"
	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestHeadlessModePollsAndExitsCleanly(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := db.CreateSession(conn, db.Session{
		ID:          "s1",
		Title:       "worker",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s1",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   time.Now().Unix(),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["tmux-s1"] = "Some output\n> "

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)

	var out bytes.Buffer
	if err := cmd.RunWithContextAndClient(ctx, []string{"--headless", "--poll", "10ms"}, &out, fake); err != nil {
		t.Fatalf("run headless: %v", err)
	}

	session, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.Status != "waiting" {
		t.Fatalf("status: got %q want waiting", session.Status)
	}
}

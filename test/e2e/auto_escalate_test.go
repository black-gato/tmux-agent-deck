//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func TestAutoEscalateForwardsToCondcutor(t *testing.T) {
	env := suite.NewTest(t)
	conn := env.OpenDB(t)

	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", DefaultTool: "bash"}); err != nil {
		t.Fatalf("create group: %v", err)
	}

	conductorTitle := env.Prefix + "-conductor"
	workerTitle := env.Prefix + "-worker"
	conductorID := env.Prefix + "-conductor-id"
	workerID := env.Prefix + "-worker-id"
	now := time.Now().Unix()

	for _, s := range []db.Session{
		{
			ID: conductorID, Title: conductorTitle, GroupPath: "work",
			TmuxSession: conductorTitle, ProjectPath: "/tmp", Tool: "bash",
			Status: "running", CreatedAt: now,
		},
		{
			ID: workerID, Title: workerTitle, GroupPath: "work",
			TmuxSession: workerTitle, ProjectPath: "/tmp", Tool: "bash",
			Status: "running", CreatedAt: now,
		},
	} {
		if err := db.CreateSession(conn, s); err != nil {
			t.Fatalf("create session %s: %v", s.Title, err)
		}
		NewTmuxSession(t, s.TmuxSession, s.ProjectPath, "bash")
	}

	if err := db.SetGroupConductor(conn, "work", conductorID); err != nil {
		t.Fatalf("set conductor: %v", err)
	}

	// Give the worker a moment of activity so the poller sees running→waiting.
	SendTmuxKeys(t, workerTitle, "echo working", "Enter")

	env.StartHeadlessPoller(t, "--auto-escalate", "--poll", "200ms")

	waitForStatus(t, env, workerTitle, "waiting", 6*time.Second)

	WaitForPaneText(t, paneTarget(conductorTitle, 0), "Escalation from "+workerTitle)
}

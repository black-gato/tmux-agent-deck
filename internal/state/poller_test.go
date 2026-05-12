package state_test

import (
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

type stubTmux struct {
	output string
	exists bool
}

func (s *stubTmux) CapturePaneOutput(name string) (string, error) { return s.output, nil }
func (s *stubTmux) SessionExists(name string) (bool, error)       { return s.exists, nil }

func TestPollerUpdatesStatusToWaiting(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateSession(conn, db.Session{
		ID:          "s1",
		Title:       "test",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s1",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	p := state.New(conn, stub)
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "waiting" {
		t.Errorf("status: got %q want waiting", s.Status)
	}
}

func TestPollerMarksErrorWhenSessionGone(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateSession(conn, db.Session{
		ID:          "s2",
		Title:       "gone",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s2",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   now,
	})

	stub := &stubTmux{exists: false}
	p := state.New(conn, stub)
	p.PollOnce()

	s, _ := db.GetSession(conn, "s2")
	if s.Status != "error" {
		t.Errorf("status: got %q want error", s.Status)
	}
}

func TestPollerSkipsStoppedSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateSession(conn, db.Session{
		ID:          "s3",
		Title:       "stopped-one",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s3",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "stopped",
		CreatedAt:   now,
	})

	callCount := 0
	stub := &countingStub{callCount: &callCount}
	p := state.New(conn, stub)
	p.PollOnce()

	if callCount > 0 {
		t.Errorf("CapturePaneOutput called for stopped session")
	}
}

func TestPollerTracksWaitingSinceForExistingWaitingSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID:          "s4",
		Title:       "waiting-one",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s4",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "waiting",
		CreatedAt:   now.Unix(),
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	p := state.NewWithClock(conn, stub, func() time.Time { return now })
	p.PollOnce()

	waitingSince := p.WaitingSinceSnapshot()
	got, ok := waitingSince["s4"]
	if !ok {
		t.Fatal("expected waiting timestamp for s4")
	}
	if !got.Equal(now) {
		t.Fatalf("waiting timestamp: got %v want %v", got, now)
	}
}

func TestPollerClearsWaitingSinceWhenSessionLeavesWaiting(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID:          "s5",
		Title:       "waiting-to-running",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-s5",
		ProjectPath: "/p",
		Tool:        "claude",
		Status:      "waiting",
		CreatedAt:   current.Unix(),
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	p := state.NewWithClock(conn, stub, func() time.Time { return current })
	p.PollOnce()

	current = current.Add(35 * time.Second)
	stub.output = "Thinking hard..."
	p.PollOnce()

	waitingSince := p.WaitingSinceSnapshot()
	if _, ok := waitingSince["s5"]; ok {
		t.Fatal("expected waiting timestamp to clear after leaving waiting")
	}

	s, _ := db.GetSession(conn, "s5")
	if s.Status != "running" {
		t.Fatalf("status: got %q want running", s.Status)
	}
}

type countingStub struct{ callCount *int }

func (s *countingStub) CapturePaneOutput(name string) (string, error) {
	*s.callCount++
	return "", nil
}
func (s *countingStub) SessionExists(name string) (bool, error) { return true, nil }

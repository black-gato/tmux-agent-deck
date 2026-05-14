package state_test

import (
	"strings"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/notify"
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
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return now })
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
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
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

type recordingNotifier struct {
	enabled bool
	style   notify.Style
	calls   []notifyCall
}

type notifyCall struct {
	title string
	body  string
}

func (n *recordingNotifier) Enabled() bool       { return n.enabled }
func (n *recordingNotifier) Style() notify.Style { return n.style }
func (n *recordingNotifier) Notify(title, body string) error {
	n.calls = append(n.calls, notifyCall{title: title, body: body})
	return nil
}

func TestPollerWaitingStyleSendsDirectAlert(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker", GroupPath: "my-sessions", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	notifier := &recordingNotifier{enabled: true, style: notify.StyleWaiting}
	p := state.NewWithClock(conn, stub, notifier, time.Now)
	p.PollOnce()

	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.calls))
	}
	if notifier.calls[0].title != "Agent waiting" {
		t.Fatalf("title: got %q want %q", notifier.calls[0].title, "Agent waiting")
	}
}

func TestPollerStartHonorsConfiguredInterval(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	p := state.NewWithClockInterval(conn, &stubTmux{exists: true}, notify.New(notify.Config{}), time.Now, 25*time.Millisecond)
	if got := p.Interval(); got != 25*time.Millisecond {
		t.Fatalf("interval: got %v want %v", got, 25*time.Millisecond)
	}
}

func TestPollerConductorStyleTargetsConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "lead"})
	db.CreateSession(conn, db.Session{
		ID: "lead", Title: "lead", GroupPath: "work", TmuxSession: "tmux-lead",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	notifier := &recordingNotifier{enabled: true, style: notify.StyleConductor}
	p := state.NewWithClock(conn, stub, notifier, time.Now)
	p.PollOnce()

	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.calls))
	}
	found := false
	for _, call := range notifier.calls {
		if call.title == "Conductor alert" && call.body == "lead: worker is waiting" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected conductor alert, got %#v", notifier.calls)
	}
}

func TestPollerDigestStyleSuppressesImmediateAlert(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "lead"})
	db.CreateSession(conn, db.Session{
		ID: "lead", Title: "lead", GroupPath: "work", TmuxSession: "tmux-lead",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now + 1,
	})
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker", GroupPath: "work", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now, Notes: "blocked on review",
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	notifier := &recordingNotifier{enabled: true, style: notify.StyleDigest}
	p := state.NewWithClock(conn, stub, notifier, time.Now)
	p.PollOnce()

	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.calls))
	}
	if notifier.calls[0].title != "Conductor digest" {
		t.Fatalf("title: got %q want %q", notifier.calls[0].title, "Conductor digest")
	}
	if !strings.Contains(notifier.calls[0].body, "lead: waiting sessions") {
		t.Fatalf("digest missing conductor header: %q", notifier.calls[0].body)
	}
	if !strings.Contains(notifier.calls[0].body, "- worker: blocked on review") {
		t.Fatalf("digest missing waiting child summary: %q", notifier.calls[0].body)
	}
}

func TestPollerDigestSummarizesAllWaitingChildren(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "lead"})
	for _, session := range []db.Session{
		{
			ID: "lead", Title: "lead", GroupPath: "work", TmuxSession: "tmux-lead",
			ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now + 2,
		},
		{
			ID: "worker-1", Title: "worker-1", GroupPath: "work", TmuxSession: "tmux-worker-1",
			ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: now + 1, Notes: "needs API key",
		},
		{
			ID: "worker-2", Title: "worker-2", GroupPath: "work", TmuxSession: "tmux-worker-2",
			ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
		},
	} {
		if err := db.CreateSession(conn, session); err != nil {
			t.Fatal(err)
		}
	}

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	notifier := &recordingNotifier{enabled: true, style: notify.StyleDigest}
	p := state.NewWithClock(conn, stub, notifier, time.Now)
	p.PollOnce()

	if len(notifier.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifier.calls))
	}
	body := notifier.calls[0].body
	if !strings.Contains(body, "- worker-1: needs API key") {
		t.Fatalf("digest missing existing waiting child: %q", body)
	}
	if !strings.Contains(body, "- worker-2") {
		t.Fatalf("digest missing newly waiting child: %q", body)
	}
	if strings.Contains(body, "- lead") {
		t.Fatalf("digest should exclude conductor: %q", body)
	}
}

func TestPollerWaitingCooldownSuppressesRepeatAlerts(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	if err := db.CreateSession(conn, db.Session{
		ID: "s1", Title: "worker", GroupPath: "work", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	stub := &stubTmux{output: "Some output\n>", exists: true}
	calls := 0
	notifier := notify.NewWithClockRunner(
		notify.Config{Enabled: true, Style: notify.StyleWaiting, Quiet: "cooldown=10m"},
		func() time.Time { return now },
		func(title, body string) error {
			calls++
			return nil
		},
	)
	p := state.NewWithClock(conn, stub, notifier, func() time.Time { return now })

	p.PollOnce()
	now = now.Add(time.Minute)
	stub.output = "Thinking hard..."
	p.PollOnce()
	now = now.Add(time.Minute)
	stub.output = "Some output\n>"
	p.PollOnce()

	s, err := db.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != "waiting" {
		t.Fatalf("status: got %q want waiting", s.Status)
	}
	if calls != 1 {
		t.Fatalf("calls: got %d want 1", calls)
	}
}

func TestPollerDigestQuietHoursSuppressesDigestAlert(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Date(2026, 5, 12, 22, 30, 0, 0, time.Local)
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "lead"})
	for _, session := range []db.Session{
		{
			ID: "lead", Title: "lead", GroupPath: "work", TmuxSession: "tmux-lead",
			ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now.Unix() + 1,
		},
		{
			ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
			ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now.Unix(),
		},
	} {
		if err := db.CreateSession(conn, session); err != nil {
			t.Fatal(err)
		}
	}

	calls := 0
	stub := &stubTmux{output: "Some output\n>", exists: true}
	notifier := notify.NewWithClockRunner(
		notify.Config{Enabled: true, Style: notify.StyleDigest, Quiet: "hours=22:00-07:00"},
		func() time.Time { return now },
		func(title, body string) error {
			calls++
			return nil
		},
	)
	p := state.NewWithClock(conn, stub, notifier, func() time.Time { return now })
	p.PollOnce()

	if calls != 0 {
		t.Fatalf("calls: got %d want 0", calls)
	}
}

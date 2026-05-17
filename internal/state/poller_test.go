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
	output     string
	outputs    map[string]string // per-session output overrides
	exists     bool
	activity   time.Time
	activities map[string]time.Time
}

func (s *stubTmux) CapturePaneOutput(name string) (string, error) {
	if s.outputs != nil && s.outputs[name] != "" {
		return s.outputs[name], nil
	}
	return s.output, nil
}
func (s *stubTmux) SessionExists(name string) (bool, error) { return s.exists, nil }
func (s *stubTmux) SessionActivity(name string) (time.Time, error) {
	if s.activities != nil && !s.activities[name].IsZero() {
		return s.activities[name], nil
	}
	return s.activity, nil
}

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
func (s *countingStub) SessionActivity(name string) (time.Time, error) {
	return time.Time{}, nil
}

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

func TestPollerStoresContextPct(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "test", GroupPath: "my-sessions",
		TmuxSession: "tmux-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: time.Now().Unix(),
	})

	stub := &stubTmux{output: "Some output\n75% context used\n> ", exists: true}
	p := state.New(conn, stub)
	p.PollOnce()

	snap := p.ContextPctSnapshot()
	if snap["s1"] == nil || *snap["s1"] != 75 {
		t.Fatalf("expected context pct 75 for s1, got %v", snap["s1"])
	}
}

func TestPollerMarksSessionIdleWhenOutputUnchanged(t *testing.T) {
	// BUG-005: a session whose pane output has not changed for >30s should
	// transition to idle, even if the tail still contains "Thinking" or a spinner.
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "stale", GroupPath: "g", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: current.Unix(),
	})

	stub := &stubTmux{output: "⠋ Thinking...", exists: true}
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
	p.PollOnce()

	current = current.Add(31 * time.Second)
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "idle" {
		t.Fatalf("status: got %q want idle", s.Status)
	}
}

func TestPollerKeepsRunningWhenOutputKeepsChanging(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "active", GroupPath: "g", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: current.Unix(),
	})

	stub := &stubTmux{output: "⠋ Thinking...", exists: true}
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
	p.PollOnce()

	current = current.Add(31 * time.Second)
	stub.output = "⠙ Thinking..."
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "running" {
		t.Fatalf("status: got %q want running", s.Status)
	}
}

func TestPollerMarksOldSessionIdleOnFirstPoll(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "old", GroupPath: "g", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: current.Add(-48 * time.Hour).Unix(),
	})

	stub := &stubTmux{output: "Thinking about your request", exists: true, activity: current.Add(-48 * time.Hour)}
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "idle" {
		t.Fatalf("status: got %q want idle", s.Status)
	}
}

func TestPollerKeepsRecentlyActiveSessionRunningOnFirstPoll(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "recent", GroupPath: "g", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: current.Unix(),
	})

	stub := &stubTmux{output: "Thinking about your request", exists: true, activity: current.Add(-5 * time.Second)}
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "running" {
		t.Fatalf("status: got %q want running", s.Status)
	}
}

func TestPollerKeepsOldPromptWaitingOnFirstPoll(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	current := time.Unix(1_700_000_000, 0)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "prompt", GroupPath: "g", TmuxSession: "tmux-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: current.Add(-48 * time.Hour).Unix(),
	})

	stub := &stubTmux{output: "Done\n> ", exists: true, activity: current.Add(-48 * time.Hour)}
	p := state.NewWithClock(conn, stub, notify.New(notify.Config{}), func() time.Time { return current })
	p.PollOnce()

	s, _ := db.GetSession(conn, "s1")
	if s.Status != "waiting" {
		t.Fatalf("status: got %q want waiting", s.Status)
	}
}

func TestPollerClearsContextPctWhenSessionStopped(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	db.CreateSession(conn, db.Session{
		ID: "s1", Title: "test", GroupPath: "my-sessions",
		TmuxSession: "tmux-s1", ProjectPath: "/p", Tool: "claude",
		Status: "running", CreatedAt: time.Now().Unix(),
	})

	stub := &stubTmux{output: "75% context used\n> ", exists: true}
	p := state.New(conn, stub)
	p.PollOnce()

	db.UpdateSessionStatus(conn, "s1", "stopped")
	p.PollOnce()

	snap := p.ContextPctSnapshot()
	if snap["s1"] != nil {
		t.Fatalf("expected nil context pct after session stopped, got %d", *snap["s1"])
	}
}

type stubSender struct {
	calls    []sentKeysCall
	rawCalls []sentKeysCall
}

type sentKeysCall struct {
	session string
	pane    int
	keys    string
}

func (s *stubSender) SendKeys(session string, pane int, keys string) error {
	s.calls = append(s.calls, sentKeysCall{session, pane, keys})
	return nil
}

func (s *stubSender) SendRawKeys(session string, pane int, keys string) error {
	s.rawCalls = append(s.rawCalls, sentKeysCall{session, pane, keys})
	return nil
}

func TestAutoEscalateSendsToConductorOnWaitingTransition(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "conductor"})
	db.CreateSession(conn, db.Session{
		ID: "conductor", Title: "conductor", GroupPath: "work", TmuxSession: "tmux-conductor",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now, Notes: "stuck on auth",
	})

	stub := &stubTmux{
		outputs: map[string]string{
			"tmux-conductor": "Thinking about this...\n",
			"tmux-worker": strings.Join([]string{
				"line1",
				"line2",
				"line3",
				"❯ ",
				"────────────────────────",
				"  anthonymirville@host project [Sonnet 4.6] ctx:12%",
				"  -- INSERT -- ← for agents",
				"",
			}, "\n"),
		},
		exists: true,
	}
	sender := &stubSender{}
	p := state.New(conn, stub)
	p.SetSender(sender)
	p.PollOnce()

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 SendKeys call, got %d", len(sender.calls))
	}
	got := sender.calls[0]
	if got.session != "tmux-conductor" {
		t.Errorf("session: got %q want %q", got.session, "tmux-conductor")
	}
	if got.pane != 0 {
		t.Errorf("pane: got %d want 0", got.pane)
	}
	if !strings.Contains(got.keys, "Escalation from worker") {
		t.Errorf("keys missing escalation header: %q", got.keys)
	}
	if !strings.Contains(got.keys, "Notes: stuck on auth") {
		t.Errorf("keys missing notes: %q", got.keys)
	}
	if !strings.Contains(got.keys, "Context:") {
		t.Errorf("keys missing context section: %q", got.keys)
	}
	if !strings.Contains(got.keys, "line3") {
		t.Errorf("keys missing useful context line: %q", got.keys)
	}
	if strings.Contains(got.keys, "-- INSERT --") {
		t.Errorf("keys should omit terminal footer: %q", got.keys)
	}
	if len(sender.rawCalls) != 1 {
		t.Fatalf("expected 1 SendRawKeys call, got %d", len(sender.rawCalls))
	}
	if sender.rawCalls[0].session != "tmux-conductor" || sender.rawCalls[0].keys != "Enter" {
		t.Fatalf("submit call: got %#v", sender.rawCalls[0])
	}
}

func TestAutoEscalateFiresOncePerTransition(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "conductor"})
	db.CreateSession(conn, db.Session{
		ID: "conductor", Title: "conductor", GroupPath: "work", TmuxSession: "tmux-conductor",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{
		outputs: map[string]string{
			"tmux-conductor": "Thinking about this...\n",
			"tmux-worker":    "Some output\n> ",
		},
		exists: true,
	}
	sender := &stubSender{}
	p := state.New(conn, stub)
	p.SetSender(sender)
	p.PollOnce() // running → waiting, should fire
	p.PollOnce() // still waiting, should NOT fire again

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 SendKeys call across 2 polls, got %d", len(sender.calls))
	}
}

func TestAutoEscalateSkipsWhenSenderNil(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "conductor"})
	db.CreateSession(conn, db.Session{
		ID: "conductor", Title: "conductor", GroupPath: "work", TmuxSession: "tmux-conductor",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	p := state.New(conn, stub) // no SetSender call
	p.PollOnce()               // should not panic or send anything
}

func TestAutoEscalateSkipsWhenNoConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work"}) // no ConductorSessionID
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	sender := &stubSender{}
	p := state.New(conn, stub)
	p.SetSender(sender)
	p.PollOnce()

	if len(sender.calls) != 0 {
		t.Fatalf("expected 0 SendKeys calls when no conductor, got %d", len(sender.calls))
	}
}

func TestAutoEscalateSkipsWhenSessionIsConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "conductor"})
	db.CreateSession(conn, db.Session{
		ID: "conductor", Title: "conductor", GroupPath: "work", TmuxSession: "tmux-conductor",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	sender := &stubSender{}
	p := state.New(conn, stub)
	p.SetSender(sender)
	p.PollOnce() // conductor itself transitions to waiting — should not self-escalate

	if len(sender.calls) != 0 {
		t.Fatalf("expected 0 SendKeys calls when conductor is the waiting session, got %d", len(sender.calls))
	}
}

func TestAutoEscalateSkipsWhenConductorUnavailable(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	db.CreateGroup(conn, db.Group{Path: "work", Name: "work", ConductorSessionID: "conductor"})
	db.CreateSession(conn, db.Session{
		ID: "conductor", Title: "conductor", GroupPath: "work", TmuxSession: "",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now,
	})
	db.CreateSession(conn, db.Session{
		ID: "worker", Title: "worker", GroupPath: "work", TmuxSession: "tmux-worker",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	stub := &stubTmux{output: "Some output\n> ", exists: true}
	sender := &stubSender{}
	p := state.New(conn, stub)
	p.SetSender(sender)
	p.PollOnce()

	if len(sender.calls) != 0 {
		t.Fatalf("expected 0 SendKeys calls when conductor unavailable, got %d", len(sender.calls))
	}
}

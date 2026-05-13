package notify_test

import (
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/notify"
)

func TestNotifySkipsWhenDisabled(t *testing.T) {
	called := false
	n := notify.NewWithRunner(notify.Config{}, func(title, body string) error {
		called = true
		return nil
	})

	if err := n.Notify("title", "body"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("notify runner should not be called when disabled")
	}
}

func TestNotifyUsesRunnerWhenEnabled(t *testing.T) {
	called := false
	n := notify.NewWithRunner(notify.Config{Enabled: true, Style: notify.StyleConductor}, func(title, body string) error {
		called = true
		if title != "title" || body != "body" {
			t.Fatalf("got %q / %q", title, body)
		}
		return nil
	})

	if err := n.Notify("title", "body"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("notify runner should be called when enabled")
	}
	if n.Style() != notify.StyleConductor {
		t.Fatalf("style: got %q want %q", n.Style(), notify.StyleConductor)
	}
}

func TestNotifyQuietCooldownSuppressesRepeat(t *testing.T) {
	now := time.Date(2026, 5, 12, 9, 0, 0, 0, time.Local)
	callCount := 0
	n := notify.NewWithClockRunner(
		notify.Config{Enabled: true, Quiet: "cooldown=10m"},
		func() time.Time { return now },
		func(title, body string) error {
			callCount++
			return nil
		},
	)

	if err := n.Notify("Agent waiting", "worker is waiting"); err != nil {
		t.Fatal(err)
	}
	if err := n.Notify("Agent waiting", "worker is waiting"); err != nil {
		t.Fatal(err)
	}

	now = now.Add(11 * time.Minute)
	if err := n.Notify("Agent waiting", "worker is waiting"); err != nil {
		t.Fatal(err)
	}

	if callCount != 2 {
		t.Fatalf("call count: got %d want 2", callCount)
	}
}

func TestNotifyQuietHoursSuppressWithinWindow(t *testing.T) {
	now := time.Date(2026, 5, 12, 22, 30, 0, 0, time.Local)
	called := false
	n := notify.NewWithClockRunner(
		notify.Config{Enabled: true, Quiet: "hours=22:00-07:00"},
		func() time.Time { return now },
		func(title, body string) error {
			called = true
			return nil
		},
	)

	if err := n.Notify("Agent waiting", "worker is waiting"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("notify runner should not be called during quiet hours")
	}
}

func TestNotifyQuietHoursAllowsOutsideWindow(t *testing.T) {
	now := time.Date(2026, 5, 12, 14, 0, 0, 0, time.Local)
	called := false
	n := notify.NewWithClockRunner(
		notify.Config{Enabled: true, Quiet: "hours=22:00-07:00"},
		func() time.Time { return now },
		func(title, body string) error {
			called = true
			return nil
		},
	)

	if err := n.Notify("Agent waiting", "worker is waiting"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("notify runner should be called outside quiet hours")
	}
}

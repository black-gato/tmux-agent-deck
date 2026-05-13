package notify_test

import (
	"testing"

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

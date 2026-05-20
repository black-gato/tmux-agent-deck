package state_test

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
)

func TestEscalationMessageIncludesWorkerID(t *testing.T) {
	s := db.Session{ID: "worker-42", Title: "my-worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "")
	if !strings.Contains(msg, "Worker ID: worker-42") {
		t.Errorf("message missing worker ID: %q", msg)
	}
}

func TestEscalationMessageIncludesReplySyntax(t *testing.T) {
	s := db.Session{ID: "worker-42", Title: "my-worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "")
	if !strings.Contains(msg, "@deck-reply worker=worker-42") {
		t.Errorf("message missing reply syntax: %q", msg)
	}
	if !strings.Contains(msg, "@deck-end") {
		t.Errorf("message missing @deck-end: %q", msg)
	}
}

func TestEscalationMessageIncludesNotes(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting", Notes: "stuck on auth"}
	msg := state.EscalationMessage(s, "")
	if !strings.Contains(msg, "Notes: stuck on auth") {
		t.Errorf("message missing notes: %q", msg)
	}
}

func TestEscalationMessageIncludesContext(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "some relevant output line")
	if !strings.Contains(msg, "some relevant output line") {
		t.Errorf("message missing context: %q", msg)
	}
}

func TestEscalationMessageFiltersClaudeChrome(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting"}
	output := strings.Join([]string{
		"⏺ Here is the actual answer Claude gave.",
		"✻ Cooked for 1m 18s",
		"※ recap: some recap line that should be dropped",
		"❯ what is the bug we see now",
		"⎿  Interrupted · What should Claude do instead?",
		"──────────────────────────────",
		"anthonymirville@host repo (main) [Sonnet 4.6] ctx:27%",
		"⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")
	msg := state.EscalationMessage(s, output)
	if !strings.Contains(msg, "Here is the actual answer Claude gave.") {
		t.Errorf("expected Claude response line in message: %q", msg)
	}
	for _, banned := range []string{
		"Cooked for",
		"recap:",
		"what is the bug",
		"Interrupted",
		"ctx:27%",
		"bypass permissions",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("expected %q to be filtered out, got: %q", banned, msg)
		}
	}
}

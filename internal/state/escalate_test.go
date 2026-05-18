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

package cmd

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/hook"
)

func TestRunHookHandlerWritesStatus(t *testing.T) {
	dir := t.TempDir()
	payload := `{"hook_event_name":"UserPromptSubmit","session_id":"claude-1"}`

	runHookHandler(strings.NewReader(payload), "inst-42", dir)

	got, ok := hook.ReadStatus(dir, "inst-42")
	if !ok {
		t.Fatal("no status file written")
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.SessionID != "claude-1" {
		t.Errorf("session id = %q, want claude-1", got.SessionID)
	}
}

func TestRunHookHandlerNoInstanceID(t *testing.T) {
	dir := t.TempDir()
	runHookHandler(strings.NewReader(`{"hook_event_name":"Stop"}`), "", dir)
	if len(hook.ListStatuses(dir)) != 0 {
		t.Error("expected no write when instance id is empty")
	}
}

func TestRunHookHandlerIgnoredEvent(t *testing.T) {
	dir := t.TempDir()
	runHookHandler(strings.NewReader(`{"hook_event_name":"PreCompact"}`), "inst-1", dir)
	if len(hook.ListStatuses(dir)) != 0 {
		t.Error("expected no write for unmapped event")
	}
}

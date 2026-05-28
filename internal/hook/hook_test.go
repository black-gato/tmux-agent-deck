package hook_test

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/hook"
)

func TestParseEvent_Stop(t *testing.T) {
	r := strings.NewReader(`{"hook_event_name":"Stop","session_id":"abc"}`)
	ev, err := hook.ParseEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.EventName != "Stop" {
		t.Errorf("got EventName %q, want %q", ev.EventName, "Stop")
	}
	if ev.ToolName != "" {
		t.Errorf("got ToolName %q, want empty", ev.ToolName)
	}
	if ev.Message != "" {
		t.Errorf("got Message %q, want empty", ev.Message)
	}
}

func TestParseEvent_PermissionRequest(t *testing.T) {
	r := strings.NewReader(`{"hook_event_name":"PermissionRequest","tool_name":"Bash"}`)
	ev, err := hook.ParseEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.EventName != "PermissionRequest" {
		t.Errorf("got EventName %q, want %q", ev.EventName, "PermissionRequest")
	}
	if ev.ToolName != "Bash" {
		t.Errorf("got ToolName %q, want %q", ev.ToolName, "Bash")
	}
}

func TestParseEvent_Notification(t *testing.T) {
	r := strings.NewReader(`{"hook_event_name":"Notification","message":"needs input"}`)
	ev, err := hook.ParseEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Message != "needs input" {
		t.Errorf("got Message %q, want %q", ev.Message, "needs input")
	}
}

func TestParseEvent_UnknownEvent(t *testing.T) {
	r := strings.NewReader(`{"hook_event_name":"SomeFutureEvent"}`)
	ev, err := hook.ParseEvent(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.EventName != "SomeFutureEvent" {
		t.Errorf("got EventName %q, want %q", ev.EventName, "SomeFutureEvent")
	}
}

func TestParseEvent_InvalidJSON(t *testing.T) {
	r := strings.NewReader(`not json`)
	_, err := hook.ParseEvent(r)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestEventToStatus(t *testing.T) {
	cases := map[string]string{
		"SessionStart":      "waiting",
		"UserPromptSubmit":  "running",
		"Stop":              "waiting",
		"PermissionRequest": "waiting",
		"SessionEnd":        "dead",
		"PreCompact":        "",
		"Unknown":           "",
	}
	for event, want := range cases {
		if got := hook.EventToStatus(event); got != want {
			t.Errorf("EventToStatus(%q) = %q, want %q", event, got, want)
		}
	}
}

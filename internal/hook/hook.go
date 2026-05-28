package hook

import (
	"encoding/json"
	"io"
)

type HookEvent struct {
	EventName string
	ToolName  string
	Message   string
	SessionID string
}

type hookPayload struct {
	EventName string `json:"hook_event_name"`
	ToolName  string `json:"tool_name"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
}

func ParseEvent(r io.Reader) (HookEvent, error) {
	var p hookPayload
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return HookEvent{}, err
	}
	return HookEvent{
		EventName: p.EventName,
		ToolName:  p.ToolName,
		Message:   p.Message,
		SessionID: p.SessionID,
	}, nil
}

func EventToStatus(event string) string {
	switch event {
	case "SessionStart", "Stop", "PermissionRequest":
		return "waiting"
	case "UserPromptSubmit":
		return "running"
	case "SessionEnd":
		return "dead"
	default:
		return ""
	}
}

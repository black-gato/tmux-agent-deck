package hook

import (
	"encoding/json"
	"io"
)

type HookEvent struct {
	EventName string
	ToolName  string
	Message   string
}

type hookPayload struct {
	EventName string `json:"hook_event_name"`
	ToolName  string `json:"tool_name"`
	Message   string `json:"message"`
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
	}, nil
}

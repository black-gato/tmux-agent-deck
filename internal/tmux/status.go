package tmux

import (
	"strings"
	"time"
)

type Status = string

const (
	StatusRunning = "running"
	StatusWaiting = "waiting"
	StatusIdle    = "idle"
	StatusError   = "error"
	StatusStopped = "stopped"
)

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func DetectStatus(output string, lastChange time.Time) Status {
	trimmed := strings.TrimRight(output, " \t")

	// waiting: Claude prompt visible at end of pane
	if strings.HasSuffix(trimmed, "> ") || strings.HasSuffix(trimmed, ">") {
		return StatusWaiting
	}

	// running: spinner or thinking text in tail of output
	tail := output
	if len(tail) > 200 {
		tail = tail[len(tail)-200:]
	}
	for _, ch := range spinnerChars {
		if strings.Contains(tail, ch) {
			return StatusRunning
		}
	}
	if strings.Contains(tail, "Thinking") || strings.Contains(tail, "Running") {
		return StatusRunning
	}

	// idle: nothing recognizable, and no change for >30s
	if time.Since(lastChange) > 30*time.Second {
		return StatusIdle
	}

	return StatusRunning
}

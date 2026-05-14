package tmux

import (
	"strconv"
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

func ParseContextPct(output string) *int {
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "context used") &&
			!strings.Contains(lower, "context window") &&
			!strings.Contains(lower, "% context") {
			continue
		}
		idx := strings.Index(line, "%")
		if idx <= 0 {
			continue
		}
		start := idx - 1
		for start > 0 && line[start-1] >= '0' && line[start-1] <= '9' {
			start--
		}
		if start == idx {
			continue
		}
		n, err := strconv.Atoi(line[start:idx])
		if err != nil || n < 0 || n > 100 {
			continue
		}
		return &n
	}
	return nil
}

func lastLine(s string) string {
	if idx := strings.LastIndex(s, "\n"); idx >= 0 {
		return strings.TrimRight(s[idx+1:], " \t")
	}
	return s
}

func DetectStatus(output string, lastChange time.Time, tool string) Status {
	trimmed := strings.TrimRight(output, " \t\r\n")

	switch tool {
	case "aider":
		if strings.HasSuffix(trimmed, "aider> ") || strings.HasSuffix(trimmed, "aider>") {
			return StatusWaiting
		}
	case "copilot":
		if strings.HasSuffix(trimmed, "❯ ") || strings.HasSuffix(trimmed, "❯") ||
			strings.HasSuffix(trimmed, "> ") || strings.HasSuffix(trimmed, ">") {
			return StatusWaiting
		}
	default: // "claude", "", and any other tool
		ll := lastLine(trimmed)
		// Shell prompts: last line ends with $ or #
		if strings.HasSuffix(ll, "$") || strings.HasSuffix(ll, "#") {
			return StatusWaiting
		}
		// claude-style prompt: last line is a standalone >
		if ll == ">" {
			return StatusWaiting
		}
	}

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

	if time.Since(lastChange) > 30*time.Second {
		return StatusIdle
	}

	return StatusRunning
}

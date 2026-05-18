package tmux

import (
	"regexp"
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
var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

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

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func recentNonEmptyLines(s string, n int) []string {
	lines := strings.Split(s, "\n")
	recent := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(recent) < n; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		recent = append(recent, line)
	}
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	return recent
}

func isAgentPromptLine(line string) bool {
	return line == ">" || line == "❯"
}

func isClaudeTool(tool string) bool {
	return tool == "claude" || strings.HasPrefix(tool, "claude-")
}

func DetectStatus(output string, lastChange, now time.Time, tool string) Status {
	output = stripANSI(output)
	trimmed := strings.TrimRight(output, " \t\r\n")
	lastNonEmpty := lastNonEmptyLine(trimmed)

	switch {
	case isClaudeTool(tool):
		for _, line := range recentNonEmptyLines(trimmed, 8) {
			if isAgentPromptLine(line) {
				return StatusWaiting
			}
		}
	case tool == "aider":
		if strings.HasSuffix(lastNonEmpty, "aider>") {
			return StatusWaiting
		}
	case tool == "copilot":
		if lastNonEmpty == "❯" || lastNonEmpty == ">" {
			return StatusWaiting
		}
	default: // "", "bash", and any other shell-like tool
		ll := lastLine(trimmed)
		if strings.HasSuffix(ll, "$") || strings.HasSuffix(ll, "#") {
			return StatusWaiting
		}
		if strings.TrimSpace(ll) == ">" {
			return StatusWaiting
		}
	}

	// lastChange is the time the pane output last changed. If the pane has been
	// silent for >30s, treat it as idle even if the tail still contains stale
	// "Thinking" / spinner text from a long-finished operation (BUG-005).
	if now.Sub(lastChange) > 30*time.Second {
		return StatusIdle
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

	return StatusRunning
}

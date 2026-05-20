package state

import (
	"fmt"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func EscalationMessage(session db.Session, lastOutput string) string {
	parts := []string{
		fmt.Sprintf("Escalation from %s", session.Title),
		fmt.Sprintf("Worker ID: %s", session.ID),
		fmt.Sprintf("Status: %s", session.Status),
	}
	if session.Notes != "" {
		parts = append(parts, fmt.Sprintf("Notes: %s", session.Notes))
	}
	parts = append(parts, fmt.Sprintf("Reply with: @deck-reply worker=%s ... @deck-end", session.ID))
	context := strings.Join(contextLines(lastOutput, 5), " | ")
	if strings.TrimSpace(context) != "" {
		parts = append(parts, fmt.Sprintf("Context: %s", context))
	}
	return strings.Join(parts, " | ") + "\n"
}

func contextLines(output string, n int) []string {
	if n <= 0 || output == "" {
		return nil
	}
	all := strings.Split(output, "\n")
	lines := make([]string, 0, n)
	for i := len(all) - 1; i >= 0 && len(lines) < n; i-- {
		line := strings.TrimSpace(all[i])
		if !isContextLine(line) {
			continue
		}
		lines = append(lines, line)
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}

func isContextLine(line string) bool {
	if line == "" || line == ">" || line == "❯" {
		return false
	}
	if strings.Contains(line, "-- INSERT --") {
		return false
	}
	if strings.Contains(line, "ctx:") {
		return false
	}
	if strings.HasPrefix(line, "❯ ") {
		return false
	}
	if strings.HasPrefix(line, "✻ ") {
		return false
	}
	if strings.HasPrefix(line, "※ ") {
		return false
	}
	if strings.HasPrefix(line, "⎿") {
		return false
	}
	if strings.Contains(line, "⏵⏵") {
		return false
	}
	return strings.Trim(line, "─━═- ") != ""
}

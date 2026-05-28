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
	parts = append(parts, fmt.Sprintf("Reply to worker %s using your conductor role reply protocol.", session.ID))
	ctx := lastClaudeBlock(lastOutput)
	if len(ctx) == 0 {
		ctx = contextLines(lastOutput, 5)
	}
	context := strings.Join(ctx, " | ")
	if strings.TrimSpace(context) != "" {
		parts = append(parts, fmt.Sprintf("Context: %s", context))
	}
	return strings.Join(parts, " | ") + "\n"
}

// claudeBullet (U+23FA) prefixes every Claude Code assistant block — prose
// responses and tool calls alike. It is distinct from the feedback-survey
// bullet ● (U+25CF), so surveys are not mistaken for responses.
const claudeBullet = "⏺"

// lastClaudeBlock extracts Claude's final assistant block: the last line
// starting with claudeBullet plus its indented continuation lines, stopping at
// the next bullet, timing line, separator, or input chrome. This captures the
// whole last message regardless of length, where the old line-window approach
// truncated long answers and bled into the survey and the user's unsent prompt.
// Returns nil when no Claude block is present (e.g. shell/aider workers), so the
// caller can fall back to the generic line scan.
func lastClaudeBlock(output string) []string {
	all := strings.Split(output, "\n")
	start := -1
	for i := len(all) - 1; i >= 0; i-- {
		if strings.HasPrefix(all[i], claudeBullet) {
			start = i
			break
		}
	}
	if start == -1 {
		return nil
	}
	var lines []string
	for i := start; i < len(all); i++ {
		line := all[i]
		if i != start && strings.TrimSpace(line) != "" && !strings.HasPrefix(line, " ") {
			break
		}
		text := strings.TrimSpace(strings.TrimPrefix(line, claudeBullet))
		if text == "" || strings.Trim(text, "─━═- ") == "" {
			continue
		}
		lines = append(lines, text)
	}
	return lines
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
	if strings.HasPrefix(line, "❯") { // Claude renders ❯ + U+00A0, not ❯ + space
		return false
	}
	if strings.HasPrefix(line, "●") { // feedback survey bullet (U+25CF)
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

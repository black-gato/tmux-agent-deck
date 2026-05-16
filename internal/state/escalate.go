package state

import (
	"fmt"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func escalationMessage(session db.Session, lastOutput string) string {
	lines := []string{
		fmt.Sprintf("Escalation from %s", session.Title),
		fmt.Sprintf("Status: %s", session.Status),
	}
	if session.Notes != "" {
		lines = append(lines, fmt.Sprintf("Notes: %s", session.Notes))
	}
	context := strings.Join(tailLines(lastOutput, 3), "\n")
	if strings.TrimSpace(context) != "" {
		lines = append(lines, "Current issue context:")
		lines = append(lines, context)
	}
	return strings.Join(lines, "\n")
}

func tailLines(output string, n int) []string {
	if n <= 0 || output == "" {
		return nil
	}
	all := strings.Split(output, "\n")
	if len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	if len(all) <= n {
		return all
	}
	return all[len(all)-n:]
}

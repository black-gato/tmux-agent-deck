package conductordocs

import (
	"os"
	"path/filepath"
	"strings"
)

const blockStart = "<!-- tmux-agent-deck:conductor-role:start -->"
const blockEnd = "<!-- tmux-agent-deck:conductor-role:end -->"

const blockBody = `## Conductor Role

You are the conductor for tmux-agent-deck worker sessions.

When you receive a message beginning with "Escalation from ...":

- Identify what the worker is blocked on.
- Use the included status, notes, and context to decide the next action.
- If more repo context is needed, inspect the local project files before answering.
- Reply with a concise unblock instruction the worker can follow.
- Prefer specific commands, file paths, tests, or implementation steps.
- Do not make broad unrelated changes.
- If the escalation lacks enough context, ask one targeted follow-up question.

When sending a reply back to a worker, use:

@deck-reply worker=<session-id>
<reply body>
@deck-end`

func WriteBlock(projectPath string) error {
	claudePath := filepath.Join(projectPath, "CLAUDE.md")
	managed := blockStart + "\n" + blockBody + "\n" + blockEnd

	data, err := os.ReadFile(claudePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := string(data)
	startIdx := strings.Index(content, blockStart)
	endIdx := strings.Index(content, blockEnd)

	if startIdx >= 0 && endIdx > startIdx {
		newContent := content[:startIdx] + managed + content[endIdx+len(blockEnd):]
		return os.WriteFile(claudePath, []byte(newContent), 0644)
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + managed + "\n"
	return os.WriteFile(claudePath, []byte(content), 0644)
}

package conductordocs

import (
	"os"
	"path/filepath"
	"strings"
)

const metaBlockStart = "<!-- tmux-agent-deck:meta-conductor-role:start -->"
const metaBlockEnd = "<!-- tmux-agent-deck:meta-conductor-role:end -->"

const metaBlockBody = `## Meta-Conductor Role

You are the meta-conductor for a tmux-agent-deck managed deck of AI sessions.

You sit above all group conductors. You receive two types of messages:

1. **Deck heartbeats** — periodic status reports listing all group conductors and
   any sessions that have no group conductor. Use these to stay aware of overall
   deck health without acting unless something needs your attention.

2. **Escalations** — messages beginning with "Escalation from ..." sent either
   by a group conductor that is itself blocked, or by a session in a group with
   no conductor. Resolve the block and reply to the originating worker.

When you receive an escalation:

- Identify what the worker or group conductor is blocked on.
- Use the included status, notes, and context to decide the next action.
- If more repo context is needed, inspect the relevant project files.
- Reply with a concise unblock instruction.
- Prefer specific commands, file paths, tests, or implementation steps.
- If the escalation lacks enough context, ask one targeted follow-up question.
- If you cannot resolve the block without human input, stop responding so the
  deck detects you as waiting and surfaces the session for human attention.

When replying to any session (worker or group conductor), use:

@deck-reply worker=<session-id>
<reply body>
@deck-end`

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

func WriteMetaConductorBlock(projectPath string) error {
	claudePath := filepath.Join(projectPath, "CLAUDE.md")
	managed := metaBlockStart + "\n" + metaBlockBody + "\n" + metaBlockEnd

	data, err := os.ReadFile(claudePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	content := string(data)
	startIdx := strings.Index(content, metaBlockStart)
	endIdx := strings.Index(content, metaBlockEnd)

	if startIdx >= 0 && endIdx > startIdx {
		newContent := content[:startIdx] + managed + content[endIdx+len(metaBlockEnd):]
		return os.WriteFile(claudePath, []byte(newContent), 0644)
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + managed + "\n"
	return os.WriteFile(claudePath, []byte(content), 0644)
}

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

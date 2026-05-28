package state_test

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/state"
)

func TestEscalationMessageIncludesWorkerID(t *testing.T) {
	s := db.Session{ID: "worker-42", Title: "my-worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "")
	if !strings.Contains(msg, "Worker ID: worker-42") {
		t.Errorf("message missing worker ID: %q", msg)
	}
}

func TestEscalationMessageOmitsLiteralReplyMarkers(t *testing.T) {
	// Embedding literal @deck-reply / @deck-end markers in the escalation echoed
	// them back into the conductor's own pane, where the reply parser then
	// matched the echo as a phantom reply with body "..." and routed it to the
	// worker. The message must not contain literal marker substrings.
	s := db.Session{ID: "worker-42", Title: "my-worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "")
	if strings.Contains(msg, "@deck-reply") {
		t.Errorf("message must not contain literal @deck-reply marker: %q", msg)
	}
	if strings.Contains(msg, "@deck-end") {
		t.Errorf("message must not contain literal @deck-end marker: %q", msg)
	}
}

func TestEscalationMessageIncludesNotes(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting", Notes: "stuck on auth"}
	msg := state.EscalationMessage(s, "")
	if !strings.Contains(msg, "Notes: stuck on auth") {
		t.Errorf("message missing notes: %q", msg)
	}
}

func TestEscalationMessageIncludesContext(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting"}
	msg := state.EscalationMessage(s, "some relevant output line")
	if !strings.Contains(msg, "some relevant output line") {
		t.Errorf("message missing context: %q", msg)
	}
}

func TestEscalationMessageCapturesFullLastBlock(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting"}
	// Modeled on a real Claude pane: a multi-line answer block (more than the
	// old 5-line window) followed by the feedback survey, separators, and the
	// user's typed-but-unsent next prompt. The "❯" prompt uses U+00A0 (NBSP)
	// after the glyph, exactly as Claude Code renders it.
	output := strings.Join([]string{
		"⏺ For Neovim, there's no official Claude Code extension, but a few approaches work well:",
		"",
		"  Auto-reload edited buffers",
		"",
		"  Add a PostToolUse hook so Neovim reloads files Claude edits.",
		"",
		"  Community Plugin",
		"",
		"  There's a community plugin claude-code.nvim that wraps the CLI.",
		"",
		"  Want me to set up the hooks configuration in your .claude/settings.json?",
		"",
		"✻ Churned for 18s",
		"",
		"● How is Claude doing this session? (optional)",
		"  1: Bad    2: Fine   3: Good   0: Dismiss",
		"",
		"───────────────────────────────────────",
		"❯ set up the hooks config in my settings.json",
		"───────────────────────────────────────",
		"  anthonymirville@host repo (main) [Sonnet 4.6] ctx:11%",
		"  -- INSERT -- ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")
	msg := state.EscalationMessage(s, output)

	// Must include both the start and end of the answer block.
	for _, want := range []string{
		"For Neovim, there's no official Claude Code extension",
		"Community Plugin",
		"Want me to set up the hooks configuration",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected answer line %q in message, got: %q", want, msg)
		}
	}
	// Must exclude survey, rating menu, and the user's unsent next prompt.
	for _, banned := range []string{
		"How is Claude doing",
		"1: Bad",
		"set up the hooks config in my settings.json",
		"Churned for",
		"ctx:11%",
		"bypass permissions",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("expected %q to be excluded, got: %q", banned, msg)
		}
	}
}

func TestEscalationMessageFiltersClaudeChrome(t *testing.T) {
	s := db.Session{ID: "w1", Title: "worker", Status: "waiting"}
	output := strings.Join([]string{
		"⏺ Here is the actual answer Claude gave.",
		"✻ Cooked for 1m 18s",
		"※ recap: some recap line that should be dropped",
		"❯ what is the bug we see now",
		"⎿  Interrupted · What should Claude do instead?",
		"──────────────────────────────",
		"anthonymirville@host repo (main) [Sonnet 4.6] ctx:27%",
		"⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")
	msg := state.EscalationMessage(s, output)
	if !strings.Contains(msg, "Here is the actual answer Claude gave.") {
		t.Errorf("expected Claude response line in message: %q", msg)
	}
	for _, banned := range []string{
		"Cooked for",
		"recap:",
		"what is the bug",
		"Interrupted",
		"ctx:27%",
		"bypass permissions",
	} {
		if strings.Contains(msg, banned) {
			t.Errorf("expected %q to be filtered out, got: %q", banned, msg)
		}
	}
}

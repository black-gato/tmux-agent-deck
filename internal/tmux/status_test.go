package tmux_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

func TestParseContextPctDetectsPercentage(t *testing.T) {
	cases := []struct {
		output string
		want   *int
	}{
		{"Some output\n75% context used · /compact\n> ", intPtr(75)},
		{"context window: 50%\n> ", intPtr(50)},
		{"100% context used", intPtr(100)},
		{"0% context used", intPtr(0)},
		{"no percentage here\n> ", nil},
		{"75% complete (not context)", nil},
		{"", nil},
	}
	for _, tc := range cases {
		got := tmux.ParseContextPct(tc.output)
		if tc.want == nil && got != nil {
			t.Errorf("output %q: expected nil, got %d", tc.output, *got)
		} else if tc.want != nil && (got == nil || *got != *tc.want) {
			gotStr := "<nil>"
			if got != nil {
				gotStr = fmt.Sprintf("%d", *got)
			}
			t.Errorf("output %q: expected %d, got %s", tc.output, *tc.want, gotStr)
		}
	}
}

func intPtr(n int) *int { return &n }

func TestDetectStatusWaiting(t *testing.T) {
	now := time.Now()
	for _, output := range []string{
		"Some output\n> ",
		"last line\n>",
	} {
		status := tmux.DetectStatus(output, now, now, "claude")
		if status != tmux.StatusWaiting {
			t.Errorf("output %q: got %q want %q", output, status, tmux.StatusWaiting)
		}
	}
}

func TestDetectStatusClaudePromptAboveStatusFooter(t *testing.T) {
	now := time.Now()
	lastChange := now.Add(-10 * time.Second)
	output := strings.Join([]string{
		"※ recap: Ready to resume work.",
		"────────────────────────",
		"❯\u00a0",
		"────────────────────────",
		"  anthonymirville@host project [Sonnet 4.6] ctx:12%",
		"  -- INSERT -- ← for agents",
		"",
		"",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusWaiting {
		t.Errorf("got %q want %q", status, tmux.StatusWaiting)
	}
}

func TestDetectStatusClaudePromptWithANSI(t *testing.T) {
	now := time.Now()
	lastChange := now.Add(-10 * time.Second)
	output := strings.Join([]string{
		"※ recap: Ready to resume work.",
		"\x1b[32m❯\x1b[0m",
		"────────────────────────",
		"  anthonymirville@host project [Sonnet 4.6] ctx:12%",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusWaiting {
		t.Errorf("got %q want %q", status, tmux.StatusWaiting)
	}
}

func TestDetectStatusClaudePresetPromptAboveStatusFooter(t *testing.T) {
	now := time.Now()
	lastChange := now.Add(-10 * time.Second)
	output := strings.Join([]string{
		"⏺ Hey! What are you working on?",
		"✻ Brewed for 25s",
		"────────────────────────",
		"❯",
		"────────────────────────",
		"  anthonymirville@host tmux-agent-deck (main) [Sonnet 4.6] ctx:14%",
		"  -- INSERT -- ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude-dangerous")
	if status != tmux.StatusWaiting {
		t.Errorf("got %q want %q", status, tmux.StatusWaiting)
	}
}

func TestDetectStatusBashDoesNotMatchOldPromptWhileCommandRuns(t *testing.T) {
	now := time.Now()
	lastChange := now.Add(-31 * time.Second)
	output := "user@host:~$ sleep 35\n"

	status := tmux.DetectStatus(output, lastChange, now, "bash")
	if status != tmux.StatusIdle {
		t.Errorf("got %q want %q", status, tmux.StatusIdle)
	}
}

func TestDetectStatusRunning(t *testing.T) {
	now := time.Now()
	for _, output := range []string{
		"⠋ Thinking...",
		"⠙ Working...",
		"● Running",
		"Thinking about your request",
	} {
		status := tmux.DetectStatus(output, now, now, "claude")
		if status != tmux.StatusRunning {
			t.Errorf("output %q: got %q want running", output, status)
		}
	}
}

func TestDetectStatusClaudeRunningWithActiveThinker(t *testing.T) {
	// ✢ (active spinner) + bare ❯ input box — real pane layout while Claude works.
	// ✢ must take precedence over the always-visible ❯ chrome.
	now := time.Now()
	output := strings.Join([]string{
		"❯ What needs to be created to make this a possibility",
		"✢ Canoodling… (5s · ↓ 166 tokens · thinking with medium effort)",
		"────────────────────────",
		"❯",
		"────────────────────────",
		"  anthonymirville@Anthonys-MacBook-Air Projects [Opus 4.7] ctx:3%",
		"  -- INSERT -- ⏵⏵ bypass permissions on (shift+tab to cycle)",
	}, "\n")

	status := tmux.DetectStatus(output, now, now, "claude")
	if status != tmux.StatusRunning {
		t.Errorf("got %q want running — ✢ should take precedence over ❯ chrome", status)
	}
}

func TestDetectStatusClaudeRunningWithTokenStream(t *testing.T) {
	// Token-stream status line is another running indicator when ✢ is not present.
	now := time.Now()
	output := strings.Join([]string{
		"· ↓ 166 tokens · thinking with medium effort",
		"────────────────────────",
		"❯",
		"────────────────────────",
		"  -- INSERT -- ⏵⏵ bypass permissions on",
	}, "\n")

	status := tmux.DetectStatus(output, now, now, "claude")
	if status != tmux.StatusRunning {
		t.Errorf("got %q want running", status)
	}
}

func TestDetectStatusClaudeWaitingWithDisplayTextInInputBox(t *testing.T) {
	// Claude Code shows display text in the input box (not user input, not agent output).
	// ❯ followed by any text must still be detected as waiting.
	now := time.Now()
	lastChange := now.Add(-10 * time.Second)
	output := strings.Join([]string{
		"✻ Cogitated for 1s",
		"────────────────────────────────────────────────────────────────────────────",
		"❯ Stop the escalation recaps for this worker",
		"────────────────────────────────────────────────────────────────────────────",
		"  anthonymirville@Anthonys-MacBook-Air tmux-agent-deck (main) [Opus 4.7] ctx:3%",
		"  -- INSERT -- ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusWaiting {
		t.Errorf("got %q want waiting — ❯ with display text should be detected as waiting", status)
	}
}

func TestDetectStatusClaudeIdleAfterSixtySecondsWaiting(t *testing.T) {
	// A session at the ❯ prompt with no pane activity for 60s+ is idle.
	now := time.Now()
	lastChange := now.Add(-61 * time.Second)
	output := strings.Join([]string{
		"✻ Cogitated for 1s",
		"────────────────────────",
		"❯ Stop the escalation recaps for this worker",
		"────────────────────────",
		"  anthonymirville@Anthonys-MacBook-Air tmux-agent-deck (main) [Opus 4.7] ctx:3%",
		"  -- INSERT -- ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusIdle {
		t.Errorf("got %q want idle — ❯ prompt silent for 60s+ should be idle", status)
	}
}

func TestDetectStatusClaudeStaleActiveThinkerIsWaiting(t *testing.T) {
	// A frozen ✢ line (pane silent >30s) with bare ❯ should be waiting, not running.
	now := time.Now()
	lastChange := now.Add(-31 * time.Second)
	output := strings.Join([]string{
		"✢ Canoodling… (5s · ↓ 166 tokens · thinking with medium effort)",
		"────────────────────────",
		"❯",
		"────────────────────────",
		"  -- INSERT -- ⏵⏵ bypass permissions on",
	}, "\n")

	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusWaiting {
		t.Errorf("got %q want waiting — stale ✢ must not pin running when ❯ is present", status)
	}
}

func TestDetectStatusIdle(t *testing.T) {
	output := "Some old output without a prompt"
	now := time.Now()
	lastChange := now.Add(-31 * time.Second)
	status := tmux.DetectStatus(output, lastChange, now, "claude")
	if status != tmux.StatusIdle {
		t.Errorf("got %q want idle", status)
	}
}

func TestDetectStatusStaleRunningMarkerBecomesIdle(t *testing.T) {
	// BUG-005: stale "Thinking" / spinner text must not pin a session at running
	// when the pane output has not changed in over 30 seconds.
	now := time.Now()
	lastChange := now.Add(-31 * time.Second)
	for _, output := range []string{
		"⠋ Thinking...",
		"Thinking about your request",
		"● Running tests",
	} {
		status := tmux.DetectStatus(output, lastChange, now, "claude")
		if status != tmux.StatusIdle {
			t.Errorf("stale output %q: got %q want idle", output, status)
		}
	}
}

func TestDetectStatusRecentActivityIsRunning(t *testing.T) {
	now := time.Now()
	output := "Some output without a prompt"
	status := tmux.DetectStatus(output, now, now, "claude")
	if status != tmux.StatusRunning {
		t.Errorf("got %q want running (recent activity)", status)
	}
}

func TestDetectStatusAiderWaiting(t *testing.T) {
	now := time.Now()
	for _, output := range []string{
		"Some output\naider> ",
		"Some output\naider>",
	} {
		status := tmux.DetectStatus(output, now, now, "aider")
		if status != tmux.StatusWaiting {
			t.Errorf("aider output %q: got %q want waiting", output, status)
		}
	}
}

func TestDetectStatusAiderPromptNotMatchedForClaude(t *testing.T) {
	now := time.Now()
	output := "Some output\naider> "
	status := tmux.DetectStatus(output, now, now, "claude")
	if status == tmux.StatusWaiting {
		t.Errorf("aider> should not match waiting for claude tool")
	}
}

func TestDetectStatusCopilotWaiting(t *testing.T) {
	now := time.Now()
	for _, output := range []string{
		"Some output\n❯ ",
		"Some output\n❯",
		"Some output\n> ",
	} {
		status := tmux.DetectStatus(output, now, now, "copilot")
		if status != tmux.StatusWaiting {
			t.Errorf("copilot output %q: got %q want waiting", output, status)
		}
	}
}

func TestDetectStatusBashWaiting(t *testing.T) {
	now := time.Now()
	for _, output := range []string{
		"user@host:~$ ",
		"root@host:~# ",
		"Some output\n> ",
		"command output\nuser@host:~$ \n\n\n",
	} {
		status := tmux.DetectStatus(output, now, now, "")
		if status != tmux.StatusWaiting {
			t.Errorf("bash output %q: got %q want waiting", output, status)
		}
	}
}

func TestParseBindingCommand(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		input string
		want  string
	}{
		{
			name:  "simple command",
			key:   "C-q",
			input: "bind-key -T root C-q display-panes",
			want:  "display-panes",
		},
		{
			name:  "multi-word command",
			key:   "C-q",
			input: "bind-key -T root C-q run-shell 'echo hi'",
			want:  "run-shell 'echo hi'",
		},
		{
			name:  "repeatable flag",
			key:   "C-q",
			input: "bind-key -rT root C-q resize-pane -D 5",
			want:  "resize-pane -D 5",
		},
		{
			name:  "empty output means no binding",
			key:   "C-q",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only",
			key:   "C-q",
			input: "   ",
			want:  "",
		},
		{
			name:  "key not present returns empty",
			key:   "C-q",
			input: "bind-key -T root x some-command",
			want:  "",
		},
		{
			name:  "single-char key still works",
			key:   "q",
			input: "bind-key -T prefix q send-keys q Enter",
			want:  "send-keys q Enter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmux.ParseBindingCommand(tt.input, tt.key)
			if got != tt.want {
				t.Errorf("ParseBindingCommand(%q, %q) = %q, want %q", tt.input, tt.key, got, tt.want)
			}
		})
	}
}

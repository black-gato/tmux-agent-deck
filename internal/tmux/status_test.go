package tmux_test

import (
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

func TestDetectStatusWaiting(t *testing.T) {
	for _, output := range []string{
		"Some output\n> ",
		"last line\n>",
	} {
		status := tmux.DetectStatus(output, time.Now(), "claude")
		if status != tmux.StatusWaiting {
			t.Errorf("output %q: got %q want %q", output, status, tmux.StatusWaiting)
		}
	}
}

func TestDetectStatusRunning(t *testing.T) {
	for _, output := range []string{
		"⠋ Thinking...",
		"⠙ Working...",
		"● Running",
		"Thinking about your request",
	} {
		status := tmux.DetectStatus(output, time.Now(), "claude")
		if status != tmux.StatusRunning {
			t.Errorf("output %q: got %q want running", output, status)
		}
	}
}

func TestDetectStatusIdle(t *testing.T) {
	output := "Some old output without a prompt"
	lastChange := time.Now().Add(-31 * time.Second)
	status := tmux.DetectStatus(output, lastChange, "claude")
	if status != tmux.StatusIdle {
		t.Errorf("got %q want idle", status)
	}
}

func TestDetectStatusRecentActivityIsRunning(t *testing.T) {
	output := "Some output without a prompt"
	status := tmux.DetectStatus(output, time.Now(), "claude")
	if status != tmux.StatusRunning {
		t.Errorf("got %q want running (recent activity)", status)
	}
}

func TestDetectStatusAiderWaiting(t *testing.T) {
	for _, output := range []string{
		"Some output\naider> ",
		"Some output\naider>",
	} {
		status := tmux.DetectStatus(output, time.Now(), "aider")
		if status != tmux.StatusWaiting {
			t.Errorf("aider output %q: got %q want waiting", output, status)
		}
	}
}

func TestDetectStatusAiderPromptNotMatchedForClaude(t *testing.T) {
	// "aider> " at end should NOT trigger waiting for claude tool
	output := "Some output\naider> "
	status := tmux.DetectStatus(output, time.Now(), "claude")
	if status == tmux.StatusWaiting {
		t.Errorf("aider> should not match waiting for claude tool")
	}
}

func TestDetectStatusCopilotWaiting(t *testing.T) {
	for _, output := range []string{
		"Some output\n❯ ",
		"Some output\n❯",
		"Some output\n> ",
	} {
		status := tmux.DetectStatus(output, time.Now(), "copilot")
		if status != tmux.StatusWaiting {
			t.Errorf("copilot output %q: got %q want waiting", output, status)
		}
	}
}

func TestDetectStatusBashWaiting(t *testing.T) {
	for _, output := range []string{
		"user@host:~$ ",
		"root@host:~# ",
		"Some output\n> ",
		"command output\nuser@host:~$ \n\n\n",
	} {
		status := tmux.DetectStatus(output, time.Now(), "")
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

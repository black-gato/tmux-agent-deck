package tmux

import (
	"strings"
	"testing"
)

func TestPaneTarget(t *testing.T) {
	tests := []struct {
		session string
		pane    int
		want    string
	}{
		{"mysession", 0, "mysession:0.0"},
		{"ad-abc12345", 2, "ad-abc12345:0.2"},
	}
	for _, tc := range tests {
		got := paneTarget(tc.session, tc.pane)
		if got != tc.want {
			t.Errorf("paneTarget(%q, %d) = %q, want %q", tc.session, tc.pane, got, tc.want)
		}
	}
}

func TestParsePanesOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Pane
	}{
		{
			name:  "two panes",
			input: "0 claude\n1 bash\n",
			want:  []Pane{{Index: 0, Command: "claude"}, {Index: 1, Command: "bash"}},
		},
		{
			name:  "empty output",
			input: "",
			want:  []Pane{},
		},
		{
			name:  "single pane",
			input: "0 nvim\n",
			want:  []Pane{{Index: 0, Command: "nvim"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePanesOutput(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d want %d", len(got), len(tc.want))
			}
			for i, p := range got {
				if p.Index != tc.want[i].Index || p.Command != tc.want[i].Command {
					t.Errorf("[%d] got %+v want %+v", i, p, tc.want[i])
				}
			}
		})
	}
}

func TestBuildLaunchCommand(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		toolFlags string
		want      string
	}{
		{"no flags", "claude", "", "claude"},
		{"with flags", "claude", "--dangerously-skip-permissions", "claude --dangerously-skip-permissions"},
		{"multiple flags", "claude", "--model opus --dangerously-skip-permissions", "claude --model opus --dangerously-skip-permissions"},
		{"shell tool with flags", "shell", "--flag", "zsh -il --flag"},
		{"empty tool no flags", "", "", ""},
		{"flags trimmed", "claude", "  --resume  ", "claude --resume"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildLaunchCommand(tc.tool, tc.toolFlags); got != tc.want {
				t.Fatalf("buildLaunchCommand(%q, %q) = %q, want %q", tc.tool, tc.toolFlags, got, tc.want)
			}
		})
	}
}

func TestNewSessionArgsIncludesInstanceEnv(t *testing.T) {
	args := newSessionArgs("sess", "/work", "claude-dangerous", "", "inst-7")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-e AGENTDECK_INSTANCE_ID=inst-7") {
		t.Errorf("missing -e env flag: %v", args)
	}
	if !strings.Contains(joined, "claude --dangerously-skip-permissions") {
		t.Errorf("missing resolved launch command: %v", args)
	}
	if strings.Index(joined, "-e") > strings.Index(joined, "claude") {
		t.Errorf("-e must precede the command: %v", args)
	}
}

func TestNewSessionArgsNoInstanceID(t *testing.T) {
	args := newSessionArgs("sess", "/work", "shell", "", "")
	if strings.Contains(strings.Join(args, " "), "AGENTDECK_INSTANCE_ID") {
		t.Errorf("should not set env when instance id is empty: %v", args)
	}
}

func TestResolveLaunchCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "shell profile launches login zsh",
			input: "shell",
			want:  "zsh -il",
		},
		{
			name:  "claude-dangerous adds skip-permissions flag",
			input: "claude-dangerous",
			want:  "claude --dangerously-skip-permissions",
		},
		{
			name:  "other commands pass through",
			input: "claude",
			want:  "claude",
		},
		{
			name:  "empty stays empty",
			input: "",
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLaunchCommand(tc.input); got != tc.want {
				t.Fatalf("resolveLaunchCommand(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

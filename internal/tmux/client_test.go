package tmux

import "testing"

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

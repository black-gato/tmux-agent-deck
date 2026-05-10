package tmux

import "testing"

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

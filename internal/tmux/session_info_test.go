package tmux_test

import (
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

func TestParseSessionInfoOutput(t *testing.T) {
	got, err := tmux.ParseSessionInfoOutput("my-sess", "/tmp/work|1716480000")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "my-sess" {
		t.Errorf("name: got %q want %q", got.Name, "my-sess")
	}
	if got.CurrentPath != "/tmp/work" {
		t.Errorf("path: got %q want %q", got.CurrentPath, "/tmp/work")
	}
	if got.Activity.Unix() != 1716480000 {
		t.Errorf("activity: got %d want %d", got.Activity.Unix(), 1716480000)
	}
}

func TestParseSessionInfoOutputBlankActivity(t *testing.T) {
	got, err := tmux.ParseSessionInfoOutput("s", "/tmp|0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.Activity.IsZero() {
		t.Errorf("expected zero activity, got %v", got.Activity)
	}
}

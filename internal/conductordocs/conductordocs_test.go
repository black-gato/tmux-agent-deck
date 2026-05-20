package conductordocs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/conductordocs"
)

func TestWriteBlockCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := conductordocs.WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<!-- tmux-agent-deck:conductor-role:start -->") {
		t.Error("missing block start marker")
	}
	if !strings.Contains(string(data), "<!-- tmux-agent-deck:conductor-role:end -->") {
		t.Error("missing block end marker")
	}
	if !strings.Contains(string(data), "@deck-reply worker=<session-id>") {
		t.Error("missing reply syntax in block")
	}
}

func TestWriteBlockReplacesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	initial := "# My Project\n\n<!-- tmux-agent-deck:conductor-role:start -->\nold content\n<!-- tmux-agent-deck:conductor-role:end -->\n\nmore user content\n"
	os.WriteFile(path, []byte(initial), 0644)

	if err := conductordocs.WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "old content") {
		t.Error("old block content should be replaced")
	}
	if !strings.Contains(content, "more user content") {
		t.Error("user content after block should be preserved")
	}
	if !strings.Contains(content, "# My Project") {
		t.Error("user content before block should be preserved")
	}
}

func TestWriteBlockAppendsWhenNoBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	os.WriteFile(path, []byte("# My Project\n\nExisting content.\n"), 0644)

	if err := conductordocs.WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# My Project") {
		t.Error("existing content should be preserved")
	}
	if !strings.Contains(content, "<!-- tmux-agent-deck:conductor-role:start -->") {
		t.Error("block should be appended")
	}
}

func TestWriteBlockIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := conductordocs.WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	if err := conductordocs.WriteBlock(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	content := string(data)
	startCount := strings.Count(content, "<!-- tmux-agent-deck:conductor-role:start -->")
	if startCount != 1 {
		t.Errorf("expected exactly 1 start marker after two calls, got %d", startCount)
	}
}

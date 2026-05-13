package cmd_test

import (
	"bytes"
	"testing"

	"github.com/black-gato/tmux-agent-deck/cmd"
)

func TestAddAndList(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"add", "--title", "test-app", "--project", "/tmp"}, &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	out.Reset()
	if err := cmd.RunWith([]string{"list"}, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("test-app")) {
		t.Errorf("list output missing 'test-app': %s", out.String())
	}
}

func TestGroupCreateAndMove(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"group", "create", "work"}, &out); err != nil {
		t.Fatalf("group create: %v", err)
	}
	if err := cmd.RunWith([]string{"add", "--title", "my-app", "--project", "/tmp"}, &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := cmd.RunWith([]string{"group", "move", "my-app", "work"}, &out); err != nil {
		t.Fatalf("group move: %v", err)
	}

	out.Reset()
	if err := cmd.RunWith([]string{"list"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("my-app")) {
		t.Errorf("session missing from list after move: %s", out.String())
	}
}

func TestListAcceptsNotifyFlags(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	t.Setenv("AGENT_DECK_DB", dbPath)

	var out bytes.Buffer
	if err := cmd.RunWith([]string{"--notify", "--notify-style", "digest", "list"}, &out); err != nil {
		t.Fatalf("list with notify flags: %v", err)
	}
}

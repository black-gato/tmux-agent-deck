package state_test

import (
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/state"
)

func TestParseReplyBlocksComplete(t *testing.T) {
	input := "@deck-reply worker=abc\nhello world\n@deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].WorkerID != "abc" {
		t.Errorf("workerID: got %q want %q", blocks[0].WorkerID, "abc")
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksIncompleteIgnored(t *testing.T) {
	input := "@deck-reply worker=abc\nhello world"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestParseReplyBlocksMultiple(t *testing.T) {
	input := "@deck-reply worker=a\nfoo\n@deck-end\n@deck-reply worker=b\nbar\n@deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].WorkerID != "a" || blocks[1].WorkerID != "b" {
		t.Errorf("workerIDs: got %q %q", blocks[0].WorkerID, blocks[1].WorkerID)
	}
}

func TestParseReplyBlocksEmptyBodyIgnored(t *testing.T) {
	input := "@deck-reply worker=abc\n\n@deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty body, got %d", len(blocks))
	}
}

func TestParseReplyBlocksMultilineBodyNormalized(t *testing.T) {
	input := "@deck-reply worker=abc\nline one\nline two\n@deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Body != "line one | line two" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "line one | line two")
	}
}

func TestNewOutputSinceBaseline(t *testing.T) {
	baseline := "old content"
	current := "old content\nnew line"
	got := state.NewOutputSince(baseline, current)
	if got != "\nnew line" {
		t.Errorf("got %q want %q", got, "\nnew line")
	}
}

func TestNewOutputSinceBaselineNotFound(t *testing.T) {
	baseline := "old content"
	current := "completely different"
	got := state.NewOutputSince(baseline, current)
	if got != "completely different" {
		t.Errorf("got %q want %q", got, "completely different")
	}
}

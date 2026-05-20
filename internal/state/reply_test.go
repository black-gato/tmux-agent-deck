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

func TestParseReplyBlocksSingleLine(t *testing.T) {
	input := "@deck-reply worker=abc-123 hello world @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].WorkerID != "abc-123" {
		t.Errorf("workerID: got %q want %q", blocks[0].WorkerID, "abc-123")
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksSingleLineIndented(t *testing.T) {
	input := "  @deck-reply worker=abc-123 hello world @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksPromptPrefixedSingleLine(t *testing.T) {
	input := "❯ @deck-reply worker=abc-123 hello world @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].WorkerID != "abc-123" {
		t.Errorf("workerID: got %q want %q", blocks[0].WorkerID, "abc-123")
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksPromptPrefixedMultiline(t *testing.T) {
	input := "❯ @deck-reply worker=abc-123\n  hello world\n  @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksMixedForm(t *testing.T) {
	// Body on @deck-reply line, @deck-end on next line
	input := "❯ @deck-reply worker=abc-123 hello world\n  @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Body != "hello world" {
		t.Errorf("body: got %q want %q", blocks[0].Body, "hello world")
	}
}

func TestParseReplyBlocksSingleLineEmptyBodyIgnored(t *testing.T) {
	input := "@deck-reply worker=abc-123 @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty single-line body, got %d", len(blocks))
	}
}

func TestParseReplyBlocksSingleLineBodyMentionsEndMarker(t *testing.T) {
	input := "@deck-reply worker=abc remember to close with @deck-end on its own line @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	want := "remember to close with @deck-end on its own line"
	if blocks[0].Body != want {
		t.Errorf("body: got %q want %q", blocks[0].Body, want)
	}
}

func TestParseReplyBlocksMultilineBodyMentionsEndMarker(t *testing.T) {
	input := "@deck-reply worker=abc\nUse @deck-end at the end of replies.\nStand by.\n@deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	want := "Use @deck-end at the end of replies. | Stand by."
	if blocks[0].Body != want {
		t.Errorf("body: got %q want %q", blocks[0].Body, want)
	}
}

func TestParseReplyBlocksWrappedEndMarker(t *testing.T) {
	// Simulates tmux line-wrapping a single-line @deck-reply ... @deck-end:
	// the body continues on the next line and ends with @deck-end inline.
	input := "❯ @deck-reply worker=abc Good answer. Records set in Chicago.\n Stand by for your next task. @deck-end"
	blocks := state.ParseReplyBlocks(input)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	want := "Good answer. Records set in Chicago. | Stand by for your next task."
	if blocks[0].Body != want {
		t.Errorf("body: got %q want %q", blocks[0].Body, want)
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

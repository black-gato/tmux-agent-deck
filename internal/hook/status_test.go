package hook_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/hook"
)

func TestWriteReadStatus(t *testing.T) {
	dir := t.TempDir()

	if err := hook.WriteStatus(dir, "abc-123", "running", "claude-sess", "UserPromptSubmit"); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	got, ok := hook.ReadStatus(dir, "abc-123")
	if !ok {
		t.Fatal("ReadStatus: not found after write")
	}
	if got.Status != "running" || got.Event != "UserPromptSubmit" || got.SessionID != "claude-sess" {
		t.Errorf("ReadStatus = %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
}

func TestReadStatusMissing(t *testing.T) {
	if _, ok := hook.ReadStatus(t.TempDir(), "nope"); ok {
		t.Error("expected not-found for missing file")
	}
}

func TestReadStatusRejectsTraversal(t *testing.T) {
	if _, ok := hook.ReadStatus(t.TempDir(), "../escape"); ok {
		t.Error("expected traversal id to be rejected")
	}
}

func TestFresh(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	fresh := hook.HookStatus{UpdatedAt: now.Add(-time.Minute)}
	stale := hook.HookStatus{UpdatedAt: now.Add(-3 * time.Minute)}
	if !hook.Fresh(fresh, now) {
		t.Error("1m-old status should be fresh")
	}
	if hook.Fresh(stale, now) {
		t.Error("3m-old status should be stale")
	}
}

func TestDeckStatus(t *testing.T) {
	cases := map[string]string{"running": "running", "waiting": "waiting", "dead": "error"}
	for raw, want := range cases {
		hs := hook.HookStatus{Status: raw}
		if got := hs.DeckStatus(); got != want {
			t.Errorf("DeckStatus(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestListStatuses(t *testing.T) {
	dir := t.TempDir()
	_ = hook.WriteStatus(dir, "one", "running", "", "UserPromptSubmit")
	_ = hook.WriteStatus(dir, "two", "waiting", "", "Stop")
	all := hook.ListStatuses(dir)
	if len(all) != 2 || all["one"].Status != "running" || all["two"].Status != "waiting" {
		t.Errorf("ListStatuses = %+v", all)
	}
}

func TestListStatusesSkipsCorrupt(t *testing.T) {
	dir := t.TempDir()
	_ = hook.WriteStatus(dir, "valid", "running", "", "UserPromptSubmit")
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json}"), 0600); err != nil {
		t.Fatal(err)
	}
	all := hook.ListStatuses(dir)
	if len(all) != 1 || all["valid"].Status != "running" {
		t.Errorf("corrupt file should be skipped; got %+v", all)
	}
}

func TestWriteStatusRejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	err := hook.WriteStatus(dir, "../escape", "running", "", "UserPromptSubmit")
	if err != nil {
		t.Errorf("expected nil error (silent no-op), got %v", err)
	}
	if len(hook.ListStatuses(dir)) != 0 {
		t.Error("expected no file written for traversal instance id")
	}
}

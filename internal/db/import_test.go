package db_test

import (
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

func TestImportSessionDefaults(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["scratch-foo"] = ""
	fake.SessionInfos["scratch-foo"] = tmux.SessionInfo{Name: "scratch-foo", CurrentPath: "/tmp/work"}

	got, err := db.ImportSession(conn, fake, db.ImportRequest{TmuxName: "scratch-foo"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got.Title != "scratch-foo" {
		t.Errorf("title: %q", got.Title)
	}
	if got.GroupPath != "my-sessions" {
		t.Errorf("group: %q", got.GroupPath)
	}
	if got.ProjectPath != "/tmp/work" {
		t.Errorf("project: %q", got.ProjectPath)
	}
	if got.Tool != "claude" {
		t.Errorf("tool: %q", got.Tool)
	}
	if got.TmuxSession != "scratch-foo" {
		t.Errorf("tmux: %q", got.TmuxSession)
	}
	if got.Status != "unknown" {
		t.Errorf("status: %q", got.Status)
	}
	if got.ID == "" {
		t.Errorf("expected generated id")
	}

	roundtrip, err := db.GetSession(conn, got.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if roundtrip.TmuxSession != "scratch-foo" {
		t.Errorf("roundtrip tmux: %q", roundtrip.TmuxSession)
	}
}

func TestImportSessionRejectsDuplicate(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["dup"] = ""
	if _, err := db.ImportSession(conn, fake, db.ImportRequest{TmuxName: "dup"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := db.ImportSession(conn, fake, db.ImportRequest{TmuxName: "dup"})
	if err == nil || !strings.Contains(err.Error(), "already imported") {
		t.Errorf("expected already-imported error, got %v", err)
	}
}

func TestImportSessionRejectsMissing(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	fake := testutil.NewFakeTmuxClient()
	_, err := db.ImportSession(conn, fake, db.ImportRequest{TmuxName: "ghost"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestImportSessionInheritsGroupTool(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.CreateGroup(conn, db.Group{Path: "work", Name: "work", DefaultTool: "gemini"}); err != nil {
		t.Fatalf("group: %v", err)
	}
	fake := testutil.NewFakeTmuxClient()
	fake.Sessions["s"] = ""
	got, err := db.ImportSession(conn, fake, db.ImportRequest{TmuxName: "s", GroupPath: "work"})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if got.Tool != "gemini" {
		t.Errorf("tool: got %q want gemini", got.Tool)
	}
}

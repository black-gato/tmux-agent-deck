package db_test

import (
	"testing"
	"time"

	dbpkg "github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestSessionCreateAndGet(t *testing.T) {
	conn := testutil.OpenTestDB(t)

	s := dbpkg.Session{
		ID:          "abc-123",
		Title:       "my-app",
		GroupPath:   "my-sessions",
		TmuxSession: "tmux-abc-123",
		ProjectPath: "/home/user/projects/my-app",
		Tool:        "claude",
		Status:      "stopped",
		CreatedAt:   time.Now().Unix(),
	}
	if err := dbpkg.CreateSession(conn, s); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := dbpkg.GetSession(conn, "abc-123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "my-app" {
		t.Errorf("title: got %q want %q", got.Title, "my-app")
	}
	if got.ProjectPath != "/home/user/projects/my-app" {
		t.Errorf("project_path mismatch")
	}
}

func TestListSessionsByGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s2", Title: "b", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s3", Title: "c", GroupPath: "work", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	sessions, err := dbpkg.ListSessionsByGroup(conn, "my-sessions")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2, got %d", len(sessions))
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.UpdateSessionStatus(conn, "s1", "running"); err != nil {
		t.Fatal(err)
	}
	s, _ := dbpkg.GetSession(conn, "s1")
	if s.Status != "running" {
		t.Errorf("status: got %q want running", s.Status)
	}
}

func TestMoveSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.MoveSession(conn, "s1", "work"); err != nil {
		t.Fatal(err)
	}
	s, _ := dbpkg.GetSession(conn, "s1")
	if s.GroupPath != "work" {
		t.Errorf("group_path: got %q want work", s.GroupPath)
	}
}

func TestDeleteSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.DeleteSession(conn, "s1"); err != nil {
		t.Fatal(err)
	}
	_, err := dbpkg.GetSession(conn, "s1")
	if err == nil {
		t.Errorf("expected error after delete")
	}
}

func TestRenameSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "old-name", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.RenameSession(conn, "s1", "new-name"); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "new-name" {
		t.Errorf("title: got %q want new-name", s.Title)
	}
}

func TestUpdateSessionTmuxName(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.UpdateSessionTmuxName(conn, "s1", "tmux-session-abc"); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.TmuxSession != "tmux-session-abc" {
		t.Errorf("tmux_session: got %q want tmux-session-abc", s.TmuxSession)
	}
}

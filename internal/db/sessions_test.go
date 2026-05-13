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

func TestMoveSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s2", Title: "b", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.MoveSessions(conn, []string{"s1", "s2"}, "work"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s1", "s2"} {
		s, err := dbpkg.GetSession(conn, id)
		if err != nil {
			t.Fatal(err)
		}
		if s.GroupPath != "work" {
			t.Errorf("%s group_path: got %q want work", id, s.GroupPath)
		}
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

func TestDeleteSessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s1", Title: "a", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})
	dbpkg.CreateSession(conn, dbpkg.Session{ID: "s2", Title: "b", GroupPath: "my-sessions", ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now})

	if err := dbpkg.DeleteSessions(conn, []string{"s1", "s2"}); err != nil {
		t.Fatal(err)
	}
	sessions, err := dbpkg.ListSessions(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
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

func TestUpdateSessionNotes(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now,
	})

	if err := dbpkg.UpdateSessionNotes(conn, "s1", "check divergences first"); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Notes != "check divergences first" {
		t.Errorf("notes: got %q want %q", s.Notes, "check divergences first")
	}
}

func TestSessionNotesDefaultsToEmpty(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now,
	})
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Notes != "" {
		t.Errorf("notes should default to empty, got %q", s.Notes)
	}
}

func TestSessionArchiveDefaultsToFalse(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now,
	})
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Archived {
		t.Fatal("archived should default to false")
	}
	if s.Tags != "" {
		t.Fatalf("tags should default to empty, got %q", s.Tags)
	}
}

func TestSetSessionArchived(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	})

	if err := dbpkg.SetSessionArchived(conn, "s1", true); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Archived {
		t.Fatal("archived should be true after archiving")
	}
	if s.GroupPath != "archived" {
		t.Fatalf("group_path: got %q want archived", s.GroupPath)
	}
	if s.Status != "stopped" {
		t.Fatalf("status: got %q want stopped", s.Status)
	}
	if s.TmuxSession != "" {
		t.Fatalf("tmux_session: got %q want empty", s.TmuxSession)
	}
}

func TestUpdateSessionTags(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "a", GroupPath: "my-sessions",
		ProjectPath: "/p", Tool: "claude", Status: "stopped", CreatedAt: now,
	})

	if err := dbpkg.UpdateSessionTags(conn, "s1", "backend api"); err != nil {
		t.Fatal(err)
	}
	s, err := dbpkg.GetSession(conn, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Tags != "backend api" {
		t.Fatalf("tags: got %q want %q", s.Tags, "backend api")
	}
}

func TestGetGroupConductorSession(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work", ConductorSessionID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s1", Title: "lead", GroupPath: "work", TmuxSession: "ad-s1",
		ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.CreateSession(conn, dbpkg.Session{
		ID: "s2", Title: "worker", GroupPath: "work", TmuxSession: "ad-s2",
		ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	s, err := dbpkg.GetGroupConductorSession(conn, "work")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "s1" {
		t.Fatalf("conductor id: got %q want %q", s.ID, "s1")
	}
}

func TestListWaitingGroupChildren(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	if err := dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work", ConductorSessionID: "lead"}); err != nil {
		t.Fatal(err)
	}
	for _, session := range []dbpkg.Session{
		{
			ID: "lead", Title: "lead", GroupPath: "work", TmuxSession: "ad-lead",
			ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: now,
		},
		{
			ID: "worker-1", Title: "worker-1", GroupPath: "work", TmuxSession: "ad-worker-1",
			ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: now + 1, Notes: "needs review",
		},
		{
			ID: "worker-2", Title: "worker-2", GroupPath: "work", TmuxSession: "ad-worker-2",
			ProjectPath: "/p", Tool: "claude", Status: "waiting", CreatedAt: now + 2,
		},
		{
			ID: "running", Title: "running", GroupPath: "work", TmuxSession: "ad-running",
			ProjectPath: "/p", Tool: "claude", Status: "running", CreatedAt: now + 3,
		},
	} {
		if err := dbpkg.CreateSession(conn, session); err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := dbpkg.ListWaitingGroupChildren(conn, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 waiting children, got %d", len(sessions))
	}
	if sessions[0].ID != "worker-2" || sessions[1].ID != "worker-1" {
		t.Fatalf("unexpected sessions order/content: %#v", sessions)
	}
}

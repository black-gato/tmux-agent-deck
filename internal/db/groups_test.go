package db_test

import (
	"testing"

	dbpkg "github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestGroupCreateAndGet(t *testing.T) {
	conn := testutil.OpenTestDB(t)

	g := dbpkg.Group{
		Path:        "work/frontend",
		Name:        "frontend",
		DefaultPath: "/home/user/projects",
		DefaultTool: "claude",
		Expanded:    true,
	}
	if err := dbpkg.CreateGroup(conn, g); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := dbpkg.GetGroup(conn, "work/frontend")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "frontend" {
		t.Errorf("name: got %q want %q", got.Name, "frontend")
	}
	if got.DefaultTool != "claude" {
		t.Errorf("tool: got %q want %q", got.DefaultTool, "claude")
	}
}

func TestSetGroupConductor(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := dbpkg.SetGroupConductor(conn, "work", "session-1"); err != nil {
		t.Fatalf("set conductor: %v", err)
	}
	got, err := dbpkg.GetGroup(conn, "work")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ConductorSessionID != "session-1" {
		t.Fatalf("conductor_session_id: got %q want %q", got.ConductorSessionID, "session-1")
	}
}

func TestListGroups(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	// my-sessions already created by migration
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"})
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work/frontend", Name: "frontend"})

	groups, err := dbpkg.ListGroups(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) < 3 {
		t.Errorf("expected at least 3 groups, got %d", len(groups))
	}
}

func TestChildGroups(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"})
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work/frontend", Name: "frontend"})
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work/backend", Name: "backend"})
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "personal", Name: "personal"})

	children, err := dbpkg.ChildGroups(conn, "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestUpdateGroupExpanded(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work", Expanded: true})

	if err := dbpkg.SetGroupExpanded(conn, "work", false); err != nil {
		t.Fatal(err)
	}
	g, _ := dbpkg.GetGroup(conn, "work")
	if g.Expanded {
		t.Errorf("expected expanded=false")
	}
}

func TestDeleteGroup(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	dbpkg.CreateGroup(conn, dbpkg.Group{Path: "work", Name: "work"})
	if err := dbpkg.DeleteGroup(conn, "work"); err != nil {
		t.Fatal(err)
	}
	_, err := dbpkg.GetGroup(conn, "work")
	if err == nil {
		t.Errorf("expected error after delete")
	}
}

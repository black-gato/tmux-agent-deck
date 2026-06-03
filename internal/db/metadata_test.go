package db_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestGetMetaConductorID_EmptyByDefault(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	id, err := db.GetMetaConductorID(conn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty id, got %q", id)
	}
}

func TestSetAndGetMetaConductorID(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.SetMetaConductorID(conn, "session-abc"); err != nil {
		t.Fatalf("set: %v", err)
	}
	id, err := db.GetMetaConductorID(conn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if id != "session-abc" {
		t.Errorf("got %q want %q", id, "session-abc")
	}
}

func TestSetMetaConductorID_ClearWithEmpty(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	if err := db.SetMetaConductorID(conn, "session-xyz"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := db.SetMetaConductorID(conn, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	id, err := db.GetMetaConductorID(conn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty after clear, got %q", id)
	}
}

func TestGetMetaConductorSession_NotSet(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	_, err := db.GetMetaConductorSession(conn)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestGetMetaConductorSession_Set(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	now := time.Now().Unix()
	s := db.Session{
		ID:          "meta-001",
		Title:       "meta conductor",
		GroupPath:   "my-sessions",
		TmuxSession: "meta-tmux",
		ProjectPath: "/tmp",
		Tool:        "claude",
		Status:      "running",
		CreatedAt:   now,
		LastActive:  now,
	}
	if err := db.CreateSession(conn, s); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := db.SetMetaConductorID(conn, "meta-001"); err != nil {
		t.Fatalf("set id: %v", err)
	}
	got, err := db.GetMetaConductorSession(conn)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "meta-001" {
		t.Errorf("got id %q want %q", got.ID, "meta-001")
	}
}

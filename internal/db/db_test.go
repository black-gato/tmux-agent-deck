package db_test

import (
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/testutil"
)

func TestOpenCreatesSchema(t *testing.T) {
	conn := testutil.OpenTestDB(t)

	for _, table := range []string{"groups", "sessions", "metadata"} {
		var n int
		err := conn.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&n)
		if err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %q not created", table)
		}
	}
}

func TestOpenCreatesMySessions(t *testing.T) {
	conn := testutil.OpenTestDB(t)
	var n int
	conn.QueryRow(`SELECT count(*) FROM groups WHERE path='my-sessions'`).Scan(&n)
	if n != 1 {
		t.Errorf("my-sessions group not seeded")
	}
}

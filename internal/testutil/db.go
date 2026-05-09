package testutil

import (
	"database/sql"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

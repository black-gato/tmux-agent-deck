package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`
		PRAGMA journal_mode=WAL;
		PRAGMA busy_timeout=5000;
	`); err != nil {
		return fmt.Errorf("pragmas: %w", err)
	}
	_, err := conn.Exec(`
		CREATE TABLE IF NOT EXISTS metadata (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS groups (
			path         TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			default_path TEXT NOT NULL DEFAULT '',
			default_tool TEXT NOT NULL DEFAULT 'claude',
			expanded     INTEGER NOT NULL DEFAULT 1,
			sort_order   INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS sessions (
			id           TEXT PRIMARY KEY,
			title        TEXT NOT NULL,
			group_path   TEXT NOT NULL DEFAULT 'my-sessions',
			tmux_session TEXT NOT NULL DEFAULT '',
			project_path TEXT NOT NULL,
			tool         TEXT NOT NULL DEFAULT 'claude',
			status       TEXT NOT NULL DEFAULT 'stopped',
			created_at   INTEGER NOT NULL,
			last_active  INTEGER NOT NULL DEFAULT 0
		);
		INSERT OR IGNORE INTO metadata (key, value) VALUES ('schema_version', '1');
		INSERT OR IGNORE INTO groups (path, name) VALUES ('my-sessions', 'my-sessions');
	`)
	return err
}

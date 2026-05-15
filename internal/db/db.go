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
			conductor_session_id TEXT NOT NULL DEFAULT '',
			expanded     INTEGER NOT NULL DEFAULT 1,
			sort_order   INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS sessions (
			id             TEXT PRIMARY KEY,
			title          TEXT NOT NULL,
			group_path     TEXT NOT NULL DEFAULT 'my-sessions',
			tmux_session   TEXT NOT NULL DEFAULT '',
			project_path   TEXT NOT NULL,
			tool           TEXT NOT NULL DEFAULT 'claude',
			status         TEXT NOT NULL DEFAULT 'stopped',
			created_at     INTEGER NOT NULL,
			last_active    INTEGER NOT NULL DEFAULT 0,
			notes          TEXT NOT NULL DEFAULT '',
			archived       INTEGER NOT NULL DEFAULT 0,
			tags           TEXT NOT NULL DEFAULT '',
			startup_script TEXT NOT NULL DEFAULT ''
		);
		INSERT OR IGNORE INTO metadata (key, value) VALUES ('schema_version', '5');
		INSERT OR IGNORE INTO groups (path, name) VALUES ('my-sessions', 'my-sessions');
		INSERT OR IGNORE INTO groups (path, name) VALUES ('archived', 'archived');
	`)
	if err != nil {
		return err
	}
	var version string
	if err := conn.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&version); err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}
	if version == "1" {
		if _, err := conn.Exec(`ALTER TABLE sessions ADD COLUMN notes TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if _, err := conn.Exec(`UPDATE metadata SET value = '2' WHERE key = 'schema_version'`); err != nil {
			return err
		}
		version = "2"
	}
	if version == "2" {
		if _, err := conn.Exec(`ALTER TABLE sessions ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := conn.Exec(`ALTER TABLE sessions ADD COLUMN tags TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if _, err := conn.Exec(`INSERT OR IGNORE INTO groups (path, name) VALUES ('archived', 'archived')`); err != nil {
			return err
		}
		if _, err := conn.Exec(`UPDATE metadata SET value = '3' WHERE key = 'schema_version'`); err != nil {
			return err
		}
		version = "3"
	}
	if version == "3" {
		if _, err := conn.Exec(`ALTER TABLE groups ADD COLUMN conductor_session_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if _, err := conn.Exec(`UPDATE metadata SET value = '4' WHERE key = 'schema_version'`); err != nil {
			return err
		}
		version = "4"
	}
	if version == "4" {
		if _, err := conn.Exec(`ALTER TABLE sessions ADD COLUMN startup_script TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if _, err := conn.Exec(`UPDATE metadata SET value = '5' WHERE key = 'schema_version'`); err != nil {
			return err
		}
	}
	return nil
}

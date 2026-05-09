package db

import (
	"database/sql"
	"fmt"
)

type Session struct {
	ID          string
	Title       string
	GroupPath   string
	TmuxSession string
	ProjectPath string
	Tool        string
	Status      string
	CreatedAt   int64
	LastActive  int64
}

func CreateSession(conn *sql.DB, s Session) error {
	_, err := conn.Exec(
		`INSERT INTO sessions (id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.GroupPath, s.TmuxSession, s.ProjectPath, s.Tool, s.Status, s.CreatedAt, s.LastActive,
	)
	return err
}

func GetSession(conn *sql.DB, id string) (Session, error) {
	var s Session
	err := conn.QueryRow(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active
		 FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive)
	if err != nil {
		return Session{}, fmt.Errorf("get session %q: %w", id, err)
	}
	return s, nil
}

func GetSessionByTitle(conn *sql.DB, title string) (Session, error) {
	var s Session
	err := conn.QueryRow(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active
		 FROM sessions WHERE title = ? LIMIT 1`, title,
	).Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive)
	if err != nil {
		return Session{}, fmt.Errorf("get session by title %q: %w", title, err)
	}
	return s, nil
}

func ListSessions(conn *sql.DB) ([]Session, error) {
	rows, err := conn.Query(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active
		 FROM sessions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func ListSessionsByGroup(conn *sql.DB, groupPath string) ([]Session, error) {
	rows, err := conn.Query(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active
		 FROM sessions WHERE group_path = ? ORDER BY created_at DESC`, groupPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func UpdateSessionStatus(conn *sql.DB, id, status string) error {
	_, err := conn.Exec(
		`UPDATE sessions SET status = ?, last_active = strftime('%s','now') WHERE id = ?`,
		status, id,
	)
	return err
}

func UpdateSessionTmuxName(conn *sql.DB, id, tmuxSession string) error {
	_, err := conn.Exec(`UPDATE sessions SET tmux_session = ? WHERE id = ?`, tmuxSession, id)
	return err
}

func RenameSession(conn *sql.DB, id, newTitle string) error {
	_, err := conn.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, newTitle, id)
	return err
}

func MoveSession(conn *sql.DB, id, groupPath string) error {
	_, err := conn.Exec(`UPDATE sessions SET group_path = ? WHERE id = ?`, groupPath, id)
	return err
}

func DeleteSession(conn *sql.DB, id string) error {
	_, err := conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	sessions := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

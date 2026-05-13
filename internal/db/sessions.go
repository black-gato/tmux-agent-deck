package db

import (
	"database/sql"
	"fmt"
	"strings"
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
	Notes       string
	Archived    bool
	Tags        string
}

func CreateSession(conn *sql.DB, s Session) error {
	archived := 0
	if s.Archived {
		archived = 1
	}
	_, err := conn.Exec(
		`INSERT INTO sessions (id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active, notes, archived, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Title, s.GroupPath, s.TmuxSession, s.ProjectPath, s.Tool, s.Status, s.CreatedAt, s.LastActive, s.Notes, archived, s.Tags,
	)
	return err
}

func GetSession(conn *sql.DB, id string) (Session, error) {
	var s Session
	var archived int
	err := conn.QueryRow(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active, notes, archived, tags
		 FROM sessions WHERE id = ?`, id,
	).Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive, &s.Notes, &archived, &s.Tags)
	if err != nil {
		return Session{}, fmt.Errorf("get session %q: %w", id, err)
	}
	s.Archived = archived == 1
	return s, nil
}

func GetSessionByTitle(conn *sql.DB, title string) (Session, error) {
	var s Session
	var archived int
	err := conn.QueryRow(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active, notes, archived, tags
		 FROM sessions WHERE title = ? LIMIT 1`, title,
	).Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive, &s.Notes, &archived, &s.Tags)
	if err != nil {
		return Session{}, fmt.Errorf("get session by title %q: %w", title, err)
	}
	s.Archived = archived == 1
	return s, nil
}

func ListSessions(conn *sql.DB) ([]Session, error) {
	rows, err := conn.Query(
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active, notes, archived, tags
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
		`SELECT id, title, group_path, tmux_session, project_path, tool, status, created_at, last_active, notes, archived, tags
		 FROM sessions WHERE group_path = ? ORDER BY created_at DESC`, groupPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func UpdateSessionStatus(conn *sql.DB, id, status string) error {
	res, err := conn.Exec(
		`UPDATE sessions SET status = ?, last_active = strftime('%s','now') WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update status %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func UpdateSessionTmuxName(conn *sql.DB, id, tmuxSession string) error {
	res, err := conn.Exec(`UPDATE sessions SET tmux_session = ? WHERE id = ?`, tmuxSession, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update tmux name %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func UpdateSessionNotes(conn *sql.DB, id, notes string) error {
	res, err := conn.Exec(`UPDATE sessions SET notes = ? WHERE id = ?`, notes, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("update notes %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func RenameSession(conn *sql.DB, id, newTitle string) error {
	res, err := conn.Exec(`UPDATE sessions SET title = ? WHERE id = ?`, newTitle, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("rename session %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func MoveSession(conn *sql.DB, id, groupPath string) error {
	res, err := conn.Exec(`UPDATE sessions SET group_path = ? WHERE id = ?`, groupPath, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("move session %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func MoveSessions(conn *sql.DB, ids []string, groupPath string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	for _, id := range ids {
		res, err := tx.Exec(`UPDATE sessions SET group_path = ? WHERE id = ?`, groupPath, id)
		if err != nil {
			tx.Rollback()
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			tx.Rollback()
			return fmt.Errorf("move session %q: %w", id, sql.ErrNoRows)
		}
	}
	return tx.Commit()
}

func DeleteSession(conn *sql.DB, id string) error {
	_, err := conn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func DeleteSessions(conn *sql.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	_, err := conn.Exec(
		fmt.Sprintf(`DELETE FROM sessions WHERE id IN (%s)`, strings.Join(placeholders, ",")),
		args...,
	)
	return err
}

func SetSessionArchived(conn *sql.DB, id string, archived bool) error {
	groupPath := "my-sessions"
	archivedValue := 0
	if archived {
		groupPath = "archived"
		archivedValue = 1
	}
	res, err := conn.Exec(
		`UPDATE sessions SET group_path = ?, status = 'stopped', tmux_session = '', archived = ? WHERE id = ?`,
		groupPath, archivedValue, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("archive session %q: %w", id, sql.ErrNoRows)
	}
	return nil
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	sessions := []Session{}
	for rows.Next() {
		var s Session
		var archived int
		if err := rows.Scan(&s.ID, &s.Title, &s.GroupPath, &s.TmuxSession, &s.ProjectPath, &s.Tool, &s.Status, &s.CreatedAt, &s.LastActive, &s.Notes, &archived, &s.Tags); err != nil {
			return nil, err
		}
		s.Archived = archived == 1
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

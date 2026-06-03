package db

import (
	"database/sql"
	"errors"
	"fmt"
)

func GetMetaConductorID(conn *sql.DB) (string, error) {
	var id string
	err := conn.QueryRow(`SELECT value FROM metadata WHERE key = 'meta_conductor_session_id'`).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("get meta conductor id: %w", err)
	}
	return id, nil
}

func SetMetaConductorID(conn *sql.DB, sessionID string) error {
	_, err := conn.Exec(`UPDATE metadata SET value = ? WHERE key = 'meta_conductor_session_id'`, sessionID)
	return err
}

func GetMetaConductorSession(conn *sql.DB) (Session, error) {
	id, err := GetMetaConductorID(conn)
	if err != nil {
		return Session{}, err
	}
	if id == "" {
		return Session{}, fmt.Errorf("get meta conductor session: %w", sql.ErrNoRows)
	}
	s, err := GetSession(conn, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, fmt.Errorf("get meta conductor session: %w", sql.ErrNoRows)
		}
		return Session{}, err
	}
	return s, nil
}

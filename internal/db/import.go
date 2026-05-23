package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/google/uuid"
)

// ImportInspector is the subset of tmux.ClientIface needed to import sessions.
type ImportInspector interface {
	ListSessions() ([]string, error)
	SessionInfo(name string) (tmux.SessionInfo, error)
}

type ImportRequest struct {
	TmuxName  string
	Title     string
	GroupPath string
}

func ImportSession(conn *sql.DB, tc ImportInspector, req ImportRequest) (Session, error) {
	if req.TmuxName == "" {
		return Session{}, errors.New("tmux name required")
	}

	names, err := tc.ListSessions()
	if err != nil {
		return Session{}, fmt.Errorf("list tmux sessions: %w", err)
	}
	found := false
	for _, n := range names {
		if n == req.TmuxName {
			found = true
			break
		}
	}
	if !found {
		return Session{}, fmt.Errorf("tmux session %q not found", req.TmuxName)
	}

	if existing, err := GetSessionByTmuxName(conn, req.TmuxName); err == nil {
		return Session{}, fmt.Errorf("tmux session %q already imported (deck id %s)", req.TmuxName, existing.ID)
	}

	title := req.Title
	if title == "" {
		title = req.TmuxName
	}
	groupPath := req.GroupPath
	if groupPath == "" {
		groupPath = "my-sessions"
	}

	tool := "claude"
	if g, err := GetGroup(conn, groupPath); err == nil && g.DefaultTool != "" {
		tool = g.DefaultTool
	}

	info, err := tc.SessionInfo(req.TmuxName)
	if err != nil {
		return Session{}, fmt.Errorf("session info %q: %w", req.TmuxName, err)
	}

	s := Session{
		ID:          uuid.New().String(),
		Title:       title,
		GroupPath:   groupPath,
		TmuxSession: req.TmuxName,
		ProjectPath: info.CurrentPath,
		Tool:        tool,
		Status:      "unknown",
		CreatedAt:   time.Now().Unix(),
	}
	if !info.Activity.IsZero() {
		s.LastActive = info.Activity.Unix()
	}
	if err := CreateSession(conn, s); err != nil {
		return Session{}, fmt.Errorf("create imported session: %w", err)
	}
	return s, nil
}

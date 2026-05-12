//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

type QAFixture struct {
	RootDir  string
	FrontDir string
	BackDir  string

	Root    db.Session
	Front   db.Session
	Back    db.Session
	Stopped db.Session
}

func (e *TestEnv) CreateQAFixture(t *testing.T) QAFixture {
	t.Helper()
	conn := e.OpenDB(t)

	rootDir := filepath.Join(t.TempDir(), "qa")
	frontDir := filepath.Join(rootDir, "frontend")
	backDir := filepath.Join(rootDir, "backend")
	for _, dir := range []string{rootDir, frontDir, backDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create fixture dir %s: %v", dir, err)
		}
	}

	groups := []db.Group{
		{Path: "qa", Name: "qa", DefaultTool: "bash", Expanded: true},
		{Path: "qa/frontend", Name: "frontend", DefaultTool: "bash", Expanded: true},
		{Path: "qa/backend", Name: "backend", DefaultTool: "bash", Expanded: true},
	}
	for _, g := range groups {
		if err := db.CreateGroup(conn, g); err != nil {
			t.Fatalf("create group %s: %v", g.Path, err)
		}
	}

	fx := QAFixture{
		RootDir:  rootDir,
		FrontDir: frontDir,
		BackDir:  backDir,
		Root:     e.fixtureSession("root", "qa", rootDir, "running", true),
		Front:    e.fixtureSession("front", "qa/frontend", frontDir, "running", true),
		Back:     e.fixtureSession("back", "qa/backend", backDir, "running", true),
		Stopped:  e.fixtureSession("stopped", "qa", rootDir, "stopped", false),
	}
	for _, s := range []db.Session{fx.Root, fx.Front, fx.Back, fx.Stopped} {
		if err := db.CreateSession(conn, s); err != nil {
			t.Fatalf("create session %s: %v", s.Title, err)
		}
		if s.TmuxSession != "" {
			NewTmuxSession(t, s.TmuxSession, s.ProjectPath, "bash")
			SendTmuxKeys(t, s.TmuxSession, "printf '"+s.Title+" ready\\n'", "Enter")
		}
	}
	return fx
}

func (e *TestEnv) fixtureSession(kind, group, project, status string, running bool) db.Session {
	title := e.Prefix + "-" + kind
	tmuxName := ""
	if running {
		tmuxName = title
	}
	return db.Session{
		ID:          fmt.Sprintf("%s-%s-id-000000000000", e.Prefix, kind),
		Title:       title,
		GroupPath:   group,
		TmuxSession: tmuxName,
		ProjectPath: project,
		Tool:        "bash",
		Status:      status,
		CreatedAt:   time.Now().Unix() + int64(len(kind)),
	}
}

//go:build e2e

package e2e

import (
	"bytes"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

const defaultTimeout = 8 * time.Second

type Suite struct {
	RootDir string
	BinPath string
	TempDir string
	Prefix  string
}

type TestEnv struct {
	Suite  *Suite
	DBPath string
	Prefix string
}

func (s *Suite) NewTest(t *testing.T) *TestEnv {
	t.Helper()
	name := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(strings.ToLower(t.Name()), "-")
	name = strings.Trim(name, "-")
	if len(name) > 42 {
		name = name[:42]
	}
	env := &TestEnv{
		Suite:  s,
		DBPath: filepath.Join(t.TempDir(), "state.db"),
		Prefix: s.Prefix + "-" + name,
	}
	t.Cleanup(func() { env.CleanupNow(t) })
	return env
}

func (e *TestEnv) Env() []string {
	return append(os.Environ(),
		"AGENT_DECK_DB="+e.DBPath,
		"TERM=xterm-256color",
	)
}

func (e *TestEnv) OpenDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(e.DBPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (e *TestEnv) RunCLI(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(e.Suite.BinPath, args...)
	cmd.Env = e.Env()
	cmd.Dir = e.Suite.RootDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v failed: %v\nstdout:\n%s\nstderr:\n%s", e.Suite.BinPath, args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func (e *TestEnv) StartTUI(t *testing.T, width, height int) *TUIDriver {
	t.Helper()
	driver := StartTUI(t, e.Suite.BinPath, e.Env(), width, height)
	t.Cleanup(func() { driver.Close(t) })
	return driver
}

func (e *TestEnv) CleanupNow(t *testing.T) {
	t.Helper()
	e.Suite.KillSessionsByPrefix(e.Prefix)
	if e.DBPath != "" {
		conn, err := db.Open(e.DBPath)
		if err == nil {
			sessions, _ := db.ListSessions(conn)
			for _, s := range sessions {
				if s.TmuxSession != "" {
					_ = KillTmuxSession(s.TmuxSession)
				}
				if len(s.ID) >= 8 {
					_ = KillTmuxSession("ad-" + s.ID[:8])
				}
			}
			_ = conn.Close()
		}
	}
}

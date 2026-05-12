//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func TestStatusPollingLifecycle(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectRoot(t, d, fx)

	waitForStatus(t, env, fx.Root.Title, "waiting", 6*time.Second)

	SendTmuxKeys(t, fx.Root.TmuxSession, "sleep 35", "Enter")
	waitForStatus(t, env, fx.Root.Title, "running", 6*time.Second)
	waitForStatus(t, env, fx.Root.Title, "idle", 40*time.Second)

	if err := KillTmuxSession(fx.Root.TmuxSession); err != nil {
		t.Fatalf("kill root tmux session: %v", err)
	}
	waitForStatus(t, env, fx.Root.Title, "error", 6*time.Second)
	d.AssertStillRunning(t)
}

func waitForStatus(t *testing.T, env *TestEnv, title, want string, timeout time.Duration) {
	t.Helper()
	AssertEventually(t, timeout, func() bool {
		conn, err := db.Open(env.DBPath)
		if err != nil {
			return false
		}
		defer conn.Close()
		s, err := db.GetSessionByTitle(conn, title)
		return err == nil && s.Status == want
	}, func() string {
		conn, err := db.Open(env.DBPath)
		if err != nil {
			return fmt.Sprintf("open db while waiting for %q status %q: %v", title, want, err)
		}
		defer conn.Close()
		s, err := db.GetSessionByTitle(conn, title)
		if err != nil {
			return fmt.Sprintf("session %q status did not become %q: %v", title, want, err)
		}
		return fmt.Sprintf("session %q status=%q, want %q", title, s.Status, want)
	})
}

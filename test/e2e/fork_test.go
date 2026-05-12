//go:build e2e

package e2e

import (
	"fmt"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func TestForkSessionCreatesStoppedCopy(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectRoot(t, d, fx)

	forkTitle := env.Prefix + "-forked"
	d.SendKeys(t, "f")
	d.Send(t, forkTitle)
	d.SendKeys(t, "\r")

	AssertEventually(t, defaultTimeout, func() bool {
		conn := env.OpenDB(t)
		_, err := db.GetSessionByTitle(conn, forkTitle)
		return err == nil
	}, func() string {
		return "forked session did not appear in DB"
	})

	AssertSession(t, env, forkTitle, func(s db.Session) error {
		if err := expectEqual("group_path", s.GroupPath, fx.Root.GroupPath); err != nil {
			return err
		}
		if err := expectEqual("project_path", s.ProjectPath, fx.Root.ProjectPath); err != nil {
			return err
		}
		if err := expectEqual("tool", s.Tool, fx.Root.Tool); err != nil {
			return err
		}
		if err := expectEqual("status", s.Status, "stopped"); err != nil {
			return err
		}
		if s.TmuxSession != "" {
			return fmt.Errorf("tmux_session=%q, want empty", s.TmuxSession)
		}
		return nil
	})
	AssertTmuxSessionExists(t, forkTitle, false)
}

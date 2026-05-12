//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"

	"github.com/black-gato/tmux-agent-deck/internal/db"
)

func TestAttachStoppedSessionStartsTmux(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectStopped(t, d, fx)

	d.SendKeys(t, "\r")
	AssertEventually(t, defaultTimeout, func() bool {
		s := AssertSession(t, env, fx.Stopped.Title, nil)
		return s.TmuxSession != "" && TmuxSessionExists(s.TmuxSession)
	}, func() string {
		return "stopped session did not get a tmux session after attach"
	})
}

func TestAttachRunningSessionDoesNotCreateDuplicate(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectRoot(t, d, fx)

	d.SendKeys(t, "\r")
	AssertEventually(t, defaultTimeout, func() bool {
		return TmuxSessionExists(fx.Root.TmuxSession)
	}, func() string {
		return fmt.Sprintf("running tmux session %q disappeared", fx.Root.TmuxSession)
	})

	AssertSession(t, env, fx.Root.Title, func(s db.Session) error {
		if s.TmuxSession != fx.Root.TmuxSession {
			return fmt.Errorf("tmux_session=%q, want %q", s.TmuxSession, fx.Root.TmuxSession)
		}
		if strings.HasPrefix(s.TmuxSession, "ad-") {
			return fmt.Errorf("created duplicate ad-* session %q", s.TmuxSession)
		}
		return nil
	})
}

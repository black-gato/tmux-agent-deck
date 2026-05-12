//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestSendToPane(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectRoot(t, d, fx)

	token := "send-pane-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "x")
	d.Send(t, token)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), token)
	d.AssertStillRunning(t)

	SendTmuxKeys(t, fx.Root.TmuxSession, "Enter")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), "command not found")
	SendTmuxKeys(t, fx.Root.TmuxSession, "sleep 20", "Enter")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), "sleep 20")
	d.SendKeys(t, "x", "\x03", "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), "^C")
	d.AssertStillRunning(t)
}

func TestSendToStoppedSessionIsNoop(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectStopped(t, d, fx)

	d.SendKeys(t, "x")
	d.Send(t, "no-target")
	d.SendKeys(t, "\r")
	time.Sleep(250 * time.Millisecond)
	d.AssertStillRunning(t)
	if fx.Stopped.TmuxSession != "" {
		t.Fatalf("stopped fixture unexpectedly had tmux session %q", fx.Stopped.TmuxSession)
	}
}

//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestBroadcastDirectGroup(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 130, 36)
	selectQAGroup(t, d, fx)

	token := "broadcast-direct-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "b")
	d.Send(t, token)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), token)
	WaitForNoPaneText(t, paneTarget(fx.Front.TmuxSession, 0), token, 750*time.Millisecond)
	WaitForNoPaneText(t, paneTarget(fx.Back.TmuxSession, 0), token, 750*time.Millisecond)
}

func TestBroadcastSubGroups(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 130, 36)
	selectQAGroup(t, d, fx)

	token := "broadcast-sub-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "b", "\t")
	d.Send(t, token)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), token)
	WaitForPaneText(t, paneTarget(fx.Front.TmuxSession, 0), token)
	WaitForPaneText(t, paneTarget(fx.Back.TmuxSession, 0), token)
	if fx.Stopped.TmuxSession != "" {
		t.Fatalf("stopped fixture unexpectedly had tmux session %q", fx.Stopped.TmuxSession)
	}
}

func TestBroadcastFromSessionUsesSessionGroup(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 130, 36)
	selectFrontSession(t, d, fx)

	token := "broadcast-row-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "b")
	d.Send(t, token)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Front.TmuxSession, 0), token)
	WaitForNoPaneText(t, paneTarget(fx.Root.TmuxSession, 0), token, 750*time.Millisecond)
	WaitForNoPaneText(t, paneTarget(fx.Back.TmuxSession, 0), token, 750*time.Millisecond)
}

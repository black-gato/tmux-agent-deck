//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestPaneTargeting(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	SplitPane(t, fx.Root.TmuxSession)

	d := env.StartTUI(t, 140, 38)
	selectRoot(t, d, fx)
	d.WaitForText(t, "[0] bash")
	d.WaitForText(t, "[1] bash")

	tokenPane1 := "pane-one-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "\t", "x")
	d.Send(t, tokenPane1)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 1), tokenPane1)
	WaitForNoPaneText(t, paneTarget(fx.Root.TmuxSession, 0), tokenPane1, 750*time.Millisecond)

	time.Sleep(1200 * time.Millisecond)
	tokenPane0 := "pane-zero-" + strings.ReplaceAll(t.Name(), "/", "-")
	d.SendKeys(t, "x")
	d.Send(t, tokenPane0)
	d.SendKeys(t, "\r")
	WaitForPaneText(t, paneTarget(fx.Root.TmuxSession, 0), tokenPane0)
	WaitForNoPaneText(t, paneTarget(fx.Root.TmuxSession, 1), tokenPane0, 750*time.Millisecond)
}

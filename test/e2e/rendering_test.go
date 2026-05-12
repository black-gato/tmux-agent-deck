//go:build e2e

package e2e

import "testing"

func TestRenderingAtNarrowAndWideSizes(t *testing.T) {
	env := suite.NewTest(t)
	env.CreateQAFixture(t)

	narrow := env.StartTUI(t, 72, 24)
	narrow.WaitForText(t, "Agent Deck")
	narrow.WaitForText(t, "SESSIONS")
	narrow.WaitForText(t, "Enter Attach")
	narrow.Close(t)

	wide := env.StartTUI(t, 150, 42)
	wide.WaitForText(t, "Agent Deck")
	wide.WaitForText(t, "SESSIONS")
	wide.WaitForText(t, "Enter Attach")
}

func TestDialogsEscapeAndResize(t *testing.T) {
	env := suite.NewTest(t)
	fx := env.CreateQAFixture(t)
	d := env.StartTUI(t, 120, 36)
	selectRoot(t, d, fx)

	d.SendKeys(t, "x")
	d.WaitForText(t, "Send:")
	d.SendKeys(t, "\x1b")
	d.AssertStillRunning(t)

	d.SendKeys(t, "f")
	d.WaitForText(t, "Fork title:")
	d.SendKeys(t, "\x1b")
	d.AssertStillRunning(t)

	d.SendKeys(t, "b")
	d.WaitForText(t, "Broadcast")
	d.SendKeys(t, "\x1b")
	d.AssertStillRunning(t)

	d.Resize(t, 90, 28)
	d.WaitForText(t, "Agent Deck")
	d.WaitForText(t, "Enter Attach")
	d.AssertStillRunning(t)
}

//go:build e2e

package e2e

import "testing"

func selectStopped(t *testing.T, d *TUIDriver, fx QAFixture) {
	t.Helper()
	_ = fx
	d.WaitForText(t, "qa")
	d.SendKeys(t, "j", "j")
}

func selectRoot(t *testing.T, d *TUIDriver, fx QAFixture) {
	t.Helper()
	_ = fx
	d.WaitForText(t, "qa")
	d.SendKeys(t, "j", "j", "j")
}

func selectQAGroup(t *testing.T, d *TUIDriver, fx QAFixture) {
	t.Helper()
	_ = fx
	d.WaitForText(t, "qa")
	d.SendKeys(t, "j")
}

func selectFrontGroup(t *testing.T, d *TUIDriver, fx QAFixture) {
	t.Helper()
	_ = fx
	d.WaitForText(t, "qa")
	d.SendKeys(t, "j", "j", "j", "j", "j", "j")
}

func selectFrontSession(t *testing.T, d *TUIDriver, fx QAFixture) {
	t.Helper()
	_ = fx
	d.WaitForText(t, "qa")
	d.SendKeys(t, "j", "j", "j", "j", "j", "j", "j")
}

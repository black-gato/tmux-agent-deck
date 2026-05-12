//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%w: %s", err, msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func TmuxSessionExists(name string) bool {
	_, err := tmux("has-session", "-t", name)
	return err == nil
}

func AssertTmuxSessionExists(t *testing.T, name string, want bool) {
	t.Helper()
	got := TmuxSessionExists(name)
	if got != want {
		t.Fatalf("tmux session %q exists=%v, want %v", name, got, want)
	}
}

func KillTmuxSession(name string) error {
	_, err := tmux("kill-session", "-t", name)
	return err
}

func (s *Suite) KillSessionsByPrefix(prefix string) {
	out, err := tmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return
	}
	for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(name, prefix) {
			_ = KillTmuxSession(name)
		}
	}
}

func NewTmuxSession(t *testing.T, name, dir, command string) {
	t.Helper()
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	if command != "" {
		args = append(args, command)
	}
	if _, err := tmux(args...); err != nil {
		t.Fatalf("new tmux session %q: %v", name, err)
	}
}

func SplitPane(t *testing.T, session string) {
	t.Helper()
	if _, err := tmux("split-window", "-t", session, "-h", "bash"); err != nil {
		t.Fatalf("split pane for %q: %v", session, err)
	}
}

func SendTmuxKeys(t *testing.T, target string, keys ...string) {
	t.Helper()
	args := append([]string{"send-keys", "-t", target}, keys...)
	if _, err := tmux(args...); err != nil {
		t.Fatalf("send tmux keys to %q: %v", target, err)
	}
}

func CapturePane(t *testing.T, target string) string {
	t.Helper()
	out, err := tmux("capture-pane", "-t", target, "-p")
	if err != nil {
		t.Fatalf("capture pane %q: %v", target, err)
	}
	return out
}

func WaitForPaneText(t *testing.T, target, text string) {
	t.Helper()
	AssertEventually(t, defaultTimeout, func() bool {
		return strings.Contains(CapturePane(t, target), text)
	}, func() string {
		return fmt.Sprintf("pane %s did not contain %q; output:\n%s", target, text, CapturePane(t, target))
	})
}

func WaitForNoPaneText(t *testing.T, target, text string, d timeDuration) {
	t.Helper()
	deadline := now().Add(d)
	for now().Before(deadline) {
		if strings.Contains(CapturePane(t, target), text) {
			t.Fatalf("pane %s unexpectedly contained %q:\n%s", target, text, CapturePane(t, target))
		}
		sleep(100)
	}
}

func paneTarget(session string, idx int) string {
	return fmt.Sprintf("%s:0.%d", session, idx)
}

func isNoSessionErr(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "can't find session") || errors.Is(err, exec.ErrNotFound))
}

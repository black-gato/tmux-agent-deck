package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) NewSession(name, startDir, command string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", startDir}
	if command != "" {
		args = append(args, command)
	}
	return runCmd("tmux", args...)
}

func (c *Client) AttachSession(name string) error {
	return runCmd("tmux", "attach-session", "-t", name)
}

func (c *Client) KillSession(name string) error {
	return runCmd("tmux", "kill-session", "-t", name)
}

func (c *Client) SessionExists(name string) (bool, error) {
	err := runCmd("tmux", "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func (c *Client) CapturePaneOutput(name string) (string, error) {
	out, err := cmdOutput("tmux", "capture-pane", "-t", name, "-p")
	if err != nil {
		return "", fmt.Errorf("capture-pane %q: %w", name, err)
	}
	return string(out), nil
}

func (c *Client) ListSessions() ([]string, error) {
	out, err := cmdOutput("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// tmux exits 1 when there are no sessions
			return []string{}, nil
		}
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func runCmd(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func cmdOutput(name string, args ...string) ([]byte, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

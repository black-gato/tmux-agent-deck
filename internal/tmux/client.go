package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ClientIface is the full tmux capability surface used by the app.
type ClientIface interface {
	NewSession(name, startDir, command string) error
	AttachSession(name string) error
	KillSession(name string) error
	SessionExists(name string) (bool, error)
	CapturePaneOutput(name string) (string, error)
	ListSessions() ([]string, error)
}

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
	if os.Getenv("TMUX") != "" {
		return runInteractive("tmux", "switch-client", "-t", name)
	}
	return runInteractive("tmux", "attach-session", "-t", name)
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

// ParseBindingCommand extracts the command from a tmux list-keys output line.
// Input format: "bind-key [-r|-rT] -T prefix q <command>"
// Returns "" if the input is empty or unparseable.
func ParseBindingCommand(listKeysOutput string) string {
	line := strings.TrimSpace(listKeysOutput)
	if line == "" {
		return ""
	}
	// Find " q " and take everything after it — the key is always "q" here.
	idx := strings.Index(line, " q ")
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(line[idx+3:])
}

func runCmd(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func runInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdOutput(name string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const logPrefix = "tmux-agent-deck: "

// ClientIface is the full tmux capability surface used by the app.
type ClientIface interface {
	NewSession(name, startDir, tool, toolFlags string) error
	AttachSession(name string) error
	KillSession(name string) error
	SessionExists(name string) (bool, error)
	SessionActivity(name string) (time.Time, error)
	CapturePaneOutput(name string) (string, error)
	ListSessions() ([]string, error)
	ListPanes(session string) ([]Pane, error)
	SendKeys(session string, paneIndex int, keys string) error
	SendRawKeys(session string, paneIndex int, keys string) error
}

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func resolveLaunchCommand(command string) string {
	switch strings.TrimSpace(command) {
	case "shell":
		return "zsh -il"
	case "claude-dangerous":
		return "claude --dangerously-skip-permissions"
	default:
		return command
	}
}

func buildLaunchCommand(tool, toolFlags string) string {
	base := resolveLaunchCommand(tool)
	if base == "" {
		return ""
	}
	flags := strings.TrimSpace(toolFlags)
	if flags == "" {
		return base
	}
	return base + " " + flags
}

func (c *Client) NewSession(name, startDir, tool, toolFlags string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", startDir}
	if launch := buildLaunchCommand(tool, toolFlags); launch != "" {
		args = append(args, launch)
	}
	return runCmd("tmux", args...)
}

func (c *Client) AttachSession(name string) error {
	if os.Getenv("TMUX") != "" {
		return runInteractive("tmux", "switch-client", "-t", name)
	}
	saved := saveBinding("C-q")
	setBinding("C-q", "detach-client")
	defer restoreBinding("C-q", saved)
	return runInteractive("tmux", "attach-session", "-t", name)
}

func (c *Client) KillSession(name string) error {
	exists, err := c.SessionExists(name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := runCmd("tmux", "kill-session", "-t", name); err != nil {
		exists, existsErr := c.SessionExists(name)
		if existsErr == nil && !exists {
			return nil
		}
		return err
	}
	return nil
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

func (c *Client) SessionActivity(name string) (time.Time, error) {
	out, err := cmdOutput("tmux", "display-message", "-p", "-t", name, "#{session_activity}")
	if err != nil {
		return time.Time{}, fmt.Errorf("session activity %q: %w", name, err)
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse session activity %q: %w", name, err)
	}
	if sec <= 0 {
		return time.Time{}, nil
	}
	return time.Unix(sec, 0), nil
}

func (c *Client) CapturePaneOutput(name string) (string, error) {
	out, err := cmdOutput("tmux", "capture-pane", "-t", name, "-p", "-S", "-")
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

type Pane struct {
	Index   int
	Command string
}

func (c *Client) ListPanes(session string) ([]Pane, error) {
	out, err := cmdOutput("tmux", "list-panes", "-t", session, "-F", "#{pane_index} #{pane_current_command}")
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return []Pane{}, nil
		}
		return nil, fmt.Errorf("list-panes %q: %w", session, err)
	}
	return parsePanesOutput(string(out)), nil
}

func paneTarget(session string, paneIndex int) string {
	return fmt.Sprintf("%s:0.%d", session, paneIndex)
}

func (c *Client) SendKeys(session string, paneIndex int, keys string) error {
	if keys == "" {
		return nil
	}
	return runCmd("tmux", "send-keys", "-l", "-t", paneTarget(session, paneIndex), keys)
}

func (c *Client) SendRawKeys(session string, paneIndex int, keys string) error {
	if keys == "" {
		return nil
	}
	return runCmd("tmux", "send-keys", "-t", paneTarget(session, paneIndex), keys)
}

func parsePanesOutput(out string) []Pane {
	var panes []Pane
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var idx int
		var cmd string
		if _, err := fmt.Sscanf(line, "%d %s", &idx, &cmd); err != nil {
			continue
		}
		panes = append(panes, Pane{Index: idx, Command: cmd})
	}
	if panes == nil {
		return []Pane{}
	}
	return panes
}

// ParseBindingCommand extracts the command from a tmux list-keys output line.
// Input format: "bind-key [-r|-rT] -T <table> <key> <command>"
// Returns "" if the input is empty or the key separator is not found.
func ParseBindingCommand(listKeysOutput, key string) string {
	line := strings.TrimSpace(listKeysOutput)
	if line == "" {
		return ""
	}
	sep := " " + key + " "
	idx := strings.Index(line, sep)
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(line[idx+len(sep):])
}

func saveBinding(key string) string {
	out, err := cmdOutput("tmux", "list-keys", "-T", "root", key)
	if err != nil {
		return ""
	}
	return ParseBindingCommand(string(out), key)
}

func setBinding(key, command string) {
	if err := runCmd("tmux", "bind-key", "-T", "root", key, command); err != nil {
		fmt.Fprintf(os.Stderr, logPrefix+"bind-key %s: %v\n", key, err)
	}
}

func restoreBinding(key, savedCmd string) {
	var err error
	if savedCmd == "" {
		err = runCmd("tmux", "unbind-key", "-T", "root", key)
	} else {
		err = runCmd("tmux", "bind-key", "-T", "root", key, savedCmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, logPrefix+"restore binding %s: %v\n", key, err)
	}
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

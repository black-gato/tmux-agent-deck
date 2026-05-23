package cmd

import (
	"database/sql"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/hook"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/spf13/cobra"
)

type hookSender interface {
	SendKeys(session string, pane int, keys string) error
	SendRawKeys(session string, pane int, keys string) error
}

// HookHandlerDeps holds injectable dependencies for hook-handler so tests can swap them.
type HookHandlerDeps struct {
	ResolveSession func() (string, error)
	Sender         hookSender
}

var hookHandlerCmd = &cobra.Command{
	Use:   "hook-handler",
	Short: "Handle Claude Code lifecycle hook events (called by Claude Code hooks)",
	RunE: func(cmd *cobra.Command, args []string) error {
		conn, err := openDB()
		if err != nil {
			return err
		}
		defer conn.Close()
		return RunHookHandlerWith(os.Stdin, conn, HookHandlerDeps{
			ResolveSession: resolveCurrentTmuxSession,
			Sender:         tmux.NewClient(),
		})
	},
}

func resolveCurrentTmuxSession() (string, error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#S").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RunHookHandlerWith is exported so tests in package cmd_test can call it directly.
func RunHookHandlerWith(r io.Reader, conn *sql.DB, deps HookHandlerDeps) error {
	event, err := hook.ParseEvent(r)
	if err != nil || event.EventName == "" {
		return nil
	}

	tmuxName, err := deps.ResolveSession()
	if err != nil || tmuxName == "" {
		return nil
	}

	session, err := db.GetSessionByTmuxName(conn, tmuxName)
	if err != nil {
		return nil
	}

	conductor, err := db.GetGroupConductorSession(conn, session.GroupPath)
	if err != nil || conductor.Title == "" || conductor.TmuxSession == "" {
		return nil
	}
	if conductor.Status == tmux.StatusStopped || conductor.Status == tmux.StatusError {
		return nil
	}

	msg := hookMessage(session.Title, event)
	if err := deps.Sender.SendKeys(conductor.TmuxSession, 0, msg); err != nil {
		return err
	}
	return deps.Sender.SendRawKeys(conductor.TmuxSession, 0, "Enter")
}

func hookMessage(title string, event hook.HookEvent) string {
	base := "[deck] " + title + " | " + event.EventName
	switch event.EventName {
	case "Stop":
		return base + " | task complete"
	case "PermissionRequest":
		if event.ToolName != "" {
			return base + " | tool: " + event.ToolName
		}
	case "Notification":
		if event.Message != "" {
			msg := event.Message
			if len(msg) > 60 {
				msg = msg[:60]
			}
			return base + " | " + msg
		}
	}
	return base
}

func init() {
	rootCmd.AddCommand(hookHandlerCmd)
}

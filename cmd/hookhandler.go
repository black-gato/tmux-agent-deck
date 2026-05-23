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

type hookHandlerDeps struct {
	resolveSession func() (string, error)
	sender         hookSender
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
		return runHookHandlerWith(os.Stdin, conn, hookHandlerDeps{
			resolveSession: resolveCurrentTmuxSession,
			sender:         tmux.NewClient(),
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

func runHookHandlerWith(r io.Reader, conn *sql.DB, deps hookHandlerDeps) error {
	event, err := hook.ParseEvent(r)
	if err != nil || event.EventName == "" {
		return nil
	}

	tmuxName, err := deps.resolveSession()
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
	if conductor.TmuxSession == tmuxName {
		return nil
	}

	msg := hookMessage(session.Title, event)
	if err := deps.sender.SendKeys(conductor.TmuxSession, 0, msg); err != nil {
		return err
	}
	return deps.sender.SendRawKeys(conductor.TmuxSession, 0, "Enter")
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
			runes := []rune(msg)
			if len(runes) > 60 {
				msg = string(runes[:60])
			}
			return base + " | " + msg
		}
	}
	return base
}

func init() {
	rootCmd.AddCommand(hookHandlerCmd)
}

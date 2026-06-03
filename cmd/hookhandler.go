package cmd

import (
	"bytes"
	"io"
	"os"

	"github.com/black-gato/tmux-agent-deck/internal/hook"
	"github.com/spf13/cobra"
)

const maxHookPayloadSize = 1 << 20 // 1 MB

var hookHandlerCmd = &cobra.Command{
	Use:   "hook-handler",
	Short: "Handle Claude Code lifecycle hook events (called by Claude Code hooks)",
	RunE: func(cmd *cobra.Command, args []string) error {
		runHookHandler(os.Stdin, os.Getenv("AGENTDECK_INSTANCE_ID"), hook.HooksDir())
		return nil
	},
}

// runHookHandler reads a hook payload, maps it to a status, and writes a status
// file for instanceID into dir. Hooks must not block Claude Code, so failures
// are intentionally swallowed.
func runHookHandler(r io.Reader, instanceID, dir string) {
	if instanceID == "" {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r, maxHookPayloadSize))
	if err != nil || len(data) == 0 {
		return
	}
	event, err := hook.ParseEvent(bytes.NewReader(data))
	if err != nil || event.EventName == "" {
		return
	}
	status := hook.EventToStatus(event.EventName)
	if status == "" {
		return
	}
	_ = hook.WriteStatus(dir, instanceID, status, event.SessionID, event.EventName)
}

func init() {
	rootCmd.AddCommand(hookHandlerCmd)
}

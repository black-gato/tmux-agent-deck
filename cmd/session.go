package cmd

import (
	"fmt"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage session lifecycle",
}

var sessionStartCmd = &cobra.Command{
	Use:   "start <id|title>",
	Short: "Spawn a tmux session for this entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := resolveSession(args[0])
		if err != nil {
			return err
		}
		tc := tmux.NewClient()
		tmuxName := fmt.Sprintf("ad-%s", s.ID[:8])
		if err := tc.NewSession(tmuxName, s.ProjectPath, s.Tool, s.ToolFlags, s.ID); err != nil {
			return fmt.Errorf("start tmux session: %w", err)
		}
		if err := db.UpdateSessionTmuxName(rootDB, s.ID, tmuxName); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: update tmux name: %v\n", err)
		}
		if err := db.UpdateSessionStatus(rootDB, s.ID, "waiting"); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: update status: %v\n", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Started %q as tmux session %q\n", s.Title, tmuxName)
		return nil
	},
}

var sessionStopCmd = &cobra.Command{
	Use:   "stop <id|title>",
	Short: "Kill the tmux session for this entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := resolveSession(args[0])
		if err != nil {
			return err
		}
		if s.TmuxSession != "" {
			tc := tmux.NewClient()
			if err := tc.KillSession(s.TmuxSession); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: kill session: %v\n", err)
			}
		}
		if err := db.UpdateSessionStatus(rootDB, s.ID, "stopped"); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Stopped %q\n", s.Title)
		return nil
	},
}

var sessionAttachCmd = &cobra.Command{
	Use:   "attach <id|title>",
	Short: "Attach to the tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := resolveSession(args[0])
		if err != nil {
			return err
		}
		if s.TmuxSession == "" {
			return fmt.Errorf("session %q has no tmux session — run 'session start' first", s.Title)
		}
		return tmux.NewClient().AttachSession(s.TmuxSession)
	},
}

func resolveSession(ref string) (db.Session, error) {
	s, err := db.GetSession(rootDB, ref)
	if err != nil {
		s, err = db.GetSessionByTitle(rootDB, ref)
	}
	if err != nil {
		return db.Session{}, fmt.Errorf("session not found: %q", ref)
	}
	return s, nil
}

func init() {
	sessionCmd.AddCommand(sessionStartCmd)
	sessionCmd.AddCommand(sessionStopCmd)
	sessionCmd.AddCommand(sessionAttachCmd)
	rootCmd.AddCommand(sessionCmd)
}

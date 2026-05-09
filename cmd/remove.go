package cmd

import (
	"fmt"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <id|title>",
	Short: "Remove a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := resolveSession(args[0])
		if err != nil {
			return err
		}
		if err := db.DeleteSession(rootDB, s.ID); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed session %q\n", s.Title)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

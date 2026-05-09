package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sessions, err := db.ListSessions(rootDB)
		if err != nil {
			return err
		}
		useJSON, _ := cmd.Flags().GetBool("json")
		if useJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(sessions)
		}
		for _, s := range sessions {
			fmt.Fprintf(cmd.OutOrStdout(), "%-36s  %-20s  %-15s  %s\n",
				s.ID, s.Title, s.GroupPath, s.Status)
		}
		return nil
	},
}

func init() {
	listCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(listCmd)
}

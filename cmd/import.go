package cmd

import (
	"fmt"
	"sort"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import [tmux-name]",
	Short: "Import an existing tmux session into the deck",
	Long: `Import a running tmux session that isn't tracked by the deck.

Use --list to print untracked tmux session names (one per line).
Use --all to import every untracked session with defaults.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		listOnly, _ := cmd.Flags().GetBool("list")
		all, _ := cmd.Flags().GetBool("all")
		title, _ := cmd.Flags().GetString("title")
		group, _ := cmd.Flags().GetString("group")

		tc := rootTmuxClient
		if tc == nil {
			tc = tmux.NewClient()
		}

		if listOnly {
			names, err := db.ListUntrackedTmuxSessions(rootDB, tc)
			if err != nil {
				return err
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		}

		if all {
			names, err := db.ListUntrackedTmuxSessions(rootDB, tc)
			if err != nil {
				return err
			}
			sort.Strings(names)
			var firstErr error
			for _, n := range names {
				if _, err := db.ImportSession(rootDB, tc, db.ImportRequest{TmuxName: n, GroupPath: group}); err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "  FAIL %s: %v\n", n, err)
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  imported %s\n", n)
			}
			return firstErr
		}

		if len(args) != 1 {
			return fmt.Errorf("tmux-name required (or use --list / --all)")
		}
		s, err := db.ImportSession(rootDB, tc, db.ImportRequest{
			TmuxName:  args[0],
			Title:     title,
			GroupPath: group,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Imported %q as %q in group %q\n", s.TmuxSession, s.Title, s.GroupPath)
		return nil
	},
}

func init() {
	importCmd.Flags().Bool("list", false, "List untracked tmux sessions and exit")
	importCmd.Flags().Bool("all", false, "Import every untracked tmux session with defaults")
	importCmd.Flags().String("title", "", "Title for the imported session (defaults to tmux name)")
	importCmd.Flags().StringP("group", "g", "my-sessions", "Group path")
	rootCmd.AddCommand(importCmd)
}

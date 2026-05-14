package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/spf13/cobra"
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage groups",
}

var groupCreateCmd = &cobra.Command{
	Use:   "create <path>",
	Args:  cobra.ExactArgs(1),
	Short: "Create a new group",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		tool, _ := cmd.Flags().GetString("tool")
		parts := strings.Split(path, "/")
		name := parts[len(parts)-1]
		g := db.Group{
			Path:        path,
			Name:        name,
			DefaultTool: tool,
			Expanded:    true,
		}
		if err := db.CreateGroup(rootDB, g); err != nil {
			if errors.Is(err, db.ErrGroupExists) {
				return fmt.Errorf("group %q already exists", path)
			}
			return fmt.Errorf("create group: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created group %q\n", path)
		return nil
	},
}

var groupDeleteCmd = &cobra.Command{
	Use:   "delete <path>",
	Short: "Delete a group",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := db.DeleteGroup(rootDB, args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted group %q\n", args[0])
		return nil
	},
}

var groupMoveCmd = &cobra.Command{
	Use:   "move <session-id-or-title> <group-path>",
	Short: "Move a session to a different group",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := resolveSession(args[0])
		if err != nil {
			return err
		}
		if err := db.MoveSession(rootDB, s.ID, args[1]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved %q to %q\n", s.Title, args[1])
		return nil
	},
}

func init() {
	groupCreateCmd.Flags().String("tool", "claude", "Default AI tool for sessions in this group")
	groupCmd.AddCommand(groupCreateCmd)
	groupCmd.AddCommand(groupDeleteCmd)
	groupCmd.AddCommand(groupMoveCmd)
	rootCmd.AddCommand(groupCmd)
}

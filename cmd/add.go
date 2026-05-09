package cmd

import (
	"fmt"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new session",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		group, _ := cmd.Flags().GetString("group")
		project, _ := cmd.Flags().GetString("project")
		tool, _ := cmd.Flags().GetString("tool")

		// inherit group's default tool if --tool was not explicitly set
		if !cmd.Flags().Changed("tool") {
			if g, err := db.GetGroup(rootDB, group); err == nil && g.DefaultTool != "" {
				tool = g.DefaultTool
			}
		}

		s := db.Session{
			ID:          uuid.New().String(),
			Title:       title,
			GroupPath:   group,
			ProjectPath: project,
			Tool:        tool,
			Status:      "stopped",
			CreatedAt:   time.Now().Unix(),
		}
		if err := db.CreateSession(rootDB, s); err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created session %q in group %q\n", title, group)
		return nil
	},
}

func init() {
	addCmd.Flags().String("title", "", "Session title (required)")
	addCmd.Flags().StringP("group", "g", "my-sessions", "Group path")
	addCmd.Flags().StringP("project", "p", ".", "Project directory")
	addCmd.Flags().StringP("tool", "c", "claude", "AI tool")
	addCmd.MarkFlagRequired("title")
	rootCmd.AddCommand(addCmd)
}

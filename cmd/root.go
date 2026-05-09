package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tmux-agent-deck",
	Short: "Manage AI coding agent sessions in tmux",
}

func Execute() error {
	return rootCmd.Execute()
}

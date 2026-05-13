package cmd

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/notify"
	"github.com/black-gato/tmux-agent-deck/internal/state"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
	"github.com/black-gato/tmux-agent-deck/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var rootDB *sql.DB
var notifyEnabled bool
var notifyStyle string
var notifyQuiet string

var rootCmd = &cobra.Command{
	Use:   "tmux-agent-deck",
	Short: "Manage AI coding agent sessions in tmux",
	RunE: func(cmd *cobra.Command, args []string) error {
		return launchTUI(rootDB)
	},
}

func Execute() error {
	conn, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	rootDB = conn
	return rootCmd.Execute()
}

// RunWith is used by tests to inject args and capture output without os.Exit.
func RunWith(args []string, out io.Writer) error {
	conn, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	rootDB = conn
	rootCmd.SetOut(out)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func openDB() (*sql.DB, error) {
	path := os.Getenv("AGENT_DECK_DB")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".tmux-agent-deck", "state.db")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	return db.Open(path)
}

func launchTUI(conn *sql.DB) error {
	tc := tmux.NewClient()
	for {
		poller := state.NewWithNotifier(conn, tc, notify.New(notify.Config{
			Enabled: notifyEnabled,
			Style:   notify.Style(notifyStyle),
			Quiet:   notifyQuiet,
		}))
		poller.Start()

		m := ui.NewModel(conn, tc, poller)
		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			return err
		}
		fm, ok := finalModel.(*ui.Model)
		if !ok || fm.PendingAttach == "" {
			return nil
		}
		exists, _ := tc.SessionExists(fm.PendingAttach)
		if !exists {
			return fmt.Errorf("tmux session %q exited before attach", fm.PendingAttach)
		}
		if err := tc.AttachSession(fm.PendingAttach); err != nil {
			return err
		}
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&notifyEnabled, "notify", false, "Enable desktop notifications")
	rootCmd.PersistentFlags().StringVar(&notifyStyle, "notify-style", "waiting", "Notification style: waiting, conductor, digest")
	rootCmd.PersistentFlags().StringVar(&notifyQuiet, "notify-quiet", "", "Quiet hours / cooldown policy")
}

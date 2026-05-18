package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
var pollInterval time.Duration
var headlessMode bool
var autoEscalate bool
var conductorHeartbeat time.Duration
var rootTmuxClient tmux.ClientIface

var rootCmd = &cobra.Command{
	Use:   "tmux-agent-deck",
	Short: "Manage AI coding agent sessions in tmux",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRoot(cmd.Context(), rootDB)
	},
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	conn, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	return executeWith(ctx, conn, nil, os.Stdout, nil)
}

// RunWith is used by tests to inject args and capture output without os.Exit.
func RunWith(args []string, out io.Writer) error {
	return RunWithContextAndClient(context.Background(), args, out, nil)
}

func RunWithContext(ctx context.Context, args []string, out io.Writer) error {
	return RunWithContextAndClient(ctx, args, out, nil)
}

func RunWithContextAndClient(ctx context.Context, args []string, out io.Writer, tc tmux.ClientIface) error {
	conn, err := openDB()
	if err != nil {
		return err
	}
	defer conn.Close()
	return executeWith(ctx, conn, args, out, tc)
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

func executeWith(ctx context.Context, conn *sql.DB, args []string, out io.Writer, tc tmux.ClientIface) error {
	resetRootOptions()
	rootDB = conn
	rootTmuxClient = tc
	rootCmd.SetOut(out)
	rootCmd.SetArgs(args)
	rootCmd.SetContext(ctx)
	err := rootCmd.Execute()
	rootTmuxClient = nil
	rootDB = nil
	return err
}

func resetRootOptions() {
	notifyEnabled = false
	notifyStyle = "waiting"
	notifyQuiet = ""
	pollInterval = time.Second
	headlessMode = false
	autoEscalate = false
	conductorHeartbeat = 0
	_ = rootCmd.PersistentFlags().Set("conductor-heartbeat", "0s")
	_ = rootCmd.PersistentFlags().Set("notify", "false")
	_ = rootCmd.PersistentFlags().Set("notify-style", "waiting")
	_ = rootCmd.PersistentFlags().Set("notify-quiet", "")
	_ = rootCmd.PersistentFlags().Set("poll", time.Second.String())
	_ = rootCmd.PersistentFlags().Set("headless", "false")
	_ = rootCmd.PersistentFlags().Set("auto-escalate", "false")
	_ = rootCmd.Flags().Set("help", "false")
}

func runRoot(ctx context.Context, conn *sql.DB) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tc := rootTmuxClient
	if tc == nil {
		tc = tmux.NewClient()
	}
	if headlessMode {
		return launchHeadless(ctx, conn, tc)
	}
	return launchTUI(conn, tc)
}

func launchTUI(conn *sql.DB, tc tmux.ClientIface) error {
	for {
		poller := state.NewWithNotifierInterval(conn, tc, notify.New(notify.Config{
			Enabled: notifyEnabled,
			Style:   notify.Style(notifyStyle),
			Quiet:   notifyQuiet,
		}), pollInterval)
		if autoEscalate {
			poller.SetSender(tc)
		}
		if conductorHeartbeat > 0 {
			poller.SetConductorHeartbeat(conductorHeartbeat)
		}
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
		if fm.PendingStartupScript != "" {
			time.Sleep(2 * time.Second)
			_ = tc.SendKeys(fm.PendingAttach, 0, fm.PendingStartupScript)
			_ = tc.SendRawKeys(fm.PendingAttach, 0, "Enter")
		}
		if err := tc.AttachSession(fm.PendingAttach); err != nil {
			return err
		}
	}
}

func launchHeadless(ctx context.Context, conn *sql.DB, tc tmux.ClientIface) error {
	poller := state.NewWithNotifierInterval(conn, tc, notify.New(notify.Config{
		Enabled: notifyEnabled,
		Style:   notify.Style(notifyStyle),
		Quiet:   notifyQuiet,
	}), pollInterval)
	if autoEscalate {
		poller.SetSender(tc)
	}
	if conductorHeartbeat > 0 {
		poller.SetConductorHeartbeat(conductorHeartbeat)
	}
	poller.Start()
	defer poller.Stop()
	<-ctx.Done()
	return nil
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&notifyEnabled, "notify", false, "Enable desktop notifications")
	rootCmd.PersistentFlags().StringVar(&notifyStyle, "notify-style", "waiting", `Notification routing style:
  waiting    alert per session the moment it goes waiting
  conductor  alert names the group conductor
  digest     one combined alert per poll cycle for waiting workers`)
	rootCmd.PersistentFlags().StringVar(&notifyQuiet, "notify-quiet", "", `Quiet hours and cooldown policy (comma-separated key=value):
  cooldown=5m        suppress duplicate alerts within this duration
  hours=22:00-08:00  suppress all alerts during this time window
  example: --notify-quiet "cooldown=5m,hours=22:00-08:00"`)
	rootCmd.PersistentFlags().DurationVar(&pollInterval, "poll", time.Second, "Poll interval")
	rootCmd.PersistentFlags().BoolVar(&headlessMode, "headless", false, "Run the poller without launching the TUI")
	rootCmd.PersistentFlags().BoolVar(&autoEscalate, "auto-escalate", false, "Automatically send escalation message to group conductor when a worker session goes waiting")
	rootCmd.PersistentFlags().DurationVar(&conductorHeartbeat, "conductor-heartbeat", 0, "Send a waiting-worker digest to each group conductor on this interval (0 disables)")
}

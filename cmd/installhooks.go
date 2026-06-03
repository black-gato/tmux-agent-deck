package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const deckHookCommand = "tmux-agent-deck hook-handler"
const legacyHookCommand = "agent-deck hook-handler"

var managedHookEvents = []struct {
	name  string
	async bool
}{
	{"Stop", true},
	{"SessionStart", true},
	{"SessionEnd", true},
	{"UserPromptSubmit", true},
	{"PermissionRequest", false},
	{"PreCompact", false},
	{"Notification", true},
}

var installHooksUninstall bool

var installHooksCmd = &cobra.Command{
	Use:   "install-hooks",
	Short: "Register tmux-agent-deck hook-handler in ~/.claude/settings.json",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path := filepath.Join(home, ".claude", "settings.json")
		return runInstallHooks(path, installHooksUninstall, cmd.OutOrStdout())
	},
}

func runInstallHooks(settingsPath string, uninstall bool, out io.Writer) error {
	settings, err := readSettings(settingsPath)
	if err != nil {
		return err
	}

	hooks := settingsHooks(settings)

	for _, ev := range managedHookEvents {
		arr := hooksForEvent(hooks, ev.name)
		if uninstall {
			before := len(arr)
			arr = removeOurEntry(arr)
			if len(arr) == 0 {
				delete(hooks, ev.name)
			} else {
				hooks[ev.name] = arr
			}
			if len(arr) < before {
				fmt.Fprintf(out, "%-20s removed\n", ev.name)
			} else {
				fmt.Fprintf(out, "%-20s not registered\n", ev.name)
			}
		} else {
			arr = removeLegacyEntry(arr)
			if hasOurEntry(arr) {
				hooks[ev.name] = arr
				fmt.Fprintf(out, "%-20s already registered\n", ev.name)
			} else {
				hooks[ev.name] = append(arr, buildEntry(ev.async))
				fmt.Fprintf(out, "%-20s added\n", ev.name)
			}
		}
	}

	settings["hooks"] = hooks
	return writeSettings(settingsPath, settings)
}

func readSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func writeSettings(path string, settings map[string]any) error {
	data, err := json.MarshalIndent(settings, "", "    ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func settingsHooks(settings map[string]any) map[string]any {
	if h, ok := settings["hooks"].(map[string]any); ok {
		return h
	}
	h := map[string]any{}
	settings["hooks"] = h
	return h
}

func hooksForEvent(hooks map[string]any, event string) []any {
	arr, _ := hooks[event].([]any)
	return arr
}

func hasOurEntry(arr []any) bool {
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		// flat format (old/incorrect — detect for idempotency during migration)
		if m["command"] == deckHookCommand {
			return true
		}
		// correct wrapped format
		if hooks, ok := m["hooks"].([]any); ok {
			for _, h := range hooks {
				hm, ok := h.(map[string]any)
				if ok && hm["command"] == deckHookCommand {
					return true
				}
			}
		}
	}
	return false
}

func removeOurEntry(arr []any) []any {
	var out []any
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		// flat format (old/incorrect) — drop entirely
		if m["command"] == deckHookCommand {
			continue
		}
		// wrapped format — filter our command out of the hooks array
		if hooks, ok := m["hooks"].([]any); ok {
			var remaining []any
			for _, h := range hooks {
				hm, ok := h.(map[string]any)
				if ok && hm["command"] == deckHookCommand {
					continue
				}
				remaining = append(remaining, h)
			}
			if len(remaining) == 0 {
				continue // group is now empty, drop it
			}
			updated := make(map[string]any, len(m))
			for k, v := range m {
				updated[k] = v
			}
			updated["hooks"] = remaining
			out = append(out, updated)
			continue
		}
		out = append(out, m)
	}
	return out
}

// removeLegacyEntry strips upstream agent-deck hook-handler entries so install
// converges to one deck-managed command.
func removeLegacyEntry(arr []any) []any {
	var out []any
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			out = append(out, entry)
			continue
		}
		if m["command"] == legacyHookCommand {
			continue
		}
		if hooks, ok := m["hooks"].([]any); ok {
			var remaining []any
			for _, h := range hooks {
				hm, ok := h.(map[string]any)
				if ok && hm["command"] == legacyHookCommand {
					continue
				}
				remaining = append(remaining, h)
			}
			if len(remaining) == 0 {
				continue
			}
			updated := make(map[string]any, len(m))
			for k, v := range m {
				updated[k] = v
			}
			updated["hooks"] = remaining
			out = append(out, updated)
			continue
		}
		out = append(out, m)
	}
	return out
}

func buildEntry(async bool) map[string]any {
	hook := map[string]any{
		"type":    "command",
		"command": deckHookCommand,
	}
	if async {
		hook["async"] = true
	}
	return map[string]any{
		"hooks": []any{hook},
	}
}

func init() {
	installHooksCmd.Flags().BoolVar(&installHooksUninstall, "uninstall", false, "Remove tmux-agent-deck hook-handler entries")
	rootCmd.AddCommand(installHooksCmd)
}

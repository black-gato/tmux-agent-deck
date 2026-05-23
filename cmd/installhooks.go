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
			if hasOurEntry(arr) {
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

func readSettings(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func writeSettings(path string, settings map[string]interface{}) error {
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

func settingsHooks(settings map[string]interface{}) map[string]interface{} {
	if h, ok := settings["hooks"].(map[string]interface{}); ok {
		return h
	}
	h := map[string]interface{}{}
	settings["hooks"] = h
	return h
}

func hooksForEvent(hooks map[string]interface{}, event string) []interface{} {
	arr, _ := hooks[event].([]interface{})
	return arr
}

func hasOurEntry(arr []interface{}) bool {
	for _, entry := range arr {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if m["command"] == deckHookCommand {
			return true
		}
	}
	return false
}

func removeOurEntry(arr []interface{}) []interface{} {
	var out []interface{}
	for _, entry := range arr {
		m, ok := entry.(map[string]interface{})
		if ok && m["command"] == deckHookCommand {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func buildEntry(async bool) map[string]interface{} {
	entry := map[string]interface{}{
		"type":    "command",
		"command": deckHookCommand,
	}
	if async {
		entry["async"] = true
	}
	return entry
}

func init() {
	installHooksCmd.Flags().BoolVar(&installHooksUninstall, "uninstall", false, "Remove tmux-agent-deck hook-handler entries")
	rootCmd.AddCommand(installHooksCmd)
}

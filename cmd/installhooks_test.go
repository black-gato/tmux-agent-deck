package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// countWrappedCommand counts how many hook groups in arr contain command in their "hooks" array.
func countWrappedCommand(arr []any, command string) int {
	count := 0
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if hooks, ok := m["hooks"].([]any); ok {
			for _, h := range hooks {
				hm, ok := h.(map[string]any)
				if ok && hm["command"] == command {
					count++
				}
			}
		}
	}
	return count
}

func TestInstallHooks_FreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := runInstallHooks(path, false, io.Discard); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("expected hooks object")
	}
	for _, event := range []string{"Stop", "SessionStart", "SessionEnd", "UserPromptSubmit", "PermissionRequest", "PreCompact", "Notification"} {
		arr, ok := hooks[event].([]any)
		if !ok || len(arr) == 0 {
			t.Errorf("event %q: expected at least one hook entry", event)
			continue
		}
		if countWrappedCommand(arr, "tmux-agent-deck hook-handler") == 0 {
			t.Errorf("event %q: tmux-agent-deck hook-handler not found in wrapped hooks", event)
		}
	}
}

func TestInstallHooks_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := runInstallHooks(path, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := runInstallHooks(path, false, io.Discard); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var settings map[string]any
	_ = json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	count := countWrappedCommand(stop, "tmux-agent-deck hook-handler")
	if count != 1 {
		t.Errorf("expected exactly 1 hook-handler entry for Stop after two installs, got %d", count)
	}
}

func TestInstallHooks_PreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	existing := map[string]any{
		"env": map[string]any{"EDITOR": "nvim"},
		"hooks": map[string]any{
			"Stop": []any{
				map[string]any{"type": "command", "command": "other-tool", "async": true},
			},
		},
	}
	data, _ := json.Marshal(existing)
	_ = os.WriteFile(path, data, 0644)

	if err := runInstallHooks(path, false, io.Discard); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(path)
	var settings map[string]any
	_ = json.Unmarshal(data, &settings)

	// env preserved
	env, ok := settings["env"].(map[string]any)
	if !ok || env["EDITOR"] != "nvim" {
		t.Error("expected env.EDITOR to be preserved")
	}

	// other-tool preserved in Stop
	hooks := settings["hooks"].(map[string]any)
	stop := hooks["Stop"].([]any)
	foundOther := false
	for _, entry := range stop {
		m, ok := entry.(map[string]any)
		if ok && m["command"] == "other-tool" {
			foundOther = true
		}
	}
	if !foundOther {
		t.Error("expected other-tool entry to be preserved in Stop hooks")
	}
}

func TestInstallHooks_Uninstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// install first
	_ = runInstallHooks(path, false, io.Discard)
	// then uninstall
	if err := runInstallHooks(path, true, io.Discard); err != nil {
		t.Fatalf("uninstall error: %v", err)
	}

	data, _ := os.ReadFile(path)
	var settings map[string]any
	_ = json.Unmarshal(data, &settings)
	hooks, _ := settings["hooks"].(map[string]any)
	for _, event := range []string{"Stop", "SessionStart", "SessionEnd", "UserPromptSubmit", "PermissionRequest", "PreCompact", "Notification"} {
		if _, exists := hooks[event]; exists {
			t.Errorf("event %q: key still present in hooks after uninstall", event)
		}
	}
}

func TestInstallHooks_PermissionRequestIsSync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	_ = runInstallHooks(path, false, io.Discard)

	data, _ := os.ReadFile(path)
	var settings map[string]any
	_ = json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]any)
	arr := hooks["PermissionRequest"].([]any)
	for _, group := range arr {
		gm, ok := group.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := gm["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hm, ok := h.(map[string]any)
			if !ok || hm["command"] != "tmux-agent-deck hook-handler" {
				continue
			}
			if v, exists := hm["async"]; exists && v == true {
				t.Error("PermissionRequest hook-handler must not be async")
			}
		}
	}
}

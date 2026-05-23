package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON written: %v", err)
	}
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected hooks object")
	}
	for _, event := range []string{"Stop", "SessionStart", "SessionEnd", "UserPromptSubmit", "PermissionRequest", "PreCompact", "Notification"} {
		arr, ok := hooks[event].([]interface{})
		if !ok || len(arr) == 0 {
			t.Errorf("event %q: expected at least one hook entry", event)
			continue
		}
		found := false
		for _, entry := range arr {
			m, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if m["command"] == "tmux-agent-deck hook-handler" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %q: tmux-agent-deck hook-handler not found in hooks", event)
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
	var settings map[string]interface{}
	_ = json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	count := 0
	for _, entry := range stop {
		m, ok := entry.(map[string]interface{})
		if ok && m["command"] == "tmux-agent-deck hook-handler" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 hook-handler entry for Stop after two installs, got %d", count)
	}
}

func TestInstallHooks_PreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	existing := map[string]interface{}{
		"env": map[string]interface{}{"EDITOR": "nvim"},
		"hooks": map[string]interface{}{
			"Stop": []interface{}{
				map[string]interface{}{"type": "command", "command": "other-tool", "async": true},
			},
		},
	}
	data, _ := json.Marshal(existing)
	_ = os.WriteFile(path, data, 0644)

	if err := runInstallHooks(path, false, io.Discard); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(path)
	var settings map[string]interface{}
	_ = json.Unmarshal(data, &settings)

	// env preserved
	env, ok := settings["env"].(map[string]interface{})
	if !ok || env["EDITOR"] != "nvim" {
		t.Error("expected env.EDITOR to be preserved")
	}

	// other-tool preserved in Stop
	hooks := settings["hooks"].(map[string]interface{})
	stop := hooks["Stop"].([]interface{})
	foundOther := false
	for _, entry := range stop {
		m, ok := entry.(map[string]interface{})
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
	var settings map[string]interface{}
	_ = json.Unmarshal(data, &settings)
	hooks, _ := settings["hooks"].(map[string]interface{})
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
	var settings map[string]interface{}
	_ = json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]interface{})
	arr := hooks["PermissionRequest"].([]interface{})
	for _, entry := range arr {
		m, ok := entry.(map[string]interface{})
		if !ok || m["command"] != "tmux-agent-deck hook-handler" {
			continue
		}
		// async must be absent (false/omitted) for PermissionRequest
		if v, exists := m["async"]; exists && v == true {
			t.Error("PermissionRequest hook-handler must not be async")
		}
	}
}

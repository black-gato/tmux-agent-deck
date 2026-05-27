package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const FreshWindow = 2 * time.Minute

var validInstanceID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type HookStatus struct {
	Status    string
	SessionID string
	Event     string
	UpdatedAt time.Time
}

type statusFile struct {
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
	Event     string `json:"event"`
	Timestamp int64  `json:"ts"`
}

func (h HookStatus) DeckStatus() string {
	if h.Status == "dead" {
		return "error"
	}
	return h.Status
}

func Fresh(h HookStatus, now time.Time) bool {
	return !h.UpdatedAt.IsZero() && now.Sub(h.UpdatedAt) <= FreshWindow
}

func HooksDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), ".tmux-agent-deck", "hooks")
	}
	return filepath.Join(home, ".tmux-agent-deck", "hooks")
}

func validID(instanceID string) bool {
	return validInstanceID.MatchString(instanceID) && !strings.Contains(instanceID, "..")
}

func WriteStatus(dir, instanceID, status, sessionID, event string) error {
	if !validID(instanceID) {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(statusFile{
		Status:    status,
		SessionID: sessionID,
		Event:     event,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	final := filepath.Join(dir, instanceID+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func ReadStatus(dir, instanceID string) (HookStatus, bool) {
	if !validID(instanceID) {
		return HookStatus{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, instanceID+".json"))
	if err != nil {
		return HookStatus{}, false
	}
	var f statusFile
	if err := json.Unmarshal(data, &f); err != nil || f.Status == "" {
		return HookStatus{}, false
	}
	return HookStatus{
		Status:    f.Status,
		SessionID: f.SessionID,
		Event:     f.Event,
		UpdatedAt: time.Unix(f.Timestamp, 0),
	}, true
}

func ListStatuses(dir string) map[string]HookStatus {
	out := make(map[string]HookStatus)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if hs, ok := ReadStatus(dir, id); ok {
			out[id] = hs
		}
	}
	return out
}

package testutil

import (
	"fmt"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

// FakeTmuxClient implements tmux.ClientIface for tests.
// Sessions maps session name → pane output.
// Activities maps session name → last tmux activity timestamp.
// Panes maps session name → pane list.
// SentKeys records all SendKeys calls.
type FakeTmuxClient struct {
	Sessions        map[string]string
	Activities      map[string]time.Time
	Panes           map[string][]tmux.Pane
	NewSessionCalls []NewSessionCall
	AttachCalls     []string
	KillCalls       []string
	SentKeys        []SentKeysCall
	SentRawKeys     []SentKeysCall
	NewSessionErr   error
	AttachErr       error
}

type NewSessionCall struct {
	Name    string
	Dir     string
	Command string
}

type SentKeysCall struct {
	Session   string
	PaneIndex int
	Keys      string
}

func NewFakeTmuxClient() *FakeTmuxClient {
	return &FakeTmuxClient{
		Sessions:   make(map[string]string),
		Activities: make(map[string]time.Time),
		Panes:      make(map[string][]tmux.Pane),
	}
}

func (f *FakeTmuxClient) NewSession(name, startDir, command string) error {
	f.NewSessionCalls = append(f.NewSessionCalls, NewSessionCall{name, startDir, command})
	if f.NewSessionErr != nil {
		return f.NewSessionErr
	}
	f.Sessions[name] = ""
	return nil
}

func (f *FakeTmuxClient) AttachSession(name string) error {
	f.AttachCalls = append(f.AttachCalls, name)
	return f.AttachErr
}

func (f *FakeTmuxClient) KillSession(name string) error {
	f.KillCalls = append(f.KillCalls, name)
	delete(f.Sessions, name)
	return nil
}

func (f *FakeTmuxClient) SessionExists(name string) (bool, error) {
	_, ok := f.Sessions[name]
	return ok, nil
}

func (f *FakeTmuxClient) SessionActivity(name string) (time.Time, error) {
	if activity, ok := f.Activities[name]; ok {
		return activity, nil
	}
	return time.Time{}, nil
}

func (f *FakeTmuxClient) CapturePaneOutput(name string) (string, error) {
	out, ok := f.Sessions[name]
	if !ok {
		return "", fmt.Errorf("no session %q", name)
	}
	return out, nil
}

func (f *FakeTmuxClient) ListSessions() ([]string, error) {
	names := make([]string, 0, len(f.Sessions))
	for name := range f.Sessions {
		names = append(names, name)
	}
	return names, nil
}

func (f *FakeTmuxClient) ListPanes(session string) ([]tmux.Pane, error) {
	panes, ok := f.Panes[session]
	if !ok {
		return []tmux.Pane{}, nil
	}
	return panes, nil
}

func (f *FakeTmuxClient) SendKeys(session string, paneIndex int, keys string) error {
	f.SentKeys = append(f.SentKeys, SentKeysCall{Session: session, PaneIndex: paneIndex, Keys: keys})
	return nil
}

func (f *FakeTmuxClient) SendRawKeys(session string, paneIndex int, keys string) error {
	f.SentRawKeys = append(f.SentRawKeys, SentKeysCall{Session: session, PaneIndex: paneIndex, Keys: keys})
	return nil
}

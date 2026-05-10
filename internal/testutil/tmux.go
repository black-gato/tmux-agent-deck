package testutil

import "fmt"

// FakeTmuxClient implements tmux.ClientIface for tests.
// Configure Sessions to control which sessions "exist".
// NewSessionCalls and AttachCalls record what was called.
type FakeTmuxClient struct {
	Sessions       map[string]string // session name → pane output
	NewSessionCalls []NewSessionCall
	AttachCalls    []string
	KillCalls      []string
	NewSessionErr  error
	AttachErr      error
}

type NewSessionCall struct {
	Name    string
	Dir     string
	Command string
}

func NewFakeTmuxClient() *FakeTmuxClient {
	return &FakeTmuxClient{Sessions: make(map[string]string)}
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

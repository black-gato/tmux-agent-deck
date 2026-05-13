package notify

import (
	"fmt"
	"os/exec"
)

type Style string

const (
	StyleWaiting   Style = "waiting"
	StyleConductor Style = "conductor"
	StyleDigest    Style = "digest"
)

type Config struct {
	Enabled bool
	Style   Style
	Quiet   string
}

type Runner func(title, body string) error

type Notifier struct {
	config Config
	run    Runner
}

func New(config Config) *Notifier {
	return NewWithRunner(config, runAppleScript)
}

func NewWithRunner(config Config, runner Runner) *Notifier {
	if config.Style == "" {
		config.Style = StyleWaiting
	}
	return &Notifier{config: config, run: runner}
}

func (n *Notifier) Enabled() bool {
	return n != nil && n.config.Enabled
}

func (n *Notifier) Style() Style {
	if n == nil {
		return StyleWaiting
	}
	return n.config.Style
}

func (n *Notifier) Notify(title, body string) error {
	if !n.Enabled() {
		return nil
	}
	return n.run(title, body)
}

func runAppleScript(title, body string) error {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	return exec.Command("osascript", "-e", script).Run()
}

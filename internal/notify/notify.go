package notify

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
	now    func() time.Time
	policy quietPolicy
	last   map[string]time.Time
	run    Runner
}

func New(config Config) *Notifier {
	return NewWithClockRunner(config, time.Now, runAppleScript)
}

func NewWithRunner(config Config, runner Runner) *Notifier {
	return NewWithClockRunner(config, time.Now, runner)
}

func NewWithClockRunner(config Config, now func() time.Time, runner Runner) *Notifier {
	if config.Style == "" {
		config.Style = StyleWaiting
	}
	return &Notifier{
		config: config,
		now:    now,
		policy: parseQuietPolicy(config.Quiet),
		last:   make(map[string]time.Time),
		run:    runner,
	}
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
	now := time.Now()
	if n != nil && n.now != nil {
		now = n.now()
	}
	key := notificationKey(title, body)
	if !n.policy.allow(now, key, n.last) {
		return nil
	}
	return n.run(title, body)
}

type quietPolicy struct {
	cooldown time.Duration
	windows  []quietWindow
}

type quietWindow struct {
	start time.Duration
	end   time.Duration
}

func parseQuietPolicy(raw string) quietPolicy {
	var policy quietPolicy
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "cooldown":
			d, err := time.ParseDuration(strings.TrimSpace(value))
			if err == nil && d > 0 {
				policy.cooldown = d
			}
		case "hours":
			window, ok := parseQuietWindow(strings.TrimSpace(value))
			if ok {
				policy.windows = append(policy.windows, window)
			}
		}
	}
	return policy
}

func parseQuietWindow(raw string) (quietWindow, bool) {
	startRaw, endRaw, ok := strings.Cut(raw, "-")
	if !ok {
		return quietWindow{}, false
	}
	start, ok := parseClock(strings.TrimSpace(startRaw))
	if !ok {
		return quietWindow{}, false
	}
	end, ok := parseClock(strings.TrimSpace(endRaw))
	if !ok {
		return quietWindow{}, false
	}
	return quietWindow{start: start, end: end}, true
}

func parseClock(raw string) (time.Duration, bool) {
	hourRaw, minuteRaw, ok := strings.Cut(raw, ":")
	if !ok {
		return 0, false
	}
	hour, err := strconv.Atoi(hourRaw)
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(minuteRaw)
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, true
}

func (p quietPolicy) allow(now time.Time, key string, last map[string]time.Time) bool {
	if p.inQuietHours(now) {
		return false
	}
	if p.cooldown <= 0 || key == "" {
		return true
	}
	lastSent, ok := last[key]
	if ok && now.Sub(lastSent) < p.cooldown {
		return false
	}
	last[key] = now
	return true
}

func (p quietPolicy) inQuietHours(now time.Time) bool {
	if len(p.windows) == 0 {
		return false
	}
	current := time.Duration(now.Hour())*time.Hour + time.Duration(now.Minute())*time.Minute
	for _, window := range p.windows {
		if window.contains(current) {
			return true
		}
	}
	return false
}

func (w quietWindow) contains(current time.Duration) bool {
	if w.start == w.end {
		return true
	}
	if w.start < w.end {
		return current >= w.start && current < w.end
	}
	return current >= w.start || current < w.end
}

func notificationKey(title, body string) string {
	if title == "Conductor digest" {
		if first, _, ok := strings.Cut(body, "\n"); ok {
			return title + "|" + first
		}
	}
	return title + "|" + body
}

func runAppleScript(title, body string) error {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	return exec.Command("osascript", "-e", script).Run()
}

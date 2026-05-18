package state

import (
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/notify"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

// TmuxReader is the subset of tmux.Client used by the poller.
// Defined here so tests can stub it without importing the real client.
type TmuxReader interface {
	CapturePaneOutput(name string) (string, error)
	SessionExists(name string) (bool, error)
	SessionActivity(name string) (time.Time, error)
}

type Poller struct {
	conn         *sql.DB
	tmux         TmuxReader
	notifier     waitingNotifier
	sender       TmuxSender
	now          func() time.Time
	interval     time.Duration
	mu           sync.RWMutex
	lastChange        map[string]time.Time
	lastOutput        map[string]string
	waitingSince      map[string]time.Time
	contextPct        map[string]*int
	conductorBaseline map[string]string
	processedReplies  map[string]bool
	done              chan struct{}
}

type waitingNotifier interface {
	Enabled() bool
	Style() notify.Style
	Notify(title, body string) error
}

type TmuxSender interface {
	SendKeys(session string, pane int, keys string) error
	SendRawKeys(session string, pane int, keys string) error
}

func New(conn *sql.DB, tc TmuxReader) *Poller {
	return NewWithNotifier(conn, tc, notify.New(notify.Config{}))
}

func NewWithNotifier(conn *sql.DB, tc TmuxReader, notifier waitingNotifier) *Poller {
	return NewWithNotifierInterval(conn, tc, notifier, time.Second)
}

func NewWithClock(conn *sql.DB, tc TmuxReader, notifier waitingNotifier, now func() time.Time) *Poller {
	return NewWithClockInterval(conn, tc, notifier, now, time.Second)
}

func NewWithNotifierInterval(conn *sql.DB, tc TmuxReader, notifier waitingNotifier, interval time.Duration) *Poller {
	return NewWithClockInterval(conn, tc, notifier, time.Now, interval)
}

func NewWithClockInterval(conn *sql.DB, tc TmuxReader, notifier waitingNotifier, now func() time.Time, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = time.Second
	}
	return &Poller{
		conn:         conn,
		tmux:         tc,
		notifier:     notifier,
		now:          now,
		interval:     interval,
		lastChange:        make(map[string]time.Time),
		lastOutput:        make(map[string]string),
		waitingSince:      make(map[string]time.Time),
		contextPct:        make(map[string]*int),
		conductorBaseline: make(map[string]string),
		processedReplies:  make(map[string]bool),
		done:              make(chan struct{}),
	}
}

func (p *Poller) SetSender(s TmuxSender) {
	p.sender = s
}

func (p *Poller) Start() {
	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.PollOnce()
			case <-p.done:
				return
			}
		}
	}()
}

func (p *Poller) Stop() {
	close(p.done)
}

func (p *Poller) WaitingSinceSnapshot() map[string]time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()

	snapshot := make(map[string]time.Time, len(p.waitingSince))
	for id, ts := range p.waitingSince {
		snapshot[id] = ts
	}
	return snapshot
}

func (p *Poller) Now() time.Time {
	return p.now()
}

func (p *Poller) Interval() time.Duration {
	return p.interval
}

func (p *Poller) PollOnce() {
	sessions, err := db.ListSessions(p.conn)
	if err != nil {
		log.Printf("poller: list sessions: %v", err)
		return
	}
	digestGroups := make(map[string]bool)
	for _, s := range sessions {
		if s.Status == tmux.StatusStopped || s.TmuxSession == "" {
			p.clearSessionState(s.ID)
			continue
		}
		exists, err := p.tmux.SessionExists(s.TmuxSession)
		if err != nil {
			log.Printf("poller: session exists %q: %v", s.TmuxSession, err)
			continue
		}
		if !exists {
			if err := db.UpdateSessionStatus(p.conn, s.ID, tmux.StatusError); err != nil {
				log.Printf("poller: update status error %q: %v", s.ID, err)
			}
			p.clearSessionState(s.ID)
			continue
		}
		out, err := p.tmux.CapturePaneOutput(s.TmuxSession)
		if err != nil {
			log.Printf("poller: capture pane %q: %v", s.TmuxSession, err)
			continue
		}

		p.setContextPct(s.ID, tmux.ParseContextPct(out))

		now := p.now()
		lc := p.observeOutput(s.ID, out, now, p.initialLastChange(s, now))

		newStatus := tmux.DetectStatus(out, lc, now, s.Tool)
		p.updateWaitingState(s.ID, s.Status, newStatus, now)
		if newStatus != s.Status {
			if err := db.UpdateSessionStatus(p.conn, s.ID, newStatus); err != nil {
				log.Printf("poller: update status %q: %v", s.ID, err)
			}
			if s.Status != tmux.StatusWaiting && newStatus == tmux.StatusWaiting {
				if p.notifier != nil && p.notifier.Style() == notify.StyleDigest {
					digestGroups[s.GroupPath] = true
				} else {
					p.notifyWaiting(s)
				}
				p.autoEscalate(s, out)
			}
		}
	}
	for groupPath := range digestGroups {
		p.notifyDigest(groupPath)
	}
	p.scanConductorReplies(sessions)
}

func (p *Poller) notifyWaiting(session db.Session) {
	if p.notifier == nil || !p.notifier.Enabled() {
		return
	}
	switch p.notifier.Style() {
	case notify.StyleDigest:
		return
	case notify.StyleConductor:
		conductor, err := db.GetGroupConductorSession(p.conn, session.GroupPath)
		if err != nil || conductor.Title == "" {
			return
		}
		if conductor.ID == session.ID {
			return
		}
		title := "Conductor alert"
		body := conductor.Title + ": " + session.Title + " is waiting"
		if err := p.notifier.Notify(title, body); err != nil {
			log.Printf("poller: notify conductor %q: %v", session.ID, err)
		}
	default:
		title := "Agent waiting"
		body := session.Title + " is waiting"
		if err := p.notifier.Notify(title, body); err != nil {
			log.Printf("poller: notify waiting %q: %v", session.ID, err)
		}
	}
}

func (p *Poller) notifyDigest(groupPath string) {
	conductor, err := db.GetGroupConductorSession(p.conn, groupPath)
	if err != nil || conductor.Title == "" {
		return
	}
	waiting, err := db.ListWaitingGroupChildren(p.conn, groupPath)
	if err != nil || len(waiting) == 0 {
		return
	}
	if err := p.notifier.Notify("Conductor digest", p.digestBody(conductor.Title, waiting)); err != nil {
		log.Printf("poller: notify digest %q: %v", groupPath, err)
	}
}

func (p *Poller) digestBody(conductorTitle string, sessions []db.Session) string {
	lines := make([]string, 0, len(sessions)+1)
	lines = append(lines, conductorTitle+": waiting sessions")
	for _, session := range sessions {
		line := "- " + session.Title
		if session.Notes != "" {
			line += ": " + session.Notes
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (p *Poller) autoEscalate(session db.Session, output string) {
	if p.sender == nil {
		return
	}
	conductor, err := db.GetGroupConductorSession(p.conn, session.GroupPath)
	if err != nil || conductor.Title == "" {
		return
	}
	if conductor.ID == session.ID {
		return
	}
	if conductorUnavailable(conductor) {
		log.Printf("poller: auto-escalate %q: conductor %q unavailable", session.ID, conductor.Title)
		return
	}
	if err := p.sender.SendKeys(conductor.TmuxSession, 0, EscalationMessage(session, output)); err != nil {
		log.Printf("poller: auto-escalate send keys %q: %v", session.ID, err)
		return
	}
	if err := p.sender.SendRawKeys(conductor.TmuxSession, 0, "Enter"); err != nil {
		log.Printf("poller: auto-escalate submit %q: %v", session.ID, err)
	}
}

func (p *Poller) scanConductorReplies(sessions []db.Session) {
	if p.sender == nil {
		return
	}
	groups, err := db.ListGroups(p.conn)
	if err != nil {
		log.Printf("poller: scan replies list groups: %v", err)
		return
	}
	conductorIDs := make(map[string]bool, len(groups))
	for _, g := range groups {
		if g.ConductorSessionID != "" {
			conductorIDs[g.ConductorSessionID] = true
		}
	}
	for _, s := range sessions {
		if !conductorIDs[s.ID] || s.TmuxSession == "" {
			continue
		}
		out, err := p.tmux.CapturePaneOutput(s.TmuxSession)
		if err != nil {
			continue
		}
		p.mu.Lock()
		baseline, seen := p.conductorBaseline[s.ID]
		if !seen {
			p.conductorBaseline[s.ID] = out
			p.mu.Unlock()
			continue
		}
		newOut := NewOutputSince(baseline, out)
		p.mu.Unlock()

		for _, block := range ParseReplyBlocks(newOut) {
			fingerprint := block.WorkerID + ":" + block.Body
			p.mu.Lock()
			if p.processedReplies[fingerprint] {
				p.mu.Unlock()
				continue
			}
			p.processedReplies[fingerprint] = true
			p.mu.Unlock()

			worker, err := db.GetSession(p.conn, block.WorkerID)
			if err != nil {
				log.Printf("poller: reply routing: unknown worker %q", block.WorkerID)
				continue
			}
			if worker.Status == tmux.StatusStopped || worker.Status == tmux.StatusError || worker.TmuxSession == "" {
				log.Printf("poller: reply routing: worker %q not active (status=%s)", block.WorkerID, worker.Status)
				continue
			}
			msg := "Conductor reply: " + block.Body
			if err := p.sender.SendKeys(worker.TmuxSession, 0, msg); err != nil {
				log.Printf("poller: reply routing send %q: %v", block.WorkerID, err)
				continue
			}
			if err := p.sender.SendRawKeys(worker.TmuxSession, 0, "Enter"); err != nil {
				log.Printf("poller: reply routing submit %q: %v", block.WorkerID, err)
			}
		}
	}
}

func conductorUnavailable(conductor db.Session) bool {
	return conductor.TmuxSession == "" || conductor.Status == tmux.StatusStopped || conductor.Status == tmux.StatusError
}

func (p *Poller) clearSessionState(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.lastChange, id)
	delete(p.lastOutput, id)
	delete(p.waitingSince, id)
	delete(p.contextPct, id)
}

func (p *Poller) setContextPct(id string, pct *int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pct == nil {
		delete(p.contextPct, id)
		return
	}
	v := *pct
	p.contextPct[id] = &v
}

func (p *Poller) ContextPctSnapshot() map[string]*int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snap := make(map[string]*int, len(p.contextPct))
	for id, pct := range p.contextPct {
		v := *pct
		snap[id] = &v
	}
	return snap
}

// observeOutput records the latest pane output for a session and returns the
// timestamp of the last time the output actually changed. The returned time is
// used by DetectStatus to decide whether stale "Thinking" / spinner markers
// should still count as activity.
func (p *Poller) observeOutput(id, out string, now, initialLastChange time.Time) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	prev, hadPrev := p.lastOutput[id]
	if !hadPrev {
		p.lastOutput[id] = out
		p.lastChange[id] = initialLastChange
	} else if prev != out {
		p.lastOutput[id] = out
		p.lastChange[id] = now
	} else if _, ok := p.lastChange[id]; !ok {
		p.lastChange[id] = initialLastChange
	}
	return p.lastChange[id]
}

func (p *Poller) initialLastChange(s db.Session, now time.Time) time.Time {
	if activity, err := p.tmux.SessionActivity(s.TmuxSession); err == nil && !activity.IsZero() {
		return activity
	}
	if s.LastActive > 0 {
		return time.Unix(s.LastActive, 0)
	}
	return now
}

func (p *Poller) updateWaitingState(id, previousStatus, newStatus string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if newStatus != tmux.StatusWaiting {
		delete(p.waitingSince, id)
		return
	}
	if _, ok := p.waitingSince[id]; !ok || previousStatus != tmux.StatusWaiting {
		p.waitingSince[id] = now
	}
}

package state

import (
	"database/sql"
	"log"
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
}

type Poller struct {
	conn         *sql.DB
	tmux         TmuxReader
	notifier     waitingNotifier
	now          func() time.Time
	mu           sync.RWMutex
	lastChange   map[string]time.Time
	waitingSince map[string]time.Time
	done         chan struct{}
}

type waitingNotifier interface {
	Enabled() bool
	Style() notify.Style
	Notify(title, body string) error
}

func New(conn *sql.DB, tc TmuxReader) *Poller {
	return NewWithNotifier(conn, tc, notify.New(notify.Config{}))
}

func NewWithNotifier(conn *sql.DB, tc TmuxReader, notifier waitingNotifier) *Poller {
	return NewWithClock(conn, tc, notifier, time.Now)
}

func NewWithClock(conn *sql.DB, tc TmuxReader, notifier waitingNotifier, now func() time.Time) *Poller {
	return &Poller{
		conn:         conn,
		tmux:         tc,
		notifier:     notifier,
		now:          now,
		lastChange:   make(map[string]time.Time),
		waitingSince: make(map[string]time.Time),
		done:         make(chan struct{}),
	}
}

func (p *Poller) Start() {
	go func() {
		ticker := time.NewTicker(time.Second)
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

func (p *Poller) PollOnce() {
	sessions, err := db.ListSessions(p.conn)
	if err != nil {
		log.Printf("poller: list sessions: %v", err)
		return
	}
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

		now := p.now()
		lc := p.lastObservedChange(s.ID, now)

		newStatus := tmux.DetectStatus(out, lc, s.Tool)
		p.updateWaitingState(s.ID, s.Status, newStatus, now)
		if newStatus != s.Status {
			p.setLastChange(s.ID, now)
			if err := db.UpdateSessionStatus(p.conn, s.ID, newStatus); err != nil {
				log.Printf("poller: update status %q: %v", s.ID, err)
			}
			if s.Status != tmux.StatusWaiting && newStatus == tmux.StatusWaiting {
				p.notifyWaiting(s)
			}
		}
	}
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

func (p *Poller) clearSessionState(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.lastChange, id)
	delete(p.waitingSince, id)
}

func (p *Poller) lastObservedChange(id string, now time.Time) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	lc, ok := p.lastChange[id]
	if !ok {
		p.lastChange[id] = now
		return now
	}
	return lc
}

func (p *Poller) setLastChange(id string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastChange[id] = now
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

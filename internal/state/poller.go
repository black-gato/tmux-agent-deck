package state

import (
	"database/sql"
	"log"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

// TmuxReader is the subset of tmux.Client used by the poller.
// Defined here so tests can stub it without importing the real client.
type TmuxReader interface {
	CapturePaneOutput(name string) (string, error)
	SessionExists(name string) (bool, error)
}

type Poller struct {
	conn       *sql.DB
	tmux       TmuxReader
	lastChange map[string]time.Time
	done       chan struct{}
}

func New(conn *sql.DB, tc TmuxReader) *Poller {
	return &Poller{
		conn:       conn,
		tmux:       tc,
		lastChange: make(map[string]time.Time),
		done:       make(chan struct{}),
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

func (p *Poller) PollOnce() {
	sessions, err := db.ListSessions(p.conn)
	if err != nil {
		log.Printf("poller: list sessions: %v", err)
		return
	}
	for _, s := range sessions {
		if s.Status == tmux.StatusStopped || s.TmuxSession == "" {
			delete(p.lastChange, s.ID)
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
			delete(p.lastChange, s.ID)
			continue
		}
		out, err := p.tmux.CapturePaneOutput(s.TmuxSession)
		if err != nil {
			log.Printf("poller: capture pane %q: %v", s.TmuxSession, err)
			continue
		}

		lc, ok := p.lastChange[s.ID]
		if !ok {
			lc = time.Now()
			p.lastChange[s.ID] = lc
		}

		newStatus := tmux.DetectStatus(out, lc, s.Tool)
		if newStatus != s.Status {
			p.lastChange[s.ID] = time.Now()
			if err := db.UpdateSessionStatus(p.conn, s.ID, newStatus); err != nil {
				log.Printf("poller: update status %q: %v", s.ID, err)
			}
		}
	}
}

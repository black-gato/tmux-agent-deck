package state

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/black-gato/tmux-agent-deck/internal/db"
	"github.com/black-gato/tmux-agent-deck/internal/hook"
	"github.com/black-gato/tmux-agent-deck/internal/notify"
	"github.com/black-gato/tmux-agent-deck/internal/tmux"
)

// TmuxReader is the subset of tmux.Client used by the poller.
// Defined here so tests can stub it without importing the real client.
type TmuxReader interface {
	CapturePaneOutput(name string) (string, error)
	CapturePaneView(session string, pane int) (string, error)
	SessionExists(name string) (bool, error)
	SessionActivity(name string) (time.Time, error)
}

type Poller struct {
	conn              *sql.DB
	tmux              TmuxReader
	notifier          waitingNotifier
	sender            TmuxSender
	now               func() time.Time
	interval          time.Duration
	mu                sync.RWMutex
	lastChange        map[string]time.Time
	lastOutput        map[string]string
	waitingSince      map[string]time.Time
	contextPct        map[string]*int
	hooksDir          string
	hookStatus        map[string]hook.HookStatus
	lastHooksMod      time.Time
	lastHooksScan     time.Time
	refresh           func()
	lastNotified      map[string]string
	conductorSeen     map[string]bool // conductor IDs whose pre-existing blocks have been seeded
	processedReplies  map[string]bool
	heartbeatInterval time.Duration
	lastHeartbeat     map[string]time.Time
	done              chan struct{}
	stopOnce          sync.Once
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
		conn:             conn,
		tmux:             tc,
		notifier:         notifier,
		now:              now,
		interval:         interval,
		lastChange:       make(map[string]time.Time),
		lastOutput:       make(map[string]string),
		waitingSince:     make(map[string]time.Time),
		contextPct:       make(map[string]*int),
		hooksDir:         hook.HooksDir(),
		hookStatus:       make(map[string]hook.HookStatus),
		lastNotified:     make(map[string]string),
		conductorSeen:    make(map[string]bool),
		processedReplies: make(map[string]bool),
		lastHeartbeat:    make(map[string]time.Time),
		done:             make(chan struct{}),
	}
}

func (p *Poller) SetSender(s TmuxSender) {
	p.sender = s
}

func (p *Poller) SetConductorHeartbeat(interval time.Duration) {
	p.heartbeatInterval = interval
}

// SetHooksDir overrides the hooks directory. It is primarily used by tests.
func (p *Poller) SetHooksDir(dir string) {
	p.mu.Lock()
	p.hooksDir = dir
	p.lastHooksMod = time.Time{}
	p.lastHooksScan = time.Time{}
	p.mu.Unlock()
}

// SetRefresh registers a callback fired when hook-driven status changes should
// be pushed to the UI immediately.
func (p *Poller) SetRefresh(fn func()) {
	p.mu.Lock()
	p.refresh = fn
	p.mu.Unlock()
}

const hookInterval = 250 * time.Millisecond
const hookForcedRescan = 5 * time.Second

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
	go func() {
		ticker := time.NewTicker(hookInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if p.PollHooksOnce() {
					p.mu.RLock()
					fn := p.refresh
					p.mu.RUnlock()
					if fn != nil {
						fn()
					}
				}
			case <-p.done:
				return
			}
		}
	}()
}

func (p *Poller) Stop() {
	p.stopOnce.Do(func() { close(p.done) })
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

// PollHooksOnce reads changed hook status files behind a directory-mtime gate,
// applies fresh hook status to the DB, and reports whether anything changed.
func (p *Poller) PollHooksOnce() bool {
	p.mu.RLock()
	dir := p.hooksDir
	lastMod := p.lastHooksMod
	lastScan := p.lastHooksScan
	p.mu.RUnlock()

	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	now := p.now()
	forced := lastMod.IsZero() || now.Sub(lastScan) > hookForcedRescan
	if !forced && info.ModTime().Equal(lastMod) {
		return false
	}

	statuses := hook.ListStatuses(dir)
	fresh := make(map[string]hook.HookStatus, len(statuses))
	for id, hs := range statuses {
		if hook.Fresh(hs, now) {
			fresh[id] = hs
		}
	}

	p.mu.Lock()
	p.lastHooksMod = info.ModTime()
	p.lastHooksScan = now
	p.hookStatus = fresh
	p.mu.Unlock()

	sessions, err := db.ListSessions(p.conn)
	if err != nil {
		return false
	}
	changed := false
	for _, s := range sessions {
		hs, ok := fresh[s.ID]
		if !ok || s.Status == tmux.StatusStopped {
			continue
		}
		want := hs.DeckStatus()
		if want == "" || want == s.Status {
			continue
		}
		if _, seen := p.notifiedStatus(s.ID); !seen {
			p.setNotifiedStatus(s.ID, s.Status)
		}
		if err := db.UpdateSessionStatus(p.conn, s.ID, want); err != nil {
			log.Printf("poller: hook status %q: %v", s.ID, err)
			continue
		}
		changed = true
	}
	return changed
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

		paneStatus := tmux.DetectStatus(out, lc, now, s.Tool)
		newStatus := p.effectiveStatus(s.ID, paneStatus)
		p.updateWaitingState(s.ID, s.Status, newStatus, now)
		if newStatus != s.Status {
			if err := db.UpdateSessionStatus(p.conn, s.ID, newStatus); err != nil {
				log.Printf("poller: update status %q: %v", s.ID, err)
			}
		}
		prevNotified, seenNotified := p.notifiedStatus(s.ID)
		if !seenNotified {
			p.setNotifiedStatus(s.ID, newStatus)
			if s.Status == newStatus {
				continue
			}
			prevNotified = s.Status
		}
		if prevNotified != newStatus {
			p.setNotifiedStatus(s.ID, newStatus)
			if prevNotified != tmux.StatusWaiting && newStatus == tmux.StatusWaiting {
				s.Status = newStatus
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
	p.runHeartbeats(sessions)
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

func isClaudeSession(tool string) bool {
	return tool == "claude" || tool == "claude-dangerous"
}

func isInVimInsertMode(paneContent string) bool {
	return strings.Contains(paneContent, "-- INSERT --")
}

// sendToSession sends text to a tmux session and submits with Enter.
// For claude tool sessions it captures the current pane view first and only
// sends Escape+i if the session is NOT already in vim INSERT mode. This
// avoids the ESC disambiguation problem where ESC immediately followed by i
// is read as the escape sequence Meta-i rather than two separate keystrokes.
func (p *Poller) sendToSession(session string, tool string, text string) error {
	if isClaudeSession(tool) {
		view, err := p.tmux.CapturePaneView(session, 0)
		if err != nil || !isInVimInsertMode(view) {
			if err := p.sender.SendRawKeys(session, 0, "Escape"); err != nil {
				return err
			}
			if err := p.sender.SendKeys(session, 0, "i"); err != nil {
				return err
			}
		}
	}
	if err := p.sender.SendKeys(session, 0, text); err != nil {
		return err
	}
	return p.sender.SendRawKeys(session, 0, "Enter")
}

func (p *Poller) autoEscalate(session db.Session, output string) {
	if p.sender == nil {
		return
	}
	conductor, err := db.GetGroupConductorSession(p.conn, session.GroupPath)
	if err == nil && conductor.Title != "" && conductor.ID != session.ID && !conductorUnavailable(conductor) {
		if err := p.sendToSession(conductor.TmuxSession, conductor.Tool, EscalationMessage(session, output)); err != nil {
			log.Printf("poller: auto-escalate send %q: %v", session.ID, err)
		}
		return
	}
	// no group conductor or session is the group conductor — fall through to meta-conductor
	meta, err := db.GetMetaConductorSession(p.conn)
	if err != nil || meta.ID == session.ID || conductorUnavailable(meta) {
		return
	}
	if err := p.sendToSession(meta.TmuxSession, meta.Tool, EscalationMessage(session, output)); err != nil {
		log.Printf("poller: auto-escalate send %q to meta-conductor: %v", session.ID, err)
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
	// include meta-conductor in the set to scan
	meta, err := db.GetMetaConductorSession(p.conn)
	if err == nil && meta.TmuxSession != "" {
		conductorIDs[meta.ID] = true
	}
	for _, s := range sessions {
		if !conductorIDs[s.ID] || s.TmuxSession == "" {
			continue
		}
		out, err := p.tmux.CapturePaneOutput(s.TmuxSession)
		if err != nil {
			continue
		}
		blocks := ParseReplyBlocks(out)
		p.mu.Lock()
		if !p.conductorSeen[s.ID] {
			for _, b := range blocks {
				p.processedReplies[b.WorkerID+":"+b.Body] = true
			}
			p.conductorSeen[s.ID] = true
			p.mu.Unlock()
			continue
		}
		p.mu.Unlock()

		for _, block := range blocks {
			fingerprint := block.WorkerID + ":" + block.Body
			p.mu.Lock()
			if p.processedReplies[fingerprint] {
				p.mu.Unlock()
				continue
			}
			p.processedReplies[fingerprint] = true
			p.mu.Unlock()

			if strings.Contains(block.Body, replyPrefix) {
				log.Printf("poller: reply routing: body for worker %q contains @deck-reply marker, discarding", block.WorkerID)
				continue
			}

			worker, err := db.GetSession(p.conn, block.WorkerID)
			if err != nil {
				log.Printf("poller: reply routing: unknown worker %q", block.WorkerID)
				continue
			}
			if worker.Status == tmux.StatusStopped || worker.Status == tmux.StatusError || worker.TmuxSession == "" {
				log.Printf("poller: reply routing: worker %q not active (status=%s)", block.WorkerID, worker.Status)
				continue
			}
			if err := p.sendToSession(worker.TmuxSession, worker.Tool, block.Body); err != nil {
				log.Printf("poller: reply routing send %q: %v", block.WorkerID, err)
			}
		}
	}
}

const metaHeartbeatKey = "__meta__"

func (p *Poller) runHeartbeats(sessions []db.Session) {
	if p.heartbeatInterval <= 0 || p.sender == nil {
		return
	}
	now := p.now()
	groups, err := db.ListGroups(p.conn)
	if err != nil {
		log.Printf("poller: heartbeat list groups: %v", err)
		return
	}
	sessionMap := make(map[string]db.Session, len(sessions))
	for _, s := range sessions {
		sessionMap[s.ID] = s
	}
	for _, g := range groups {
		if g.ConductorSessionID == "" {
			continue
		}
		conductor, ok := sessionMap[g.ConductorSessionID]
		if !ok || conductor.TmuxSession == "" {
			continue
		}
		p.mu.RLock()
		last := p.lastHeartbeat[g.Path]
		p.mu.RUnlock()
		if last.IsZero() {
			p.mu.Lock()
			p.lastHeartbeat[g.Path] = now
			p.mu.Unlock()
			continue
		}
		if now.Sub(last) < p.heartbeatInterval {
			continue
		}
		groupSessions, err := db.ListGroupChildSessions(p.conn, g.Path, g.ConductorSessionID)
		if err != nil {
			log.Printf("poller: heartbeat list sessions %q: %v", g.Path, err)
			continue
		}
		workers := make([]db.Session, 0, len(groupSessions))
		for _, s := range groupSessions {
			if s.ID != g.ConductorSessionID {
				workers = append(workers, s)
			}
		}
		msg := p.heartbeatMessage(g.Path, workers)
		if err := p.sendToSession(conductor.TmuxSession, conductor.Tool, msg); err != nil {
			log.Printf("poller: heartbeat send %q: %v", g.Path, err)
			continue
		}
		p.mu.Lock()
		p.lastHeartbeat[g.Path] = now
		p.mu.Unlock()
	}
	p.runMetaHeartbeat(sessions, groups, sessionMap, now)
}

func (p *Poller) runMetaHeartbeat(sessions []db.Session, groups []db.Group, sessionMap map[string]db.Session, now time.Time) {
	meta, err := db.GetMetaConductorSession(p.conn)
	if err != nil || meta.TmuxSession == "" || conductorUnavailable(meta) {
		return
	}
	p.mu.RLock()
	last := p.lastHeartbeat[metaHeartbeatKey]
	p.mu.RUnlock()
	if last.IsZero() {
		p.mu.Lock()
		p.lastHeartbeat[metaHeartbeatKey] = now
		p.mu.Unlock()
		return
	}
	if now.Sub(last) < p.heartbeatInterval {
		return
	}
	msg := p.metaHeartbeatMessage(groups, sessionMap, meta.ID)
	if err := p.sendToSession(meta.TmuxSession, meta.Tool, msg); err != nil {
		log.Printf("poller: meta heartbeat send: %v", err)
		return
	}
	p.mu.Lock()
	p.lastHeartbeat[metaHeartbeatKey] = now
	p.mu.Unlock()
}

func (p *Poller) metaHeartbeatMessage(groups []db.Group, sessionMap map[string]db.Session, metaID string) string {
	conductorIDs := make(map[string]bool)
	var conductorLines []string
	for _, g := range groups {
		if g.ConductorSessionID == "" {
			continue
		}
		conductorIDs[g.ConductorSessionID] = true
		c, ok := sessionMap[g.ConductorSessionID]
		if !ok {
			continue
		}
		conductorLines = append(conductorLines, fmt.Sprintf("Group conductor: %s | Status: %s | Group: %s", c.Title, c.Status, g.Path))
	}
	var orphanLines []string
	for _, s := range sessionMap {
		if s.ID == metaID || conductorIDs[s.ID] || s.Status == tmux.StatusStopped {
			continue
		}
		grp, hasGroup := func() (db.Group, bool) {
			for _, g := range groups {
				if g.Path == s.GroupPath {
					return g, true
				}
			}
			return db.Group{}, false
		}()
		if hasGroup && grp.ConductorSessionID != "" {
			continue
		}
		orphanLines = append(orphanLines, fmt.Sprintf("Session: %s | Status: %s | Group: %s", s.Title, s.Status, s.GroupPath))
	}
	parts := []string{fmt.Sprintf("Deck heartbeat | %d groups | %d conductor-less sessions", len(conductorLines), len(orphanLines))}
	parts = append(parts, conductorLines...)
	parts = append(parts, orphanLines...)
	return strings.Join(parts, "\n")
}

func (p *Poller) heartbeatMessage(groupPath string, sessions []db.Session) string {
	var waiting []db.Session
	for _, s := range sessions {
		if s.Status == tmux.StatusWaiting {
			waiting = append(waiting, s)
		}
	}
	if len(waiting) == 0 {
		return fmt.Sprintf("Heartbeat for %s | All clear | %d sessions active", groupPath, len(sessions))
	}
	parts := []string{fmt.Sprintf("Heartbeat for %s | %d waiting", groupPath, len(waiting))}
	p.mu.RLock()
	now := p.now()
	for _, s := range waiting {
		part := fmt.Sprintf("Worker ID: %s | Title: %s", s.ID, s.Title)
		if since, ok := p.waitingSince[s.ID]; ok {
			part += fmt.Sprintf(" | Waiting: %s", now.Sub(since).Round(time.Second))
		}
		if s.Notes != "" {
			part += fmt.Sprintf(" | Notes: %s", s.Notes)
		}
		parts = append(parts, part)
	}
	p.mu.RUnlock()
	return strings.Join(parts, " | ")
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
	delete(p.hookStatus, id)
	delete(p.lastNotified, id)
}

func (p *Poller) effectiveStatus(id, paneStatus string) string {
	p.mu.RLock()
	hs, ok := p.hookStatus[id]
	p.mu.RUnlock()
	if ok {
		if want := hs.DeckStatus(); want != "" {
			return want
		}
	}
	return paneStatus
}

func (p *Poller) notifiedStatus(id string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	status, ok := p.lastNotified[id]
	return status, ok
}

func (p *Poller) setNotifiedStatus(id, status string) {
	p.mu.Lock()
	p.lastNotified[id] = status
	p.mu.Unlock()
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

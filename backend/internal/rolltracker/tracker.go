package rolltracker

import (
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// pendingTTL bounds how long an unmatched EventRollAnnounce stays valid
// while waiting for its EventRollResult partner. EQ always logs the two
// lines back-to-back with identical timestamps, so even a tiny window
// catches every legitimate pair; the cap exists only so a malformed log
// (announce with no result) doesn't poison the next real roll.
const pendingTTL = 2 * time.Second

// maxSessions caps the number of sessions kept in memory. Once exceeded,
// the oldest stopped session is dropped to keep the broadcast payload
// bounded during an extended raid. Active sessions are never evicted.
const maxSessions = 20

// staleAfter is the inactivity window after which a session is auto-stopped.
// Users typically stop sessions manually, but if they forget, we don't want
// rolls from a brand-new drop with the same Max to merge into an old one.
const staleAfter = 5 * time.Minute

// Tracker maintains the live set of /random sessions inferred from the
// EQ log feed. It is safe for concurrent use.
type Tracker struct {
	hub *ws.Hub

	mu       sync.Mutex
	sessions []*Session // newest-first
	rule     WinnerRule

	// pendingRoller holds the name from the most recent EventRollAnnounce
	// while we wait for its matching EventRollResult.
	pendingRoller string
	pendingAt     time.Time
}

// New returns an initialised Tracker with the default WinnerHighest rule.
func New(hub *ws.Hub) *Tracker {
	return &Tracker{
		hub:  hub,
		rule: WinnerHighest,
	}
}

// Handle routes a parsed log event into the tracker. Only roll events do
// anything; the rest are ignored so callers can subscribe Tracker.Handle
// to the same dispatch as every other log consumer.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventRollAnnounce:
		data, ok := ev.Data.(logparser.RollAnnounceData)
		if !ok {
			return
		}
		t.recordAnnounce(data.Roller, ev.Timestamp)
	case logparser.EventRollResult:
		data, ok := ev.Data.(logparser.RollResultData)
		if !ok {
			return
		}
		t.recordResult(data.Min, data.Max, data.Value, ev.Timestamp)
	}
}

func (t *Tracker) recordAnnounce(roller string, ts time.Time) {
	t.mu.Lock()
	t.pendingRoller = roller
	t.pendingAt = ts
	t.mu.Unlock()
}

func (t *Tracker) recordResult(_, max, value int, ts time.Time) {
	t.mu.Lock()
	roller := t.pendingRoller
	pendingAt := t.pendingAt
	t.pendingRoller = ""
	t.pendingAt = time.Time{}
	// Drop an orphan result whose announce is too old — guards against
	// a torn pair where the announce line was lost or already consumed.
	if roller == "" || ts.Sub(pendingAt) > pendingTTL {
		t.mu.Unlock()
		return
	}

	sess := t.sessionForLocked(max, ts)
	dup := false
	for i := range sess.Rolls {
		if sess.Rolls[i].Roller == roller {
			dup = true
			break
		}
	}
	sess.Rolls = append(sess.Rolls, Roll{
		Roller:    roller,
		Value:     value,
		Timestamp: ts,
		Duplicate: dup,
	})
	sess.LastRollAt = ts
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// sessionForLocked returns the active session for max, opening a new one
// if there isn't one or the existing one has gone stale. mu must be held.
func (t *Tracker) sessionForLocked(max int, ts time.Time) *Session {
	for _, s := range t.sessions {
		if s.Max == max && s.Active && ts.Sub(s.LastRollAt) <= staleAfter {
			return s
		}
	}
	s := &Session{
		Max:        max,
		StartedAt:  ts,
		LastRollAt: ts,
		Active:     true,
	}
	// Prepend so newest sessions appear first in the broadcast payload.
	t.sessions = append([]*Session{s}, t.sessions...)
	t.evictOldestStoppedLocked()
	return s
}

// evictOldestStoppedLocked drops the oldest stopped session if we're over
// maxSessions. mu must be held.
func (t *Tracker) evictOldestStoppedLocked() {
	if len(t.sessions) <= maxSessions {
		return
	}
	for i := len(t.sessions) - 1; i >= 0; i-- {
		if !t.sessions[i].Active {
			t.sessions = append(t.sessions[:i], t.sessions[i+1:]...)
			return
		}
	}
}

// Stop marks the session with the given Max inactive. Returns true if a
// matching active session was found.
func (t *Tracker) Stop(max int) bool {
	t.mu.Lock()
	var found bool
	for _, s := range t.sessions {
		if s.Max == max && s.Active {
			s.Active = false
			found = true
			break
		}
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
	return found
}

// Clear removes every session.
func (t *Tracker) Clear() {
	t.mu.Lock()
	t.sessions = nil
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// SetWinnerRule swaps the global winner-selection rule.
func (t *Tracker) SetWinnerRule(rule WinnerRule) {
	if rule != WinnerHighest && rule != WinnerLowest {
		return
	}
	t.mu.Lock()
	if t.rule == rule {
		t.mu.Unlock()
		return
	}
	t.rule = rule
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// State returns a snapshot of the current tracker state, safe to marshal.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateLocked()
}

func (t *Tracker) stateLocked() State {
	out := State{
		WinnerRule: t.rule,
		Sessions:   make([]Session, 0, len(t.sessions)),
	}
	for _, s := range t.sessions {
		rolls := make([]Roll, len(s.Rolls))
		copy(rolls, s.Rolls)
		out.Sessions = append(out.Sessions, Session{
			Max:        s.Max,
			StartedAt:  s.StartedAt,
			LastRollAt: s.LastRollAt,
			Active:     s.Active,
			Rolls:      rolls,
		})
	}
	return out
}

func (t *Tracker) broadcast(state State) {
	if t.hub == nil {
		return
	}
	t.hub.Broadcast(ws.Event{Type: WSEventRolls, Data: state})
}

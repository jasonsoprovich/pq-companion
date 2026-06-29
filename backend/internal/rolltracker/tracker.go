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

	mu              sync.Mutex
	sessions        []*Session // newest-first
	rule            WinnerRule
	mode            Mode
	autoStopSeconds int
	autoStops       map[uint64]*time.Timer // session ID → pending auto-stop
	nextID          uint64

	// pendingRoller holds the name from the most recent EventRollAnnounce
	// while we wait for its matching EventRollResult.
	pendingRoller string
	pendingAt     time.Time

	// onChange, if set, is invoked (outside the lock) whenever the winner
	// rule, mode, or auto-stop window changes, so the caller can persist the
	// new settings. It is never fired by Configure.
	onChange func(rule WinnerRule, mode Mode, autoStopSeconds int)
}

// New returns an initialised Tracker with the default WinnerHighest rule.
func New(hub *ws.Hub) *Tracker {
	return &Tracker{
		hub:             hub,
		rule:            WinnerHighest,
		mode:            ModeManual,
		autoStopSeconds: DefaultAutoStopSeconds,
		autoStops:       make(map[uint64]*time.Timer),
	}
}

// Configure seeds the tracker's settings from persisted preferences. Invalid
// or zero values fall back to the built-in defaults. It does not fire
// onChange — it's meant to be called once at startup before any events flow.
func (t *Tracker) Configure(rule WinnerRule, mode Mode, autoStopSeconds int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rule == WinnerHighest || rule == WinnerLowest {
		t.rule = rule
	}
	if mode == ModeManual || mode == ModeTimer {
		t.mode = mode
	}
	if autoStopSeconds >= 5 && autoStopSeconds <= 600 {
		t.autoStopSeconds = autoStopSeconds
	}
}

// SetOnChange registers a callback fired whenever the winner rule, mode, or
// auto-stop window changes, so settings can be persisted.
func (t *Tracker) SetOnChange(fn func(rule WinnerRule, mode Mode, autoStopSeconds int)) {
	t.mu.Lock()
	t.onChange = fn
	t.mu.Unlock()
}

// notifyChangeLocked snapshots the current settings and schedules the
// onChange callback to run after the lock is released. mu must be held.
func (t *Tracker) notifyChangeLocked() {
	if t.onChange == nil {
		return
	}
	rule, mode, secs := t.rule, t.mode, t.autoStopSeconds
	fn := t.onChange
	go fn(rule, mode, secs)
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

func (t *Tracker) recordResult(min, max, value int, ts time.Time) {
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

	sess := t.sessionForLocked(min, max, ts)
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

// sessionForLocked returns the active session for the min–max range,
// opening a new one if there isn't one or the existing one has gone
// stale. Sessions key on both bounds so a "/random 222 611" roll lands
// in its own 222–611 session instead of the 0–611 one. mu must be held.
func (t *Tracker) sessionForLocked(min, max int, ts time.Time) *Session {
	for _, s := range t.sessions {
		if s.Min == min && s.Max == max && s.Active && ts.Sub(s.LastRollAt) <= staleAfter {
			return s
		}
	}
	t.nextID++
	s := &Session{
		ID:         t.nextID,
		Min:        min,
		Max:        max,
		StartedAt:  ts,
		LastRollAt: ts,
		Active:     true,
	}
	if t.mode == ModeTimer && t.autoStopSeconds > 0 {
		dur := time.Duration(t.autoStopSeconds) * time.Second
		// AutoStopAt is anchored to wall-clock time.Now() rather than the
		// log timestamp ts: the user sees the countdown ticking against
		// the clock on their wall, so a log timestamp pulled from
		// minutes-old backlog would otherwise show as "expired" the
		// instant it appeared. time.AfterFunc is also wall-clock by
		// nature, so both halves agree.
		s.AutoStopAt = time.Now().Add(dur)
		id := s.ID
		t.autoStops[id] = time.AfterFunc(dur, func() { t.fireAutoStop(id) })
	}
	// Prepend so newest sessions appear first in the broadcast payload.
	t.sessions = append([]*Session{s}, t.sessions...)
	t.evictOldestStoppedLocked()
	return s
}

// fireAutoStop is the callback time.AfterFunc invokes when a timer-mode
// session's window expires. It mirrors Stop without requiring the caller
// to know whether the session still exists or has been manually stopped
// in the interim.
func (t *Tracker) fireAutoStop(id uint64) {
	t.mu.Lock()
	delete(t.autoStops, id)
	var found bool
	for _, s := range t.sessions {
		if s.ID == id && s.Active {
			s.Active = false
			s.AutoStopAt = time.Time{}
			found = true
			break
		}
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
}

// cancelAutoStopLocked stops a pending auto-stop timer for the given
// session ID if one is registered. mu must be held.
func (t *Tracker) cancelAutoStopLocked(id uint64) {
	if timer, ok := t.autoStops[id]; ok {
		timer.Stop()
		delete(t.autoStops, id)
	}
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

// Stop marks the session with the given ID inactive. Returns true if a
// matching active session was found.
func (t *Tracker) Stop(id uint64) bool {
	t.mu.Lock()
	var found bool
	for _, s := range t.sessions {
		if s.ID == id && s.Active {
			s.Active = false
			s.AutoStopAt = time.Time{}
			found = true
			break
		}
	}
	if found {
		t.cancelAutoStopLocked(id)
	}
	state := t.stateLocked()
	t.mu.Unlock()
	if found {
		t.broadcast(state)
	}
	return found
}

// Remove deletes the session with the given ID outright. Returns true if a
// matching session was found.
func (t *Tracker) Remove(id uint64) bool {
	t.mu.Lock()
	var found bool
	for i, s := range t.sessions {
		if s.ID == id {
			t.sessions = append(t.sessions[:i], t.sessions[i+1:]...)
			found = true
			break
		}
	}
	if found {
		t.cancelAutoStopLocked(id)
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
	for id, timer := range t.autoStops {
		timer.Stop()
		delete(t.autoStops, id)
	}
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
	t.notifyChangeLocked()
	state := t.stateLocked()
	t.mu.Unlock()
	t.broadcast(state)
}

// SetMode swaps the session-closing mode. Switching modes affects only
// future sessions — currently-active sessions keep their existing
// behavior (a Live session with a timer stays scheduled; a Live manual
// session does not gain a timer). Callers can also pass autoStopSeconds
// to update the timer-mode window in the same call. Pass 0 to leave the
// existing value untouched.
func (t *Tracker) SetMode(mode Mode, autoStopSeconds int) {
	if mode != ModeManual && mode != ModeTimer {
		return
	}
	t.mu.Lock()
	changed := false
	if t.mode != mode {
		t.mode = mode
		changed = true
	}
	if autoStopSeconds > 0 && t.autoStopSeconds != autoStopSeconds {
		t.autoStopSeconds = autoStopSeconds
		changed = true
	}
	if !changed {
		t.mu.Unlock()
		return
	}
	t.notifyChangeLocked()
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
		WinnerRule:      t.rule,
		Mode:            t.mode,
		AutoStopSeconds: t.autoStopSeconds,
		Sessions:        make([]Session, 0, len(t.sessions)),
	}
	for _, s := range t.sessions {
		rolls := make([]Roll, len(s.Rolls))
		copy(rolls, s.Rolls)
		out.Sessions = append(out.Sessions, Session{
			ID:         s.ID,
			Min:        s.Min,
			Max:        s.Max,
			StartedAt:  s.StartedAt,
			LastRollAt: s.LastRollAt,
			Active:     s.Active,
			AutoStopAt: s.AutoStopAt,
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

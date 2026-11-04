package spelltimer

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// The player's buffs fade on the player's own server tick: EQMacEmu
// Client::Process runs ProcessFatigue (which sends OP_Stamina) and then
// BuffProcess (which counts every buff down and fades it at zero) on the same
// tic_timer pass. Zeal times that tick from the OP_Stamina packet and reports
// it as gauge 24, so with the gauge a self-buff's fade can be placed on the
// exact tick instead of anywhere in a 6-second window.

const (
	// serverTickPeriod is the average tick length Zeal measures the gauge
	// against (Zeal tick.cpp kAverageTickDuration).
	serverTickPeriod = 6010 * time.Millisecond

	// tickAnchorAgree is how closely two gauge readings at least
	// tickAnchorSpacing apart must agree on where the tick boundary falls
	// before the phase is trusted. Zeal reads 0 ("tick due now") until the
	// first tick after zoning; readings that always say "now" drift by the
	// time between them and never agree.
	tickAnchorAgree   = 250 * time.Millisecond
	tickAnchorSpacing = time.Second

	// tickAnchorTTL is how long a trusted phase survives without a fresh
	// agreeing reading (pipe paused, zoning, a different character).
	tickAnchorTTL = 3 * time.Second

	// tickSnapMove is the smallest change worth writing to a timer, so tiny
	// gauge jitter doesn't rewrite every timer on every frame.
	tickSnapMove = 150 * time.Millisecond

	// tickSnapLandSlack allows for log timestamps being truncated to the
	// second: the true land moment, and so the formula's expiry, is up to a
	// second after what the log says. Half a second splits the difference:
	// the snap lands one tick off only when a tick boundary falls in that
	// half-second, about one time in twelve, against an error of up to 6 s
	// on every timer without snapping.
	tickSnapLandSlack = 500 * time.Millisecond
)

// tickClock tracks the active player's server tick phase from gauge 24.
type tickClock struct {
	anchor     time.Time // a tick boundary
	confirmed  time.Time // when anchor was last re-confirmed
	candidate  time.Time // unconfirmed boundary from an earlier reading
	candidateT time.Time // when candidate was read
}

// msUntilTick decodes a gauge 24 reading into the time left until the next
// tick. Zeal sends value = ms_until * 1000 / 6010 and text = whole seconds
// left (1-6); its /tickreverse option flips the value but not the text, so
// the direction is whichever reading agrees with the text. ok is false for
// unreadable text.
func msUntilTick(value float64, text string) (time.Duration, bool) {
	secs, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || secs < 1 || secs > 6 {
		return 0, false
	}
	// How far a reading is from the whole-second band the text names. The
	// value is truncated to 1/1000 of a tick, so right at a band edge even
	// the correct direction can miss by a few milliseconds; pick the closer.
	lo := time.Duration(secs-1) * time.Second
	hi := time.Duration(secs) * time.Second
	if secs == 6 {
		hi = serverTickPeriod
	}
	miss := func(d time.Duration) time.Duration {
		switch {
		case d < lo:
			return lo - d
		case d > hi:
			return d - hi
		}
		return 0
	}
	toDur := func(v float64) time.Duration {
		return time.Duration(v * float64(serverTickPeriod) / 1000)
	}
	fwd, rev := toDur(value), toDur(1000-value)
	if miss(rev) < miss(fwd) {
		return rev, true
	}
	return fwd, true
}

// nextTick returns the first tick boundary at or after now, and whether the
// phase is currently trusted.
func (c *tickClock) nextTick(now time.Time) (time.Time, bool) {
	if c.anchor.IsZero() || now.Sub(c.confirmed) > tickAnchorTTL {
		return time.Time{}, false
	}
	d := now.Sub(c.anchor) % serverTickPeriod
	if d < 0 {
		d += serverTickPeriod
	}
	if d == 0 {
		return now, true
	}
	return now.Add(serverTickPeriod - d), true
}

// observe records one gauge reading taken at now.
func (c *tickClock) observe(until time.Duration, now time.Time) {
	b := now.Add(until)
	if c.candidateT.IsZero() || now.Before(c.candidateT) {
		c.candidate, c.candidateT = b, now
		return
	}
	if now.Sub(c.candidateT) < tickAnchorSpacing {
		// Between spaced checks, a reading that agrees with the trusted
		// phase keeps it alive and fresh.
		if !c.anchor.IsZero() && phaseDiff(b, c.anchor) <= tickAnchorAgree {
			c.anchor, c.confirmed = b, now
		}
		return
	}
	if phaseDiff(b, c.candidate) <= tickAnchorAgree {
		c.anchor, c.confirmed = b, now
	}
	c.candidate, c.candidateT = b, now
}

// phaseDiff is the distance between two instants modulo the tick period.
func phaseDiff(a, b time.Time) time.Duration {
	d := a.Sub(b) % serverTickPeriod
	if d < 0 {
		d = -d
	}
	if alt := serverTickPeriod - d; alt < d {
		d = alt
	}
	return d
}

// SetPipeServerTick feeds a Zeal gauge 24 reading for character. Readings
// for any character other than the active one are ignored: the phase is per
// client. Once the phase is trusted, self-buff timers not already placed by
// the pipe's tick count are moved onto the tick they will fade on.
func (e *Engine) SetPipeServerTick(character string, value float64, text string) {
	e.setPipeServerTickAt(character, value, text, time.Now())
}

func (e *Engine) setPipeServerTickAt(character string, value float64, text string, now time.Time) {
	until, ok := msUntilTick(value, text)
	if !ok {
		return
	}
	active := e.activePlayerName()
	if active == "You" || !strings.EqualFold(character, active) {
		return
	}
	e.mu.Lock()
	e.tick.observe(until, now)
	changed := e.snapSelfBuffsToTickLocked(active, now)
	var snap TimerState
	if changed {
		snap = e.snapshot(now)
	}
	e.mu.Unlock()
	if changed {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// snapSelfBuffsToTickLocked places self-buff timers that the pipe has no tick
// count for (formula timers) on the tick they will fade on. Their ExpiresAt is
// the formula's land + duration, which is the latest the fade can be: the
// first of the buff's ticks comes somewhere in the 6 s after it lands. So the
// fade is the last tick boundary at or before that.
//
// The formula value is remembered per timer (tickBase) so repeated snapping
// always starts from it rather than from the previous snap, and a recast or
// refresh (which rewrites ExpiresAt) is noticed and re-based. e.mu must be
// held. Returns true when any timer changed.
func (e *Engine) snapSelfBuffsToTickLocked(active string, now time.Time) bool {
	next, ok := e.tick.nextTick(now)
	if !ok {
		return false
	}
	if e.tickBase == nil {
		e.tickBase = make(map[string]time.Time)
		e.tickSnapped = make(map[string]time.Time)
	}
	changed := false
	for key, t := range e.timers {
		if !isSelfBuff(t, active) || e.pipeTicked[key] {
			continue
		}
		if last, ok := e.tickSnapped[key]; !ok || !t.ExpiresAt.Equal(last) {
			e.tickBase[key] = t.ExpiresAt
		}
		base := e.tickBase[key]
		if !base.After(now) {
			continue
		}
		latest := base.Add(tickSnapLandSlack)
		if latest.Before(next) {
			// Its last tick has already passed; expiry handles it.
			continue
		}
		k := latest.Sub(next) / serverTickPeriod
		fade := next.Add(k * serverTickPeriod)
		if d := fade.Sub(t.ExpiresAt); d > tickSnapMove || d < -tickSnapMove {
			t.ExpiresAt = fade
			changed = true
		}
		e.tickSnapped[key] = t.ExpiresAt
	}
	for key := range e.tickBase {
		if _, ok := e.timers[key]; !ok {
			delete(e.tickBase, key)
			delete(e.tickSnapped, key)
		}
	}
	if changed {
		slog.Debug("timer-debug: self buffs snapped to server tick")
	}
	return changed
}

// pipeTickFade returns the moment a buff with n remaining ticks fades: the
// n-th tick from now, the last of the ticks it has left. ok is false while
// the tick phase isn't trusted. e.mu must be held.
func (e *Engine) pipeTickFadeLocked(n int, now time.Time) (time.Time, bool) {
	next, ok := e.tick.nextTick(now)
	if !ok {
		return time.Time{}, false
	}
	return next.Add(time.Duration(n-1) * serverTickPeriod), true
}

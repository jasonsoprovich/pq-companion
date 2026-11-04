package spelltimer

import (
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// PipeBuff is one occupied buff slot from the Zeal pipe's label message.
//
// Zeal sends the long-buff slots (labels 45-59) with meta.ticks set to the
// client's own remaining tick count for that slot (Zeal/named_pipe.cpp, read
// from char_info Buff[i].Ticks). The short-buff/song slots (labels 135-140)
// carry no ticks, so Ticks is 0 for them and they can confirm a buff is
// present but never say how long it has left.
type PipeBuff struct {
	Name  string
	Ticks int
}

// eqTickDuration is one EverQuest server tick (eqTickSeconds) as a Duration.
const eqTickDuration = time.Duration(eqTickSeconds * float64(time.Second))

const (
	// pipePermanentTicks is the tick count the server gives a permanent
	// (duration formula 50) buff: EQMacEmu CalcBuffDuration_formula returns
	// 0xFFFE. Anything at or above it is not a countdown.
	pipePermanentTicks = 0xFFFE

	// pipeTickSlack widens the window a timer's remaining time may sit in
	// before the pipe corrects it. The client's tick counter steps a moment
	// after the server tick, so right at a boundary the two can disagree by
	// a fraction of a second; without slack a correct timer would be nudged
	// on every boundary.
	pipeTickSlack = 1500 * time.Millisecond

	// pipeFadeConfirm is how long a confirmed buff must stay missing from a
	// non-empty slot snapshot before its timer is ended. Rides out a dropped
	// or partial frame.
	pipeFadeConfirm = 2 * time.Second
)

// SetPipeBuffSlots receives the current buff-slot snapshot from the pipe for
// character (the envelope's character). The pipe is the client's own view of
// its buff window, so for the active character it is ground truth:
//
//   - A self-buff timer whose spell is in a ticked slot is kept inside the
//     remaining-time window the tick count allows. This corrects buffs whose
//     real duration depends on someone else's AAs and focus items, which the
//     duration formula can't know.
//   - A self-buff timer the pipe has confirmed is ended when its spell leaves
//     the buff window (clicked off, dispelled, overwritten).
//
// A snapshot for any other character (the pipe can be connected to a
// different client than the log being read) is only recorded, never applied.
// Pass nil to clear the snapshot (pipe disconnected).
func (e *Engine) SetPipeBuffSlots(character string, buffs []PipeBuff) {
	e.setPipeBuffSlotsAt(character, buffs, time.Now())
}

func (e *Engine) setPipeBuffSlotsAt(character string, buffs []PipeBuff, now time.Time) {
	set := make(map[string]bool, len(buffs))
	ticks := make(map[string]int, len(buffs))
	for _, b := range buffs {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			continue
		}
		set[name] = true
		if b.Ticks > ticks[name] {
			ticks[name] = b.Ticks
		}
	}
	active := e.activePlayerName()
	applies := active != "You" && strings.EqualFold(character, active)

	e.mu.Lock()
	e.pipeBuffSlots = set
	changed := false
	if applies {
		changed = e.reconcilePipeBuffsLocked(active, set, ticks, now)
	}
	missingFromPipe, missingFromTimers := e.pipeDivergenceLocked(active, set)
	pipeKey := divergenceKey(missingFromPipe)
	timersKey := divergenceKey(missingFromTimers)
	logPipe := pipeKey != e.lastDivergenceFromPipe
	logTimers := timersKey != e.lastDivergenceFromTimers
	e.lastDivergenceFromPipe = pipeKey
	e.lastDivergenceFromTimers = timersKey
	var snap TimerState
	if changed {
		snap = e.snapshot(now)
	}
	e.mu.Unlock()

	if changed {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
	if logPipe && len(missingFromPipe) > 0 {
		slog.Info("zealpipe-divergence: timers think buff is active, pipe does not",
			"spells", missingFromPipe)
	}
	if logTimers && len(missingFromTimers) > 0 {
		slog.Info("zealpipe-divergence: pipe has buff slot, timers don't",
			"spells", missingFromTimers)
	}
}

// isSelfBuff reports whether t is a buff timer on the active player,
// i.e. one that should appear in the player's own buff window.
func isSelfBuff(t *ActiveTimer, active string) bool {
	return t.Category == CategoryBuff && (t.TargetName == "You" || t.TargetName == active)
}

// reconcilePipeBuffsLocked applies the pipe snapshot to the active player's
// buff timers. Returns true when any timer changed. e.mu must be held.
func (e *Engine) reconcilePipeBuffsLocked(active string, set map[string]bool, ticks map[string]int, now time.Time) bool {
	if e.pipeConfirmed == nil {
		e.pipeConfirmed = make(map[string]bool)
		e.pipeMissingSince = make(map[string]time.Time)
	}
	changed := false
	for key, t := range e.timers {
		if !isSelfBuff(t, active) {
			continue
		}
		n := ticks[t.SpellName]
		if set[t.SpellName] {
			delete(e.pipeMissingSince, key)
			if n <= 0 || n >= pipePermanentTicks {
				continue
			}
			e.pipeConfirmed[key] = true
			if clampToPipeTicks(t, n, now) {
				changed = true
				slog.Debug("timer-debug: buff synced to Zeal ticks",
					"spell", t.SpellName, "ticks", n,
					"remaining", t.ExpiresAt.Sub(now).Round(time.Second))
			}
			continue
		}
		// Not in the window. An empty window is a zone, camp or a frame with
		// no buff labels, not every buff fading at once.
		if !e.pipeConfirmed[key] || len(set) == 0 || !t.ExpiresAt.After(now) {
			continue
		}
		since, ok := e.pipeMissingSince[key]
		if !ok {
			e.pipeMissingSince[key] = now
			continue
		}
		if now.Sub(since) < pipeFadeConfirm {
			continue
		}
		// Expire it now and let the normal expiry path drop it (or keep it
		// as an overdue row when the user keeps expired timers).
		t.ExpiresAt = now
		delete(e.pipeMissingSince, key)
		changed = true
		slog.Debug("timer-debug: buff left the Zeal buff window", "spell", t.SpellName)
	}
	for key := range e.pipeConfirmed {
		if _, ok := e.timers[key]; !ok {
			delete(e.pipeConfirmed, key)
			delete(e.pipeMissingSince, key)
		}
	}
	return changed
}

// clampToPipeTicks keeps t's remaining time inside the window n remaining
// ticks allows. The buff fades on the tick its counter reaches zero, so with
// n ticks left the true remaining time is between (n-1)*6 and n*6 seconds.
// A timer already inside the window keeps its own sub-tick precision; one
// outside it is moved to the window's midpoint, where it stays inside as both
// count down. Returns true when ExpiresAt changed.
func clampToPipeTicks(t *ActiveTimer, n int, now time.Time) bool {
	hi := time.Duration(n) * eqTickDuration
	lo := hi - eqTickDuration
	remaining := t.ExpiresAt.Sub(now)
	if remaining >= lo-pipeTickSlack && remaining <= hi+pipeTickSlack {
		return false
	}
	t.ExpiresAt = now.Add(hi - eqTickDuration/2)
	if d := t.ExpiresAt.Sub(t.StartsAt).Seconds(); d > 0 {
		t.DurationSeconds = d
	}
	return true
}

// pipeDivergenceLocked lists self-buff timers the pipe doesn't show and pipe
// buffs no timer covers — logged as a data feed for tightening the parser.
func (e *Engine) pipeDivergenceLocked(active string, set map[string]bool) (missingFromPipe, missingFromTimers []string) {
	covered := make(map[string]bool)
	for _, t := range e.timers {
		if !isSelfBuff(t, active) {
			continue
		}
		covered[t.SpellName] = true
		if !set[t.SpellName] {
			missingFromPipe = append(missingFromPipe, t.SpellName)
		}
	}
	for name := range set {
		if !covered[name] {
			missingFromTimers = append(missingFromTimers, name)
		}
	}
	return missingFromPipe, missingFromTimers
}

// divergenceKey returns a stable identifier for a set of spell names so
// SetPipeBuffSlots can detect when a divergence repeats verbatim across
// successive pipe pulses.
func divergenceKey(names []string) string {
	if len(names) == 0 {
		return ""
	}
	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

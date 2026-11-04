// Package regen measures the active character's HP and mana regen per
// server tick from Zeal's live HP/mana labels.
//
// EQ applies regen once per server tick. The tracker keeps the last reading
// taken before each tick boundary (gauge 24 gives the phase) and compares
// consecutive ones, so each pair spans exactly one tick. A pair only counts
// when the tick was clean: nothing hit you or was hit by you, no heal landed
// on you, you didn't cast, zone or die, and max HP/mana didn't change. The
// reported rate is the median of the last few clean ticks.
package regen

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// WSEventRegen is broadcast when a character's measured regen changes.
const WSEventRegen = "regen:state"

const (
	// medianTicks is how many clean ticks the median is taken over.
	medianTicks = 4

	// minTicks is how many clean ticks a rate needs before it is reported.
	minTicks = 2

	// tickGuard keeps readings away from a tick boundary. The phase from
	// gauge 24 is good to a few hundred ms, so a reading right at the
	// boundary may be from either side of the tick.
	tickGuard = 400 * time.Millisecond

	// logLag is how long after a reading a disqualifying log line may still
	// arrive for something that happened before it (log write and tail
	// delay). A pair is judged only once this has passed.
	logLag = 1500 * time.Millisecond
)

// Snapshot is one character's measured regen.
type Snapshot struct {
	Character   string    `json:"character"`
	HPPerTick   *int      `json:"hp_per_tick,omitempty"`
	ManaPerTick *int      `json:"mana_per_tick,omitempty"`
	HPTicks     int       `json:"hp_ticks"`
	ManaTicks   int       `json:"mana_ticks"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type sample struct {
	at                       time.Time
	hp, maxHP, mana, maxMana int
}

type result struct {
	hp, mana  []int // newest last, at most medianTicks
	updatedAt time.Time
}

// Tracker measures regen. Safe for concurrent use.
type Tracker struct {
	hub      *ws.Hub
	nextTick func(now time.Time) (time.Time, bool)
	period   time.Duration

	mu        sync.Mutex
	character string
	cur       *sample   // latest usable reading in the current tick
	curStart  time.Time // start of the tick cur belongs to
	prev      *sample   // last usable reading of the tick before
	prevStart time.Time
	pending   [][2]sample // pairs waiting out logLag
	dirtyAt   []time.Time // disqualifying events not yet aged out
	results   map[string]*result
}

// NewTracker returns a tracker. nextTick reports the next server tick
// boundary and whether the phase is trusted; period is the tick length.
func NewTracker(hub *ws.Hub, nextTick func(time.Time) (time.Time, bool), period time.Duration) *Tracker {
	return &Tracker{hub: hub, nextTick: nextTick, period: period, results: map[string]*result{}}
}

// Handle marks the current tick dirty for log events that move HP or mana
// other than through regen.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	t.handleAt(ev, time.Now())
}

func (t *Tracker) handleAt(ev logparser.LogEvent, now time.Time) {
	switch d := ev.Data.(type) {
	case logparser.CombatHitData:
		if d.Actor != "You" && d.Target != "You" {
			return
		}
	case logparser.CombatMissData:
		if d.Actor != "You" && d.Target != "You" {
			return
		}
	case logparser.HealData:
		if d.Target != "You" {
			return
		}
	default:
		switch ev.Type {
		case logparser.EventSpellCast, logparser.EventZone, logparser.EventDeath:
		default:
			return
		}
	}
	t.mu.Lock()
	t.dirtyAt = append(t.dirtyAt, now)
	t.mu.Unlock()
}

// SetVitals feeds one reading of character's HP and mana. maxMana is 0 for
// a class without mana.
func (t *Tracker) SetVitals(character string, hp, maxHP, mana, maxMana int) {
	t.setVitalsAt(character, hp, maxHP, mana, maxMana, time.Now())
}

func (t *Tracker) setVitalsAt(character string, hp, maxHP, mana, maxMana int, now time.Time) {
	if character == "" || maxHP <= 0 {
		return
	}
	t.mu.Lock()
	if !strings.EqualFold(character, t.character) {
		t.character = character
		t.resetLocked()
	}
	changed := t.judgeLocked(now)

	next, ok := t.nextTick(now)
	if !ok {
		t.resetLocked()
	} else {
		start := next.Add(-t.period)
		if next.Equal(now) {
			start = now
		}
		if t.cur != nil && !sameTick(start, t.curStart, t.period) {
			// The tick cur belongs to has ended: cur is its last reading.
			if t.prev != nil && sameTick(t.prevStart.Add(t.period), t.curStart, t.period) {
				t.pending = append(t.pending, [2]sample{*t.prev, *t.cur})
			}
			t.prev, t.prevStart = t.cur, t.curStart
			t.cur = nil
		}
		if now.Sub(start) >= tickGuard && next.Sub(now) >= tickGuard {
			t.cur = &sample{at: now, hp: hp, maxHP: maxHP, mana: mana, maxMana: maxMana}
			t.curStart = start
		}
	}
	var snap Snapshot
	if changed {
		snap = t.snapshotLocked(t.character)
	}
	t.mu.Unlock()
	if changed && t.hub != nil {
		t.hub.Broadcast(ws.Event{Type: WSEventRegen, Data: snap})
	}
}

// sameTick reports whether two tick starts are the same boundary, allowing
// for the phase estimate moving a little between readings.
func sameTick(a, b time.Time, period time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d < period/2
}

func (t *Tracker) resetLocked() {
	t.cur, t.prev, t.pending = nil, nil, nil
}

// judgeLocked scores pending pairs that have waited out logLag. Returns
// true when a result changed.
func (t *Tracker) judgeLocked(now time.Time) bool {
	changed := false
	keep := t.pending[:0]
	for _, p := range t.pending {
		a, b := p[0], p[1]
		if now.Sub(b.at) < logLag {
			keep = append(keep, p)
			continue
		}
		if t.dirtyBetween(a.at, b.at.Add(logLag)) {
			continue
		}
		r := t.results[t.character]
		if r == nil {
			r = &result{}
			t.results[t.character] = r
		}
		if d, ok := delta(a.hp, b.hp, a.maxHP, b.maxHP); ok {
			r.hp = pushCapped(r.hp, d)
			changed = true
		}
		if d, ok := delta(a.mana, b.mana, a.maxMana, b.maxMana); ok {
			r.mana = pushCapped(r.mana, d)
			changed = true
		}
		if changed {
			r.updatedAt = b.at
		}
	}
	t.pending = keep

	// Drop disqualifying events no pending or future pair can reach.
	cut := 0
	oldest := now.Add(-2*t.period - logLag)
	if len(t.pending) > 0 && t.pending[0][0].at.Before(oldest) {
		oldest = t.pending[0][0].at
	}
	for cut < len(t.dirtyAt) && t.dirtyAt[cut].Before(oldest) {
		cut++
	}
	t.dirtyAt = t.dirtyAt[cut:]
	return changed
}

func (t *Tracker) dirtyBetween(from, to time.Time) bool {
	for _, at := range t.dirtyAt {
		if at.After(from) && !at.After(to) {
			return true
		}
	}
	return false
}

// delta is the regen between two readings of one pool, or ok=false when
// the pair can't measure it: no pool, a max change, a full pool (regen is
// capped there), or a drop (something the log didn't show).
func delta(a, b, maxA, maxB int) (int, bool) {
	if maxA <= 0 || maxA != maxB || a >= maxA || b >= maxB || b < a {
		return 0, false
	}
	return b - a, true
}

func pushCapped(s []int, v int) []int {
	s = append(s, v)
	if len(s) > medianTicks {
		s = s[len(s)-medianTicks:]
	}
	return s
}

func median(s []int) int {
	c := slices.Clone(s)
	slices.Sort(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2] + 1) / 2
}

func (t *Tracker) snapshotLocked(character string) Snapshot {
	snap := Snapshot{Character: character}
	for name, r := range t.results {
		if !strings.EqualFold(name, character) {
			continue
		}
		snap.Character = name
		snap.HPTicks, snap.ManaTicks = len(r.hp), len(r.mana)
		snap.UpdatedAt = r.updatedAt
		if len(r.hp) >= minTicks {
			v := median(r.hp)
			snap.HPPerTick = &v
		}
		if len(r.mana) >= minTicks {
			v := median(r.mana)
			snap.ManaPerTick = &v
		}
	}
	return snap
}

// Snapshot returns character's measured regen this session.
func (t *Tracker) Snapshot(character string) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked(character)
}

package threat

import (
	"sort"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// mobExpiry is how long a mob lingers in the meter after the last hate event
// before it is dropped as stale. Covers the cases that produce no kill line on
// our client (the mob wanders off, someone else finishes it out of range, the
// player gates away mid-pull). Matches the spirit of the combat tracker's
// with-damage fight expiry.
const mobExpiry = 60 * time.Second

// mobState is the running per-mob hate accumulator. Hate is kept as a float so
// the static hatemod multiplier doesn't round on every event; it is rounded
// only at snapshot time.
type mobState struct {
	hate  float64
	first time.Time
	last  time.Time
	timer *time.Timer
}

// Tracker accumulates the active character's estimated personal hate per mob
// from parsed log events and broadcasts a ThreatState snapshot on every change.
//
// It is deliberately self-contained: player-outgoing damage always arrives with
// Actor=="You", so the mob is simply the hit's Target — none of the combat
// tracker's third-party/pet attribution is needed. Keying by the same mob
// display name keeps this overlay aligned with the DPS meter for free.
type Tracker struct {
	hub  *ws.Hub
	calc *Calculator

	mu   sync.Mutex
	mobs map[string]*mobState

	// pipeTarget is the player's current target name from the Zeal pipe, fed in
	// exactly as the combat tracker receives it. Drives which mob the overlay
	// highlights and which mob a no-target spell cast is attributed to.
	pipeTarget string
	// lastEngaged is the most recently damaged mob, used as the highlight and
	// spell-attribution fallback when no pipe target is available.
	lastEngaged string
	// hatemodFn returns the static gear/AA hate modifier as a signed percentage,
	// supplied by the user in settings (logs can't reveal it). Read live so a
	// settings change takes effect immediately; applied to generated hate going
	// forward, never retroactively. May be nil (treated as 0).
	hatemodFn func() int
}

// NewTracker returns an initialised threat Tracker. calc may be nil, in which
// case spell-cast hate (instant hate / aggro shedders) is skipped and the meter
// runs on observed damage alone. hatemodFn may be nil (no static modifier).
func NewTracker(hub *ws.Hub, calc *Calculator, hatemodFn func() int) *Tracker {
	return &Tracker{
		hub:       hub,
		calc:      calc,
		mobs:      make(map[string]*mobState),
		hatemodFn: hatemodFn,
	}
}

// hatemod returns the current static hate modifier percentage (0 when unset).
func (t *Tracker) hatemod() int {
	if t.hatemodFn == nil {
		return 0
	}
	return t.hatemodFn()
}

// SetPipeTarget records the player's current target from Zeal LabelTargetName,
// mirroring combat.Tracker.SetPipeTarget. Changes the highlighted mob; empty
// clears the highlight (falls back to the most recently engaged mob).
func (t *Tracker) SetPipeTarget(name string) {
	t.mu.Lock()
	if name == t.pipeTarget {
		t.mu.Unlock()
		return
	}
	t.pipeTarget = name
	snap := t.snapshotLocked(time.Now())
	t.mu.Unlock()
	t.broadcast(snap)
}

// Handle processes a single parsed log event.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventCombatHit:
		data, ok := ev.Data.(logparser.CombatHitData)
		if !ok || data.Actor != "You" {
			// Only the active character's own outgoing damage generates hate we
			// can attribute. Incoming and third-party hits are ignored.
			return
		}
		t.addHate(data.Target, float64(data.Damage), ev.Timestamp)

	case logparser.EventSpellCast:
		data, ok := ev.Data.(logparser.SpellCastData)
		if !ok {
			return
		}
		t.recordCast(data.SpellName, ev.Timestamp)

	case logparser.EventKill:
		if data, ok := ev.Data.(logparser.KillData); ok {
			t.endMob(data.Target, ev.Timestamp)
		}

	case logparser.EventZone, logparser.EventDeath:
		// Zoning (including gate/egress/evac, which all emit a zone line) and
		// the player's own death both wipe all hate.
		t.endAll(ev.Timestamp)
	}
}

// recordCast applies the non-damage hate of a spell the player just cast. The
// cast line names no target, so the hate is attributed to the current target
// (Zeal pipe) or, failing that, the most recently engaged mob. Spells that
// generate no instant hate (the common case) are a no-op.
func (t *Tracker) recordCast(spellName string, ts time.Time) {
	hate, found := t.calc.CastHate(spellName)
	if !found || hate == 0 {
		return
	}
	t.mu.Lock()
	mob := t.castTargetLocked()
	t.mu.Unlock()
	if mob == "" {
		return
	}
	t.addHate(mob, float64(hate), ts)
}

// castTargetLocked returns the mob a no-target spell cast should be attributed
// to: the live pipe target when set, else the most recently engaged mob.
// Caller must hold t.mu.
func (t *Tracker) castTargetLocked() string {
	if t.pipeTarget != "" {
		return t.pipeTarget
	}
	return t.lastEngaged
}

// addHate adds amount (pre-hatemod) to the named mob's running total, applies
// the static hatemod, (re)arms its expiry timer, and broadcasts. amount may be
// negative (an aggro-shedding spell), which can drive the raw total below zero;
// the displayed value is floored at snapshot time.
func (t *Tracker) addHate(mob string, amount float64, ts time.Time) {
	if mob == "" || mob == "You" {
		return
	}
	adjusted := amount * float64(100+t.hatemod()) / 100

	t.mu.Lock()
	m := t.mobs[mob]
	if m == nil {
		m = &mobState{first: ts}
		t.mobs[mob] = m
	}
	m.hate += adjusted
	if m.first.IsZero() {
		m.first = ts
	}
	m.last = ts
	t.lastEngaged = mob
	t.armExpiryLocked(mob, m)
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// armExpiryLocked (re)starts the per-mob staleness timer. Caller must hold
// t.mu.
func (t *Tracker) armExpiryLocked(mob string, m *mobState) {
	if m.timer != nil {
		m.timer.Stop()
	}
	m.timer = time.AfterFunc(mobExpiry, func() {
		t.endMob(mob, time.Now())
	})
}

// endMob drops a single mob (kill or staleness) and broadcasts the change.
func (t *Tracker) endMob(mob string, ts time.Time) {
	if mob == "" {
		return
	}
	t.mu.Lock()
	m, ok := t.mobs[mob]
	if !ok {
		t.mu.Unlock()
		return
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	delete(t.mobs, mob)
	if t.lastEngaged == mob {
		t.lastEngaged = ""
	}
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// endAll wipes every tracked mob (zone change or player death).
func (t *Tracker) endAll(ts time.Time) {
	t.mu.Lock()
	if len(t.mobs) == 0 {
		t.mu.Unlock()
		return
	}
	for _, m := range t.mobs {
		if m.timer != nil {
			m.timer.Stop()
		}
	}
	t.mobs = make(map[string]*mobState)
	t.lastEngaged = ""
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// Reset clears all accumulated hate without restarting the process (the
// overlay's manual "reset" button). Preserves the configured hatemod and the
// current pipe target — those describe settings/world state, not session hate.
func (t *Tracker) Reset() {
	t.endAll(time.Now())
}

// GetState returns a point-in-time snapshot for the REST endpoint.
func (t *Tracker) GetState() ThreatState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked(time.Now())
}

// snapshotLocked builds an immutable ThreatState from current state. The
// highlighted Target is the pipe target when it is tracked, else the most
// recently engaged mob, else the highest-hate mob. Caller must hold t.mu.
func (t *Tracker) snapshotLocked(now time.Time) ThreatState {
	state := ThreatState{
		InCombat:    len(t.mobs) > 0,
		HatemodPct:  t.hatemod(),
		LastUpdated: now,
		Mobs:        make([]MobThreat, 0, len(t.mobs)),
	}

	highlight := t.highlightLocked()
	for name, m := range t.mobs {
		hate := m.hate
		if hate < 0 {
			hate = 0
		}
		span := m.last.Sub(m.first).Seconds()
		tps := 0.0
		if span >= 1 {
			tps = hate / span
		} else if hate > 0 {
			tps = hate // sub-second engagement: report the burst as-is
		}
		state.Mobs = append(state.Mobs, MobThreat{
			Name:     name,
			Hate:     int64(hate + 0.5),
			TPS:      tps,
			IsTarget: name == highlight,
		})
	}

	sort.Slice(state.Mobs, func(i, j int) bool {
		return state.Mobs[i].Hate > state.Mobs[j].Hate
	})

	for i := range state.Mobs {
		if state.Mobs[i].IsTarget {
			tgt := state.Mobs[i]
			state.Target = &tgt
			break
		}
	}
	return state
}

// highlightLocked returns the mob name to highlight: the pipe target if we hold
// hate on it, else the most recently engaged mob if still tracked, else the
// highest-hate mob. Caller must hold t.mu.
func (t *Tracker) highlightLocked() string {
	if t.pipeTarget != "" {
		if _, ok := t.mobs[t.pipeTarget]; ok {
			return t.pipeTarget
		}
	}
	if t.lastEngaged != "" {
		if _, ok := t.mobs[t.lastEngaged]; ok {
			return t.lastEngaged
		}
	}
	var best string
	var bestHate float64 = -1
	for name, m := range t.mobs {
		if m.hate > bestHate {
			bestHate = m.hate
			best = name
		}
	}
	return best
}

func (t *Tracker) broadcast(state ThreatState) {
	if t.hub == nil {
		return
	}
	t.hub.Broadcast(ws.Event{Type: WSEventThreat, Data: state})
}

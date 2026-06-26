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

// tickDuration is the EQ buff "tick" used to convert a spell's BuffDuration
// (in ticks) to wall-clock for expiring an active hate-mod buff.
const tickDuration = 6 * time.Second

// mobState is the running per-mob hate accumulator. Hate is kept as a float so
// the static hatemod multiplier doesn't round on every event; it is rounded
// only at snapshot time.
type mobState struct {
	hate  float64
	first time.Time
	last  time.Time
	timer *time.Timer

	// meleeSum/meleeCount track this character's landed melee swings on the mob,
	// giving an average swing used to estimate the (otherwise unknown) hate of a
	// melee MISS — the server adds per-swing hate regardless of hit or miss.
	meleeSum   int64
	meleeCount int
}

// modState is one active hate-generation modifier from a self-buff (SPA 114/130,
// e.g. Glamorous Visage -10%, Voice of Terris +10%). pct is the signed percent;
// the timer drops it when the buff's duration elapses.
type modState struct {
	pct   int
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

	// mods holds active hate-generation modifier buffs keyed by spell name.
	// Their percentages add to the static hatemod for every hate value.
	mods map[string]*modState

	// pipeTarget is the player's current target name from the Zeal pipe, fed in
	// exactly as the combat tracker receives it. Drives which mob the overlay
	// highlights and which mob a no-target spell cast is attributed to.
	pipeTarget string
	// lastEngaged is the most recently damaged mob, used as the highlight and
	// spell-attribution fallback when no pipe target is available.
	lastEngaged string
	// hatemodFn returns the static gear/AA hate modifier as a signed percentage,
	// supplied by the user in settings (logs can't reveal it). Read live so a
	// settings change takes effect immediately. May be nil (treated as 0).
	hatemodFn func() int
}

// NewTracker returns an initialised threat Tracker. calc may be nil, in which
// case spell-cast hate (instant/standard hate, aggro shedders, hate-mod buffs)
// is skipped and the meter runs on observed damage alone. hatemodFn may be nil
// (no static modifier).
func NewTracker(hub *ws.Hub, calc *Calculator, hatemodFn func() int) *Tracker {
	return &Tracker{
		hub:       hub,
		calc:      calc,
		mobs:      make(map[string]*mobState),
		mods:      make(map[string]*modState),
		hatemodFn: hatemodFn,
	}
}

// staticHatemod returns the user-configured static hate modifier percentage
// (0 when unset).
func (t *Tracker) staticHatemod() int {
	if t.hatemodFn == nil {
		return 0
	}
	return t.hatemodFn()
}

// effectiveHatemodLocked is the total hate modifier currently in effect: the
// static gear/AA value plus every active hate-mod buff. Caller must hold t.mu.
func (t *Tracker) effectiveHatemodLocked() int {
	total := t.staticHatemod()
	for _, m := range t.mods {
		total += m.pct
	}
	return total
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
		// Melee swings (a verb skill, no spell name) feed the miss-hate average;
		// "spell" hits and DoT ticks do not.
		melee := data.SpellName == "" && data.Skill != "spell"
		t.recordDamage(data.Target, data.Damage, melee, ev.Timestamp)

	case logparser.EventCombatMiss:
		data, ok := ev.Data.(logparser.CombatMissData)
		if !ok || data.Actor != "You" {
			return
		}
		t.recordMiss(data.Target, ev.Timestamp)

	case logparser.EventSpellCast:
		data, ok := ev.Data.(logparser.SpellCastData)
		if !ok {
			return
		}
		t.recordCast(data.SpellName, ev.Timestamp)

	case logparser.EventHeal:
		data, ok := ev.Data.(logparser.HealData)
		if !ok || data.Actor != "You" {
			return
		}
		t.recordHeal(data.Amount, ev.Timestamp)

	case logparser.EventKill:
		if data, ok := ev.Data.(logparser.KillData); ok {
			t.endMob(data.Target, ev.Timestamp)
		}

	case logparser.EventZone, logparser.EventDeath, logparser.EventFeignDeath:
		// Zoning (incl. gate/egress/evac, which emit a zone line), the player's
		// own death, and a successful feign death all wipe the player's hate.
		t.endAll(ev.Timestamp)
	}
}

// recordCast applies the hate of a spell the player just cast. The cast line
// names no target, so any offensive hate is attributed to the current target
// (Zeal pipe) or the most recently engaged mob. Hate-mod self-buffs register an
// active modifier instead of adding hate.
func (t *Tracker) recordCast(spellName string, ts time.Time) {
	t.mu.Lock()
	target := t.castTargetLocked()
	t.mu.Unlock()

	// Classify does DB I/O (NPC HP lookup), so it runs outside the lock.
	info := t.calc.Classify(spellName, target)
	if !info.Found {
		return
	}
	if info.HatemodPct != 0 {
		t.registerMod(spellName, info.HatemodPct, info.DurationTicks, ts)
		return
	}
	if info.OffensiveHate != 0 && target != "" {
		t.addHate(target, float64(info.OffensiveHate), ts)
	}
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

// ensureMobLocked returns the mob's state, creating it on first contact. Caller
// must hold t.mu.
func (t *Tracker) ensureMobLocked(mob string, ts time.Time) *mobState {
	m := t.mobs[mob]
	if m == nil {
		m = &mobState{first: ts}
		t.mobs[mob] = m
	}
	if m.first.IsZero() {
		m.first = ts
	}
	return m
}

// addHateLocked adds amount (pre-hatemod) to a mob's running total, applies the
// effective hatemod, and (re)arms the staleness timer. amount may be negative
// (an aggro shedder); the displayed value is floored at snapshot time. Does NOT
// touch lastEngaged — callers that represent "engaging" a mob set that. Caller
// must hold t.mu.
func (t *Tracker) addHateLocked(mob string, amount float64, ts time.Time) {
	adjusted := amount * float64(100+t.effectiveHatemodLocked()) / 100
	m := t.ensureMobLocked(mob, ts)
	m.hate += adjusted
	m.last = ts
	t.armExpiryLocked(mob, m)
}

// addHate attributes amount to a single mob and marks it as the most recently
// engaged (used for spell offensive hate). Locks, snapshots, broadcasts.
func (t *Tracker) addHate(mob string, amount float64, ts time.Time) {
	if mob == "" || mob == "You" {
		return
	}
	t.mu.Lock()
	t.addHateLocked(mob, amount, ts)
	t.lastEngaged = mob
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordDamage credits observed damage as hate (one point per point) and feeds
// the per-mob melee average used for miss-hate estimation.
func (t *Tracker) recordDamage(mob string, dmg int, melee bool, ts time.Time) {
	if mob == "" || mob == "You" || dmg <= 0 {
		return
	}
	t.mu.Lock()
	t.addHateLocked(mob, float64(dmg), ts)
	if melee {
		m := t.mobs[mob]
		m.meleeSum += int64(dmg)
		m.meleeCount++
	}
	t.lastEngaged = mob
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordMiss adds the per-swing hate of a melee MISS — estimated as this
// character's average landed swing on that mob, since the server adds per-swing
// hate whether or not the swing connects. No-op until at least one swing has
// landed (no basis to estimate) or if the mob isn't tracked.
func (t *Tracker) recordMiss(mob string, ts time.Time) {
	if mob == "" || mob == "You" {
		return
	}
	t.mu.Lock()
	m := t.mobs[mob]
	if m == nil || m.meleeCount == 0 {
		t.mu.Unlock()
		return
	}
	avg := float64(m.meleeSum) / float64(m.meleeCount)
	t.addHateLocked(mob, avg, ts)
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordHeal spreads a heal's hate across every mob currently on the player's
// list — heals aggro everything aggroed on the healer, not just the current
// target. No-op out of combat (no hate list to add to).
func (t *Tracker) recordHeal(amount int, ts time.Time) {
	h := HealHate(amount)
	if h == 0 {
		return
	}
	t.mu.Lock()
	if len(t.mobs) == 0 {
		t.mu.Unlock()
		return
	}
	for mob := range t.mobs {
		t.addHateLocked(mob, float64(h), ts)
	}
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// registerMod activates (or refreshes) a hate-mod self-buff's percentage,
// expiring it after its buff duration. A non-positive duration (e.g. a
// permanent illusion) registers with no timer and is cleared only on
// zone/death/reset.
func (t *Tracker) registerMod(spellName string, pct, durationTicks int, ts time.Time) {
	if pct == 0 || spellName == "" {
		return
	}
	t.mu.Lock()
	m := t.mods[spellName]
	if m == nil {
		m = &modState{}
		t.mods[spellName] = m
	}
	m.pct = pct
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	if durationTicks > 0 {
		m.timer = time.AfterFunc(time.Duration(durationTicks)*tickDuration, func() {
			t.expireMod(spellName)
		})
	}
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// expireMod drops a hate-mod buff when its duration elapses.
func (t *Tracker) expireMod(spellName string) {
	t.mu.Lock()
	if m, ok := t.mods[spellName]; ok {
		if m.timer != nil {
			m.timer.Stop()
		}
		delete(t.mods, spellName)
	}
	snap := t.snapshotLocked(time.Now())
	t.mu.Unlock()
	t.broadcast(snap)
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

// endAll wipes every tracked mob and active hate-mod buff (zone change, player
// death, or feign death).
func (t *Tracker) endAll(ts time.Time) {
	t.mu.Lock()
	if len(t.mobs) == 0 && len(t.mods) == 0 {
		t.mu.Unlock()
		return
	}
	for _, m := range t.mobs {
		if m.timer != nil {
			m.timer.Stop()
		}
	}
	for _, m := range t.mods {
		if m.timer != nil {
			m.timer.Stop()
		}
	}
	t.mobs = make(map[string]*mobState)
	t.mods = make(map[string]*modState)
	t.lastEngaged = ""
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// Reset clears all accumulated hate and active modifiers (the overlay's manual
// "reset" button). Preserves the configured static hatemod and pipe target.
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
		HatemodPct:  t.effectiveHatemodLocked(),
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

func (t *Tracker) broadcast(state ThreatState) {
	if t.hub == nil {
		return
	}
	t.hub.Broadcast(ws.Event{Type: WSEventThreat, Data: state})
}

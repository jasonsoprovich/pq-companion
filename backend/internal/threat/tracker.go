package threat

import (
	"context"
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

// tpsWindow is the rolling window over which the "live" hate-per-second rate is
// measured. Short enough to feel responsive to a burst, long enough not to be
// jumpy between individual swings. See mobState.recent / liveTPSLocked.
const tpsWindow = 6 * time.Second

// castResolveWindow bounds how long a pending spell cast waits for its terminal
// event (land / resist / interrupt / did-not-take-hold) before it is considered
// stale and dropped. A cast that never resolves within this window — a silent
// fizzle, or a land line the parser didn't recognise — generates no hate rather
// than binding to an unrelated later land event. Matches spelltimer's
// lastCastWindow so the two engines treat an in-flight cast consistently.
const castResolveWindow = 30 * time.Second

// feignResidualMinLevel is the NPC level at or above which a successful feign
// death does NOT reliably remove the player from the hate list. The server
// (EntityList::ClearFeignAggro in the SecretsOTheP/EQMacEmu fork) only fully
// clears a mob this level or higher on a random roll; otherwise it leaves the
// player on the list at a residual feignResidualHate. Below this level the
// player is always removed. Mobs of unknown level are treated as below it
// (full clear), matching the meter's pre-residual behaviour.
const feignResidualMinLevel = 35

// feignResidualHate is the leftover hate the server stamps on a not-cleared
// feign (SetHate(target, 64)). Tiny — it keeps the mob on the meter to show the
// player is still on its hate list (at the bottom), rather than safely off it.
const feignResidualHate = 64

// hateSample is one post-hatemod hate increment with the time it landed, kept
// in a short rolling buffer per mob to compute the live (windowed) rate.
type hateSample struct {
	ts  time.Time
	amt float64
}

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

	// recent holds the last few seconds of post-hatemod hate increments (oldest
	// first), used to derive the live rolling-window rate. Expired entries are
	// pruned lazily when the rate is read.
	recent []hateSample
}

// modState is one active hate-generation modifier from a self-buff (SPA 114/130,
// e.g. Glamorous Visage -10%, Voice of Terris +10%). pct is the signed percent;
// the timer drops it when the buff's duration elapses.
type modState struct {
	pct   int
	timer *time.Timer
}

// pendingCast holds a spell the player has begun casting but whose hate has not
// yet been applied. The hate is committed only when the cast actually resolves
// (lands, or is resisted — a resisted detrimental spell still generates aggro),
// never at "You begin casting", so an interrupted or fizzled cast adds nothing.
// Only one cast is ever in flight (EQ casts serially), so a single slot suffices;
// a new cast replaces any unresolved prior pending. Exactly one of offensiveHate
// / hatemodPct is non-zero, mirroring Calculator.Classify.
type pendingCast struct {
	spellName     string
	target        string // mob the offensive hate lands on (captured at cast time)
	offensiveHate int64  // instant + standard hate; applied on land/resist
	hatemodPct    int    // self-buff hate modifier; registered on land only
	durationTicks int
	at            time.Time
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

	// pending is the spell currently being cast, awaiting its resolve event.
	// Its hate is held here until the cast lands or is resisted; nil when no
	// hate-relevant cast is in flight. See pendingCast.
	pending *pendingCast

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

	// nowFn is the receive clock for the live (rolling-window) rate: hate
	// samples are stamped with it and the window is measured against it. It is
	// deliberately NOT the event's log timestamp — both the live tailer and the
	// replayer feed events in real wall-clock order, so a wall clock gives a
	// consistent window in both modes (a replayed historical log would otherwise
	// always read zero, its samples being months older than time.Now()).
	// Overridable in tests. Defaults to time.Now.
	nowFn func() time.Time
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
		nowFn:     time.Now,
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
		// Hate is not applied here ("You begin casting") — only recorded as
		// pending. It commits when the cast resolves below.
		t.recordCast(data.SpellName, ev.Timestamp)

	case logparser.EventSpellLanded:
		// The cast took hold: apply our own pending offensive/buff hate.
		t.commitPending(ev.Timestamp, commitLand)
		// A hate-mod buff landing ON us also registers — this is the only signal
		// for one cast by ANOTHER player (no local "You begin casting" line).
		if data, ok := ev.Data.(logparser.SpellLandedData); ok &&
			data.Kind == logparser.SpellLandedKindYou {
			t.registerLandedBuff(data, ev.Timestamp)
		}

	case logparser.EventSpellResist, logparser.EventSpellDidNotTakeHold:
		// Resisted or immune: the spell still hit the mob, so its offensive
		// (aggro) component lands, but a beneficial hate-mod buff never took
		// hold and is dropped.
		t.commitPending(ev.Timestamp, commitAggroOnly)

	case logparser.EventSpellInterrupt:
		// Cast aborted before completion — no hate at all.
		t.commitPending(ev.Timestamp, commitDrop)

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

	case logparser.EventZone, logparser.EventDeath:
		// Zoning (incl. gate/egress/evac, which emit a zone line) and the
		// player's own death remove the player from every hate list entirely.
		t.endAll(ev.Timestamp)

	case logparser.EventFeignDeath:
		// A successful feign is NOT a clean wipe on raid mobs — see feignDeath.
		t.feignDeath(ev.Timestamp)
	}
}

// recordCast records a spell the player just began casting as pending, WITHOUT
// applying its hate. The hate is committed later, when the cast resolves (see
// commitPending) — so an interrupted or fizzled cast generates nothing. The cast
// line names no target, so any offensive hate is attributed to the current
// target (Zeal pipe) or the most recently engaged mob, captured now.
func (t *Tracker) recordCast(spellName string, ts time.Time) {
	t.mu.Lock()
	target := t.castTargetLocked()
	t.mu.Unlock()

	// Classify does DB I/O (NPC HP lookup), so it runs outside the lock.
	info := t.calc.Classify(spellName, target)

	t.mu.Lock()
	defer t.mu.Unlock()
	// A new cast supersedes any unresolved prior pending (EQ casts serially), so
	// even a cast that generates no hate clears the slot — a stale pending must
	// not survive to bind to this cast's later resolve event.
	if !info.Found || (info.OffensiveHate == 0 && info.HatemodPct == 0) {
		t.pending = nil
		return
	}
	t.pending = &pendingCast{
		spellName:     spellName,
		target:        target,
		offensiveHate: info.OffensiveHate,
		hatemodPct:    info.HatemodPct,
		durationTicks: info.DurationTicks,
		at:            ts,
	}
}

// commitMode selects how a resolved cast's pending hate is applied.
type commitMode int

const (
	// commitLand — the cast took hold: apply offensive hate and/or register a
	// hate-mod self-buff.
	commitLand commitMode = iota
	// commitAggroOnly — the cast resolved without taking hold (resisted or
	// immune): the offensive aggro component still lands, but a beneficial
	// hate-mod buff did not, so it is not registered.
	commitAggroOnly
	// commitDrop — the cast was interrupted before completion: nothing applies.
	commitDrop
)

// commitPending applies (or discards) the spell whose cast is awaiting a resolve
// event, per mode, then clears the slot. A pending older than castResolveWindow
// is dropped untouched — its resolve line was missed, so this event belongs to a
// different (untracked) cast rather than the one we recorded.
func (t *Tracker) commitPending(ts time.Time, mode commitMode) {
	t.mu.Lock()
	p := t.pending
	t.pending = nil
	if p == nil || ts.Sub(p.at) > castResolveWindow {
		t.mu.Unlock()
		return
	}

	changed := false
	if mode != commitDrop {
		switch {
		case p.hatemodPct != 0:
			// A hate-mod self-buff applies only on a true land; a resist/immune
			// means it never took hold.
			if mode == commitLand {
				t.registerModLocked(p.spellName, p.hatemodPct, p.durationTicks)
				changed = true
			}
		case p.offensiveHate != 0 && p.target != "":
			// Offensive (aggro) hate lands on a successful cast AND on a resist —
			// a resisted detrimental spell still adds aggro.
			t.addHateLocked(p.target, float64(p.offensiveHate), ts)
			t.lastEngaged = p.target
			changed = true
		}
	}

	if !changed {
		t.mu.Unlock()
		return
	}
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
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
	// Stamp with the receive clock (not the event ts) so the live window is
	// wall-clock consistent in both live-tail and replay modes — see nowFn.
	m.recent = append(m.recent, hateSample{ts: t.nowFn(), amt: adjusted})
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

// registerModLocked activates (or refreshes) a hate-mod self-buff's percentage,
// expiring it after its buff duration. A non-positive duration (e.g. a permanent
// illusion) registers with no timer and is cleared only on zone/death/reset.
// Caller must hold t.mu and is responsible for snapshotting/broadcasting.
func (t *Tracker) registerModLocked(spellName string, pct, durationTicks int) {
	if pct == 0 || spellName == "" {
		return
	}
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
}

// registerLandedBuff registers a hate-generation buff (Voice of Terris,
// Glamorous Visage, ...) that LANDED on the active player. Its main purpose is
// to catch a buff cast by ANOTHER player — that produces no local "You begin
// casting" line, so the pending-cast path never sees it and the land-on-you
// event is the only signal. For the player's own self-buffs this is idempotent
// with the pending path (registerModLocked refreshes the same entry). The buff
// duration is the spell's listed value (the real one scales with the unknown
// caster's level — an accepted estimate, same as for self-casts).
func (t *Tracker) registerLandedBuff(data logparser.SpellLandedData, ts time.Time) {
	if t.calc == nil {
		return
	}
	for _, name := range candidateNames(data) {
		pct, dur, ok := t.calc.HateModBuff(name)
		if !ok {
			continue
		}
		t.mu.Lock()
		t.registerModLocked(name, pct, dur)
		snap := t.snapshotLocked(ts)
		t.mu.Unlock()
		t.broadcast(snap)
		return
	}
}

// candidateNames returns the spell name(s) a landed-spell event may refer to:
// the resolved name when the cast text is unambiguous, else every candidate
// that shares the ambiguous text.
func candidateNames(data logparser.SpellLandedData) []string {
	if data.SpellName != "" {
		return []string{data.SpellName}
	}
	names := make([]string, 0, len(data.Candidates))
	for _, c := range data.Candidates {
		names = append(names, c.SpellName)
	}
	return names
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
	if len(t.mobs) == 0 && len(t.mods) == 0 && t.pending == nil {
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
	t.pending = nil
	t.lastEngaged = ""
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// npcLevel returns the tracked mob's database level, or 0 (unknown) when no
// calculator/NPC source is wired.
func (t *Tracker) npcLevel(mob string) int {
	if t.calc == nil {
		return 0
	}
	return t.calc.NPCLevel(mob)
}

// feignDeath models a successful feign on the player's hate. Mirrors
// EntityList::ClearFeignAggro: a mob at/above feignResidualMinLevel keeps the
// player on its hate list at feignResidualHate (the server's clear roll is
// unobservable, so the meter assumes the common not-cleared outcome — better to
// show "still on the list at the bottom" than a false all-clear); a lower or
// unknown-level mob is fully removed. An in-flight cast is interrupted (pending
// dropped); hate-mod self-buffs persist through feign, so mods are kept.
func (t *Tracker) feignDeath(ts time.Time) {
	// Resolve levels outside the lock — NPCLevel may do DB I/O.
	t.mu.Lock()
	names := make([]string, 0, len(t.mobs))
	for name := range t.mobs {
		names = append(names, name)
	}
	hadPending := t.pending != nil
	t.mu.Unlock()

	if len(names) == 0 && !hadPending {
		return
	}

	levels := make(map[string]int, len(names))
	for _, name := range names {
		levels[name] = t.npcLevel(name)
	}

	t.mu.Lock()
	t.pending = nil
	for name, m := range t.mobs {
		if levels[name] >= feignResidualMinLevel {
			// Residual: still on the hate list, reset to the bottom.
			m.hate = feignResidualHate
			m.recent = nil
			m.meleeSum, m.meleeCount = 0, 0
			continue
		}
		// Fully cleared.
		if m.timer != nil {
			m.timer.Stop()
		}
		delete(t.mobs, name)
		if t.lastEngaged == name {
			t.lastEngaged = ""
		}
	}
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

// PersonalHate returns the active character's current estimated hate per mob
// (display name → hate, floored at zero), for the raid threat estimator to
// splice in as the high-fidelity "You" row. Read-only snapshot.
func (t *Tracker) PersonalHate() map[string]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]int64, len(t.mobs))
	for name, m := range t.mobs {
		h := m.hate
		if h < 0 {
			h = 0
		}
		out[name] = int64(h + 0.5)
	}
	return out
}

// RunLiveTicker re-snapshots and rebroadcasts on a fixed interval while at
// least one mob is tracked, so the live (rolling-window) rate visibly decays
// toward zero between log events instead of freezing at its last value. Idle
// (no tracked mobs) ticks broadcast nothing. Blocks until ctx is cancelled;
// run it in its own goroutine.
func (t *Tracker) RunLiveTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.mu.Lock()
			if len(t.mobs) == 0 {
				t.mu.Unlock()
				continue
			}
			snap := t.snapshotLocked(t.nowFn())
			t.mu.Unlock()
			t.broadcast(snap)
		}
	}
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
			Name: name,
			Hate: int64(hate + 0.5),
			TPS:  tps,
			// Live rate is measured on the receive clock (nowFn), independent of
			// the snapshot's event-time `now`, so event-driven and ticker-driven
			// snapshots agree.
			LiveTPS:  liveTPSLocked(m, t.nowFn()),
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

// liveTPSLocked returns the mob's recent rolling-window hate rate: hate added
// in the last tpsWindow seconds divided by that window. It prunes expired
// samples in place (they arrive in time order). Floored at zero so an
// aggro-shedder in the window can't show a negative rate. Caller must hold
// t.mu.
func liveTPSLocked(m *mobState, now time.Time) float64 {
	cutoff := now.Add(-tpsWindow)
	i := 0
	for i < len(m.recent) && m.recent[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		m.recent = m.recent[i:]
	}
	var sum float64
	for _, s := range m.recent {
		sum += s.amt
	}
	if sum < 0 {
		sum = 0
	}
	return sum / tpsWindow.Seconds()
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

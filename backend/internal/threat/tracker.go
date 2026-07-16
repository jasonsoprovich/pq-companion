package threat

import (
	"context"
	"sort"
	"strings"
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

// rogueEvadeExpectedPct / rogueEvadeMinHate model Mob::RogueEvade
// (SecretsOTheP/EQMacEmu zone/aggro.cpp): a successful Rogue Evade ("You duck
// away from the main combat.") sets the rogue's hate on the current target to
// a random 40-70% of its prior value, floored at 100. The per-attempt roll
// isn't observable from the log (the success message carries no number), so
// the meter uses the range's midpoint as its best single-shot estimate.
const (
	rogueEvadeExpectedPct = 55
	rogueEvadeMinHate     = 100
)

// tauntBump is the hate a successful taunt adds above the current top of the
// list (EQMacEmu Mob::Taunt: myHate = topHate + 10). Mirrors raidthreat's
// constant of the same name/value.
const tauntBump = 10

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
	// expiryGen increments each time the staleness timer is (re)armed. The
	// timer callback captures the value it was armed with and only ends the mob
	// if it still matches — so a timer that had already fired and was queued
	// behind t.mu when fresh activity re-armed the expiry doesn't delete a mob
	// that's actively being refreshed.
	expiryGen uint64

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
	// gen increments each time the mod is (re)registered; expireMod only drops
	// the mod if its captured gen still matches, so a recast that refreshed the
	// buff isn't deleted by a stale, already-queued expiry timer.
	gen uint64
}

// pendingCast holds a spell the player has begun casting but whose hate has not
// yet been applied. The hate is committed only when the cast actually resolves
// (lands, or is resisted — a resisted detrimental spell still generates aggro),
// never at "You begin casting", so an interrupted or fizzled cast adds nothing.
// Only one cast is ever in flight (EQ casts serially), so a single slot suffices;
// a new cast replaces any unresolved prior pending. A hate-mod buff carries only
// hatemodPct; every other cast carries offensiveHate and/or damageHate, mirroring
// Calculator.Classify.
type pendingCast struct {
	spellName     string
	target        string // mob the offensive hate lands on (captured at cast time)
	offensiveHate int64  // instant + flat + standard hate; applied on land/resist
	damageHate    int64  // base direct damage; applied on land/resist (not crits/resist-scaled)
	directDamage  bool   // resolves from its own damage line or a full resist, not a land message
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

	// meleeSwingHateFn returns the flat hate of a single melee swing — the
	// equipped weapon's damage rating plus the primary-hand bonus, identical on
	// every swing regardless of the damage rolled. 0 means the weapon/level isn't
	// known, in which case the meter falls back to observed damage. May be nil.
	// Supplied by the wiring layer (it reads the Zeal inventory + character row).
	meleeSwingHateFn func() int

	// backstabHateFn returns the flat hate of a single backstab — its base
	// damage ((skill*0.02)+2)*weaponDamage, not the large rolled number — from
	// the equipped primary piercer. 0 means the weapon isn't a known piercer, in
	// which case a backstab line falls back to observed damage. May be nil.
	backstabHateFn func() int

	// topHateFn returns the best-known current hate on a mob held by anyone
	// OTHER than the active character (an estimate derived from raid-wide
	// observed damage), used to model a successful Taunt's real "topHate + 10"
	// effect. This tracker only ever sees the active character's own outgoing
	// damage, so without this it has no way to know what "the top" even is.
	// 0 (or a nil func) means no raid-wide visibility is wired, in which case a
	// taunt still contributes tauntBump above the character's own prior hate
	// rather than nothing. May be nil.
	topHateFn func(mob string) int64

	// selfNameFn returns the active character's display name, so an
	// EventTaunt (whose Taunter names whoever taunted, not necessarily us) can
	// be recognised as our own action rather than another raid member's. May
	// be nil, in which case taunts are never attributed to us.
	selfNameFn func() string

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

// SetMeleeSwingHateFn installs the flat per-swing melee-hate provider. Safe to
// call once at wiring time; the provider is read live on each swing.
func (t *Tracker) SetMeleeSwingHateFn(fn func() int) {
	t.mu.Lock()
	t.meleeSwingHateFn = fn
	t.mu.Unlock()
}

// meleeSwingHate returns the current flat per-swing melee hate, or 0 when no
// provider is wired or the equipped weapon is unknown. Called WITHOUT t.mu held
// (the provider may do its own I/O), so it reads meleeSwingHateFn under a brief
// lock and invokes it outside.
func (t *Tracker) meleeSwingHate() int {
	t.mu.Lock()
	fn := t.meleeSwingHateFn
	t.mu.Unlock()
	if fn == nil {
		return 0
	}
	return fn()
}

// SetBackstabHateFn installs the flat backstab-hate provider (see backstabHateFn).
func (t *Tracker) SetBackstabHateFn(fn func() int) {
	t.mu.Lock()
	t.backstabHateFn = fn
	t.mu.Unlock()
}

// backstabHate returns the current flat backstab hate, or 0 when no provider is
// wired or the equipped primary isn't a known piercer. Read outside t.mu (the
// provider may do I/O), like meleeSwingHate.
func (t *Tracker) backstabHate() int {
	t.mu.Lock()
	fn := t.backstabHateFn
	t.mu.Unlock()
	if fn == nil {
		return 0
	}
	return fn()
}

// SetTopHateFn installs the raid-wide top-hate estimator (see topHateFn).
func (t *Tracker) SetTopHateFn(fn func(mob string) int64) {
	t.mu.Lock()
	t.topHateFn = fn
	t.mu.Unlock()
}

// topHate returns the current best-known top hate on mob held by someone
// other than us, and whether a provider is wired at all — distinct from a
// wired provider genuinely reporting 0 (nobody else has engaged yet), which
// is a real, usable value. Read outside t.mu, like meleeSwingHate (the
// provider reads the combat tracker's own locked state).
func (t *Tracker) topHate(mob string) (top int64, ok bool) {
	t.mu.Lock()
	fn := t.topHateFn
	t.mu.Unlock()
	if fn == nil {
		return 0, false
	}
	return fn(mob), true
}

// SetSelfNameFn installs the active-character-name provider (see selfNameFn).
func (t *Tracker) SetSelfNameFn(fn func() string) {
	t.mu.Lock()
	t.selfNameFn = fn
	t.mu.Unlock()
}

// isSelf reports whether name is the active character, per selfNameFn.
func (t *Tracker) isSelf(name string) bool {
	t.mu.Lock()
	fn := t.selfNameFn
	t.mu.Unlock()
	if fn == nil || name == "" {
		return false
	}
	self := fn()
	return self != "" && self == name
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
	name = logparser.CanonicalNPCName(name)
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
		switch {
		case data.Skill == "spell":
			// Direct spell damage — our own nuke, or a weapon proc. Hate is the
			// spell's BASE damage, resolved here (see recordSpellDamage), NOT the
			// observed amount a crit or partial resist would distort.
			t.recordSpellDamage(data.Target, data.Damage, ev.Timestamp)
		case strings.EqualFold(data.Skill, "backstab"):
			// Backstab: hate is its flat base damage, not the large rolled number.
			t.recordBackstab(data.Target, data.Damage, ev.Timestamp)
		case data.SpellName == "":
			// Melee swing (a verb skill). Feeds the miss-hate average.
			t.recordDamage(data.Target, data.Damage, true, ev.Timestamp)
		default:
			// DoT tick (Skill=="dot"): per-tick hate from observed damage,
			// matching DoBuffTic (ticks add base damage, no crit/variance here).
			t.recordDamage(data.Target, data.Damage, false, ev.Timestamp)
		}

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

	case logparser.EventRogueEvade:
		t.recordRogueEvade(ev.Timestamp)

	case logparser.EventTaunt:
		data, ok := ev.Data.(logparser.TauntData)
		if !ok || data.Mob == "" || !t.isSelf(data.Taunter) {
			// Not a landed taunt we can attribute to the active character —
			// someone else's successful taunt doesn't change OUR hate.
			return
		}
		t.recordTaunt(data.Mob, ev.Timestamp)
	}
}

// recordCast records a spell the player just began casting as pending, WITHOUT
// applying its hate. The hate is committed later, when the cast resolves (see
// commitPending) — so an interrupted or fizzled cast generates nothing. The cast
// line names no target, so any offensive hate is attributed to the current
// target (Zeal pipe) or the most recently engaged mob, captured now.
//
// EQ casts serially, so beginning a new cast means any still-unresolved prior
// pending has already resolved server-side — its own land/resist text just
// hasn't reached the log yet (a spell's resolution message can lag behind the
// next cast starting, e.g. Concussion's aggro-shed landing after the wizard
// has already begun their next nuke). A non-direct-damage pending has no other
// resolution path waiting for it, so it is credited now as an aggro-only land
// rather than silently losing its hate, mirroring commitPending's
// resist/immune treatment (offensive hate lands; a hate-mod buff is not
// registered, since we never saw it actually take hold). A direct-damage
// pending is left alone for its own damage line (recordSpellDamage) to
// resolve, so a same-window nuke isn't double-counted.
func (t *Tracker) recordCast(spellName string, ts time.Time) {
	t.mu.Lock()
	target := t.castTargetLocked()
	t.mu.Unlock()

	// Classify does DB I/O (NPC HP lookup), so it runs outside the lock.
	info := t.calc.Classify(spellName, target)

	t.mu.Lock()
	changed := false
	if old := t.pending; old != nil && !old.directDamage && ts.Sub(old.at) <= castResolveWindow {
		changed = t.applyPendingLocked(old, ts, commitAggroOnly)
	}

	if !info.Found || (info.OffensiveHate == 0 && info.DamageHate == 0 && info.HatemodPct == 0) {
		t.pending = nil
	} else {
		t.pending = &pendingCast{
			spellName:     spellName,
			target:        target,
			offensiveHate: info.OffensiveHate,
			damageHate:    info.DamageHate,
			directDamage:  info.DirectDamage,
			hatemodPct:    info.HatemodPct,
			durationTicks: info.DurationTicks,
			at:            ts,
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
	if p == nil || ts.Sub(p.at) > castResolveWindow {
		t.pending = nil
		t.mu.Unlock()
		return
	}
	// A direct-damage cast is resolved by its own damage line (recordSpellDamage)
	// or by a full resist — never by a generic "lands" message, which some damage
	// spells also emit. Ignoring commitLand here keeps that pending intact so the
	// damage line resolves it once, instead of counting it on both events.
	if p.directDamage && mode == commitLand {
		t.mu.Unlock()
		return
	}
	t.pending = nil
	changed := t.applyPendingLocked(p, ts, mode)

	if !changed {
		t.mu.Unlock()
		return
	}
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// applyPendingLocked applies a resolved pending cast's hate per mode and
// reports whether the tracker's displayed state changed. Shared by
// commitPending (a real land/resist/interrupt event) and recordCast (a new
// cast implicitly superseding an unresolved one). Caller must hold t.mu.
func (t *Tracker) applyPendingLocked(p *pendingCast, ts time.Time, mode commitMode) bool {
	if mode == commitDrop {
		return false
	}
	switch {
	case p.hatemodPct != 0:
		// A hate-mod self-buff applies only on a true land; a resist/immune
		// (or an implicit supersede, where we never saw it resolve) means it
		// never took hold.
		if mode == commitLand {
			t.registerModLocked(p.spellName, p.hatemodPct, p.durationTicks)
			return true
		}
		return false
	default:
		// Offensive (aggro) + base damage hate. Both land on a successful cast
		// AND on a full resist — a resisted detrimental spell still adds the
		// same CheckAggroAmount hate (EQMacEmu ResistSpell).
		hate := p.offensiveHate + p.damageHate
		if hate != 0 && p.target != "" {
			// The hate modifier only ever scales a POSITIVE total: EQMacEmu's
			// spells.cpp SpellOnTarget routes a cast's aggro through
			// AddToHateList — where aabonuses/spellbonuses/itembonuses.hatemod
			// (Spell Casting Subtlety AA, Glamorous Visage, ...) applies — only
			// when CheckAggroAmount's return is > 0. A non-positive total (an
			// aggro shedder like Concussion, whose SE_InstantHate is itself
			// "non_modified_aggro" inside CheckAggroAmount and untouched by
			// SpellAggroMod/focus either) instead goes through
			// SetHateAmountOnEnt directly, bypassing the modifier entirely.
			t.addHateLocked(p.target, float64(hate), ts, hate > 0) // spell hate
			t.lastEngaged = p.target
			return true
		}
		return false
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

// addHateLocked adds amount (pre-hatemod) to a mob's running total and (re)arms
// the staleness timer. When applyHatemod is true the effective hate modifier
// (Spell Casting Subtlety AA + hate-mod buffs + manual gear %) scales the amount;
// the server applies that modifier to spell and heal hate ONLY, never to melee
// swings, so melee callers pass false. It also never applies to a non-positive
// total (see applyPendingLocked/recordSpellDamage) — callers pass applyHatemod
// = amount > 0 for spell hate so an aggro-shedding total like Concussion's
// -600 is credited unscaled, matching the server bypassing AddToHateList for a
// non-positive CheckAggroAmount result. amount may still be negative (an aggro
// shedder); the displayed value is floored at snapshot time. Does NOT touch
// lastEngaged — callers that represent "engaging" a mob set that. Caller must
// hold t.mu.
func (t *Tracker) addHateLocked(mob string, amount float64, ts time.Time, applyHatemod bool) {
	adjusted := amount
	if applyHatemod {
		adjusted = amount * float64(100+t.effectiveHatemodLocked()) / 100
	}
	m := t.ensureMobLocked(mob, ts)
	m.hate += adjusted
	m.last = ts
	// Stamp with the receive clock (not the event ts) so the live window is
	// wall-clock consistent in both live-tail and replay modes — see nowFn.
	m.recent = append(m.recent, hateSample{ts: t.nowFn(), amt: adjusted})
	t.armExpiryLocked(mob, m)
}

// recordSpellDamage resolves a direct spell-damage line. When it matches the
// pending cast, it credits the spell's BASE hate (computed at cast) rather than
// the observed damage — crits and partial resists never change the hate. The
// damage line is the resolution signal for a nuke (which emits no "lands"
// message). With no matching cast the line is a weapon proc or otherwise
// untracked spell: its originating spell — and thus its base damage — can't be
// read from the bare damage line, so the observed amount stands in as a proxy.
//
// The pending is resolved on the first direct-damage line within the cast window
// regardless of which mob it names (EQ casts serially, so the recent cast is
// this nuke); the hate is credited to the mob actually struck.
func (t *Tracker) recordSpellDamage(mob string, dmg int, ts time.Time) {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" || mob == "You" {
		return
	}
	t.mu.Lock()
	if p := t.pending; p != nil && p.directDamage && ts.Sub(p.at) <= castResolveWindow {
		t.pending = nil
		hate := p.offensiveHate + p.damageHate
		// See applyPendingLocked: the hate modifier bypasses a non-positive total.
		t.addHateLocked(mob, float64(hate), ts, hate > 0) // spell hate
		t.lastEngaged = mob
		snap := t.snapshotLocked(ts)
		t.mu.Unlock()
		t.broadcast(snap)
		return
	}
	if dmg <= 0 {
		t.mu.Unlock()
		return
	}
	t.addHateLocked(mob, float64(dmg), ts, true) // spell (proc) hate
	t.lastEngaged = mob
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordDamage credits a damage line's hate. For a melee swing the hate is the
// flat per-swing value (equipped weapon damage + primary-hand bonus), identical
// on every swing — NOT the white damage rolled; only when that value is unknown
// does it fall back to the observed damage. DoT ticks always credit the observed
// per-tick damage. The observed damage feeds the per-mob melee average that backs
// the miss-hate fallback. Direct spell damage goes through recordSpellDamage.
func (t *Tracker) recordDamage(mob string, dmg int, melee bool, ts time.Time) {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" || mob == "You" {
		return
	}
	// Resolve the per-swing value outside the lock (the provider may do I/O).
	amount := dmg
	if melee {
		if swing := t.meleeSwingHate(); swing > 0 {
			amount = swing
		}
	}
	if amount <= 0 {
		return
	}
	t.mu.Lock()
	// The hate modifier applies to spell/heal hate only, never melee swings.
	t.addHateLocked(mob, float64(amount), ts, !melee)
	if melee {
		// Track the OBSERVED swing damage so an unknown-weapon miss can still be
		// estimated from the landed-swing average.
		m := t.mobs[mob]
		m.meleeSum += int64(dmg)
		m.meleeCount++
	}
	t.lastEngaged = mob
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordMiss adds the per-swing hate of a melee MISS — the server adds the same
// flat per-swing hate whether or not the swing connects. It uses the equipped
// weapon's swing value when known; otherwise it estimates the miss as this
// character's average landed swing on that mob (a no-op until one has landed).
func (t *Tracker) recordMiss(mob string, ts time.Time) {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" || mob == "You" {
		return
	}
	swing := t.meleeSwingHate()
	t.mu.Lock()
	m := t.mobs[mob]
	if m == nil {
		t.mu.Unlock()
		return
	}
	amount := float64(swing)
	if swing <= 0 {
		if m.meleeCount == 0 {
			t.mu.Unlock()
			return
		}
		amount = float64(m.meleeSum) / float64(m.meleeCount)
	}
	t.addHateLocked(mob, amount, ts, false) // melee miss — no hatemod
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordBackstab credits a backstab's hate — the flat base-damage value from the
// equipped primary piercer, NOT the large rolled backstab number (the server
// adds a backstab's hate with a zero damage component). Falls back to the
// observed damage when the weapon isn't a known piercer. Melee-type hate, so the
// hate modifier never applies.
func (t *Tracker) recordBackstab(mob string, dmg int, ts time.Time) {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" || mob == "You" {
		return
	}
	amount := t.backstabHate()
	if amount <= 0 {
		amount = dmg // weapon unknown or not a piercer — coarse fallback
	}
	if amount <= 0 {
		return
	}
	t.mu.Lock()
	t.addHateLocked(mob, float64(amount), ts, false) // melee — no hatemod
	t.lastEngaged = mob
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
		t.addHateLocked(mob, float64(h), ts, true) // heal hate
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
	m.gen++
	if durationTicks > 0 {
		gen := m.gen
		m.timer = time.AfterFunc(time.Duration(durationTicks)*tickDuration, func() {
			t.expireMod(spellName, gen)
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
func (t *Tracker) expireMod(spellName string, gen uint64) {
	t.mu.Lock()
	m, ok := t.mods[spellName]
	if !ok || m.gen != gen {
		t.mu.Unlock()
		return // already gone, or refreshed by a recast
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	delete(t.mods, spellName)
	snap := t.snapshotLocked(time.Now())
	t.mu.Unlock()
	t.broadcast(snap)
}

// endMob drops a single mob (kill or staleness) and broadcasts the change.
// Returns false if no mob with that name is tracked.
func (t *Tracker) endMob(mob string, ts time.Time) bool {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" {
		return false
	}
	t.mu.Lock()
	m, ok := t.mobs[mob]
	if !ok {
		t.mu.Unlock()
		return false
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
	return true
}

// RemoveMob drops a single mob from the threat list (the overlay's per-row
// manual "x" button) and broadcasts the change. Returns false if no mob with
// that name is tracked.
func (t *Tracker) RemoveMob(name string) bool {
	return t.endMob(name, time.Now())
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

// recordRogueEvade applies a successful Rogue Evade to the current target
// (pipe target, else the most recently engaged mob) — see rogueEvadeExpectedPct.
// Unlike every other hate change, this directly rescales the running total
// rather than adding a signed amount: the server itself does the same
// (SetHateAmountOnEnt to a fraction of the current value), and the modifier
// never applies (RogueEvade doesn't go through CheckAggroAmount/AddToHateList
// at all). No-op if there's no tracked hate on the current target.
func (t *Tracker) recordRogueEvade(ts time.Time) {
	t.mu.Lock()
	mob := t.castTargetLocked()
	m := t.mobs[mob]
	if mob == "" || m == nil {
		t.mu.Unlock()
		return
	}
	m.hate = m.hate * rogueEvadeExpectedPct / 100
	if m.hate < rogueEvadeMinHate {
		m.hate = rogueEvadeMinHate
	}
	m.recent = nil // the drop isn't a "hate added" sample; keep the live-rate window honest
	m.last = ts
	t.armExpiryLocked(mob, m)
	snap := t.snapshotLocked(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// recordTaunt applies a successful taunt to the active character's own hate:
// EQMacEmu sets the taunter's hate to topHate + tauntBump outright (a direct
// SetHate, not run through AddToHateList), so this bypasses the hate modifier
// the same way melee does. This tracker has no visibility into anyone else's
// hate on its own, so topHateFn (when wired) supplies a raid-wide estimate
// from observed damage; without it, we can't tell whether we're already the
// top hater, so the taunt still guarantees at least a flat tauntBump jump
// over our own prior hate rather than landing as a no-op, which would make a
// successful taunt read as if it had done nothing.
func (t *Tracker) recordTaunt(mob string, ts time.Time) {
	mob = logparser.CanonicalNPCName(mob)
	if mob == "" {
		return
	}
	top, haveTop := t.topHate(mob)

	t.mu.Lock()
	m := t.ensureMobLocked(mob, ts)
	var delta float64
	if haveTop {
		delta = float64(top) + tauntBump - m.hate
		if delta <= 0 {
			// Already at/above the estimated top — matches the server's
			// no-op when the taunter is already the top hater.
			t.mu.Unlock()
			return
		}
	} else {
		delta = tauntBump
	}
	t.addHateLocked(mob, delta, ts, false) // taunt — flat SetHate, no hatemod
	t.lastEngaged = mob
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
	m.expiryGen++
	gen := m.expiryGen
	m.timer = time.AfterFunc(mobExpiry, func() {
		t.endMobExpired(mob, gen)
	})
}

// endMobExpired is the staleness-timer callback. It ends the mob only if it
// hasn't been refreshed since this timer was armed (expiryGen still matches) —
// otherwise a stale, already-queued fire would drop an actively-engaged mob.
func (t *Tracker) endMobExpired(mob string, gen uint64) {
	t.mu.Lock()
	m, ok := t.mobs[mob]
	if !ok || m.expiryGen != gen {
		t.mu.Unlock()
		return // already gone, or refreshed by newer activity
	}
	if m.timer != nil {
		m.timer.Stop()
	}
	delete(t.mobs, mob)
	if t.lastEngaged == mob {
		t.lastEngaged = ""
	}
	snap := t.snapshotLocked(time.Now())
	t.mu.Unlock()
	t.broadcast(snap)
}

func (t *Tracker) broadcast(state ThreatState) {
	if t.hub == nil {
		return
	}
	t.hub.Broadcast(ws.Event{Type: WSEventThreat, Data: state})
}

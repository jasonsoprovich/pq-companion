package spelltimer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// CharacterContext supplies the active character + EQ install path + 0-indexed
// EQ class so the engine can resolve item/AA duration focuses for a cast.
// Returning empty strings disables modifier resolution (timers fall back to
// base duration). Returning a class of -1 means "class unknown" — bard-specific
// rules in buffmod.Resolve are then skipped.
type CharacterContext func() (eqPath, charName string, class int)

// ScopeProvider returns the user-configured tracking scope ("self",
// "cast_by_me", or "anyone"). The engine calls this on every landed event so
// a config change takes effect immediately without restarting the engine.
// Empty / unknown values are treated as "cast_by_me" to match the current
// default behaviour.
type ScopeProvider func() string

// ClassFilterProvider returns whether to additionally filter buff timers by
// class castability, plus the active character's class index (0–14). When
// enabled is true and classIndex is in range, the engine drops buff-category
// timers whose source spell isn't castable by the player's class — so a
// paladin's Spiritual Purity or a shaman's Talisman of the Brute landing on
// an enchanter no longer clutter the enchanter's overlay.
//
// Returning enabled=false (or a nil provider) disables the filter; that's the
// default for backwards compatibility.
type ClassFilterProvider func() (enabled bool, classIndex int)

// ModeProvider returns the user-configured tracking mode ("auto" or
// "triggers_only"). In "triggers_only" the spell-landed pipeline still
// parses log lines (so cast disambiguation keeps working for any triggers
// that key off SpellID) but does NOT create timer rows itself — only
// triggers/packs do. nil provider or empty/unknown string means "auto".
type ModeProvider func() string

// OwnedItemsProvider returns the item IDs the active character currently owns
// (equipped + bags), from the most recent Zeal inventory export. Used to break
// ties when several item clickies share identical land text ("Your eyes
// tingle." is produced by Acumen, Ultravision, See Invisible, Serpent Sight
// and Chill Sight clickies) and there's no "begin casting" line to
// disambiguate: the one the player actually carries is the spell that landed.
// nil provider or empty slice means "inventory unknown" — no narrowing.
type OwnedItemsProvider func() []int

// KeepExpiredProvider returns whether expired buff/detrimental timers should
// linger in the overlay (as overdue count-ups) instead of being dropped on
// expiry or worn-off. The engine calls this live so a Settings change takes
// effect without a restart. nil provider or false means the default: remove a
// timer the moment it expires.
type KeepExpiredProvider func() bool

const (
	scopeSelf     = "self"
	scopeCastByMe = "cast_by_me"
	scopeAnyone   = "anyone"
)

const (
	modeAuto         = "auto"
	modeTriggersOnly = "triggers_only"
)

// classCannotCast is the sentinel value spells_new uses in classes1–classes15
// for "this class can never cast this spell at any level". Anything else
// (1–60 in the classic ruleset) is a valid level requirement.
const classCannotCast = 255

// targetTypeSelf is the spells_new.targettype value for a self-only spell
// under the EQMacEmu/Server SpellTargetType enum (ST_Self = 6). Used to
// recognise item-clicky self-buffs whose underlying spell is not castable
// by any class (e.g. Shield of the Eighth from the Coldain Insignia Ring).
const targetTypeSelf = 6

// isItemClicky returns true when no player class can cast the underlying
// spell — i.e. classes1..classes15 are all the cannot-cast sentinel. These
// are typically item-click effects, AA-triggered spells, or NPC-only spells
// that reached the player via a worn/clicky item. Used to bypass the
// class-castability filter on the buff path and to correct goodEffect
// mis-flagging in the categoriser.
func isItemClicky(spell *db.Spell) bool {
	for _, lvl := range spell.ClassLevels {
		if lvl < classCannotCast {
			return false
		}
	}
	return true
}

// classFilterAllowsBuff reports whether a landed buff survives the optional
// "only show buffs my class can cast" filter.
//
// Spells no player class can cast (isItemClicky == true) cover three very
// different cases that the spell data can't tell apart: item clickies the
// user triggered, OTHER players' item clickies, and NPC-only buffs/recourses
// (e.g. Fiery Might, Harmonize, Shroud of Pain Recourse — all target-self,
// all classes 255). The only signal that distinguishes "mine" from "someone
// else's" is where the spell landed: the user triggered a clicky by clicking
// the item, so it lands on *them*. We therefore exempt an all-classes-255
// spell from the filter ONLY when it lands on the active player. Without the
// isSelfTarget gate, scope=anyone surfaces every clicky and NPC self-buff
// cast by anyone in the zone, flooding the buff overlay with spells the
// user's class can't cast.
func classFilterAllowsBuff(spell *db.Spell, isSelfTarget, enabled bool, classIdx int) bool {
	if !enabled {
		return true
	}
	if isItemClicky(spell) && isSelfTarget {
		return true
	}
	if classIdx < 0 || classIdx >= len(spell.ClassLevels) {
		// Unknown / unset class — don't filter rather than hide everything.
		return true
	}
	return spell.ClassLevels[classIdx] < classCannotCast
}

// broadcastInterval is how often the engine pushes timer state updates to
// WebSocket clients while timers are active.
const broadcastInterval = time.Second

// dedupGraceWindow is the time after a timer is created during which a second
// create attempt for the same spell name (across any target) is treated as a
// redundant duplicate. This catches the case where both the spell-landed
// pipeline and a user-defined custom trigger fire for the same buff —
// whichever wins first gets to define the timer; the other is suppressed.
const dedupGraceWindow = 3 * time.Second

// lastCastWindow is the time after EventSpellCast within which a
// landed/disambiguation/did-not-take-hold message is still considered to
// refer to that cast. EQ's land messages typically follow the cast within
// a second; this allows for slow casts plus modest log latency.
const lastCastWindow = 30 * time.Second

// timerKeySep separates the spell name from the target name in a timer's
// composite key. Picked so a literal '@' never appears in either side.
const timerKeySep = "@"

// deferredRenderSpells are spells whose trigger-driven cast-begin entry
// should NOT render a timer immediately. Instead the trigger metadata
// (threshold, alerts) is stashed as a pendingArm and grafted onto the
// real timer when the spell-landed pipeline fires. This eliminates the
// brief "ghost" timer the user saw on every mez cast and avoids leaving
// a stale full-duration timer on fizzle/interrupt/push.
//
// Scope: only the three mez spells whose shared land text ("X has been
// mesmerized.") forces the trigger pack to fire on cast-begin in the
// first place. Charm/Root/Fetter cast-begin triggers stay synchronous —
// the spell-landed pipeline is unreliable for charm (empty cast_on_other)
// and the user hasn't reported the same visual artifact for those lines.
var deferredRenderSpells = map[string]bool{
	"Mesmerize":     true,
	"Mesmerization": true,
	"Dazzle":        true,
}

// pendingArmTTL is how long a pendingArm survives without a matching
// spell-landed event. Covers the longest deferred cast time (Dazzle = 2s)
// plus a generous interrupt window before we give up and assume the cast
// fizzled / was pushed / never lands. Lazy-GC'd on every StartExternal.
const pendingArmTTL = 10 * time.Second

// pendingArm holds trigger-supplied metadata for a deferred-render spell
// while we wait for the spell-landed event to materialise the real timer.
// See deferredRenderSpells.
type pendingArm struct {
	DisplayThresholdSecs int
	TimerAlerts          json.RawMessage
	ArmedAt              time.Time
}

// timerKey returns the composite map key used to identify a timer. Targets
// that aren't tied to a specific recipient (e.g. trigger-driven timers) pass
// an empty string — the resulting key still namespaces them away from any
// spell-derived timer with the same spell name.
func timerKey(spellName, targetName string) string {
	return spellName + timerKeySep + targetName
}

// sameSpellForDedup reports whether an existing timer represents the same
// underlying spell as an incoming (name, spellID) pair, for cross-pipeline
// dedup. A name match is the common case. A SpellID match additionally
// catches packs that give a trigger a combined display name — e.g. the
// Enchanter pack's "Speed of the Shissar/Brood" — while linking it to a
// single DB spell (1939). There the trigger's name differs from the
// spell-landed pipeline's resolved DB name ("Speed of the Shissar"), but
// both timers carry the same SpellID, so without this they'd show as two
// separate rows for one cast.
func sameSpellForDedup(existing *ActiveTimer, name string, spellID int) bool {
	if existing.SpellName == name {
		return true
	}
	return spellID > 0 && existing.SpellID == spellID
}

// sameLandOrphan reports whether existing is a target-less, non-charm
// detrimental created by a trigger that fired on the same land line the
// spell-landed pipeline just resolved (identical land timestamp). That makes
// it the trigger-pack twin of the spell the pipeline named — e.g. the
// Enchanter pack's broad "Tashanian" trigger firing on the shared
// "<mob> glances nervously about." text while the pipeline resolves the actual
// Tash-line spell cast. onSpellLanded absorbs such an orphan into the
// target-bound timer so the user sees one row (with a target) instead of two.
// Charm is excluded — its orphan is intentionally kept distinct (see
// removeOnKill) and never shares a land line with a different spell here.
func sameLandOrphan(existing *ActiveTimer, landedAt time.Time) bool {
	return existing.TargetName == "" &&
		!existing.IsCharm &&
		isDetrimentalCategory(existing.Category) &&
		existing.StartsAt.Equal(landedAt)
}

// Engine watches parsed log events, maintains a live map of active spell
// timers, and broadcasts state changes via WebSocket.
//
// Timers are keyed by (spell name, target). Casting the same spell again on
// the same target replaces (refreshes) its timer; casting on a different
// target creates a separate entry. This is what raid buff tracking needs —
// a Visions of Grandeur cast on three different group members produces three
// independently-tracked timers.
type Engine struct {
	hub           *ws.Hub
	db            *db.DB
	charCtx       CharacterContext
	scopeFn       ScopeProvider
	classFilterFn ClassFilterProvider
	modeFn        ModeProvider
	keepExpiredFn KeepExpiredProvider

	mu     sync.Mutex
	timers map[string]*ActiveTimer // keyed by timerKey(spell, target)

	// pendingArms holds trigger-supplied metadata for deferred-render
	// spells (see deferredRenderSpells). Populated by StartExternal,
	// consumed by onSpellLanded, lazily GC'd by pendingArmTTL.
	pendingArms map[string]*pendingArm

	// lastCastSpell and lastCastAt track the most recent EventSpellCast.
	// Used (a) to disambiguate ambiguous EventSpellLanded matches — many
	// spells share identical cast-on text — and (b) historically to
	// correlate "Your spell did not take hold." with a specific cast.
	lastCastSpell string
	lastCastAt    time.Time

	// clickableSpellIDs is the set of spell IDs produced by some item's click
	// or proc effect, lazily loaded from the DB on first need (see
	// resolveLandedSpellName). Used as a last resort to disambiguate instant
	// item-clicky land collisions: two differently-named clickies can share
	// identical cast-on-you text and emit no "begin casting" line, but only
	// one is actually produced by an item the player can trigger.
	clickableMu       sync.Mutex
	clickableSpellIDs map[int]bool
	clickableLoaded   bool

	// ownedItemsFn / ownedClicky* narrow a multi-clicky land collision to the
	// single clicky the active character actually carries. The clicky spell-ID
	// set is derived from the owned item list once and cached; RefreshModifiers
	// (fired on inventory/character change) clears it.
	ownedItemsFn        OwnedItemsProvider
	ownedMu             sync.Mutex
	ownedClickySpellIDs map[int]bool
	ownedClickyLoaded   bool

	// modifier cache: keeps the last-computed contributors per character so
	// the engine doesn't re-parse the Quarmy export on every cast. Invalidated
	// by character change or RefreshModifiers().
	modMu           sync.Mutex
	modCharName     string
	modContribs     []buffmod.Modifier
	modPermIllusion bool

	// pipeCasting is the spell name most recently reported by the Zeal pipe
	// (LabelCastingName, id 134). Empty when the player isn't casting. Used
	// purely for observability — when EventSpellCast fires from the log we
	// cross-check this value and log divergences so the log-side parser can
	// be tightened over time.
	pipeCasting string

	// lastPipeTarget is the most recent target name reported by the Zeal pipe
	// (LabelTargetName, id 28). The pipe resends the current target at ~10 Hz,
	// so HandlePipeTarget tracks the last value and only acts on transitions.
	lastPipeTarget string

	// pipeBuffSlots is the most recent self-buff slot snapshot from the pipe
	// (Buff0..14, plus 15..20 from game.dll). Stored as a set keyed by spell
	// name. Used for divergence logging against the engine's active self-buff
	// timers; behaviour (i.e. whether to prune timers the pipe disagrees
	// with) is intentionally deferred until we have a sense of how often the
	// two sources disagree in real play.
	pipeBuffSlots map[string]bool

	// lastDivergenceFromPipe / lastDivergenceFromTimers hold a sorted,
	// joined form of the divergence sets we most recently emitted to the
	// log. SetPipeBuffSlots receives a new snapshot multiple times per
	// second; without this cache we'd repeat the same "pipe has buff slot,
	// timers don't" line on every pulse, flooding the console with
	// identical noise for the duration of a long-running buff (KEI,
	// Aegolism, etc.).
	lastDivergenceFromPipe   string
	lastDivergenceFromTimers string
}

// NewEngine returns an initialised Engine ready to receive log events.
// charCtx may be nil (timers fall back to base / unextended duration).
// scopeFn may be nil (engine behaves as if scope is "anyone").
// classFilterFn may be nil (no class-castability filtering).
// modeFn may be nil (engine behaves as if mode is "auto").
// ownedItemsFn may be nil (no clicky-collision narrowing by inventory).
// keepExpiredFn may be nil (expired timers are dropped on expiry).
func NewEngine(hub *ws.Hub, database *db.DB, charCtx CharacterContext, scopeFn ScopeProvider, classFilterFn ClassFilterProvider, modeFn ModeProvider, ownedItemsFn OwnedItemsProvider, keepExpiredFn KeepExpiredProvider) *Engine {
	return &Engine{
		hub:           hub,
		db:            database,
		charCtx:       charCtx,
		scopeFn:       scopeFn,
		classFilterFn: classFilterFn,
		modeFn:        modeFn,
		ownedItemsFn:  ownedItemsFn,
		keepExpiredFn: keepExpiredFn,
		timers:        make(map[string]*ActiveTimer),
		pendingArms:   make(map[string]*pendingArm),
	}
}

// keepExpired reports whether expired timers should linger as overdue
// reminders (the user's "keep expired timers" option). Defaults to false.
func (e *Engine) keepExpired() bool {
	return e.keepExpiredFn != nil && e.keepExpiredFn()
}

// trackingMode returns the user's currently-configured mode, defaulting to
// "auto" when the provider is absent or returns an unknown value.
func (e *Engine) trackingMode() string {
	if e.modeFn == nil {
		return modeAuto
	}
	switch e.modeFn() {
	case modeTriggersOnly:
		return modeTriggersOnly
	default:
		return modeAuto
	}
}

// trackingScope returns the user's currently-configured scope, defaulting to
// "cast_by_me" when the provider is absent or returns an empty/unknown value.
func (e *Engine) trackingScope() string {
	if e.scopeFn == nil {
		return scopeCastByMe
	}
	switch s := e.scopeFn(); s {
	case scopeSelf:
		return scopeSelf
	case scopeAnyone:
		return scopeAnyone
	default:
		return scopeCastByMe
	}
}

// RefreshModifiers clears the cached buffmod contributors so the next cast
// will recompute from the current Quarmy export. Call when the active
// character's inventory or AAs are known to have changed (e.g. zeal watcher
// detected a new export).
func (e *Engine) RefreshModifiers() {
	e.modMu.Lock()
	e.modCharName = ""
	e.modContribs = nil
	e.modPermIllusion = false
	e.modMu.Unlock()

	// The owned-clicky set is derived from the same inventory export, so drop
	// it too — the next ambiguous clicky land recomputes against current bags.
	e.ownedMu.Lock()
	e.ownedClickyLoaded = false
	e.ownedClickySpellIDs = nil
	e.ownedMu.Unlock()
}

// Start runs the background ticker that prunes expired timers and broadcasts
// current state once per second. Blocks until ctx is cancelled.
func (e *Engine) Start(ctx context.Context) {
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.pruneExpired()
			e.broadcast()
		}
	}
}

// Handle processes a single parsed log event.
//
// Timer creation is driven exclusively by EventSpellLanded — the cast-begin
// event only records the spell name so an ambiguous later land event can be
// disambiguated. Resist / interrupt / did-not-take-hold mean the spell never
// landed and therefore no timer was ever created; these handlers only clear
// the recorded last-cast so a stale value can't bind to an unrelated spell.
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventSpellCast:
		data, ok := ev.Data.(logparser.SpellCastData)
		if !ok {
			slog.Debug("timer-debug: spell-cast event with bad payload", "data_type", fmt.Sprintf("%T", ev.Data))
			return
		}
		e.mu.Lock()
		e.lastCastSpell = data.SpellName
		e.lastCastAt = time.Now()
		// Cross-check: if Zeal is reporting a different in-flight cast than
		// the log, that's a parser miss worth investigating. Logged once per
		// cast event so this can't spam during fast cast chains.
		pipeName := e.pipeCasting
		e.mu.Unlock()
		if pipeName != "" && pipeName != data.SpellName {
			slog.Info("zealpipe-divergence: log cast != pipe cast",
				"log_spell", data.SpellName, "pipe_spell", pipeName, "ts", ev.Timestamp)
		}
		slog.Debug("timer-debug: spell-cast recorded (awaiting land)", "spell", data.SpellName, "ts", ev.Timestamp)

	case logparser.EventSpellLanded:
		data, ok := ev.Data.(logparser.SpellLandedData)
		if !ok {
			return
		}
		e.onSpellLanded(ev.Timestamp, data)

	case logparser.EventSpellDidNotTakeHold,
		logparser.EventSpellInterrupt,
		logparser.EventSpellResist:
		// Spell never landed — nothing to remove. Clear the recorded last
		// cast so it can't accidentally disambiguate an unrelated future
		// landed event.
		e.mu.Lock()
		e.lastCastSpell = ""
		e.lastCastAt = time.Time{}
		e.mu.Unlock()

	case logparser.EventSpellFade:
		// "Your X spell has worn off." — fires only on the active player.
		data, ok := ev.Data.(logparser.SpellFadeData)
		if !ok || data.SpellName == "" {
			return
		}
		target := e.activePlayerName()
		e.removeTimer(timerKey(data.SpellName, target))

	case logparser.EventSpellFadeFrom:
		// "Tashanian effect fades from Soandso." — target is in the event.
		data, ok := ev.Data.(logparser.SpellFadeFromData)
		if !ok || data.SpellName == "" {
			return
		}
		// Remove the entry for this exact (spell, target). Some spell-fade
		// messages use a short form of the name (e.g. "Tashanian" for the
		// Tashan line) which still matches the DB spell name we keyed under.
		e.removeTimer(timerKey(data.SpellName, data.TargetName))

	case logparser.EventIllusionFade:
		// "Your illusion fades." or "You forget Illusion: X." — neither names
		// the race, and only one illusion can be active at a time per player,
		// so wipe every "Illusion: …" timer keyed to the active player.
		e.removeIllusionsForPlayer(e.activePlayerName())

	case logparser.EventCharmBroken:
		// "Your charm spell has worn off." — EQ emits this generic line for
		// every charm spell (Charm/Beguile/Cajoling Whispers/Allure/Dictate/
		// Boltran's Agacerie), never a per-name "Your <spell> spell has worn
		// off." So a charm timer's worn-off pattern can't match it and the
		// EventSpellFade path never sees it (the parser routes it here). Only
		// one charm is active at a time, so clear every charm timer — the same
		// charm-break signal the combat tracker uses to release the pet.
		e.removeCharmTimers()

	case logparser.EventZone:
		// Zoning no longer clears timers — buffs survive a zone change in
		// EQ, and persisting them lets the user keep tracking long-running
		// raid buffs across zone lines.

	case logparser.EventDeath:
		// Active player death: EQ strips every buff (and detrimental) from you
		// when you die, so clear all timers targeting the active player. Buffs
		// we put on OTHER players (and debuffs on mobs) remain — keeping those
		// alive through our own death is the whole reason buffs-on-others don't
		// reset here. removeSelfTimers matches both the resolved character name
		// and the literal "You" placeholder.
		e.removeSelfTimers()

	case logparser.EventKill:
		// Something we (or a group member) just killed. If it had any of
		// our timers (mez, debuffs, DoTs, or even a buff we put on it),
		// drop them — they're no longer accurate.
		data, ok := ev.Data.(logparser.KillData)
		if !ok || data.Target == "" {
			return
		}
		e.removeOnKill(data.Target)
	}
}

// activePlayerName returns the active character's display name, used as the
// implicit target of cast_on_you / "Your X spell has worn off." messages.
// Falls back to the literal "You" when no character context is configured —
// this only happens in tests and during early startup.
func (e *Engine) activePlayerName() string {
	if e.charCtx == nil {
		return "You"
	}
	_, name, _ := e.charCtx()
	if name == "" {
		return "You"
	}
	return name
}

// GetState returns a point-in-time snapshot of all active timers.
func (e *Engine) GetState() TimerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot(time.Now())
}

// SetPipeCasting records the current in-flight cast as reported by the Zeal
// pipe (LabelCastingName, id 134). Empty value means "not casting." The
// engine consults this value during EventSpellCast to detect parser
// divergence; it has no other side effects in this stage.
func (e *Engine) SetPipeCasting(name string) {
	e.mu.Lock()
	e.pipeCasting = strings.TrimSpace(name)
	e.mu.Unlock()
}

// HandlePipeTarget consumes the player's current target name from the Zeal
// pipe (LabelTargetName, id 28) and uses the corpse form as a death signal for
// detrimental timers.
//
// When a mob dies while the player still has it selected, Zeal flips the
// target name to "<Name>'s corpse". That corpse form is an unambiguous,
// always-in-range death signal — unlike the log's slain-line, which never
// reaches a caster standing far from a raid boss. On the transition into a
// corpse target we drop any detrimental timers keyed to that NPC, the same
// cleanup the log-driven EventKill path performs via removeOnKill.
//
// The pipe resends the current target at ~10 Hz, so we de-dupe against the
// last seen name and act only when it changes; non-corpse and empty targets
// just update the de-dupe state. Limitation: Zeal only exposes the player's
// own target, so a debuff lingers if the caster retargets away before the boss
// dies — the log path remains the fallback for that case.
func (e *Engine) HandlePipeTarget(name string) {
	name = strings.TrimSpace(name)
	e.mu.Lock()
	if name == e.lastPipeTarget {
		e.mu.Unlock()
		return
	}
	e.lastPipeTarget = name
	e.mu.Unlock()

	if base, ok := parseCorpseTarget(name); ok {
		e.removeOnKill(base)
	}
}

// SetPipeBuffSlots records the current self-buff slot snapshot from the pipe
// (Buff0..14 and 15..20 labels). Each slot's value is the spell name in that
// slot, or empty if the slot is unoccupied. The engine compares this set
// against its active self-buff timers and logs divergences at info level —
// data feed for tightening the log parser. No timers are pruned in this
// pass; that's a deliberate behaviour change to defer until we know how
// frequently the two sources disagree under real play.
func (e *Engine) SetPipeBuffSlots(slots []string) {
	set := make(map[string]bool, len(slots))
	for _, s := range slots {
		s = strings.TrimSpace(s)
		if s != "" {
			set[s] = true
		}
	}
	active := e.activePlayerName()
	e.mu.Lock()
	e.pipeBuffSlots = set
	// Compare engine self-buff timers vs the pipe set. We only consider
	// timers that *should* appear in the player's buff window — Category=Buff
	// targeted at the active char or "You". Detrimentals on enemies are out
	// of scope.
	var missingFromPipe []string
	for _, t := range e.timers {
		if t.Category != CategoryBuff {
			continue
		}
		if t.TargetName != "You" && t.TargetName != active {
			continue
		}
		if !set[t.SpellName] {
			missingFromPipe = append(missingFromPipe, t.SpellName)
		}
	}
	var missingFromTimers []string
	for name := range set {
		found := false
		for _, t := range e.timers {
			if t.Category == CategoryBuff &&
				(t.TargetName == "You" || t.TargetName == active) &&
				t.SpellName == name {
				found = true
				break
			}
		}
		if !found {
			missingFromTimers = append(missingFromTimers, name)
		}
	}
	// Fold each divergence list into a stable, sorted key so we can dedupe
	// against the last set we logged. The pipe sends a fresh snapshot
	// several times per second and the spell list typically arrives in a
	// different order each pulse — comparing the joined-sorted form lets
	// us treat "same set, different order" as identical and avoid
	// re-logging the same divergence on every pulse.
	pipeKey := divergenceKey(missingFromPipe)
	timersKey := divergenceKey(missingFromTimers)
	logPipe := pipeKey != e.lastDivergenceFromPipe
	logTimers := timersKey != e.lastDivergenceFromTimers
	e.lastDivergenceFromPipe = pipeKey
	e.lastDivergenceFromTimers = timersKey
	e.mu.Unlock()
	if logPipe && len(missingFromPipe) > 0 {
		slog.Info("zealpipe-divergence: timers think buff is active, pipe does not",
			"spells", missingFromPipe)
	}
	if logTimers && len(missingFromTimers) > 0 {
		slog.Info("zealpipe-divergence: pipe has buff slot, timers don't",
			"spells", missingFromTimers)
	}
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

// StartExternal adds a timer not driven by a log cast event. Used by the
// trigger engine when a user-defined trigger with timer_type set matches a
// log line. Recasts within the dedup window — including ones triggered by
// the spell-landed pipeline for a real spell of the same name — are
// suppressed so the more specific path (whichever fired first) wins.
//
// Category is a plain string (to avoid a dependency cycle from the trigger
// package) and must be one of "buff", "debuff", "mez", "dot", "stun";
// anything else is treated as "debuff". durationSecs must be > 0.
//
// displayThresholdSecs lets a trigger override the user's global buff /
// detrim display threshold for the timer it creates. > 0 means "only show
// when remaining time is at or below this value"; 0 means "let the
// frontend resolve against the global default for my category".
//
// spellID, when > 0, links the timer back to a DB spell so the engine can
// apply the active character's item/AA duration focuses to durationSecs —
// matching the spell-landed pipeline. 0 means "use durationSecs as given"
// (custom triggers without a spell anchor, tests).
func (e *Engine) StartExternal(name string, category string, durationSecs, displayThresholdSecs int, startedAt time.Time, alerts json.RawMessage, spellID int, targetName string) {
	if name == "" || durationSecs <= 0 {
		return
	}
	cat := Category(category)
	switch cat {
	case CategoryBuff, CategoryDebuff, CategoryMez, CategoryDot, CategoryStun, CategoryCHChain, CategoryCHChain2, CategoryCustom:
	default:
		cat = CategoryDebuff
	}

	var resolvedIcon int
	var isCharm bool
	if spellID > 0 && e.db != nil {
		if spell, err := e.db.GetSpell(spellID); err == nil && spell != nil {
			extended := e.applyDurationModifiers(spell, float64(durationSecs))
			durationSecs = int(extended)
			resolvedIcon = spell.NewIcon
			isCharm = isCharmSpell(spell)
		}
	}

	// targetName is non-empty only when the firing trigger captured a target
	// (TimerTargetCapture) — e.g. a group buff landing on a party member. It
	// becomes part of the composite key so casting the same buff on several
	// people tracks one row each, exactly like the spell-landed pipeline. When
	// empty (most custom triggers, self-buffs, the self-cast branch of an
	// alternation) the key namespaces by name alone; the same-spell-name dedup
	// below still avoids a duplicate row when the spell-landed pipeline already
	// created one for the same buff.
	key := timerKey(name, targetName)

	e.mu.Lock()
	e.gcPendingArmsLocked(time.Now())

	// Deferred-render path: spells in deferredRenderSpells (currently the
	// three mez spells with shared land text) stash trigger metadata as a
	// pendingArm and DO NOT create a visible timer. The spell-landed
	// pipeline promotes the arm into a real timer when the spell actually
	// lands; if it never lands (fizzle, interrupt, push, resist) the arm
	// expires via pendingArmTTL or StopExternal.
	if deferredRenderSpells[name] {
		e.pendingArms[name] = &pendingArm{
			DisplayThresholdSecs: displayThresholdSecs,
			TimerAlerts:          alerts,
			ArmedAt:              startedAt,
		}
		e.mu.Unlock()
		slog.Debug("timer-debug: pending arm stored for deferred-render spell",
			"name", name,
			"threshold_secs", displayThresholdSecs,
			"alerts_bytes", len(alerts),
		)
		return
	}

	// If a same-spell-name timer was just created (typically by the
	// spell-landed pipeline firing on the same log line), don't add a second
	// row — but DO carry the trigger's per-spell metadata onto the existing
	// timer. Spell-landed alone has no way to know about user-configured
	// thresholds or fading-soon TTS; a trigger is the user's declaration of
	// "treat this spell specially," so the trigger wins on metadata while
	// the spell-landed timer wins on identity (target, accurate duration via
	// duration focuses).
	for _, existing := range e.timers {
		if sameSpellForDedup(existing, name, spellID) && time.Since(existing.CastAt) < dedupGraceWindow {
			if displayThresholdSecs > 0 {
				existing.DisplayThresholdSecs = displayThresholdSecs
			}
			if len(alerts) > 0 {
				existing.TimerAlerts = alerts
			}
			snap := e.snapshot(time.Now())
			e.mu.Unlock()
			slog.Debug("timer-debug: trigger metadata merged onto existing timer",
				"name", name,
				"existing_target", existing.TargetName,
				"existing_age_ms", time.Since(existing.CastAt).Milliseconds(),
				"applied_threshold_secs", displayThresholdSecs,
				"applied_alerts_bytes", len(alerts),
			)
			e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
			return
		}
	}
	timer := &ActiveTimer{
		ID:                   key,
		SpellName:            name,
		SpellID:              spellID,
		TargetName:           targetName,
		Icon:                 resolvedIcon,
		Category:             cat,
		CastAt:               startedAt,
		StartsAt:             startedAt,
		ExpiresAt:            startedAt.Add(time.Duration(durationSecs) * time.Second),
		DurationSeconds:      float64(durationSecs),
		DisplayThresholdSecs: displayThresholdSecs,
		TimerAlerts:          alerts,
		IsCharm:              isCharm,
	}
	e.timers[key] = timer
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Debug("timer-debug: external timer started (trigger-driven)",
		"name", name,
		"category", cat,
		"duration_secs", durationSecs,
		"active_timer_count", len(snap.Timers),
	)
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

// StopExternal removes any timer with this spell name, irrespective of
// target. Called by the trigger engine when a worn-off pattern fires; we
// match by name (not composite key) so a user-authored worn-off pattern
// also clears any spell-landed entries for the same buff.
//
// spellID, when > 0, additionally removes any timer carrying that SpellID
// even if its name differs — this clears merged timers where the spell-landed
// pipeline won identity under the DB name (e.g. "Speed of the Shissar") while
// the firing trigger used a combined pack name ("Speed of the Shissar/Brood").
// Without it, a haste buff that fades via "Your body slows." (the trigger's
// worn-off pattern, with no generic "...spell has worn off." line) would
// linger to natural expiry.
//
// Also clears any pendingArm for the spell — a resist of a deferred-render
// spell (e.g. mez) reaches us through the trigger's WornOffPattern, and
// without this the arm would linger until pendingArmTTL and could falsely
// promote a stale arm onto a later genuine cast.
func (e *Engine) StopExternal(name string, spellID int) {
	e.mu.Lock()
	delete(e.pendingArms, name)
	e.mu.Unlock()
	e.removeBySpellNameOrID(name, spellID)
}

// gcPendingArmsLocked drops pending arms older than pendingArmTTL. Lazy
// GC: called from StartExternal and onSpellLanded so we don't need a
// dedicated ticker. Caller must hold e.mu.
func (e *Engine) gcPendingArmsLocked(now time.Time) {
	for name, arm := range e.pendingArms {
		if now.Sub(arm.ArmedAt) > pendingArmTTL {
			delete(e.pendingArms, name)
			slog.Debug("timer-debug: pending arm expired (no land within TTL)",
				"name", name,
				"age_ms", now.Sub(arm.ArmedAt).Milliseconds(),
			)
		}
	}
}

// ClearAll removes every active timer and broadcasts the resulting empty
// state. Used when the user globally disables the timer system.
func (e *Engine) ClearAll() {
	e.clearAll()
}

// ClearCategory removes active timers belonging to the given category group
// and broadcasts the resulting state. Accepted values:
//
//	"buff"        — only beneficial buffs
//	"detrimental" — debuffs, dots, mez, stuns
//	"all" / ""    — every active timer
func (e *Engine) ClearCategory(group string) {
	e.mu.Lock()
	removed := 0
	for name, t := range e.timers {
		if categoryMatchesGroup(t.Category, group) {
			delete(e.timers, name)
			removed++
		}
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if removed > 0 {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

func categoryMatchesGroup(cat Category, group string) bool {
	switch group {
	case "", "all":
		return true
	case "buff":
		return cat == CategoryBuff
	case "detrimental":
		return cat == CategoryDebuff || cat == CategoryDot || cat == CategoryMez || cat == CategoryStun
	case "ch_chain":
		return cat == CategoryCHChain
	case "ch_chain_2":
		return cat == CategoryCHChain2
	case "custom":
		return cat == CategoryCustom
	}
	return false
}

// ── internal helpers ──────────────────────────────────────────────────────────

// onSpellLanded creates (or refreshes) a timer for a spell that just took
// effect. Resolves the spell name (disambiguating against lastCastSpell when
// the cast text is shared by multiple spells), the target (the active player
// for self-cast events), and the duration (with item/AA focus extensions).
func (e *Engine) onSpellLanded(landedAt time.Time, data logparser.SpellLandedData) {
	spellName := e.resolveLandedSpellName(data)
	if spellName == "" {
		return
	}

	target := data.TargetName
	if data.Kind == logparser.SpellLandedKindYou {
		target = e.activePlayerName()
	}
	if target == "" {
		// Defensive — should be unreachable now activePlayerName falls back
		// to "You". Skip rather than create a key like "Spell@".
		return
	}

	// In triggers-only mode, the spell-landed pipeline still runs (so cast
	// disambiguation keeps populating lastCastSpell for any trigger that
	// keys off it) but does NOT create timer rows — only triggers/packs do.
	if e.trackingMode() == modeTriggersOnly {
		slog.Debug("timer-debug: spell-landed skipped (mode=triggers_only)",
			"spell", spellName, "target", target)
		return
	}

	active := e.activePlayerName()
	isSelfTarget := target == active || target == "You"

	// Test-only fast path: when there's no DB wired (engine_test.go's harness
	// constructs the engine without a *db.DB), apply the legacy non-detrim
	// scope filter directly so the existing scope tests keep exercising
	// drop/keep behaviour without needing a fixture database.
	if e.db == nil {
		switch e.trackingScope() {
		case scopeSelf:
			if !isSelfTarget {
				return
			}
		case scopeCastByMe:
			if !isSelfTarget {
				return
			}
		}
		// Without a DB we can't compute duration / icon / category — nothing
		// further to do.
		return
	}

	// Look up the spell so we know its category (buff vs detrimental) and
	// class table for the filters below. A combined ambiguous-group name (e.g.
	// "Speed of the Shissar/Brood") isn't itself a DB row — fetch the group's
	// representative spell for metadata while keeping the combined display name.
	var spell *db.Spell
	var err error
	if repID := ambiguousGroupRepID(spellName); repID > 0 {
		spell, err = e.db.GetSpell(repID)
	} else {
		spell, err = e.db.GetSpellByExactName(spellName)
	}
	if err != nil {
		slog.Warn("spelltimer: DB error looking up spell", "name", spellName, "err", err)
		return
	}
	if spell == nil {
		slog.Debug("timer-debug: landed spell not found in DB (no timer created)", "name", spellName)
		return
	}

	cat := categorize(spell)

	// Tracking scope filter, split by category:
	//
	// Detrimental categories (debuff/dot/mez/stun) are always cast_by_me —
	// the user cast them on an enemy and definitely wants to see the timer.
	// They never land on the player from other players, so the buff scope
	// modes don't apply. Without this carve-out a user with scope=self would
	// silently lose every Tashan/Asphyxiate/etc. they cast on a mob.
	//
	// Buff category honours the user-configured scope:
	//   self        — drop everything not landing on the active player.
	//   cast_by_me  — keep self lands; otherwise require a recent local cast
	//                 of this spell name within lastCastWindow. EQ logs don't
	//                 record the caster of buffs landing on third parties,
	//                 so this is a heuristic.
	//   anyone      — no filtering.
	switch cat {
	case CategoryBuff:
		switch e.trackingScope() {
		case scopeSelf:
			if !isSelfTarget {
				slog.Debug("timer-debug: spell-landed skipped (scope=self, non-self target)",
					"spell", spellName, "target", target, "active", active)
				return
			}
		case scopeCastByMe:
			if !isSelfTarget {
				e.mu.Lock()
				recentMatch := e.lastCastSpell == spellName && time.Since(e.lastCastAt) <= lastCastWindow
				e.mu.Unlock()
				if !recentMatch {
					slog.Debug("timer-debug: spell-landed skipped (scope=cast_by_me, no matching local cast)",
						"spell", spellName, "target", target)
					return
				}
			}
		}
		// Optional class filter: drop buffs the player's class can't cast.
		// Item clickies the user triggered (all classes 255, landing on the
		// player) are exempt so Shield of the Eighth (Coldain Insignia Ring)
		// and friends still reach the buff overlay. The isSelfTarget gate
		// keeps OTHER players' clickies and NPC self-buffs/recourses out of
		// the overlay under scope=anyone — see classFilterAllowsBuff.
		if e.classFilterFn != nil {
			if enabled, classIdx := e.classFilterFn(); !classFilterAllowsBuff(spell, isSelfTarget, enabled, classIdx) {
				slog.Debug("timer-debug: spell-landed skipped (class filter)",
					"spell", spellName, "target", target, "class_idx", classIdx)
				return
			}
		}

	default:
		// Detrimental (debuff/dot/mez/stun): apply cast_by_me semantics
		// regardless of the user's chosen scope — see comment above.
		if !isSelfTarget {
			e.mu.Lock()
			recentMatch := e.lastCastSpell == spellName && time.Since(e.lastCastAt) <= lastCastWindow
			e.mu.Unlock()
			if !recentMatch {
				slog.Debug("timer-debug: detrimental spell-landed skipped (no matching local cast)",
					"spell", spellName, "target", target, "category", cat)
				return
			}
		}
	}

	durationTicks := SpellDurationTicks(spell, defaultCasterLevel)
	if durationTicks <= 0 {
		slog.Debug("timer-debug: landed spell has zero duration (no timer created)",
			"name", spellName,
			"formula", spell.BuffDurationFormula,
			"buff_duration", spell.BuffDuration,
		)
		return
	}

	baseDurationSec := float64(durationTicks) * eqTickSeconds
	durationSeconds := e.applyDurationModifiers(spell, baseDurationSec)
	expiresAt := landedAt.Add(time.Duration(float64(time.Second) * durationSeconds))

	slog.Debug("timer-debug: duration computed",
		"spell", spellName,
		"formula", spell.BuffDurationFormula,
		"buff_duration_ticks", spell.BuffDuration,
		"computed_ticks", durationTicks,
		"base_seconds", baseDurationSec,
		"final_seconds", durationSeconds,
	)

	key := timerKey(spellName, target)
	timer := &ActiveTimer{
		ID:              key,
		SpellName:       spellName,
		SpellID:         spell.ID,
		Icon:            spell.NewIcon,
		TargetName:      target,
		Category:        cat,
		CastAt:          landedAt,
		StartsAt:        landedAt,
		ExpiresAt:       expiresAt,
		DurationSeconds: durationSeconds,
		IsCharm:         isCharmSpell(spell),
	}

	e.mu.Lock()
	// Triggers fire BEFORE spell-landed in the tailer dispatch (raw lines
	// first, parsed events second), so a same-spell-name trigger may have
	// already created a target-less entry with user-configured threshold and
	// alerts. Graft that metadata onto the new (more specific, target-keyed)
	// timer and drop the old entry — otherwise the spell-landed timer ends
	// up with DisplayThresholdSecs=0 and the per-trigger override is lost.
	// This is the symmetric counterpart to the dedup in StartExternal.
	//
	// We also absorb a same-LAND orphan here: a trigger detrimental that fired
	// on the exact land line this spell resolved from, but under a different
	// name. The Enchanter pack's "Tashanian" trigger matches the generic
	// "<mob> glances nervously about." land text shared by the whole Tash line,
	// so casting Tashania (or any other Tash spell) spawns a phantom orphan
	// "Tashanian" timer alongside the pipeline's correct Tashania@target one.
	// Keying on an identical land timestamp keeps this tight — two genuinely
	// different debuffs land on separate log lines (separate timestamps).
	landDetrimental := isDetrimentalCategory(timer.Category)
	for existingKey, existing := range e.timers {
		if existingKey == key {
			continue
		}
		// Only absorb a target-less orphan (the trigger twin) or a same-target
		// re-land. A same-named timer bound to a DIFFERENT non-empty target is a
		// separate recipient — AoE mez or a debuff cast on several mobs — and
		// must coexist as its own row, not be overwritten. Without this guard
		// the second mob's land deleted the first mob's timer, collapsing an
		// AoE mez to a single timer (the reported bug).
		sameSpell := (existing.TargetName == "" || existing.TargetName == target) &&
			sameSpellForDedup(existing, spellName, spell.ID)
		sameLand := landDetrimental && sameLandOrphan(existing, landedAt)
		if !sameSpell && !sameLand {
			continue
		}
		if time.Since(existing.CastAt) >= dedupGraceWindow {
			continue
		}
		if existing.DisplayThresholdSecs > 0 {
			timer.DisplayThresholdSecs = existing.DisplayThresholdSecs
		}
		if len(existing.TimerAlerts) > 0 {
			timer.TimerAlerts = existing.TimerAlerts
		}
		delete(e.timers, existingKey)
		break
	}
	// Pending-arm promotion: for deferred-render spells (mez), the
	// trigger's metadata was stashed instead of creating a timer on
	// cast-begin. Graft it onto the new timer now. The arm is NOT consumed
	// here: an AoE mez lands on several mobs from one cast, and each land must
	// inherit the same threshold/alerts — deleting on the first land left the
	// other mobs' timers bare. The arm is cleared by StopExternal (worn-off /
	// resist) or aged out by pendingArmTTL; a genuine recast overwrites it with
	// a fresh timestamp, and any same-name re-graft carries identical metadata.
	e.gcPendingArmsLocked(time.Now())
	if arm, ok := e.pendingArms[spellName]; ok {
		if arm.DisplayThresholdSecs > 0 {
			timer.DisplayThresholdSecs = arm.DisplayThresholdSecs
		}
		if len(arm.TimerAlerts) > 0 {
			timer.TimerAlerts = arm.TimerAlerts
		}
		slog.Debug("timer-debug: pending arm promoted to landed timer",
			"spell", spellName,
			"target", target,
			"arm_age_ms", time.Since(arm.ArmedAt).Milliseconds(),
		)
	}
	// Recasting on the same target replaces the timer (refresh). No dedup
	// against the same key is needed; with composite keys, recasts on the
	// SAME target replace cleanly and casts on DIFFERENT targets coexist.
	e.timers[key] = timer
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Debug("timer-debug: timer created from spell-landed",
		"spell", spellName,
		"target", target,
		"category", timer.Category,
		"duration_secs", durationSeconds,
		"active_timer_count", len(snap.Timers),
	)
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

// commonCandidateName returns the shared display name when every ambiguous
// land candidate resolves to the same name, or "" when the candidates name
// different spells (so the caller must still disambiguate by recent cast).
func commonCandidateName(cands []logparser.SpellLandedCandidate) string {
	if len(cands) == 0 {
		return ""
	}
	name := cands[0].SpellName
	for _, c := range cands[1:] {
		if c.SpellName != name {
			return ""
		}
	}
	return name
}

// ambiguousLandGroup collapses two or more differently-named spells that share
// identical land text — and so can't be told apart in the log without a
// disambiguating recent cast — into one combined display name. RepSpellID
// supplies category/duration/icon for the timer; the members are mechanically
// equivalent for timer purposes (same buff, same duration). Resolved only as a
// last resort in resolveLandedSpellName, so a self-cast of a specific member
// still shows that member's name via the recent-cast match above.
//
// Without this, an ambiguous self-land (e.g. another player's group Speed of
// the Brood landing on you) resolves to "" and the pipeline drops it, leaving
// only the Enchanter pack's target-less "Speed of the Shissar/Brood" trigger
// row. Resolving to the combined name lets the pipeline create a proper
// target-keyed timer that the pack trigger then dedups into.
type ambiguousLandGroup struct {
	displayName string
	repSpellID  int
	memberIDs   map[int]bool
}

var ambiguousLandGroups = []ambiguousLandGroup{
	{
		// 1939 Speed of the Shissar (single) + 2895 Speed of the Brood (group)
		// — identical "...body pulses with the spirit of the Shissar." text.
		displayName: "Speed of the Shissar/Brood",
		repSpellID:  1939,
		memberIDs:   map[int]bool{1939: true, 2895: true},
	},
}

// matchAmbiguousLandGroup returns the group whose members are EXACTLY the
// candidate set (every candidate is a member and every member is present), or
// nil. The exact-match requirement keeps an unrelated set of shared-text spells
// from being mislabeled as this group.
func matchAmbiguousLandGroup(cands []logparser.SpellLandedCandidate) *ambiguousLandGroup {
	for i := range ambiguousLandGroups {
		g := &ambiguousLandGroups[i]
		if len(cands) != len(g.memberIDs) {
			continue
		}
		all := true
		for _, c := range cands {
			if !g.memberIDs[c.SpellID] {
				all = false
				break
			}
		}
		if all {
			return g
		}
	}
	return nil
}

// ambiguousGroupRepID returns the representative DB spell ID for a combined
// group display name (so onSpellLanded can fetch metadata for a name that isn't
// itself a real spell row), or 0 when name is an ordinary spell.
func ambiguousGroupRepID(name string) int {
	for i := range ambiguousLandGroups {
		if ambiguousLandGroups[i].displayName == name {
			return ambiguousLandGroups[i].repSpellID
		}
	}
	return 0
}

// resolveLandedSpellName returns the canonical spell name for a landed event,
// disambiguating ambiguous matches (multiple spells share the same cast text)
// against the most recently observed cast. Returns "" if the event is
// ambiguous and no recent cast points at any candidate.
func (e *Engine) resolveLandedSpellName(data logparser.SpellLandedData) string {
	if data.SpellName != "" {
		return data.SpellName
	}
	if len(data.Candidates) == 0 {
		return ""
	}
	// If every candidate carries the same display name (e.g. the two
	// "Fungal Regrowth" rows that differ only by spell ID), the *name* is
	// unambiguous even though the ID isn't — return it without needing a
	// recent cast. This rescues instant item clickies whose shared land text
	// maps onto same-named spell rows and which emit no "You begin casting"
	// line to disambiguate against.
	if name := commonCandidateName(data.Candidates); name != "" {
		return name
	}
	e.mu.Lock()
	lastSpell := e.lastCastSpell
	lastAge := time.Since(e.lastCastAt)
	e.mu.Unlock()

	if lastSpell != "" && lastAge <= lastCastWindow {
		for _, c := range data.Candidates {
			if c.SpellName == lastSpell {
				return c.SpellName
			}
		}
		slog.Debug("timer-debug: ambiguous spell-landed; recent cast doesn't match any candidate",
			"last_spell", lastSpell,
			"candidates", len(data.Candidates),
		)
	}

	// Last resort: an ambiguous self-land with no disambiguating recent cast
	// is, by construction, an instant item clicky (a player-cast spell would
	// have logged "You begin casting"). The real spell must therefore be one
	// an item can actually produce. If exactly one candidate is item-produced,
	// resolve to it — this rescues collisions like "Shield of the Eighth"
	// (Coldain ring clicky) sharing cast text with the item-less, never-
	// triggerable "Shield of the Ring".
	if name := e.soleClickableCandidate(data.Candidates); name != "" {
		slog.Debug("timer-debug: ambiguous spell-landed resolved to sole item-clicky candidate",
			"spell", name,
			"candidates", len(data.Candidates),
		)
		return name
	}

	// Known display-equivalent group (e.g. Speed of the Shissar/Brood): the
	// members are indistinguishable in the log and mechanically identical, so
	// rather than dropping the land, resolve to the combined name. The timer
	// still gets a target (you, for a self-land) and the pack trigger dedups
	// into it by SpellID.
	if g := matchAmbiguousLandGroup(data.Candidates); g != nil {
		slog.Debug("timer-debug: ambiguous spell-landed resolved to combined group name",
			"spell", g.displayName,
			"candidates", len(data.Candidates),
		)
		return g.displayName
	}

	slog.Debug("timer-debug: ambiguous spell-landed with no recent cast — skipping",
		"candidates", len(data.Candidates),
		"last_cast_age_ms", lastAge.Milliseconds(),
	)
	return ""
}

// soleClickableCandidate returns the unique candidate name produced by an item
// click/proc effect, or "" when zero or more than one candidate is item-
// produced (so the caller can't safely disambiguate). The clickable-spell set
// is loaded from the DB once and cached.
func (e *Engine) soleClickableCandidate(cands []logparser.SpellLandedCandidate) string {
	clickable := e.clickableIDs()
	if clickable == nil {
		return ""
	}
	var clicky []logparser.SpellLandedCandidate
	for _, c := range cands {
		if clickable[c.SpellID] {
			clicky = append(clicky, c)
		}
	}
	if len(clicky) == 1 {
		return clicky[0].SpellName
	}
	// More than one clicky shares this land text (e.g. the five "Your eyes
	// tingle." clickies). Break the tie with what the player actually carries:
	// if exactly one of these clickies comes from an item in their inventory,
	// that's the one that fired.
	if len(clicky) > 1 {
		if owned := e.ownedClickyIDs(); len(owned) > 0 {
			var name string
			n := 0
			for _, c := range clicky {
				if owned[c.SpellID] {
					n++
					name = c.SpellName
				}
			}
			if n == 1 {
				return name
			}
		}
	}
	return ""
}

// ownedClickyIDs returns the set of clicky spell IDs produced by items the
// active character currently carries, used to disambiguate land text shared by
// several clickies. Empty when inventory is unknown. Cached until
// RefreshModifiers fires (inventory/character change).
func (e *Engine) ownedClickyIDs() map[int]bool {
	if e.ownedItemsFn == nil || e.db == nil {
		return nil
	}
	e.ownedMu.Lock()
	defer e.ownedMu.Unlock()
	if e.ownedClickyLoaded {
		return e.ownedClickySpellIDs
	}
	e.ownedClickyLoaded = true
	items := e.ownedItemsFn()
	if len(items) == 0 {
		return nil
	}
	ids, err := e.db.ClickEffectSpellIDsForItems(items)
	if err != nil {
		slog.Warn("timer-debug: failed to load owned clicky spell IDs", "err", err)
		return nil
	}
	e.ownedClickySpellIDs = ids
	return ids
}

// clickableIDs returns the cached set of item-produced spell IDs, loading it
// from the DB on first call. Returns nil if the DB is unavailable or the load
// fails (callers treat nil as "can't disambiguate").
func (e *Engine) clickableIDs() map[int]bool {
	e.clickableMu.Lock()
	defer e.clickableMu.Unlock()
	if e.clickableLoaded {
		return e.clickableSpellIDs
	}
	e.clickableLoaded = true
	if e.db == nil {
		return nil
	}
	ids, err := e.db.InstantEffectSpellIDs()
	if err != nil {
		slog.Warn("timer-debug: failed to load clickable spell IDs", "err", err)
		return nil
	}
	e.clickableSpellIDs = ids
	return ids
}

// RemoveByID removes a single timer by its composite key (the ID field
// the frontend sees on each timer row). Used by the per-row dismiss
// button. Returns true if a timer was removed.
func (e *Engine) RemoveByID(id string) bool {
	if id == "" {
		return false
	}
	e.mu.Lock()
	_, had := e.timers[id]
	if had {
		delete(e.timers, id)
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if had {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
	return had
}

// removeTimer deletes a single timer by its composite key and broadcasts.
// No-op if the key isn't present. With the keep-expired option on, a worn-off
// signal doesn't drop the row — it flips the timer to overdue (expires now) so
// it lingers as a "needs refresh" reminder until recast or manually dismissed.
func (e *Engine) removeTimer(key string) {
	now := time.Now()
	keep := e.keepExpired()
	e.mu.Lock()
	t, had := e.timers[key]
	if had {
		if keep {
			if t.ExpiresAt.After(now) {
				t.ExpiresAt = now
			}
		} else {
			delete(e.timers, key)
		}
	}
	snap := e.snapshot(now)
	e.mu.Unlock()

	if had {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// removeSelfTimers deletes every timer targeting the active player on their own
// death. It matches BOTH the resolved character name and the literal "You"
// placeholder: timers created before the character context is available (early
// startup) carry TargetName "You", while later ones carry the real name, and a
// plain name match would leave the "You" ones lingering on the overlay after
// death. Timers on other players / mobs are left untouched.
func (e *Engine) removeSelfTimers() {
	active := e.activePlayerName()
	e.mu.Lock()
	removed := 0
	for k, t := range e.timers {
		if t.TargetName == active || t.TargetName == "You" {
			delete(e.timers, k)
			removed++
		}
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if removed > 0 {
		slog.Debug("timer-debug: removed self timers on death", "active", active, "removed", removed)
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// removeOnKill is the EventKill cleanup path: drop timers bound to the
// killed mob (exact TargetName match) AND drop any orphan target-less
// detrimental timers.
//
// Triggers (StartExternal) create detrimental timers without a target —
// the regex match line carries the target text, but the trigger engine
// hasn't been wired to capture and forward it. When the spell-landed
// pipeline doesn't ALSO fire on the same line (because the spell's
// cast_on_other DB text doesn't match, or the cast-by-me gate filters
// it), only the target-less trigger timer exists. An exact target match
// alone would never clear it because TargetName is empty.
//
// In practice the active player almost always debuffs the mob they're
// killing, so wiping orphan detrimentals on any kill matches user
// expectations ("I killed it, the debuff is gone"). Buffs are left
// alone — a target-less buff is usually a self-buff or a raid-wide
// effect that survives a single mob's death.
//
// Charm is the one detrimental excluded from the orphan sweep: a charmed
// pet is a living ally the player keeps fighting WITH, so killing the mob
// it's tanking must not drop the charm timer. Charm orphans clear via their
// charm-break worn-off message (or expiry) instead. A charm timer that IS
// bound to the slain mob (the player killed their own charm) still clears
// through the normal target match below.
func (e *Engine) removeOnKill(target string) {
	if target == "" {
		return
	}
	normTarget := normalizeNPCName(target)
	e.mu.Lock()
	removed := 0
	survivors := make([]string, 0, len(e.timers))
	for k, t := range e.timers {
		match := normalizeNPCName(t.TargetName) == normTarget
		orphan := t.TargetName == "" && isDetrimentalCategory(t.Category) && !t.IsCharm
		if match || orphan {
			delete(e.timers, k)
			removed++
			continue
		}
		if isDetrimentalCategory(t.Category) {
			survivors = append(survivors, t.SpellName+"@"+t.TargetName)
		}
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if removed > 0 {
		slog.Debug("timer-debug: removed timers on kill", "target", target, "removed", removed)
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	} else {
		slog.Debug("timer-debug: kill matched no timers",
			"target", target,
			"normalized", normTarget,
			"detrimental_survivors", survivors)
	}
}

// normalizeNPCName returns a lowercased, trimmed form of an NPC name with any
// leading article ("a ", "an ", "the ") removed. EQ logs and the Zeal pipe
// usually agree on the surface form, but case or article quirks can sneak in
// when a name is captured from different sources (kill line vs. spell-landed
// line vs. pipe target slot). Using the normalized form for equality keeps
// removeOnKill robust to those drift cases.
func normalizeNPCName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "a "):
		return s[2:]
	case strings.HasPrefix(s, "an "):
		return s[3:]
	case strings.HasPrefix(s, "the "):
		return s[4:]
	}
	return s
}

// parseCorpseTarget detects an "X's corpse" target name — the form Zeal sends
// when the player has a corpse selected — and returns the underlying NPC name
// with a flag. Both the space and underscore variants are recognised since the
// pipe may deliver either depending on the EQ build. Mirrors
// overlay.stripCorpseSuffix; kept local to avoid a cross-package dependency for
// one small string check.
func parseCorpseTarget(name string) (string, bool) {
	const spaceSuffix = "'s corpse"
	const underscoreSuffix = "'s_corpse"
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, spaceSuffix):
		return strings.TrimSpace(name[:len(name)-len(spaceSuffix)]), true
	case strings.HasSuffix(lower, underscoreSuffix):
		return strings.TrimSpace(name[:len(name)-len(underscoreSuffix)]), true
	}
	return name, false
}

// seCharm is the EQ spell effect id (SPA) for Charm in the Quarm/EQMacEmu
// spell data (spells_new.effectidN). A charm timer tracks a living pet the
// player controls, which is why it gets special handling in removeOnKill.
const seCharm = 22

// isCharmSpell reports whether a spell applies the Charm effect. Used to flag
// charm timers (Charm/Beguile/Cajoling Whispers/Allure/Dictate/Boltran's
// Agacerie) so they aren't swept up by the kill-time orphan cleanup.
func isCharmSpell(spell *db.Spell) bool {
	if spell == nil {
		return false
	}
	for _, id := range spell.EffectIDs {
		if id == seCharm {
			return true
		}
	}
	return false
}

// removeCharmTimers deletes every charm timer. Called on EventCharmBroken
// ("Your charm spell has worn off."), the generic line EQ emits for any charm
// spell breaking. Only one charm can be active per character, so clearing all
// charm-flagged timers is correct — and it's the worn-off path the charm
// triggers can't express (their per-name pattern never matches the generic
// line).
func (e *Engine) removeCharmTimers() {
	e.mu.Lock()
	removed := 0
	for k, t := range e.timers {
		if t.IsCharm {
			delete(e.timers, k)
			removed++
		}
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if removed > 0 {
		slog.Debug("timer-debug: cleared charm timers on charm break", "removed", removed)
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// isDetrimentalCategory reports whether a timer category represents a
// hostile effect cast on an enemy. Mirrors the categories the cast index
// routes through the detrimental scope path.
func isDetrimentalCategory(c Category) bool {
	switch c {
	case CategoryDebuff, CategoryDot, CategoryMez, CategoryStun:
		return true
	}
	return false
}

// removeIllusionsForPlayer deletes every "Illusion: …" buff timer keyed to
// the given player. EQ's two illusion-end messages omit the race name, so
// the engine cannot pinpoint a specific timer — but only one illusion can be
// active at a time per character, so blanket-clearing them is correct.
func (e *Engine) removeIllusionsForPlayer(player string) {
	if player == "" {
		return
	}
	e.mu.Lock()
	removed := 0
	for k, t := range e.timers {
		if t.TargetName == player && strings.HasPrefix(t.SpellName, "Illusion: ") {
			delete(e.timers, k)
			removed++
		}
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if removed > 0 {
		slog.Debug("timer-debug: removed illusion timers", "player", player, "removed", removed)
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// removeBySpellNameOrID handles a worn-off / fade signal for a spell, matching
// timers by SpellName or (when spellID > 0) SpellID, regardless of target. Used
// by StopExternal when a custom-trigger worn-off pattern fires.
//
// A "...worn off." line names no target, so it can't say WHICH instance ended.
// Policy splits by category:
//
//   - Detrimentals (mez/debuff/dot/stun) are cast on individual mobs that break
//     independently, each on its own worn-off line. So when several same-spell
//     detrimental timers are active we peel off just ONE per signal — the one
//     nearest expiry, i.e. the earliest-cast still running, which is the one
//     that would naturally drop first. Successive breaks remove the rows one at
//     a time in cast order. (Wiping all of them on the first break was the
//     reported AoE-mez bug, where every mez timer vanished at once.)
//
//   - Buffs keep wipe-all: a group buff is cast once and its members' timers
//     fade together, so the single worn-off line the caster sees clears the
//     whole set. This also preserves the SpellID-merge fade path for haste
//     buffs ("Your body slows.") that survived under the DB name.
//
// A single matching timer collapses to the same result either way.
func (e *Engine) removeBySpellNameOrID(spellName string, spellID int) {
	if spellName == "" && spellID <= 0 {
		return
	}
	now := time.Now()
	keep := e.keepExpired()
	e.mu.Lock()
	var matches []string
	allDetrimental := true
	for k, t := range e.timers {
		if t.SpellName == spellName || (spellID > 0 && t.SpellID == spellID) {
			matches = append(matches, k)
			if !isDetrimentalCategory(t.Category) {
				allDetrimental = false
			}
		}
	}

	// With keep-expired on, a worn-off signal flips the matched row(s) to
	// overdue (expires now) so they linger as reminders; otherwise it deletes.
	clear := func(k string) {
		if keep {
			if e.timers[k].ExpiresAt.After(now) {
				e.timers[k].ExpiresAt = now
			}
		} else {
			delete(e.timers, k)
		}
	}

	removed := 0
	if allDetrimental && len(matches) > 1 {
		// Peel off only the single nearest-expiry instance.
		earliest := matches[0]
		for _, k := range matches[1:] {
			if e.timers[k].ExpiresAt.Before(e.timers[earliest].ExpiresAt) {
				earliest = k
			}
		}
		clear(earliest)
		removed = 1
	} else {
		for _, k := range matches {
			clear(k)
			removed++
		}
	}
	snap := e.snapshot(now)
	e.mu.Unlock()

	if removed > 0 {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

func (e *Engine) clearAll() {
	e.mu.Lock()
	wasEmpty := len(e.timers) == 0
	e.timers = make(map[string]*ActiveTimer)
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if !wasEmpty {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// keepExpiredMaxOverdue caps how long a kept-expired timer lingers as an
// overdue reminder before the engine drops it anyway. Without this backstop a
// row the user never recasts or dismisses (zoned away, logged for the night)
// would accumulate in the overlay forever.
const keepExpiredMaxOverdue = 60 * time.Minute

// pruneExpired removes timers whose expiry time has passed. With the
// keep-expired option on, a past-expiry timer is instead left in place so
// snapshot() can surface it as an overdue count-up — until it has been overdue
// longer than keepExpiredMaxOverdue, at which point it's dropped regardless.
func (e *Engine) pruneExpired() {
	now := time.Now()
	keep := e.keepExpired()
	e.mu.Lock()
	for name, t := range e.timers {
		if !now.After(t.ExpiresAt) {
			continue
		}
		if keep && now.Sub(t.ExpiresAt) <= keepExpiredMaxOverdue {
			continue
		}
		delete(e.timers, name)
	}
	e.mu.Unlock()
}

func (e *Engine) broadcast() {
	e.mu.Lock()
	snap := e.snapshot(time.Now())
	e.mu.Unlock()
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

// snapshot builds an immutable TimerState. Must be called with e.mu held.
func (e *Engine) snapshot(now time.Time) TimerState {
	timers := make([]ActiveTimer, 0, len(e.timers))
	keep := e.keepExpired()
	for _, t := range e.timers {
		remaining := t.ExpiresAt.Sub(now).Seconds()
		entry := *t
		if remaining < 0 {
			if keep {
				// Overdue: report the (negative) seconds-since-expiry so the
				// overlay can render a count-up "needs refresh" row, and flag
				// it so the frontend can style it distinctly. These sort to the
				// top (most negative first) — most urgent.
				entry.Expired = true
			} else {
				remaining = 0
			}
		}
		entry.RemainingSeconds = remaining
		timers = append(timers, entry)
	}
	// Sort ascending by remaining time so the most urgent timers appear first.
	for i := 1; i < len(timers); i++ {
		for j := i; j > 0 && timers[j].RemainingSeconds < timers[j-1].RemainingSeconds; j-- {
			timers[j], timers[j-1] = timers[j-1], timers[j]
		}
	}
	return TimerState{
		Timers:      timers,
		LastUpdated: now,
	}
}

// applyDurationModifiers extends baseDurationSec by any matching item/AA
// duration focuses for the active character. Returns baseDurationSec unchanged
// when no character context is configured, no modifiers apply, or any lookup
// fails (errors are logged at info level — a missing Quarmy file just means
// "no extensions yet" and shouldn't break timers).
func (e *Engine) applyDurationModifiers(spell *db.Spell, baseDurationSec float64) float64 {
	if e.charCtx == nil {
		return baseDurationSec
	}
	eqPath, charName, casterClass := e.charCtx()
	if eqPath == "" || charName == "" {
		return baseDurationSec
	}

	contribs, permIllusion, ok := e.contributorsFor(eqPath, charName)
	if !ok {
		return baseDurationSec
	}

	// Permanent Illusion AA: EQMacEmu overrides the duration of any self-cast
	// illusion (SPA 58) to a flat 10000 ticks (~16h40m) — the formula duration
	// and AA/item focus percentages never enter into it.
	if permIllusion && buffmod.HasIllusionEffect(spell.EffectIDs[:]) {
		slog.Debug("timer-debug: permanent illusion override",
			"name", spell.Name,
			"base_sec", int(baseDurationSec),
			"override_sec", buffmod.PermanentIllusionDurationSec,
		)
		return float64(buffmod.PermanentIllusionDurationSec)
	}

	spellType := buffmod.SpellTypeBeneficial
	if spell.GoodEffect != 1 {
		spellType = buffmod.SpellTypeDetrimental
	}
	res := buffmod.Resolve(
		spell.ID, spell.Name,
		buffmod.SpellLevelForClass(spell.ClassLevels, casterClass),
		defaultCasterLevel,
		int(baseDurationSec),
		spellType,
		spell.EffectIDs[:],
		contribs,
		casterClass,
		spell.ClassLevels,
	)
	if res.ExtendedDurationSec <= 0 || res.ExtendedDurationSec == int(baseDurationSec) {
		return baseDurationSec
	}
	slog.Debug("timer-debug: applied duration modifiers",
		"name", spell.Name,
		"base_sec", int(baseDurationSec),
		"extended_sec", res.ExtendedDurationSec,
		"aa_pct", res.DurationAAPercent,
		"item_pct", res.DurationItemPercent,
	)
	return float64(res.ExtendedDurationSec)
}

// contributorsFor returns the cached buffmod contributors for charName plus
// whether the character has the Permanent Illusion AA, recomputing from the
// Quarmy export if the cache is empty or stale. The final bool is false when
// computation failed (e.g. no export yet) — callers should fall back to the
// unextended duration.
func (e *Engine) contributorsFor(eqPath, charName string) ([]buffmod.Modifier, bool, bool) {
	e.modMu.Lock()
	if e.modCharName == charName && e.modContribs != nil {
		c, p := e.modContribs, e.modPermIllusion
		e.modMu.Unlock()
		return c, p, true
	}
	e.modMu.Unlock()

	res, err := buffmod.Compute(eqPath, charName, e.db)
	if err != nil {
		slog.Debug("timer-debug: buffmod.Compute failed (using base duration)",
			"character", charName, "err", err)
		return nil, false, false
	}

	e.modMu.Lock()
	e.modCharName = charName
	e.modContribs = res.Contributors
	e.modPermIllusion = res.PermanentIllusion
	c, p := e.modContribs, e.modPermIllusion
	e.modMu.Unlock()
	return c, p, true
}

// categorize determines the timer category from a spell's effect IDs and the
// goodEffect flag. Checked in priority order: mez > stun > DoT > buff/debuff.
//
// The mez/stun/DoT precedence intentionally runs first so a damage-over-time
// spell with goodEffect=1 (rare but it happens for certain proc effects)
// still surfaces as a DoT.
func categorize(spell *db.Spell) Category {
	// A self-targeted beneficial spell is always a buff, even when one of its
	// effect slots carries a negative HP value. That slot is an HP cost/drain
	// component (e.g. Ancient: Master of Death's -63 HP), not a damage-over-time
	// — you can't DoT or debuff yourself — so it must not trip the DoT detection
	// below. Enemy-targeted goodEffect=1 DoT procs (target type != self) are
	// unaffected and still surface as DoTs.
	selfBuff := spell.GoodEffect == 1 && spell.TargetType == targetTypeSelf
	for i, effID := range spell.EffectIDs {
		switch effID {
		case 18: // Mesmerize
			return CategoryMez
		case 23: // Stun
			return CategoryStun
		case 0:
			// Effect 0 is HP: positive base = heal/regen, negative = damage over time.
			if spell.EffectBaseValues[i] < 0 && !selfBuff {
				return CategoryDot
			}
		}
	}
	// spells_new.goodEffect is authoritative for buff vs debuff classification:
	// EQ devs hand-flag every beneficial spell. Target type alone misses
	// single-target friendly buffs (target type 5 with goodEffect=1).
	if spell.GoodEffect == 1 {
		return CategoryBuff
	}
	// Override for self-target item clickies that the source data
	// mis-flags as detrimental. Maelin's Magical Concoction (the Velious
	// Enchanter clicky mana buff) ships as goodEffect=0 in the PEQ data
	// even though it's a beneficial self-only buff — without this it ends
	// up in the detrimental overlay. The combined "self target + no class
	// can cast it" check keeps the override narrow enough that legitimate
	// player-cast debuffs aren't reclassified.
	if spell.TargetType == targetTypeSelf && isItemClicky(spell) {
		return CategoryBuff
	}
	return CategoryDebuff
}

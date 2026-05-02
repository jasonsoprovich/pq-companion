package spelltimer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// CharacterContext supplies the active character + EQ install path so the
// engine can resolve item/AA duration focuses for a cast. Returning empty
// strings disables modifier resolution (timers fall back to base duration).
type CharacterContext func() (eqPath, charName string)

// ScopeProvider returns the user-configured tracking scope ("self" or
// "anyone"). The engine calls this on every landed event so a config change
// takes effect immediately without restarting the engine. Empty / unknown
// values are treated as "anyone" to match the default behaviour.
type ScopeProvider func() string

const (
	scopeSelf   = "self"
	scopeAnyone = "anyone"
)

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

// timerKey returns the composite map key used to identify a timer. Targets
// that aren't tied to a specific recipient (e.g. trigger-driven timers) pass
// an empty string — the resulting key still namespaces them away from any
// spell-derived timer with the same spell name.
func timerKey(spellName, targetName string) string {
	return spellName + timerKeySep + targetName
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
	hub      *ws.Hub
	db       *db.DB
	charCtx  CharacterContext
	scopeFn  ScopeProvider

	mu     sync.Mutex
	timers map[string]*ActiveTimer // keyed by timerKey(spell, target)

	// lastCastSpell and lastCastAt track the most recent EventSpellCast.
	// Used (a) to disambiguate ambiguous EventSpellLanded matches — many
	// spells share identical cast-on text — and (b) historically to
	// correlate "Your spell did not take hold." with a specific cast.
	lastCastSpell string
	lastCastAt    time.Time

	// modifier cache: keeps the last-computed contributors per character so
	// the engine doesn't re-parse the Quarmy export on every cast. Invalidated
	// by character change or RefreshModifiers().
	modMu        sync.Mutex
	modCharName  string
	modContribs  []buffmod.Modifier
}

// NewEngine returns an initialised Engine ready to receive log events.
// charCtx may be nil (timers fall back to base / unextended duration).
// scopeFn may be nil (engine behaves as if scope is "anyone").
func NewEngine(hub *ws.Hub, database *db.DB, charCtx CharacterContext, scopeFn ScopeProvider) *Engine {
	return &Engine{
		hub:     hub,
		db:      database,
		charCtx: charCtx,
		scopeFn: scopeFn,
		timers:  make(map[string]*ActiveTimer),
	}
}

// trackingScope returns the user's currently-configured scope, defaulting to
// "anyone" when the provider is absent or returns an empty/unknown value.
func (e *Engine) trackingScope() string {
	if e.scopeFn == nil {
		return scopeAnyone
	}
	switch s := e.scopeFn(); s {
	case scopeSelf:
		return scopeSelf
	default:
		return scopeAnyone
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
	e.modMu.Unlock()
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
			slog.Info("timer-debug: spell-cast event with bad payload", "data_type", fmt.Sprintf("%T", ev.Data))
			return
		}
		e.mu.Lock()
		e.lastCastSpell = data.SpellName
		e.lastCastAt = time.Now()
		e.mu.Unlock()
		slog.Info("timer-debug: spell-cast recorded (awaiting land)", "spell", data.SpellName, "ts", ev.Timestamp)

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

	case logparser.EventZone, logparser.EventDeath:
		// Zone change or death clears all active timers.
		e.clearAll()
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
	_, name := e.charCtx()
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
func (e *Engine) StartExternal(name string, category string, durationSecs, displayThresholdSecs int, startedAt time.Time) {
	if name == "" || durationSecs <= 0 {
		return
	}
	cat := Category(category)
	switch cat {
	case CategoryBuff, CategoryDebuff, CategoryMez, CategoryDot, CategoryStun:
	default:
		cat = CategoryDebuff
	}

	// Custom triggers don't carry a target. The composite key still
	// namespaces them so a trigger named "Visions of Grandeur" can't collide
	// with the per-target spell-landed entries, but we additionally dedup
	// against any same-spell-name timer to avoid the user seeing two rows
	// for the same buff (one from the spell-landed pipeline, one from a
	// custom trigger they configured before the pipeline existed).
	key := timerKey(name, "")

	e.mu.Lock()
	for _, existing := range e.timers {
		if existing.SpellName == name && time.Since(existing.CastAt) < dedupGraceWindow {
			e.mu.Unlock()
			slog.Info("timer-debug: duplicate external timer suppressed (within grace window)",
				"name", name,
				"existing_target", existing.TargetName,
				"existing_age_ms", time.Since(existing.CastAt).Milliseconds(),
			)
			return
		}
	}
	timer := &ActiveTimer{
		ID:                   key,
		SpellName:            name,
		Category:             cat,
		CastAt:               startedAt,
		StartsAt:             startedAt,
		ExpiresAt:            startedAt.Add(time.Duration(durationSecs) * time.Second),
		DurationSeconds:      float64(durationSecs),
		DisplayThresholdSecs: displayThresholdSecs,
	}
	e.timers[key] = timer
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Info("timer-debug: external timer started (trigger-driven)",
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
func (e *Engine) StopExternal(name string) {
	e.removeBySpellName(name)
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

	// Tracking scope filter: in "self" mode the engine drops every land
	// whose recipient isn't the active player. The "You" fallback is also
	// treated as self so users testing before the active character is known
	// still see their own buffs.
	if e.trackingScope() == scopeSelf {
		active := e.activePlayerName()
		if target != active && target != "You" {
			slog.Info("timer-debug: spell-landed skipped (scope=self, non-self target)",
				"spell", spellName, "target", target, "active", active)
			return
		}
	}

	spell, err := e.db.GetSpellByExactName(spellName)
	if err != nil {
		slog.Warn("spelltimer: DB error looking up spell", "name", spellName, "err", err)
		return
	}
	if spell == nil {
		slog.Info("timer-debug: landed spell not found in DB (no timer created)", "name", spellName)
		return
	}

	durationTicks := CalcDurationTicks(spell.BuffDurationFormula, spell.BuffDuration, defaultCasterLevel)
	if durationTicks <= 0 {
		slog.Info("timer-debug: landed spell has zero duration (no timer created)",
			"name", spellName,
			"formula", spell.BuffDurationFormula,
			"buff_duration", spell.BuffDuration,
		)
		return
	}

	baseDurationSec := float64(durationTicks) * eqTickSeconds
	durationSeconds := e.applyDurationModifiers(spell, baseDurationSec)
	expiresAt := landedAt.Add(time.Duration(float64(time.Second) * durationSeconds))

	key := timerKey(spellName, target)
	timer := &ActiveTimer{
		ID:              key,
		SpellName:       spellName,
		SpellID:         spell.ID,
		TargetName:      target,
		Category:        categorize(spell),
		CastAt:          landedAt,
		StartsAt:        landedAt,
		ExpiresAt:       expiresAt,
		DurationSeconds: durationSeconds,
	}

	e.mu.Lock()
	// Recasting on the same target replaces the timer (refresh). No dedup
	// against the same key is needed; with composite keys, recasts on the
	// SAME target replace cleanly and casts on DIFFERENT targets coexist.
	e.timers[key] = timer
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Info("timer-debug: timer created from spell-landed",
		"spell", spellName,
		"target", target,
		"category", timer.Category,
		"duration_secs", durationSeconds,
		"active_timer_count", len(snap.Timers),
	)
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
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
	e.mu.Lock()
	lastSpell := e.lastCastSpell
	lastAge := time.Since(e.lastCastAt)
	e.mu.Unlock()

	if lastSpell == "" || lastAge > lastCastWindow {
		slog.Info("timer-debug: ambiguous spell-landed with no recent cast — skipping",
			"candidates", len(data.Candidates),
			"last_cast_age_ms", lastAge.Milliseconds(),
		)
		return ""
	}
	for _, c := range data.Candidates {
		if c.SpellName == lastSpell {
			return c.SpellName
		}
	}
	slog.Info("timer-debug: ambiguous spell-landed; recent cast doesn't match any candidate",
		"last_spell", lastSpell,
		"candidates", len(data.Candidates),
	)
	return ""
}

// removeTimer deletes a single timer by its composite key and broadcasts.
// No-op if the key isn't present.
func (e *Engine) removeTimer(key string) {
	e.mu.Lock()
	_, had := e.timers[key]
	delete(e.timers, key)
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if had {
		e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
	}
}

// removeBySpellName deletes every timer whose SpellName matches, regardless
// of target. Used by StopExternal: a custom-trigger worn-off pattern is
// presumed to wipe the spell entirely (the user wrote it that way), and we
// also want to catch any spell-landed timer we may have created in parallel.
func (e *Engine) removeBySpellName(spellName string) {
	if spellName == "" {
		return
	}
	e.mu.Lock()
	removed := 0
	for k, t := range e.timers {
		if t.SpellName == spellName {
			delete(e.timers, k)
			removed++
		}
	}
	snap := e.snapshot(time.Now())
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

// pruneExpired removes timers whose expiry time has passed.
func (e *Engine) pruneExpired() {
	now := time.Now()
	e.mu.Lock()
	for name, t := range e.timers {
		if now.After(t.ExpiresAt) {
			delete(e.timers, name)
		}
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
	for _, t := range e.timers {
		remaining := t.ExpiresAt.Sub(now).Seconds()
		if remaining < 0 {
			remaining = 0
		}
		entry := *t
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
	eqPath, charName := e.charCtx()
	if eqPath == "" || charName == "" {
		return baseDurationSec
	}

	contribs, ok := e.contributorsFor(eqPath, charName)
	if !ok {
		return baseDurationSec
	}

	spellType := buffmod.SpellTypeBeneficial
	if spell.GoodEffect != 1 {
		spellType = buffmod.SpellTypeDetrimental
	}
	res := buffmod.Resolve(
		spell.ID, spell.Name,
		buffmod.SpellLevel(spell.ClassLevels),
		defaultCasterLevel,
		int(baseDurationSec),
		spellType,
		spell.EffectIDs[:],
		contribs,
	)
	if res.ExtendedDurationSec <= 0 || res.ExtendedDurationSec == int(baseDurationSec) {
		return baseDurationSec
	}
	slog.Info("timer-debug: applied duration modifiers",
		"name", spell.Name,
		"base_sec", int(baseDurationSec),
		"extended_sec", res.ExtendedDurationSec,
		"aa_pct", res.DurationAAPercent,
		"item_pct", res.DurationItemPercent,
	)
	return float64(res.ExtendedDurationSec)
}

// contributorsFor returns the cached buffmod contributors for charName,
// recomputing from the Quarmy export if the cache is empty or stale.
// The bool is false when computation failed (e.g. no export yet) — callers
// should fall back to the unextended duration.
func (e *Engine) contributorsFor(eqPath, charName string) ([]buffmod.Modifier, bool) {
	e.modMu.Lock()
	if e.modCharName == charName && e.modContribs != nil {
		c := e.modContribs
		e.modMu.Unlock()
		return c, true
	}
	e.modMu.Unlock()

	res, err := buffmod.Compute(eqPath, charName, e.db)
	if err != nil {
		slog.Info("timer-debug: buffmod.Compute failed (using base duration)",
			"character", charName, "err", err)
		return nil, false
	}

	e.modMu.Lock()
	e.modCharName = charName
	e.modContribs = res.Contributors
	c := e.modContribs
	e.modMu.Unlock()
	return c, true
}

// categorize determines the timer category from a spell's effect IDs and the
// goodEffect flag. Checked in priority order: mez > stun > DoT > buff/debuff.
//
// The mez/stun/DoT precedence intentionally runs first so a damage-over-time
// spell with goodEffect=1 (rare but it happens for certain proc effects)
// still surfaces as a DoT.
func categorize(spell *db.Spell) Category {
	for i, effID := range spell.EffectIDs {
		switch effID {
		case 18: // Mesmerize
			return CategoryMez
		case 23: // Stun
			return CategoryStun
		case 0:
			// Effect 0 is HP: positive base = heal/regen, negative = damage over time.
			if spell.EffectBaseValues[i] < 0 {
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
	return CategoryDebuff
}

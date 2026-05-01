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

// broadcastInterval is how often the engine pushes timer state updates to
// WebSocket clients while timers are active.
const broadcastInterval = time.Second

// dedupGraceWindow is the time after a timer is created during which a second
// create attempt for the same spell is treated as a redundant duplicate (e.g.
// the cast-event path and a user trigger both firing for the same cast).
// Outside this window a same-name create overwrites — supporting buff refresh
// on real recasts.
const dedupGraceWindow = 3 * time.Second

// didNotTakeHoldWindow is the time after EventSpellCast during which a
// "Your spell did not take hold." message is correlated to that cast and
// triggers cancellation of the just-created timer. EQ emits the failure
// message immediately after the cast resolves (sub-second), so a small
// window is enough to filter out unrelated past casts.
const didNotTakeHoldWindow = 5 * time.Second

// Engine watches parsed log events, maintains a live map of active spell
// timers, and broadcasts state changes via WebSocket.
//
// Each spell is keyed by its name. Casting the same spell a second time
// replaces the existing timer (e.g. refreshing Slow). This is the correct
// behaviour for self-buffs and single-target debuffs.
type Engine struct {
	hub      *ws.Hub
	db       *db.DB
	charCtx  CharacterContext

	mu     sync.Mutex
	timers map[string]*ActiveTimer // keyed by spell name

	// lastCastSpell and lastCastAt track the most recent EventSpellCast so the
	// engine can correlate "Your spell did not take hold." (EQ omits the spell
	// name from that message) with the spell it just tried to cast.
	lastCastSpell string
	lastCastAt    time.Time

	// modifier cache: keeps the last-computed contributors per character so
	// the engine doesn't re-parse the Quarmy export on every cast. Invalidated
	// by character change or RefreshModifiers().
	modMu        sync.Mutex
	modCharName  string
	modContribs  []buffmod.Modifier
}

// NewEngine returns an initialised Engine ready to receive log events. charCtx
// may be nil; when nil, timers fall back to base (unextended) duration.
func NewEngine(hub *ws.Hub, database *db.DB, charCtx CharacterContext) *Engine {
	return &Engine{
		hub:     hub,
		db:      database,
		charCtx: charCtx,
		timers:  make(map[string]*ActiveTimer),
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
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventSpellCast:
		data, ok := ev.Data.(logparser.SpellCastData)
		if !ok {
			slog.Info("timer-debug: spell-cast event with bad payload", "data_type", fmt.Sprintf("%T", ev.Data))
			return
		}
		slog.Info("timer-debug: spell-cast event received", "spell", data.SpellName, "ts", ev.Timestamp)
		// Record the cast so a subsequent "did not take hold" message can be
		// correlated back to this spell (EQ omits the spell name from that
		// message). Recorded with wall time, not log time, because the
		// correlation window is measured against time.Now() in Handle.
		e.mu.Lock()
		e.lastCastSpell = data.SpellName
		e.lastCastAt = time.Now()
		e.mu.Unlock()
		e.onSpellCast(ev.Timestamp, data.SpellName)

	case logparser.EventSpellDidNotTakeHold:
		// Cancel the just-created timer for the most recent cast, if any and
		// if the cast was recent enough to be the spell EQ is referring to.
		e.mu.Lock()
		spell := e.lastCastSpell
		age := time.Since(e.lastCastAt)
		e.mu.Unlock()
		if spell == "" || age > didNotTakeHoldWindow {
			slog.Info("timer-debug: did-not-take-hold seen but no recent cast to cancel",
				"last_spell", spell,
				"last_cast_age_ms", age.Milliseconds(),
			)
			return
		}
		slog.Info("timer-debug: did-not-take-hold cancelling timer",
			"spell", spell,
			"cast_age_ms", age.Milliseconds(),
		)
		e.removeTimer(spell)

	case logparser.EventSpellInterrupt:
		data, ok := ev.Data.(logparser.SpellInterruptData)
		if !ok || data.SpellName == "" {
			return
		}
		// Named interrupt — cancel the pending timer for this spell.
		e.removeTimer(data.SpellName)

	case logparser.EventSpellResist:
		data, ok := ev.Data.(logparser.SpellResistData)
		if !ok {
			return
		}
		// Spell was resisted — cancel the pending timer.
		e.removeTimer(data.SpellName)

	case logparser.EventSpellFade:
		data, ok := ev.Data.(logparser.SpellFadeData)
		if !ok {
			return
		}
		// Buff naturally wore off — remove the timer.
		e.removeTimer(data.SpellName)

	case logparser.EventSpellFadeFrom:
		data, ok := ev.Data.(logparser.SpellFadeFromData)
		if !ok {
			return
		}
		// Buff faded from a specific target — remove the timer keyed by spell name.
		e.removeTimer(data.SpellName)

	case logparser.EventZone, logparser.EventDeath:
		// Zone change or death clears all active timers.
		e.clearAll()
	}
}

// GetState returns a point-in-time snapshot of all active timers.
func (e *Engine) GetState() TimerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot(time.Now())
}

// StartExternal adds a timer not driven by a log cast event. Used by the
// trigger engine when a user-defined trigger with timer_type set matches a log
// line. If a timer with the same name already exists it is replaced. Category
// is a plain string (to avoid a dependency cycle from trigger package) and
// must be one of "buff", "debuff", "mez", "dot", "stun" — anything else is
// treated as "debuff".
//
// durationSecs must be > 0. Returns early without change if duration is 0.
func (e *Engine) StartExternal(name string, category string, durationSecs int, startedAt time.Time) {
	if name == "" || durationSecs <= 0 {
		return
	}
	cat := Category(category)
	switch cat {
	case CategoryBuff, CategoryDebuff, CategoryMez, CategoryDot, CategoryStun:
	default:
		cat = CategoryDebuff
	}
	duration := float64(durationSecs)
	timer := &ActiveTimer{
		ID:              name,
		SpellName:       name,
		Category:        cat,
		CastAt:          startedAt,
		StartsAt:        startedAt,
		ExpiresAt:       startedAt.Add(time.Duration(durationSecs) * time.Second),
		DurationSeconds: duration,
	}

	e.mu.Lock()
	if existing, ok := e.timers[name]; ok && time.Since(existing.CastAt) < dedupGraceWindow {
		e.mu.Unlock()
		slog.Info("timer-debug: duplicate external timer suppressed (within grace window)",
			"name", name,
			"existing_age_ms", time.Since(existing.CastAt).Milliseconds(),
		)
		return
	}
	e.timers[name] = timer
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

// StopExternal removes a timer by name. Used by the trigger engine when a
// "worn off" pattern matches for a timer-driven trigger.
func (e *Engine) StopExternal(name string) {
	e.removeTimer(name)
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

func (e *Engine) onSpellCast(castAt time.Time, spellName string) {
	spell, err := e.db.GetSpellByExactName(spellName)
	if err != nil {
		slog.Warn("spelltimer: DB error looking up spell", "name", spellName, "err", err)
		return
	}
	if spell == nil {
		slog.Info("timer-debug: spell not found in DB (no timer created)", "name", spellName)
		return
	}

	durationTicks := CalcDurationTicks(spell.BuffDurationFormula, spell.BuffDuration, defaultCasterLevel)
	if durationTicks <= 0 {
		slog.Info("timer-debug: spell has zero duration (no timer created)",
			"name", spellName,
			"formula", spell.BuffDurationFormula,
			"buff_duration", spell.BuffDuration,
		)
		return
	}

	baseDurationSec := float64(durationTicks) * eqTickSeconds
	durationSeconds := e.applyDurationModifiers(spell, baseDurationSec)
	startsAt := castAt.Add(time.Duration(spell.CastTime) * time.Millisecond)
	expiresAt := startsAt.Add(time.Duration(float64(time.Second) * durationSeconds))

	timer := &ActiveTimer{
		ID:              spellName,
		SpellName:       spellName,
		SpellID:         spell.ID,
		Category:        categorize(spell),
		CastAt:          castAt,
		StartsAt:        startsAt,
		ExpiresAt:       expiresAt,
		DurationSeconds: durationSeconds,
	}

	e.mu.Lock()
	if existing, ok := e.timers[spellName]; ok && time.Since(existing.CastAt) < dedupGraceWindow {
		e.mu.Unlock()
		slog.Info("timer-debug: duplicate cast suppressed (within grace window)",
			"name", spellName,
			"existing_age_ms", time.Since(existing.CastAt).Milliseconds(),
		)
		return
	}
	e.timers[spellName] = timer
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Info("timer-debug: timer created and broadcast",
		"name", spellName,
		"category", timer.Category,
		"duration_secs", durationSeconds,
		"active_timer_count", len(snap.Timers),
	)
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

func (e *Engine) removeTimer(spellName string) {
	e.mu.Lock()
	_, had := e.timers[spellName]
	delete(e.timers, spellName)
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	if had {
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

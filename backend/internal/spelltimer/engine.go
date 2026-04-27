package spelltimer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// broadcastInterval is how often the engine pushes timer state updates to
// WebSocket clients while timers are active.
const broadcastInterval = time.Second

// Engine watches parsed log events, maintains a live map of active spell
// timers, and broadcasts state changes via WebSocket.
//
// Each spell is keyed by its name. Casting the same spell a second time
// replaces the existing timer (e.g. refreshing Slow). This is the correct
// behaviour for self-buffs and single-target debuffs.
type Engine struct {
	hub *ws.Hub
	db  *db.DB

	mu     sync.Mutex
	timers map[string]*ActiveTimer // keyed by spell name
}

// NewEngine returns an initialised Engine ready to receive log events.
func NewEngine(hub *ws.Hub, database *db.DB) *Engine {
	return &Engine{
		hub:    hub,
		db:     database,
		timers: make(map[string]*ActiveTimer),
	}
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
		e.onSpellCast(ev.Timestamp, data.SpellName)

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

	durationSeconds := float64(durationTicks) * eqTickSeconds
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

// categorize determines the timer category from a spell's effect IDs and
// target type. Checked in priority order: mez > stun > DoT > buff > debuff.
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
	// Target type determines buff vs debuff for everything else.
	// 3 = Group v1, 6 = Self, 10 = Group v2, 41 = All of group
	switch spell.TargetType {
	case 3, 6, 10, 41:
		return CategoryBuff
	}
	return CategoryDebuff
}

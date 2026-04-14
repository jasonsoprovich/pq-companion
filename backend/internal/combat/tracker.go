package combat

import (
	"sort"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// internalEntity accumulates raw hit data for one combatant inside an active fight.
type internalEntity struct {
	totalDamage int64
	hitCount    int
	maxHit      int
}

// internalFight holds mutable state for the currently active fight.
type internalFight struct {
	// id is a monotonic counter used to guard against stale timer callbacks.
	id        int
	startTime time.Time
	lastHit   time.Time

	// outgoing tracks actors dealing damage to non-"You" targets (players, etc.).
	outgoing map[string]*internalEntity
	// incoming tracks actors dealing damage to "You" (NPCs hitting the player).
	incoming map[string]*internalEntity
}

// Tracker watches parsed log events, groups them into fights, and maintains
// per-entity damage statistics and session-level DPS aggregates.
type Tracker struct {
	hub *ws.Hub

	mu           sync.Mutex
	fightCounter int
	active       *internalFight
	endTimer     *time.Timer

	recentFights []FightSummary

	// session aggregates (player personal outgoing damage only)
	sessionDamage    int64
	sessionFightTime float64 // total seconds spent in completed fights
}

// NewTracker returns an initialised combat Tracker.
func NewTracker(hub *ws.Hub) *Tracker {
	return &Tracker{hub: hub}
}

// Handle processes a single parsed log event.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventCombatHit:
		data, ok := ev.Data.(logparser.CombatHitData)
		if !ok {
			return
		}
		t.recordHit(ev.Timestamp, data)

	case logparser.EventZone, logparser.EventDeath:
		t.endFight(true)
	}
}

// GetState returns a point-in-time snapshot of the current combat state.
func (t *Tracker) GetState() CombatState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot(time.Now())
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (t *Tracker) recordHit(ts time.Time, data logparser.CombatHitData) {
	t.mu.Lock()

	// Start a new fight if none is active.
	if t.active == nil {
		t.fightCounter++
		t.active = &internalFight{
			id:        t.fightCounter,
			startTime: ts,
			lastHit:   ts,
			outgoing:  make(map[string]*internalEntity),
			incoming:  make(map[string]*internalEntity),
		}
	} else {
		t.active.lastHit = ts
	}

	// Route to outgoing or incoming based on target.
	var entityMap map[string]*internalEntity
	if data.Target == "You" {
		entityMap = t.active.incoming
	} else {
		entityMap = t.active.outgoing
	}

	ent := entityMap[data.Actor]
	if ent == nil {
		ent = &internalEntity{}
		entityMap[data.Actor] = ent
	}
	ent.totalDamage += int64(data.Damage)
	ent.hitCount++
	if data.Damage > ent.maxHit {
		ent.maxHit = data.Damage
	}

	// Arm (or reset) the inactivity timer.
	fightID := t.active.id
	if t.endTimer != nil {
		t.endTimer.Stop()
	}
	t.endTimer = time.AfterFunc(combatGap, func() {
		t.timerExpired(fightID)
	})

	snap := t.snapshot(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// timerExpired is called by time.AfterFunc when no hit has landed for combatGap.
// fightID guards against ending a fight that was already replaced by a new one.
func (t *Tracker) timerExpired(fightID int) {
	t.mu.Lock()
	if t.active == nil || t.active.id != fightID {
		t.mu.Unlock()
		return
	}
	t.archiveFight(time.Now())
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
}

// endFight ends the current fight immediately (zone change, death, or test).
// If forced is true, the inactivity timer is also stopped.
func (t *Tracker) endFight(forced bool) {
	t.mu.Lock()
	if t.active == nil {
		t.mu.Unlock()
		return
	}
	if forced && t.endTimer != nil {
		t.endTimer.Stop()
		t.endTimer = nil
	}
	t.archiveFight(time.Now())
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
}

// archiveFight finalises the active fight and prepends it to recentFights.
// Must be called with t.mu held.
func (t *Tracker) archiveFight(endTime time.Time) {
	f := t.active
	t.active = nil

	duration := endTime.Sub(f.startTime).Seconds()
	if duration < 0.001 {
		duration = 0.001 // guard against zero-division
	}

	combatants := buildEntityStats(f.outgoing, duration)

	totalDmg := int64(0)
	youDmg := int64(0)
	for _, e := range combatants {
		totalDmg += e.TotalDamage
		if e.Name == "You" {
			youDmg = e.TotalDamage
		}
	}

	// Session tracking uses player personal outgoing damage only.
	t.sessionDamage += youDmg
	t.sessionFightTime += duration

	summary := FightSummary{
		StartTime:   f.startTime,
		EndTime:     endTime,
		Duration:    duration,
		Combatants:  combatants,
		TotalDamage: totalDmg,
		TotalDPS:    safeDivide(float64(totalDmg), duration),
		YouDamage:   youDmg,
		YouDPS:      safeDivide(float64(youDmg), duration),
	}

	// Prepend and cap at maxRecentFights.
	t.recentFights = append([]FightSummary{summary}, t.recentFights...)
	if len(t.recentFights) > maxRecentFights {
		t.recentFights = t.recentFights[:maxRecentFights]
	}
}

// snapshot builds an immutable CombatState from current mutable state.
// Must be called with t.mu held.
func (t *Tracker) snapshot(now time.Time) CombatState {
	state := CombatState{
		RecentFights:  t.recentFights,
		SessionDamage: t.sessionDamage,
		LastUpdated:   now,
	}

	if t.sessionFightTime > 0 {
		state.SessionDPS = safeDivide(float64(t.sessionDamage), t.sessionFightTime)
	}

	if t.active != nil {
		state.InCombat = true
		duration := now.Sub(t.active.startTime).Seconds()
		if duration < 0.001 {
			duration = 0.001
		}
		combatants := buildEntityStats(t.active.outgoing, duration)

		totalDmg := int64(0)
		youDmg := int64(0)
		for _, e := range combatants {
			totalDmg += e.TotalDamage
			if e.Name == "You" {
				youDmg = e.TotalDamage
			}
		}

		state.CurrentFight = &FightState{
			StartTime:   t.active.startTime,
			Duration:    duration,
			Combatants:  combatants,
			TotalDamage: totalDmg,
			TotalDPS:    safeDivide(float64(totalDmg), duration),
			YouDamage:   youDmg,
			YouDPS:      safeDivide(float64(youDmg), duration),
		}
	}

	return state
}

// buildEntityStats converts the raw entity map to a sorted []EntityStats slice.
// Sorted descending by total damage.
func buildEntityStats(entities map[string]*internalEntity, duration float64) []EntityStats {
	stats := make([]EntityStats, 0, len(entities))
	for name, e := range entities {
		stats = append(stats, EntityStats{
			Name:        name,
			TotalDamage: e.totalDamage,
			HitCount:    e.hitCount,
			MaxHit:      e.maxHit,
			DPS:         safeDivide(float64(e.totalDamage), duration),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalDamage > stats[j].TotalDamage
	})
	return stats
}

func (t *Tracker) broadcast(state CombatState) {
	t.hub.Broadcast(ws.Event{
		Type: WSEventCombat,
		Data: state,
	})
}

func safeDivide(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

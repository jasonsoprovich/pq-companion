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

// internalHealer accumulates raw heal data for one healer inside an active fight.
type internalHealer struct {
	totalHeal int64
	healCount int
	maxHeal   int
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
	// healers tracks entities that cast heals during this fight.
	healers map[string]*internalHealer
}

// Tracker watches parsed log events, groups them into fights, and maintains
// per-entity damage statistics, session-level DPS aggregates, and HPS data.
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

	// session heal aggregates (player personal healing done only)
	sessionHeal int64

	// death tracking
	currentZone string
	deaths      []DeathRecord
}

// NewTracker returns an initialised combat Tracker.
func NewTracker(hub *ws.Hub) *Tracker {
	return &Tracker{
		hub:          hub,
		recentFights: []FightSummary{},
		deaths:       []DeathRecord{},
	}
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

	case logparser.EventHeal:
		data, ok := ev.Data.(logparser.HealData)
		if !ok {
			return
		}
		t.recordHeal(ev.Timestamp, data)

	case logparser.EventKill:
		t.endFightAt(ev.Timestamp)

	case logparser.EventZone:
		if data, ok := ev.Data.(logparser.ZoneData); ok {
			t.mu.Lock()
			t.currentZone = data.ZoneName
			t.mu.Unlock()
		}
		t.endFight(true)

	case logparser.EventDeath:
		slainBy := ""
		if data, ok := ev.Data.(logparser.DeathData); ok {
			slainBy = data.SlainBy
		}
		t.mu.Lock()
		t.deaths = append(t.deaths, DeathRecord{
			Timestamp: ev.Timestamp,
			Zone:      t.currentZone,
			SlainBy:   slainBy,
		})
		t.mu.Unlock()
		t.endFight(true)
	}
}

// GetState returns a point-in-time snapshot of the current combat state.
func (t *Tracker) GetState() CombatState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshot(time.Now())
}

// Reset clears all fight history, session aggregates, and death records,
// returning the tracker to a clean state without restarting the process.
func (t *Tracker) Reset() {
	t.mu.Lock()
	if t.endTimer != nil {
		t.endTimer.Stop()
		t.endTimer = nil
	}
	t.active = nil
	t.recentFights = []FightSummary{}
	t.sessionDamage = 0
	t.sessionFightTime = 0
	t.sessionHeal = 0
	t.deaths = []DeathRecord{}
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
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
			healers:   make(map[string]*internalHealer),
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

// recordHeal processes a heal event and accumulates per-healer stats.
func (t *Tracker) recordHeal(ts time.Time, data logparser.HealData) {
	t.mu.Lock()

	// Only track heals during an active fight; heals outside combat are ignored.
	if t.active == nil {
		t.mu.Unlock()
		return
	}

	h := t.active.healers[data.Actor]
	if h == nil {
		h = &internalHealer{}
		t.active.healers[data.Actor] = h
	}
	h.totalHeal += int64(data.Amount)
	h.healCount++
	if data.Amount > h.maxHeal {
		h.maxHeal = data.Amount
	}

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

// endFightAt ends the active fight at the given log-event timestamp (e.g. on a
// kill event), so the archived duration reflects first-hit to kill rather than
// first-hit to inactivity-timer expiry.
func (t *Tracker) endFightAt(ts time.Time) {
	t.mu.Lock()
	if t.active == nil {
		t.mu.Unlock()
		return
	}
	if t.endTimer != nil {
		t.endTimer.Stop()
		t.endTimer = nil
	}
	t.archiveFight(ts)
	snap := t.snapshot(ts)
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

	healers := buildHealerStats(f.healers, duration)
	totalHeal := int64(0)
	youHeal := int64(0)
	for _, h := range healers {
		totalHeal += h.TotalHeal
		if h.Name == "You" {
			youHeal = h.TotalHeal
		}
	}

	// Session tracking uses player personal outgoing damage and healing only.
	t.sessionDamage += youDmg
	t.sessionFightTime += duration
	t.sessionHeal += youHeal

	summary := FightSummary{
		StartTime:   f.startTime,
		EndTime:     endTime,
		Duration:    duration,
		Combatants:  combatants,
		TotalDamage: totalDmg,
		TotalDPS:    safeDivide(float64(totalDmg), duration),
		YouDamage:   youDmg,
		YouDPS:      safeDivide(float64(youDmg), duration),
		Healers:     healers,
		TotalHeal:   totalHeal,
		TotalHPS:    safeDivide(float64(totalHeal), duration),
		YouHeal:     youHeal,
		YouHPS:      safeDivide(float64(youHeal), duration),
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
		SessionHeal:   t.sessionHeal,
		Deaths:        append([]DeathRecord(nil), t.deaths...),
		DeathCount:    len(t.deaths),
		LastUpdated:   now,
	}

	if t.sessionFightTime > 0 {
		state.SessionDPS = safeDivide(float64(t.sessionDamage), t.sessionFightTime)
		state.SessionHPS = safeDivide(float64(t.sessionHeal), t.sessionFightTime)
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

		healers := buildHealerStats(t.active.healers, duration)
		totalHeal := int64(0)
		youHeal := int64(0)
		for _, h := range healers {
			totalHeal += h.TotalHeal
			if h.Name == "You" {
				youHeal = h.TotalHeal
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
			Healers:     healers,
			TotalHeal:   totalHeal,
			TotalHPS:    safeDivide(float64(totalHeal), duration),
			YouHeal:     youHeal,
			YouHPS:      safeDivide(float64(youHeal), duration),
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

// buildHealerStats converts the raw healer map to a sorted []HealerStats slice.
// Sorted descending by total healing.
func buildHealerStats(healers map[string]*internalHealer, duration float64) []HealerStats {
	stats := make([]HealerStats, 0, len(healers))
	for name, h := range healers {
		stats = append(stats, HealerStats{
			Name:      name,
			TotalHeal: h.totalHeal,
			HealCount: h.healCount,
			MaxHeal:   h.maxHeal,
			HPS:       safeDivide(float64(h.totalHeal), duration),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].TotalHeal > stats[j].TotalHeal
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

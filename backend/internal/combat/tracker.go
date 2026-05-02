package combat

import (
	"regexp"
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
	// targetCounts tracks how many times each NPC target was hit (outgoing only).
	targetCounts map[string]int
	// youTargets holds the names of every entity attacked by "You". Because the
	// player can only attack NPCs in PvE, every entry here is a confirmed NPC.
	youTargets map[string]bool
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

	// petOwners maps a pet entity name to its controlling player. Populated by
	// EventPetOwner ("Kebartik says 'My leader is Kildrey.'") and persists for
	// the session — charm rebinds overwrite the entry, and a charm break
	// clears it lazily when the former pet is seen attacking the player.
	petOwners map[string]string
}

// NewTracker returns an initialised combat Tracker.
func NewTracker(hub *ws.Hub) *Tracker {
	return &Tracker{
		hub:          hub,
		recentFights: []FightSummary{},
		deaths:       []DeathRecord{},
		petOwners:    make(map[string]string),
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

	case logparser.EventPetOwner:
		data, ok := ev.Data.(logparser.PetOwnerData)
		if !ok {
			return
		}
		t.recordPetOwner(data.Pet, data.Owner)

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
	t.petOwners = make(map[string]string)
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (t *Tracker) recordHit(ts time.Time, data logparser.CombatHitData) {
	t.mu.Lock()

	// Start a new fight if none is active. Require "You" to be involved (as
	// actor or target) before seeding a fight — otherwise third-party damage
	// (e.g. another player's spells, or NPC AoEs hitting only group members
	// before they reach us) would create phantom fights that we'd later have
	// to discard.
	if t.active == nil {
		if data.Actor != "You" && data.Target != "You" {
			t.mu.Unlock()
			return
		}
		t.fightCounter++
		t.active = &internalFight{
			id:           t.fightCounter,
			startTime:    ts,
			lastHit:      ts,
			outgoing:     make(map[string]*internalEntity),
			incoming:     make(map[string]*internalEntity),
			healers:      make(map[string]*internalHealer),
			targetCounts: make(map[string]int),
			youTargets:   make(map[string]bool),
		}
	} else {
		t.active.lastHit = ts
	}

	// Route to outgoing or incoming based on target.
	var entityMap map[string]*internalEntity
	if data.Target == "You" {
		entityMap = t.active.incoming
		// Charm-break detection: if a known pet starts attacking the player,
		// it is no longer ours — drop the stale owner mapping so the row
		// stops rolling up under that player.
		if _, ok := t.petOwners[data.Actor]; ok {
			delete(t.petOwners, data.Actor)
		}
	} else {
		entityMap = t.active.outgoing
		t.active.targetCounts[data.Target]++
		if data.Actor == "You" {
			// Every entity attacked by "You" is a confirmed NPC (PvE only).
			t.active.youTargets[data.Target] = true
		}
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

// recordPetOwner stores a pet→owner binding announced by EQ's "My leader is X"
// message. The mapping is session-scoped (NewTracker / Reset) so it applies
// across fights — once a charm is established, every hit the pet lands until
// charm break gets attributed to the owner.
func (t *Tracker) recordPetOwner(pet, owner string) {
	if pet == "" || owner == "" {
		return
	}
	t.mu.Lock()
	t.petOwners[pet] = owner
	t.mu.Unlock()
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
	// Use the log-file timestamp of the last hit, not wall-clock time, so that
	// duration reflects in-game elapsed time rather than real time (which diverges
	// wildly when parsing historical log files).
	endTime := t.active.lastHit
	t.archiveFight(endTime)
	snap := t.snapshot(endTime)
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
	endTime := t.active.lastHit
	t.archiveFight(endTime)
	snap := t.snapshot(endTime)
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

	// A fight is meaningful only if at least one confirmed NPC was involved.
	// Without one, the recorded events are usually third-party noise (e.g. a
	// guildmate's spell on another guildmate) and should not be archived.
	npcs := confirmedNPCs(f)
	if len(npcs) == 0 {
		return
	}

	duration := endTime.Sub(f.startTime).Seconds()
	if duration < 0.001 {
		duration = 0.001 // guard against zero-division
	}

	combatants := excludeNPCs(buildEntityStats(f.outgoing, duration), npcs)
	stampPetOwners(combatants, t.petOwners)
	primaryTarget := pickPrimaryTarget(f, npcs)

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
		StartTime:     f.startTime,
		EndTime:       endTime,
		Duration:      duration,
		PrimaryTarget: primaryTarget,
		Combatants:    combatants,
		TotalDamage:   totalDmg,
		TotalDPS:      safeDivide(float64(totalDmg), duration),
		YouDamage:     youDmg,
		YouDPS:        safeDivide(float64(youDmg), duration),
		Healers:       healers,
		TotalHeal:     totalHeal,
		TotalHPS:      safeDivide(float64(totalHeal), duration),
		YouHeal:       youHeal,
		YouHPS:        safeDivide(float64(youHeal), duration),
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
		// Use log-file timestamps exclusively so that GetState() called long after
		// the last hit (e.g. during historical log replay) doesn't inflate duration
		// by comparing a past startTime against the current wall clock.
		duration := t.active.lastHit.Sub(t.active.startTime).Seconds()
		if duration < 0.001 {
			duration = 0.001
		}

		npcs := confirmedNPCs(t.active)
		combatants := excludeNPCs(buildEntityStats(t.active.outgoing, duration), npcs)
		stampPetOwners(combatants, t.petOwners)
		primaryTarget := pickPrimaryTarget(t.active, npcs)

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
			StartTime:     t.active.startTime,
			Duration:      duration,
			PrimaryTarget: primaryTarget,
			Combatants:    combatants,
			TotalDamage:   totalDmg,
			TotalDPS:      safeDivide(float64(totalDmg), duration),
			YouDamage:     youDmg,
			YouDPS:        safeDivide(float64(youDmg), duration),
			Healers:       healers,
			TotalHeal:     totalHeal,
			TotalHPS:      safeDivide(float64(totalHeal), duration),
			YouHeal:       youHeal,
			YouHPS:        safeDivide(float64(youHeal), duration),
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

// confirmedNPCs returns the set of names that are confirmed hostile NPCs for
// this fight. An entity is a confirmed NPC if it hit "You" (incoming) or if
// "You" attacked it (youTargets — the player only attacks NPCs in PvE).
func confirmedNPCs(f *internalFight) map[string]bool {
	set := make(map[string]bool, len(f.incoming)+len(f.youTargets))
	for name := range f.incoming {
		set[name] = true
	}
	for name := range f.youTargets {
		set[name] = true
	}
	return set
}

// pickPrimaryTarget chooses the NPC name that best represents this fight,
// scored by combined activity: hits dealt to it by anyone (targetCounts) plus
// hits it dealt to "You" (incoming). Ranking by combined activity rather than
// targetCounts alone ensures NPCs that mainly AoE the player still win when
// they're not directly attacked, and excludes player names that may have been
// hit by NPC spells. Returns "" if npcs is empty.
func pickPrimaryTarget(f *internalFight, npcs map[string]bool) string {
	primary := ""
	best := -1
	for name := range npcs {
		score := f.targetCounts[name]
		if ent := f.incoming[name]; ent != nil {
			score += ent.hitCount
		}
		if score > best {
			best = score
			primary = name
		}
	}
	return primary
}

// rePossessivePet matches the canonical EQ summoned-pet name format:
// "Owner`s warder", "Owner`s pet", etc. EQ uses a backtick (not apostrophe)
// for possessive in pet names, which makes it a reliable owner signal even
// without a prior "My leader is X" announcement.
var rePossessivePet = regexp.MustCompile(`^(\w+)` + "`" + `s\s+\w+`)

// stampPetOwners assigns OwnerName to each entity that maps to a known pet
// owner. The mapping table from petOwners is checked first; absent that, a
// name matching the "Owner`s <pet>" pattern is treated as a summoned pet of
// the captured player. Owners who themselves appear in combatants get their
// pets stamped, but standalone unrelated player names are left alone.
func stampPetOwners(combatants []EntityStats, petOwners map[string]string) {
	for i := range combatants {
		name := combatants[i].Name
		if name == "You" {
			continue
		}
		if owner, ok := petOwners[name]; ok && owner != name {
			combatants[i].OwnerName = owner
			continue
		}
		if owner := deriveOwnerFromName(name); owner != "" && owner != name {
			combatants[i].OwnerName = owner
		}
	}
}

// deriveOwnerFromName extracts an owner name from EQ's "Owner`s <pet>" format
// (e.g. "Grimrose`s warder" → "Grimrose"). Returns "" if the name does not
// match the possessive-pet pattern.
func deriveOwnerFromName(name string) string {
	if m := rePossessivePet.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// excludeNPCs returns combatants minus any name in the npcs set, preserving
// the original ordering. The DPS list is for player damage dealers only;
// hostile NPCs that landed outgoing damage on group members must be filtered.
func excludeNPCs(combatants []EntityStats, npcs map[string]bool) []EntityStats {
	if len(npcs) == 0 {
		return combatants
	}
	filtered := combatants[:0]
	for _, c := range combatants {
		if !npcs[c.Name] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

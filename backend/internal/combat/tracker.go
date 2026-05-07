package combat

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// looksLikeNPC returns true when an entity name has the shape EQ uses for
// hostile mobs (or named raid targets) rather than a player or a player's pet.
//
// Heuristics, in order:
//   - "Owner`s warder" / "Owner`s pet" — pet possessive: NOT an NPC, that's a
//     summoned/charmed minion attached to a player. Caller can then attribute
//     its damage to the owner.
//   - Multi-word names ("Lord Inquisitor Seru", "an orc centurion") — every
//     EQ player name is a single 4–15-char token, so anything with a space is
//     an NPC by construction.
//   - Names that start with a lowercase letter ("a wolf", "an orc") — articles
//     used by EQ for unnamed mobs. Players always start with an uppercase
//     letter.
//
// Anything else (single capitalised token) is treated as a player.
func looksLikeNPC(name string) bool {
	if name == "" || name == "You" {
		return false
	}
	if strings.Contains(name, "`s ") {
		return false
	}
	if strings.ContainsAny(name, " `") {
		return true
	}
	if c := name[0]; c >= 'a' && c <= 'z' {
		return true
	}
	return false
}

// internalEntity accumulates raw hit data for one combatant inside an active fight.
type internalEntity struct {
	totalDamage int64
	hitCount    int
	maxHit      int

	// Active-time bookkeeping — see noteActivity. activeAccrued holds the
	// duration of every CLOSED segment; segOpen + (segLast - segStart) is the
	// duration of the currently-open segment. Total active time at any
	// moment is activeAccrued + (segLast - segStart) when segOpen, else
	// activeAccrued.
	activeAccrued float64
	segStart      time.Time
	segLast       time.Time
	segOpen       bool
}

// internalHealer accumulates raw heal data for one healer inside an active fight.
type internalHealer struct {
	totalHeal int64
	healCount int
	maxHeal   int

	activeAccrued float64
	segStart      time.Time
	segLast       time.Time
	segOpen       bool
}

// noteActivity advances the active-time bookkeeping on either an
// internalEntity or internalHealer. Extracted because the segment math is
// identical for both. ts is the timestamp of a damage or heal event.
//
// Each closed segment gets at least activeMinSegment seconds of credit, so
// three isolated bursts (one hit each, far apart) accrue ~3 seconds rather
// than collapsing to 0. This matches EQLogParser's "+1 per discrete event"
// convention and keeps a single-hit fight from dividing by zero.
func noteActivityEntity(e *internalEntity, ts time.Time) {
	if !e.segOpen {
		e.segStart = ts
		e.segLast = ts
		e.segOpen = true
		return
	}
	if ts.Sub(e.segLast) > activeGapWindow {
		e.activeAccrued += creditSegment(e.segStart, e.segLast)
		e.segStart = ts
	}
	e.segLast = ts
}

func noteActivityHealer(h *internalHealer, ts time.Time) {
	if !h.segOpen {
		h.segStart = ts
		h.segLast = ts
		h.segOpen = true
		return
	}
	if ts.Sub(h.segLast) > activeGapWindow {
		h.activeAccrued += creditSegment(h.segStart, h.segLast)
		h.segStart = ts
	}
	h.segLast = ts
}

// creditSegment returns the active-time credit for one segment, applying
// the activeMinSegment floor so single-event bursts don't sum to zero.
func creditSegment(start, last time.Time) float64 {
	seg := last.Sub(start).Seconds()
	if seg < activeMinSegment {
		return activeMinSegment
	}
	return seg
}

// activeSecondsEntity returns the entity's total active time, finalising any
// currently-open segment. Read-only — does not mutate the entity.
func activeSecondsEntity(e *internalEntity) float64 {
	total := e.activeAccrued
	if e.segOpen {
		total += creditSegment(e.segStart, e.segLast)
	}
	return total
}

func activeSecondsHealer(h *internalHealer) float64 {
	total := h.activeAccrued
	if h.segOpen {
		total += creditSegment(h.segStart, h.segLast)
	}
	return total
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

// PlayerNameProvider returns the active character's display name (e.g.
// "Osui"). The combat tracker uses it to relabel internal "You" rows with
// the canonical character name on output, so they merge with pet rows whose
// OwnerName is also the character name (and so copy/exported summaries are
// readable to other people). Returning "" disables the relabel.
type PlayerNameProvider func() string

// Tracker watches parsed log events, groups them into fights, and maintains
// per-entity damage statistics, session-level DPS aggregates, and HPS data.
type Tracker struct {
	hub *ws.Hub

	playerNameFn PlayerNameProvider

	mu           sync.Mutex
	fightCounter int
	active       *internalFight
	endTimer     *time.Timer

	recentFights []FightSummary

	// lastArchived holds the raw internal state of the most recently archived
	// fight so a quick re-engage against the same enemy (within mergeWindow)
	// can reopen it instead of starting a new fight. Cleared once the merge
	// window passes or another fight is archived.
	lastArchived    *internalFight
	lastArchivedEnd time.Time

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

// NewTracker returns an initialised combat Tracker. playerNameFn may be nil
// (legacy / test callers) — passing it lets the tracker emit "Osui" instead
// of the literal "You" so pet rows merge correctly with the player row.
func NewTracker(hub *ws.Hub, playerNameFn PlayerNameProvider) *Tracker {
	return &Tracker{
		hub:          hub,
		playerNameFn: playerNameFn,
		recentFights: []FightSummary{},
		deaths:       []DeathRecord{},
		petOwners:    make(map[string]string),
	}
}

// playerName returns the active character name if the provider is wired and
// returns a non-empty, non-"You" value. Used to relabel the implicit "You"
// row on output so it merges with pet rows that already carry the
// character's actual name as their OwnerName.
func (t *Tracker) playerName() string {
	if t.playerNameFn == nil {
		return ""
	}
	name := t.playerNameFn()
	if name == "" || name == "You" {
		return ""
	}
	return name
}

// relabelYou rewrites every entity in stats whose Name is "You" to playerName.
// Used after the YouDamage/YouHeal aggregates have been computed against the
// literal "You" key, so internal accounting remains correct while the wire
// payload uses the character's canonical name.
func relabelYou(playerName string, stats []EntityStats) {
	if playerName == "" {
		return
	}
	for i := range stats {
		if stats[i].Name == "You" {
			stats[i].Name = playerName
		}
	}
}

func relabelYouHealers(playerName string, healers []HealerStats) {
	if playerName == "" {
		return
	}
	for i := range healers {
		if healers[i].Name == "You" {
			healers[i].Name = playerName
		}
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

	case logparser.EventCombatMiss:
		// Misses, dodges, parries, ripostes, and blocks are combat activity
		// even though no damage lands. Push the inactivity timer so a long
		// string of misses (or a tank tanking only via avoidance) doesn't
		// drop the fight.
		t.extendActivity(ev.Timestamp)

	case logparser.EventSpellLanded:
		// A spell landing during combat (heals, debuffs, mez, DoT applications)
		// proves combat is still live. Push the inactivity timer; never seed a
		// fresh fight from a spell-land alone (buffs land outside combat too).
		t.extendActivity(ev.Timestamp)

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
	t.lastArchived = nil
	t.lastArchivedEnd = time.Time{}
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (t *Tracker) recordHit(ts time.Time, data logparser.CombatHitData) {
	t.mu.Lock()

	// Start a new fight if none is active. Either "You" must be involved
	// directly, or one side of the hit must look like an NPC — the looser
	// heuristic seeds a fight when the player's pet/group/raid is fighting a
	// mob even though the player themselves haven't dealt damage yet (e.g.
	// healers, enchanters mid-mez, pet classes whose pet engages first).
	// Phantom fights between two players or between two unrelated NPCs are
	// still rejected; if a "You"-less fight slips in, archiveFight discards
	// it when its confirmedNPC set ends up empty.
	if t.active == nil {
		youInvolved := data.Actor == "You" || data.Target == "You"
		npcInvolved := looksLikeNPC(data.Actor) || looksLikeNPC(data.Target)
		if !youInvolved && !npcInvolved {
			t.mu.Unlock()
			return
		}
		// If the most recent archived fight ended within mergeWindow against
		// this same enemy, reopen it. Heal phases / brief pauses where the
		// inactivity timer fires before combat resumes no longer split a
		// single boss fight into pieces.
		if !t.tryReopenLastArchivedLocked(ts, data) {
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
	noteActivityEntity(ent, ts)

	t.armEndTimerLocked()

	snap := t.snapshot(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// armEndTimerLocked (re)starts the inactivity timer against the current
// active fight. Caller must hold t.mu and t.active must not be nil.
func (t *Tracker) armEndTimerLocked() {
	fightID := t.active.id
	if t.endTimer != nil {
		t.endTimer.Stop()
	}
	t.endTimer = time.AfterFunc(combatGap, func() {
		t.timerExpired(fightID)
	})
}

// extendActivity pushes the inactivity timer for the currently active fight,
// without recording any damage or healing. Used by misses, dodges, parries,
// ripostes, and spell-land events — combat activity that proves the fight is
// still alive even when no damage lands. No-op when no fight is active.
func (t *Tracker) extendActivity(ts time.Time) {
	t.mu.Lock()
	if t.active != nil {
		t.active.lastHit = ts
		t.armEndTimerLocked()
	}
	t.mu.Unlock()
}

// tryReopenLastArchivedLocked returns true when the just-arrived hit can be
// merged into the most recently archived fight (same primary enemy, ended
// within mergeWindow). On success it pops the matching FightSummary from
// recentFights and restores t.active to the saved internalFight, rolls back
// the session aggregates that were applied at archive time, and clears the
// lastArchived slot. Caller must hold t.mu.
func (t *Tracker) tryReopenLastArchivedLocked(ts time.Time, data logparser.CombatHitData) bool {
	if t.lastArchived == nil {
		return false
	}
	if ts.Sub(t.lastArchivedEnd) > mergeWindow {
		t.lastArchived = nil
		return false
	}
	npcs := confirmedNPCs(t.lastArchived, t.petOwners)
	if !npcs[data.Actor] && !npcs[data.Target] {
		// Re-engage is against a different enemy — start a fresh fight and
		// let the lastArchived slot age out naturally.
		return false
	}
	// Pop the matching summary from the front of recentFights (archiveFight
	// prepends, so the most recent fight is index 0). Roll back the session
	// aggregates so they don't double-count once the merged fight re-archives.
	if len(t.recentFights) > 0 {
		popped := t.recentFights[0]
		t.recentFights = t.recentFights[1:]
		t.sessionDamage -= popped.YouDamage
		t.sessionHeal -= popped.YouHeal
		t.sessionFightTime -= popped.Duration
	}
	t.active = t.lastArchived
	t.active.lastHit = ts
	t.lastArchived = nil
	return true
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

	// If no fight is active, seed one only when "You" is the healer or the
	// target — otherwise random raid heals on third parties (where the
	// player isn't involved at all) would each spawn a fresh fight that
	// archives empty. This is what makes pure-healer / pure-buffer characters
	// produce saved fights even when they never deal damage: their first
	// outgoing heal seeds the fight, and every NPC hit on the group then
	// rolls into it.
	if t.active == nil {
		youInvolved := data.Actor == "You" || data.Target == "You"
		if !youInvolved {
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
	noteActivityHealer(h, ts)

	// A heal during combat counts as activity — extend the inactivity timer
	// so a long heal window (e.g. group recovering after a boss spike) does
	// not end the fight prematurely.
	t.active.lastHit = ts
	t.armEndTimerLocked()

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
	archived := t.archiveFight(endTime)
	// Retain the archived fight so a quick re-engage against the same enemy
	// can pop it back out and merge into one fight.
	if archived != nil {
		t.lastArchived = archived
		t.lastArchivedEnd = endTime
	}
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
	// Forced ends (zone change, death) close the chapter — no merge possible
	// because the next fight is in a different context.
	t.lastArchived = nil
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
	// A kill event is a definitive end — the mob is dead, so there is no
	// same-enemy re-engage to merge into.
	t.lastArchived = nil
	snap := t.snapshot(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// archiveFight finalises the active fight and prepends it to recentFights.
// Returns the internalFight that was archived (so callers can retain it as
// lastArchived for the merge window) or nil if the fight was discarded as
// noise. Must be called with t.mu held.
func (t *Tracker) archiveFight(endTime time.Time) *internalFight {
	f := t.active
	t.active = nil

	// A fight is meaningful only if at least one confirmed NPC was involved.
	// Without one, the recorded events are usually third-party noise (e.g. a
	// guildmate's spell on another guildmate) and should not be archived.
	npcs := confirmedNPCs(f, t.petOwners)
	if len(npcs) == 0 {
		return nil
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

	// Relabel the literal "You" to the active character's name so the row
	// merges with pets whose OwnerName carries the same canonical name on
	// the frontend rollup. Done after the YouDamage/YouHeal aggregates so
	// internal accounting still pivots on "You".
	playerName := t.playerName()
	relabelYou(playerName, combatants)
	relabelYouHealers(playerName, healers)

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
	return f
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

		npcs := confirmedNPCs(t.active, t.petOwners)
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

		// Relabel literal "You" to the character name (post-aggregate so the
		// YouDamage / YouHeal computations above still pivot on "You").
		playerName := t.playerName()
		relabelYou(playerName, combatants)
		relabelYouHealers(playerName, healers)

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
// Sorted descending by total damage. Both DPS variants (fight-duration and
// active-time) are emitted so the frontend can switch without re-deriving.
func buildEntityStats(entities map[string]*internalEntity, duration float64) []EntityStats {
	stats := make([]EntityStats, 0, len(entities))
	for name, e := range entities {
		active := activeSecondsEntity(e)
		stats = append(stats, EntityStats{
			Name:          name,
			TotalDamage:   e.totalDamage,
			HitCount:      e.hitCount,
			MaxHit:        e.maxHit,
			DPS:           safeDivide(float64(e.totalDamage), duration),
			ActiveDPS:     safeDivide(float64(e.totalDamage), active),
			ActiveSeconds: active,
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
		active := activeSecondsHealer(h)
		stats = append(stats, HealerStats{
			Name:          name,
			TotalHeal:     h.totalHeal,
			HealCount:     h.healCount,
			MaxHeal:       h.maxHeal,
			HPS:           safeDivide(float64(h.totalHeal), duration),
			ActiveHPS:     safeDivide(float64(h.totalHeal), active),
			ActiveSeconds: active,
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
// this fight. An entity counts as an NPC if any of:
//   - it hit "You" (incoming map)
//   - "You" attacked it (youTargets — PvE means the player only attacks NPCs)
//   - it appears in the outgoing map's targetCounts AND its name passes the
//     looksLikeNPC heuristic — covers the case where the player's pet/group
//     is fighting a mob without the player having dealt direct damage yet
//   - it appears as an attacker in the outgoing map AND its name passes the
//     heuristic — covers AoE-style mob hits we observed before "You" engaged
//
// petOwners is consulted to keep charmed-pet names (which look exactly like
// hostile mobs — "a shissar revenant" charmed by an enchanter — and so trip
// looksLikeNPC's lowercase-leading rule) out of the NPC set. Without that
// the pet's row would be excludeNPCs'd off the combatants list and the
// owner's rollup row would lose all the pet damage.
func confirmedNPCs(f *internalFight, petOwners map[string]string) map[string]bool {
	set := make(map[string]bool, len(f.incoming)+len(f.youTargets))
	for name := range f.incoming {
		if _, isPet := petOwners[name]; isPet {
			continue
		}
		set[name] = true
	}
	for name := range f.youTargets {
		set[name] = true
	}
	for name := range f.targetCounts {
		if _, isPet := petOwners[name]; isPet {
			continue
		}
		if looksLikeNPC(name) {
			set[name] = true
		}
	}
	for name := range f.outgoing {
		if _, isPet := petOwners[name]; isPet {
			continue
		}
		if looksLikeNPC(name) {
			set[name] = true
		}
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

package combat

import (
	"log/slog"
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
	critCount   int
	critDamage  int64

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

// Fight holds mutable state for one currently-active combat encounter,
// scoped to a single hostile NPC. Multiple Fights run in parallel during a
// multi-mob pull; each carries its own expiry timer and damage rolls.
//
// Routing rules (see recordHit / recordHeal):
//   - Damage is added to the Fight whose npcName matches the non-"You" side
//     of the hit. Player-vs-player hits are dropped.
//   - Heals attach to the most-recently-touched active Fight (heals do not
//     name an NPC; ties to the freshest fight to keep healer numbers paired
//     with the mob the group is currently fighting).
type Fight struct {
	npcName     string
	id          int
	startTime   time.Time
	lastTouched time.Time
	hasDamage   bool

	// outgoing tracks attackers (players, pets, AoE-crossfire NPCs) hitting
	// THIS NPC. Keyed by attacker name.
	outgoing map[string]*internalEntity
	// incoming holds THIS NPC's damage to "You". Single entity since the
	// attacker is always npcName. Nil until the first incoming hit lands.
	incoming *internalEntity
	// healers tracks heals attributed to this fight, keyed by healer name.
	healers map[string]*internalHealer

	// youAttacked is true once "You" has dealt damage to this NPC at least
	// once. PvE-only assumption: any NPC the player attacks is a confirmed
	// NPC for stats purposes.
	youAttacked bool

	// timer fires when the fight has been idle long enough to archive.
	timer *time.Timer
}

// PlayerNameProvider returns the active character's display name (e.g.
// "Osui"). The combat tracker uses it to relabel internal "You" rows with
// the canonical character name on output, so they merge with pet rows whose
// OwnerName is also the character name (and so copy/exported summaries are
// readable to other people). Returning "" disables the relabel.
type PlayerNameProvider func() string

// Tracker watches parsed log events, groups them into per-NPC fights, and
// maintains per-entity damage statistics, session-level DPS aggregates, and
// HPS data.
type Tracker struct {
	hub *ws.Hub

	playerNameFn PlayerNameProvider

	mu           sync.Mutex
	fightCounter int

	// activeFights holds every Fight currently in flight, keyed by NPC name.
	// A multi-mob pull populates one entry per mob; each expires
	// independently when its own activity timer elapses.
	activeFights map[string]*Fight

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

	// pendingCrits queues crit-announcement amounts per actor. Project Quarm
	// emits a "<Actor> Scores a critical hit!(N)" line immediately before the
	// matching damage line, so we stash the amount keyed by actor and pop it
	// when the next CombatHit from that actor with the same damage arrives.
	// Bounded to maxPendingCritsPerActor so a stream of unmatched crits (e.g.
	// against a target that never gets a fight seeded) can't grow unboundedly.
	pendingCrits map[string][]int

	// confirmedHostiles records every entity name that has dealt damage to
	// "You" at least once this session. Combined with looksLikeNPC, this
	// catches single-word-named hostiles (e.g. a charmed pet that turned and
	// is now an NPC) so they're filtered out of player-DPS combatant lists in
	// subsequent fights too. Cleared by Reset; never shrinks otherwise (the
	// rare false positive — a friendly spell hitting "You" — would just hide
	// that entity from the DPS list, which is a reasonable default).
	confirmedHostiles map[string]bool

	// historyStore, when non-nil, receives every archived fight via SaveFight.
	// Optional: tests that don't care about persistence leave it nil and the
	// archive path no-ops on the store call.
	historyStore *HistoryStore
}

// SetHistoryStore wires the persistent fight history store. Called once at
// startup from main; tests typically leave it unset. Safe to call before or
// after combat events start flowing.
func (t *Tracker) SetHistoryStore(s *HistoryStore) {
	t.mu.Lock()
	t.historyStore = s
	t.mu.Unlock()
}

// maxPendingCritsPerActor caps the per-actor crit queue. In practice the
// queue should never exceed 1 — the matching damage line lands within
// microseconds — but a noisy log or a missed correlation shouldn't be able
// to grow this map without bound.
const maxPendingCritsPerActor = 8

// NewTracker returns an initialised combat Tracker. playerNameFn may be nil
// (legacy / test callers) — passing it lets the tracker emit "Osui" instead
// of the literal "You" so pet rows merge correctly with the player row.
func NewTracker(hub *ws.Hub, playerNameFn PlayerNameProvider) *Tracker {
	return &Tracker{
		hub:               hub,
		playerNameFn:      playerNameFn,
		activeFights:      make(map[string]*Fight),
		recentFights:      []FightSummary{},
		deaths:            []DeathRecord{},
		petOwners:         make(map[string]string),
		pendingCrits:      make(map[string][]int),
		confirmedHostiles: make(map[string]bool),
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
		data, ok := ev.Data.(logparser.CombatMissData)
		if !ok {
			return
		}
		t.recordMiss(ev.Timestamp, data)

	case logparser.EventSpellLanded:
		// A spell landing during combat (heals, debuffs, mez, DoT applications)
		// proves combat is still live. Push the most-recently-touched active
		// fight's timer; never seed a fresh fight from a spell-land alone
		// (buffs land outside combat too).
		t.extendMostRecentActivity(ev.Timestamp)

	case logparser.EventKill:
		data, ok := ev.Data.(logparser.KillData)
		if !ok {
			return
		}
		t.endFightByNPC(data.Target, ev.Timestamp)

	case logparser.EventZone:
		if data, ok := ev.Data.(logparser.ZoneData); ok {
			t.mu.Lock()
			t.currentZone = data.ZoneName
			t.mu.Unlock()
		}
		t.endAllFights(ev.Timestamp, true)

	case logparser.EventPetOwner:
		data, ok := ev.Data.(logparser.PetOwnerData)
		if !ok {
			return
		}
		t.recordPetOwner(data.Pet, data.Owner)

	case logparser.EventCritHit:
		data, ok := ev.Data.(logparser.CritHitData)
		if !ok {
			return
		}
		t.queuePendingCrit(data.Actor, data.Damage)

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
		t.endAllFights(ev.Timestamp, true)
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
	for _, f := range t.activeFights {
		if f.timer != nil {
			f.timer.Stop()
		}
	}
	t.activeFights = make(map[string]*Fight)
	t.recentFights = []FightSummary{}
	t.sessionDamage = 0
	t.sessionFightTime = 0
	t.sessionHeal = 0
	t.deaths = []DeathRecord{}
	t.petOwners = make(map[string]string)
	t.pendingCrits = make(map[string][]int)
	t.confirmedHostiles = make(map[string]bool)
	snap := t.snapshot(time.Now())
	t.mu.Unlock()

	t.broadcast(snap)
}

// ── routing helpers ────────────────────────────────────────────────────────

// resolveNPC determines which side of a hit/miss is the hostile NPC for the
// purpose of fight routing. Returns "" when neither side is an NPC (e.g. a
// player-vs-player hit) — the caller drops such events. Pet attackers are
// allowed: a pet's hit on an NPC routes to that NPC's fight, not the pet's.
//
// Routing precedence:
//   - "You" on either side identifies the NPC as the other side directly.
//   - Otherwise, prefer whichever side passes looksLikeNPC and is NOT in
//     petOwners. Ties (both sides look like NPCs) go to Target — the side
//     receiving the damage.
func (t *Tracker) resolveNPC(actor, target string) string {
	if target == "You" {
		// Defender is the player; the NPC must be the actor.
		if _, isPet := t.petOwners[actor]; isPet {
			// A "pet hits You" line means charm broke; caller handles.
			return actor
		}
		return actor
	}
	if actor == "You" {
		return target
	}
	// Third-party hit. Prefer Target if it looks like an NPC, then Actor.
	targetIsNPC := looksLikeNPC(target)
	if _, isPet := t.petOwners[target]; isPet {
		targetIsNPC = false
	}
	actorIsNPC := looksLikeNPC(actor)
	if _, isPet := t.petOwners[actor]; isPet {
		actorIsNPC = false
	}
	switch {
	case targetIsNPC:
		return target
	case actorIsNPC:
		return actor
	default:
		return ""
	}
}

// findOrCreateFightLocked returns the Fight for npcName, creating it if not
// already active. Caller must hold t.mu.
func (t *Tracker) findOrCreateFightLocked(npcName string, ts time.Time) *Fight {
	if f, ok := t.activeFights[npcName]; ok {
		return f
	}
	t.fightCounter++
	f := &Fight{
		npcName:     npcName,
		id:          t.fightCounter,
		startTime:   ts,
		lastTouched: ts,
		outgoing:    make(map[string]*internalEntity),
		healers:     make(map[string]*internalHealer),
	}
	t.activeFights[npcName] = f
	return f
}

// armFightTimerLocked (re)starts the per-fight inactivity timer. Uses a
// shorter timeout once the fight has any damage activity, matching
// EQLogParser's FightTimeout / MaxTimeout split. Caller must hold t.mu.
func (t *Tracker) armFightTimerLocked(f *Fight) {
	if f.timer != nil {
		f.timer.Stop()
	}
	d := fightExpiryNoDamage
	if f.hasDamage {
		d = fightExpiryWithDamage
	}
	npcName := f.npcName
	fightID := f.id
	f.timer = time.AfterFunc(d, func() {
		t.fightTimerExpired(npcName, fightID)
	})
}

// fightTimerExpired archives one fight when its inactivity timer fires.
// fightID guards against archiving a recreated-same-name fight.
func (t *Tracker) fightTimerExpired(npcName string, fightID int) {
	t.mu.Lock()
	f, ok := t.activeFights[npcName]
	if !ok || f.id != fightID {
		t.mu.Unlock()
		return
	}
	endTime := f.lastTouched
	t.archiveFightLocked(f, endTime)
	snap := t.snapshot(endTime)
	t.mu.Unlock()
	t.broadcast(snap)
}

// ── ingest paths ───────────────────────────────────────────────────────────

func (t *Tracker) recordHit(ts time.Time, data logparser.CombatHitData) {
	t.mu.Lock()

	// Charm-break detection: if a known pet starts attacking the player,
	// it is no longer ours — drop the stale owner mapping so the row stops
	// rolling up under that player. Done before resolveNPC so the actor is
	// re-classified as a real NPC for routing.
	if data.Target == "You" {
		if _, ok := t.petOwners[data.Actor]; ok {
			delete(t.petOwners, data.Actor)
		}
	}

	npcName := t.resolveNPC(data.Actor, data.Target)
	if npcName == "" {
		t.mu.Unlock()
		return
	}

	f := t.findOrCreateFightLocked(npcName, ts)
	f.lastTouched = ts
	f.hasDamage = true

	if data.Target == "You" {
		// Incoming damage: this NPC is hitting the player. Tracked separately
		// from outgoing because it isn't part of any combatant's DPS row.
		// Also record the actor as a confirmed hostile so a single-word-named
		// charm-broken pet is filtered from future fights' player-DPS lists.
		t.confirmedHostiles[data.Actor] = true
		if f.incoming == nil {
			f.incoming = &internalEntity{}
		}
		ent := f.incoming
		ent.totalDamage += int64(data.Damage)
		ent.hitCount++
		if data.Damage > ent.maxHit {
			ent.maxHit = data.Damage
		}
		noteActivityEntity(ent, ts)
	} else {
		// Outgoing damage: data.Actor is hitting this NPC. Track per-attacker
		// in the outgoing map so the DPS row can credit pets, group members,
		// and the player separately. NPC-vs-NPC AoE will end up here too —
		// excludeNPCs filters those out at render time.
		if data.Actor == "You" {
			f.youAttacked = true
		}
		ent := f.outgoing[data.Actor]
		if ent == nil {
			ent = &internalEntity{}
			f.outgoing[data.Actor] = ent
		}
		ent.totalDamage += int64(data.Damage)
		ent.hitCount++
		if data.Damage > ent.maxHit {
			ent.maxHit = data.Damage
		}
		if t.popPendingCritLocked(data.Actor, data.Damage) {
			ent.critCount++
			ent.critDamage += int64(data.Damage)
		}
		noteActivityEntity(ent, ts)
	}

	t.armFightTimerLocked(f)

	snap := t.snapshot(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// recordHeal attributes a heal event to the most-recently-touched active
// fight. Heal log lines do not name an NPC, so per-fight attribution is
// inherently approximate; pinning to the freshest fight keeps healer numbers
// paired with whatever the group is currently engaged with.
//
// If no fight is active, the heal is dropped — combat tracking requires
// at least one damage line touching an NPC to have started a fight first.
func (t *Tracker) recordHeal(ts time.Time, data logparser.HealData) {
	t.mu.Lock()
	f := t.mostRecentActiveFightLocked()
	if f == nil {
		t.mu.Unlock()
		return
	}

	h := f.healers[data.Actor]
	if h == nil {
		h = &internalHealer{}
		f.healers[data.Actor] = h
	}
	h.totalHeal += int64(data.Amount)
	h.healCount++
	if data.Amount > h.maxHeal {
		h.maxHeal = data.Amount
	}
	noteActivityHealer(h, ts)

	// A heal during combat counts as activity — extend the fight's timer.
	f.lastTouched = ts
	t.armFightTimerLocked(f)

	snap := t.snapshot(ts)
	t.mu.Unlock()

	t.broadcast(snap)
}

// recordMiss extends the activity timer of the fight that the miss touches.
// Misses do not seed fights — a string of misses with no actual damage is
// noise. If neither side is a known NPC fight, the miss is dropped.
func (t *Tracker) recordMiss(ts time.Time, data logparser.CombatMissData) {
	npcName := t.resolveNPCForMiss(data.Actor, data.Target)
	if npcName == "" {
		return
	}
	t.mu.Lock()
	if f, ok := t.activeFights[npcName]; ok {
		f.lastTouched = ts
		t.armFightTimerLocked(f)
	}
	t.mu.Unlock()
}

// resolveNPCForMiss is a thin wrapper around resolveNPC that takes the
// CombatMissData field names (Actor / Target). Kept separate so the callsite
// reads naturally.
func (t *Tracker) resolveNPCForMiss(actor, target string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.resolveNPC(actor, target)
}

// extendMostRecentActivity pushes the most-recently-touched active fight's
// timer without recording any damage. Used by spell-landed events where the
// log line proves combat is live but doesn't name a specific NPC.
func (t *Tracker) extendMostRecentActivity(ts time.Time) {
	t.mu.Lock()
	if f := t.mostRecentActiveFightLocked(); f != nil {
		f.lastTouched = ts
		t.armFightTimerLocked(f)
	}
	t.mu.Unlock()
}

// mostRecentActiveFightLocked returns the active Fight with the latest
// lastTouched timestamp. Returns nil if no fights are active. Caller must
// hold t.mu.
func (t *Tracker) mostRecentActiveFightLocked() *Fight {
	var latest *Fight
	for _, f := range t.activeFights {
		if latest == nil || f.lastTouched.After(latest.lastTouched) {
			latest = f
		}
	}
	return latest
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

// queuePendingCrit stashes a "Scores a critical hit!(N)" announcement so the
// next CombatHit from the same actor with damage == N can be marked as a crit.
// Bounded per-actor to defend against unmatched-crit accumulation.
func (t *Tracker) queuePendingCrit(actor string, dmg int) {
	if actor == "" {
		return
	}
	t.mu.Lock()
	q := t.pendingCrits[actor]
	if len(q) >= maxPendingCritsPerActor {
		// Drop the oldest entry to make room for the new one.
		q = q[1:]
	}
	t.pendingCrits[actor] = append(q, dmg)
	t.mu.Unlock()
}

// popPendingCritLocked removes the first queued crit amount for actor that
// matches dmg and returns true. Returns false if no matching pending crit
// exists. Caller must hold t.mu.
func (t *Tracker) popPendingCritLocked(actor string, dmg int) bool {
	q := t.pendingCrits[actor]
	for i, amt := range q {
		if amt == dmg {
			t.pendingCrits[actor] = append(q[:i], q[i+1:]...)
			if len(t.pendingCrits[actor]) == 0 {
				delete(t.pendingCrits, actor)
			}
			return true
		}
	}
	return false
}

// ── fight lifecycle ───────────────────────────────────────────────────────

// endFightByNPC archives the fight against the named NPC at ts. Used on
// EventKill, where the slain mob explicitly identifies the fight to close.
// If no fight is active for that NPC the call is a no-op (e.g. we missed
// the engage).
func (t *Tracker) endFightByNPC(npcName string, ts time.Time) {
	if npcName == "" {
		return
	}
	t.mu.Lock()
	f, ok := t.activeFights[npcName]
	if !ok {
		t.mu.Unlock()
		return
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	t.archiveFightLocked(f, ts)
	snap := t.snapshot(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// endAllFights archives every currently-active fight at ts. Used on zone
// changes and player deaths, where every in-flight fight is invalidated.
// forced is accepted for API symmetry with the prior implementation; both
// trigger paths stop timers, so it is currently a no-op flag.
func (t *Tracker) endAllFights(ts time.Time, forced bool) {
	_ = forced
	t.mu.Lock()
	if len(t.activeFights) == 0 {
		t.mu.Unlock()
		return
	}
	for _, f := range t.activeFights {
		if f.timer != nil {
			f.timer.Stop()
		}
		t.archiveFightLocked(f, ts)
	}
	snap := t.snapshot(ts)
	t.mu.Unlock()
	t.broadcast(snap)
}

// archiveFightLocked finalises a single fight, removes it from activeFights,
// and prepends a FightSummary to recentFights. Discards the fight as noise
// if it never accrued any outgoing combatants the player cares about.
// Caller must hold t.mu.
func (t *Tracker) archiveFightLocked(f *Fight, endTime time.Time) {
	delete(t.activeFights, f.npcName)

	duration := endTime.Sub(f.startTime).Seconds()
	if duration < 0.001 {
		duration = 0.001
	}

	combatants := excludeNPCsByName(buildEntityStats(f.outgoing, duration), f.npcName, t.petOwners, t.confirmedHostiles)
	stampPetOwners(combatants, t.petOwners)

	// Drop fights that didn't produce a meaningful player-facing combatant
	// list AND didn't produce any incoming damage we'd want recorded — these
	// are typically NPC-vs-NPC noise (e.g. two mobs AoE-trading) we picked up
	// before "You" or any group member engaged.
	if len(combatants) == 0 && (f.incoming == nil || f.incoming.totalDamage == 0) {
		return
	}

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

	playerName := t.playerName()
	relabelYou(playerName, combatants)
	relabelYouHealers(playerName, healers)

	summary := FightSummary{
		StartTime:     f.startTime,
		EndTime:       endTime,
		Duration:      duration,
		PrimaryTarget: f.npcName,
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

	t.recentFights = append([]FightSummary{summary}, t.recentFights...)
	if len(t.recentFights) > maxRecentFights {
		t.recentFights = t.recentFights[:maxRecentFights]
	}

	// Persist to user.db when wired. Performed inside the tracker mutex so
	// the in-memory ring and the on-disk record stay consistent. The store
	// uses a single open conn with a 30s busy_timeout, so a write should
	// never block long enough to be visible at this granularity (a fight
	// archive is a rare event compared to per-hit broadcasts).
	if t.historyStore != nil {
		if _, err := t.historyStore.SaveFight(summary, t.currentZone, t.playerName()); err != nil {
			// Persistence failure should not crash the live tracker — the
			// in-memory recent-fights view still works. Surface via slog so
			// disk-full / permission issues are visible in support logs.
			slog.Warn("save fight to history", "npc", summary.PrimaryTarget, "err", err)
		}
	}
}

// ── snapshot / broadcast ───────────────────────────────────────────────────

// snapshot builds an immutable CombatState from current mutable state.
// Picks the most-recently-touched active fight as CurrentFight to preserve
// the legacy single-fight panel; multi-fight UIs can be layered on later via
// an additional ActiveFights field without breaking this view.
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

	state.InCombat = len(t.activeFights) > 0
	if !state.InCombat {
		return state
	}

	f := t.mostRecentActiveFightLocked()
	if f == nil {
		return state
	}

	duration := f.lastTouched.Sub(f.startTime).Seconds()
	if duration < 0.001 {
		duration = 0.001
	}

	combatants := excludeNPCsByName(buildEntityStats(f.outgoing, duration), f.npcName, t.petOwners, t.confirmedHostiles)
	stampPetOwners(combatants, t.petOwners)

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

	playerName := t.playerName()
	relabelYou(playerName, combatants)
	relabelYouHealers(playerName, healers)

	state.CurrentFight = &FightState{
		StartTime:     f.startTime,
		Duration:      duration,
		PrimaryTarget: f.npcName,
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
			CritCount:     e.critCount,
			CritDamage:    e.critDamage,
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

// excludeNPCsByName returns combatants minus the fight's own NPC and any
// other entity that looks like an NPC (and isn't a known pet). The DPS list
// is for player damage dealers only; NPC-vs-NPC AoE crossfire would otherwise
// pollute the row list.
//
// confirmedHostiles catches single-word-named hostiles that looksLikeNPC
// can't recognise on its own — e.g. a charm-broken pet whose name still has
// the shape of a player. Once it has hit "You" it stays filtered for the
// rest of the session.
func excludeNPCsByName(combatants []EntityStats, fightNPC string, petOwners map[string]string, confirmedHostiles map[string]bool) []EntityStats {
	filtered := combatants[:0]
	for _, c := range combatants {
		if c.Name == fightNPC {
			continue
		}
		if _, isPet := petOwners[c.Name]; isPet {
			filtered = append(filtered, c)
			continue
		}
		if looksLikeNPC(c.Name) || confirmedHostiles[c.Name] {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
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


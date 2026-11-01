// Package overlay implements stateful game-overlay trackers that consume parsed
// log events, enrich them with database lookups, and broadcast typed WebSocket
// events to connected frontend overlay windows.
package overlay

import (
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// WSEventNPCTarget is the WebSocket event type broadcast when the inferred
// combat target changes or is lost.
const WSEventNPCTarget = "overlay:npc_target"

// WSEventNPCTargetDistance carries the player→target distance. It is kept
// out of TargetState so a moving player doesn't re-broadcast the whole
// variant/loot/caster payload several times a second.
const WSEventNPCTargetDistance = "overlay:npc_target_distance"

// TargetDistance is the payload for WSEventNPCTargetDistance and the REST
// response for GET /api/overlay/npc/distance.
type TargetDistance struct {
	// Distance is the 3D distance to the target in game units, or nil when
	// Zeal withheld the target position (250+ units away) or doesn't report
	// it at all.
	Distance *float64 `json:"distance"`
	// HasDescriptors is true when this Zeal build reports target
	// descriptors. With it true and Distance nil, the target is simply out
	// of the 250-unit reporting range; with it false, the distance readout
	// isn't available at all and the frontend hides it.
	HasDescriptors bool `json:"has_descriptors"`
}

// distanceMinInterval caps distance broadcasts (the pipe ticks at ~10 Hz).
const distanceMinInterval = 250 * time.Millisecond

// TargetVariant is one possible interpretation of an ambiguous target. About
// 24% of npc_types rows share a name with at least one other row; when more
// than one variant fits the player's zone + position (e.g. shared-spawngroup
// rows like ssratemple's necro/SK shissar revenant), the overlay surfaces
// the full set rather than guessing.
type TargetVariant struct {
	NPC db.NPC `json:"npc"`
	// SpecialAbilities is never nil — an NPC with no abilities marshals as []
	// rather than null. The field has no omitempty, so a nil slice would ship
	// a literal null to a frontend whose type says the array is always there.
	SpecialAbilities []db.SpecialAbility  `json:"special_abilities"`
	CasterSummary    *db.NPCCasterSummary `json:"caster_summary,omitempty"`
	// SpawnChance is the row's share (percent) of its spawn group in the
	// current zone, normalized over the era-active entries.
	// Only set on multi-variant results, where it tells same-looking rows
	// apart (a 90% row vs a 10% summoner).
	SpawnChance *float64 `json:"spawn_chance,omitempty"`
	// Distinguish lists what sets this row apart from its siblings — texture,
	// gender, size and differing key abilities ("summons", "texture 2").
	Distinguish []string `json:"distinguish,omitempty"`
}

// TargetState is the payload for WSEventNPCTarget events and the REST
// response for GET /api/overlay/npc/target.
type TargetState struct {
	// HasTarget is true when a target is currently inferred from combat events.
	HasTarget bool `json:"has_target"`
	// TargetName is the display name as it appears in the log (spaces, not underscores).
	TargetName string `json:"target_name,omitempty"`
	// NPCData is the database record for the target. When Variants has more
	// than one entry, this points at Variants[0] (deterministic, lowest npc_id)
	// so single-variant consumers still render *something*.
	NPCData *db.NPC `json:"npc_data,omitempty"`
	// SpecialAbilities is the parsed special-abilities list for NPCData.
	SpecialAbilities []db.SpecialAbility `json:"special_abilities,omitempty"`
	// CasterSummary is the distilled caster-AI readout for NPCData (Complete
	// Heal / Gate / AE highlights, procs, signature spells, class-list counts).
	// Nil when the NPC has no caster AI; the overlay hides the section then.
	CasterSummary *db.NPCCasterSummary `json:"caster_summary,omitempty"`
	// Variants is non-empty (>= 2 entries) when the target name resolves to
	// multiple npc_types rows the tracker couldn't reduce to one — e.g. rows
	// that share a spawngroup and so are picked randomly by the server at
	// spawn time. Frontend should render per-variant info (class, abilities,
	// loot) instead of relying on NPCData alone.
	Variants []TargetVariant `json:"variants,omitempty"`
	// CurrentZone is the most recently seen zone name from the log.
	CurrentZone string `json:"current_zone,omitempty"`
	// HPPercent is the target's current HP as a 0-100 percentage when fed by
	// the Zeal pipe. -1 means "unknown" (no pipe available or no value yet);
	// the overlay hides the bar when this is negative.
	HPPercent int `json:"hp_percent"`
	// PetOwner is the target's owner name when the target is a summoned pet.
	// Empty for non-pet targets. Populated from the Zeal pipe.
	PetOwner string `json:"pet_owner,omitempty"`
	// IsCorpse is true when the target name ended in "'s corpse" — the DB
	// lookup strips the suffix so loot/stats still resolve, but the frontend
	// pins the HP bar to 0% regardless of what the pipe reports.
	IsCorpse bool `json:"is_corpse,omitempty"`
	// LiveLevel / LiveClass / TargetType are the live spawn's actual level,
	// class and entity type as the client reports them (Zeal target
	// descriptors, PR #239). Nil on Zeal builds without them — the frontend
	// then falls back to the DB row's level range and class.
	LiveLevel  *int `json:"live_level,omitempty"`
	LiveClass  *int `json:"live_class,omitempty"`
	TargetType *int `json:"target_type,omitempty"`
	// LastUpdated is the wall-clock time the state last changed.
	LastUpdated time.Time `json:"last_updated"`
}

// NPCTracker watches parsed log events, infers the player's current combat
// target, queries the database for full NPC data, and broadcasts
// overlay:npc_target WebSocket events whenever the state changes.
type NPCTracker struct {
	hub *ws.Hub
	db  *db.DB
	mu  sync.RWMutex
	st  TargetState

	// Pipe-sourced player snapshot used to disambiguate same-name NPCs.
	// Held under mu. Zero-valued when no Zeal pipe data is available; in
	// that case variant lookups fall back to name-only.
	pipeZoneIDNumber int    // EQ runtime zoneidnumber (0 = unknown)
	pipeZoneShort    string // resolved from zoneidnumber via DB
	pipePlayerX      float64
	pipePlayerY      float64
	pipePlayerZ      float64
	pipePlayerKnown  bool // false until first MsgPlayer arrives

	// pipeConnected is true while the Zeal pipe is live. SetPipeTarget is
	// authoritative real-time player input in that state, so log-driven
	// inference (Handle) must not overwrite it — a Necromancer's DoT combat
	// lines keep naming the original DoT target long after the player has
	// manually retargeted (e.g. onto a friendly PC), which would otherwise
	// yank the overlay back onto the mob every tick. Log inference is only
	// trusted as a fallback when no pipe is available.
	pipeConnected bool

	// verifiedPlayers names have been seen chatting on a player-only channel
	// (EventVerifiedPlayer). Gates the pet-name heuristic below so a real
	// player whose name happens to fit the generated-pet syllable shape is
	// never mistaken for the player's own pet. Mirrors combat.Tracker's
	// verifiedPlayers set; preserved for the life of the tracker.
	verifiedPlayers map[string]bool

	// lastPipeTargetID is the current target's spawn id from the Zeal v1.4.6+
	// MsgPlayer snapshot (nil = older Zeal, no target, or not yet reported for
	// the current target). Held under mu.
	lastPipeTargetID *int

	// variantCache memoises the resolved NPC / variant set for a spawn id.
	// Re-targeting the same live mob then reuses the first resolution instead
	// of re-running the position + strength disambiguation, which can flip
	// between same-name candidates as the player or the mob moves (the Vex Thal
	// / Plane of Fear "coin-flip on re-pull"). Spawn ids are recycled on a zone
	// reset, so the whole map is flushed on every zone change.
	variantCache map[int]resolvedTarget

	// lastDesc is the most recent target descriptor set from the Zeal pipe
	// (nil = this Zeal build doesn't report them, or no target). It may
	// briefly describe the *previous* target while a new target's name label
	// has arrived but its MsgPlayer hasn't, so it is only ever used via
	// descFor, which checks the name. Held under mu.
	lastDesc *zealpipe.TargetDescriptors

	// Kill-elimination state (variant_elimination.go). deadVariants maps
	// zone|name → npc_types id → expiry for same-name raid bosses already
	// killed; pendingKills holds kills not yet tied to a row, awaiting a loot
	// line; raidVariantMemo / lootItemMemo cache DB lookups. Held under mu.
	deadVariants    map[string]map[int]time.Time
	pendingKills    map[string]pendingKill
	raidVariantMemo map[string][]db.NPCVariant
	lootItemMemo    map[int]map[int]struct{}

	// Distance feed state (see SetPipeTargetSnapshot). Held under mu.
	dist         TargetDistance
	distSentAt   time.Time
	distSentOnce bool
}

// resolvedTarget is a memoised lookupNPCVariants result, keyed in variantCache
// by spawn id. name is the corpse-stripped display name it was resolved for —
// a cache entry is only trusted when that still matches the live target name.
type resolvedTarget struct {
	name      string
	npc       *db.NPC
	abilities []db.SpecialAbility
	summary   *db.NPCCasterSummary
	variants  []TargetVariant
	// level is the live spawn level from the Zeal target descriptors the
	// entry was resolved with (0 when none were reported). Kept so the pet
	// overlay can read an exact level for the player's pet by spawn id.
	level int
	// sig is the descriptor input this entry was resolved with. An entry
	// that is still ambiguous (variants non-empty) is re-resolved when a
	// materially better input arrives — see resolveSig.changed.
	sig resolveSig
}

// resolveSig summarises the Zeal descriptor input a resolution used, so an
// ambiguous result can be retried only when the input has actually improved
// rather than on every ~10 Hz pipe tick.
type resolveSig struct {
	hasDesc  bool
	hasApp   bool
	locKnown bool
	x, y     float64
}

func sigFor(d *zealpipe.TargetDescriptors) resolveSig {
	if d == nil {
		return resolveSig{}
	}
	return resolveSig{hasDesc: true, hasApp: d.HasAppearance, locKnown: d.LocKnown, x: d.GameX, y: d.GameY}
}

// sigMoveThreshold is how far a target must move before an ambiguous
// resolution is retried with its new position.
const sigMoveThreshold = 25.0

// changed reports whether next is worth re-resolving an ambiguous entry for:
// descriptors or a position appeared, or the target moved far enough to
// possibly separate its candidates' spawn points.
func (s resolveSig) changed(next resolveSig) bool {
	if s.hasDesc != next.hasDesc || s.hasApp != next.hasApp || s.locKnown != next.locKnown {
		return true
	}
	if !next.locKnown {
		return false
	}
	return math.Hypot(next.x-s.x, next.y-s.y) > sigMoveThreshold
}

// playerPos is the pipe-sourced player position used by variant filtering.
type playerPos struct {
	known bool
	x, y  float64
}

// variantCacheMax bounds variantCache within a single zone. Reaching it just
// clears the map (next target re-resolves) — a soft cap, never a correctness
// issue. A busy zone rarely has the player target this many distinct spawns.
const variantCacheMax = 512

// NewNPCTracker returns an initialised NPCTracker. Inject the WebSocket hub
// and database so the tracker can broadcast and look up NPC data.
func NewNPCTracker(hub *ws.Hub, database *db.DB) *NPCTracker {
	return &NPCTracker{
		hub:             hub,
		db:              database,
		st:              TargetState{HPPercent: -1},
		verifiedPlayers: make(map[string]bool),
		variantCache:    make(map[int]resolvedTarget),
	}
}

// Handle processes a single parsed log event.  Call this from the log-tailer
// event handler (in addition to the existing broadcast) to keep the overlay
// state up to date.
func (t *NPCTracker) Handle(ev logparser.LogEvent) {
	switch ev.Type {

	// ── Player hits NPC → the target is the entity being hit. ─────────────────
	case logparser.EventCombatHit:
		data, ok := ev.Data.(logparser.CombatHitData)
		if !ok {
			return
		}
		// Skip entirely when the pipe is connected: DoT ticks keep generating
		// "You hit <mob>" lines against the original DoT target even after the
		// player retargets elsewhere, which would fight the pipe's
		// authoritative SetPipeTarget updates.
		if t.isPipeConnected() {
			return
		}
		switch {
		// Player lands a hit on a mob.
		case data.Actor == "You" && data.Target != "" && data.Target != "You":
			t.setTarget(data.Target)
		// The player's own pet lands a hit. A pet only attacks what its owner
		// sent it at *or* — when idle — whatever just attacked the owner, so
		// either way the pet's target reflects the player's engagement. The
		// verifiedPlayers guard keeps a real player whose name happens to fit
		// the generated-pet syllable shape from being mistaken for a pet.
		case t.isOwnPet(data.Actor) && data.Target != "" && data.Target != "You":
			t.setTarget(data.Target)
		// An NPC lands a hit on the player: being attacked is engagement with
		// that NPC, whether or not it was the player's prior target (e.g. an
		// idle pet auto-engaging whatever just hit its owner).
		case data.Target == "You" && data.Actor != "" && data.Actor != "You":
			t.setTarget(data.Actor)
		}

	// ── Player misses NPC → still implies a target. ────────────────────────────
	case logparser.EventCombatMiss:
		data, ok := ev.Data.(logparser.CombatMissData)
		if !ok {
			return
		}
		if t.isPipeConnected() {
			return
		}
		switch {
		case data.Actor == "You" && data.Target != "" && data.Target != "You":
			t.setTarget(data.Target)
		case t.isOwnPet(data.Actor) && data.Target != "" && data.Target != "You":
			t.setTarget(data.Target)
		case data.Target == "You" && data.Actor != "" && data.Actor != "You":
			t.setTarget(data.Actor)
		}

	// ── /con result → target is whatever was considered. ─────────────────────
	case logparser.EventConsidered:
		data, ok := ev.Data.(logparser.ConsideredData)
		if !ok {
			return
		}
		if !t.isPipeConnected() && data.TargetName != "" {
			t.setTarget(data.TargetName)
		}

	// ── Verified player chat → remember the name so the pet-name heuristic ───
	// never mistakes a real player for the player's own pet.
	case logparser.EventVerifiedPlayer:
		data, ok := ev.Data.(logparser.VerifiedPlayerData)
		if ok && data.Name != "" {
			t.mu.Lock()
			t.verifiedPlayers[data.Name] = true
			t.mu.Unlock()
		}

	// ── Kill → clear target only if the slain mob is the current target. ─────
	case logparser.EventKill:
		data, ok := ev.Data.(logparser.KillData)
		if !ok {
			return
		}
		t.mu.RLock()
		match := t.st.HasTarget && t.st.TargetName == data.Target
		t.mu.RUnlock()
		t.noteKill(data.Target)
		if match {
			t.clearTarget()
		}

	// ── Zone change or death → clear target. ──────────────────────────────────
	case logparser.EventZone:
		zd, ok := ev.Data.(logparser.ZoneData)
		if ok {
			t.setZone(zd.ZoneName)
		}
		t.clearTarget()

	case logparser.EventDeath:
		t.clearTarget()
	}
}

// SetPipeTarget pushes a target name from the ZealPipe IPC into the same
// downstream path that /con-driven and combat-driven updates use. Identical
// names back-to-back are de-duped by setTarget, so the pipe's ~10 Hz cadence
// doesn't cause repeated DB lookups or broadcasts. An empty name is a no-op
// (clearing is handled separately via ClearPipeTarget so the pipe stream's
// transient empties don't fight log-driven state).
func (t *NPCTracker) SetPipeTarget(displayName string) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return
	}
	t.setTarget(displayName)
}

// ClearPipeTarget clears the current target. Called when the pipe reports an
// explicit empty target (player deselected) — faster than waiting for a log
// signal.
func (t *NPCTracker) ClearPipeTarget() {
	t.clearTarget()
}

// SetPipeTargetID records the current target's spawn id with no descriptors —
// the Zeal v1.4.6/v1.4.7 shape. See SetPipeTargetSnapshot.
func (t *NPCTracker) SetPipeTargetID(id *int) {
	t.SetPipeTargetSnapshot(id, nil, 0, false)
}

// SetPipeTargetSnapshot records the current target's spawn id and, on Zeal
// builds that report them, its descriptors (level/class/race/type and, within
// 250 units, position) plus the player→target distance. The target *name*
// arrives in a separate MsgLabel a beat earlier and has already driven
// setTarget's first-look resolution; this is the authoritative follow-up that
// keys the result to a stable id so a later re-pull of the same spawn is
// sticky, and narrows it with the descriptors.
//
// On a change to a non-nil id with a live target: a cache hit swaps in the
// memoised resolution (no DB, no re-disambiguation); a miss resolves once and
// caches it under the id. With the id unchanged, a still-ambiguous entry is
// re-resolved only when the descriptor input materially improved (e.g. the
// target came within 250 units and its position appeared); a result is only
// ever replaced by a narrower one, so a resolved target never un-resolves.
// A nil id (older Zeal, or no target) just clears the tracked id — no
// re-resolution, no clearing of the overlay.
//
// Called at ~10 Hz on the pipe message goroutine.
func (t *NPCTracker) SetPipeTargetSnapshot(id *int, desc *zealpipe.TargetDescriptors, dist float64, distOK bool) {
	t.updateDistance(desc != nil, dist, distOK)

	t.mu.Lock()
	idChanged := !eqIntPtr(t.lastPipeTargetID, id)
	t.lastPipeTargetID = id
	t.lastDesc = desc
	name, isCorpse := "", false
	if t.st.HasTarget {
		name, isCorpse = stripCorpseSuffix(t.st.TargetName)
	}
	zoneShort := t.pipeZoneShort
	pos := playerPos{known: t.pipePlayerKnown, x: t.pipePlayerX, y: t.pipePlayerY}
	t.mu.Unlock()

	matched := descFor(desc, name)
	t.applyLive(name, matched)

	if id == nil || name == "" {
		return
	}

	t.mu.RLock()
	cached, ok := t.variantCache[*id]
	valid := ok && cached.name == name
	t.mu.RUnlock()

	sig := sigFor(matched)
	refine := valid && len(cached.variants) > 0 && cached.sig.changed(sig)
	if !idChanged && valid && !refine {
		return // the ~10 Hz steady state: nothing new to resolve
	}

	r := cached
	if !valid || refine {
		npc, abs, sum, vars := t.lookupNPCVariants(name, zoneShort, pos, matched)
		fresh := resolvedTarget{name: name, npc: npc, abilities: abs, summary: sum, variants: vars, sig: sig}
		if matched != nil {
			fresh.level = matched.Level
		}
		switch {
		case !valid:
			r = fresh
		case len(fresh.variants) < len(cached.variants):
			r = fresh // narrowed
		default:
			r.sig = sig // no better; remember the attempt so it isn't retried every tick
		}
		if r.level == 0 && matched != nil {
			r.level = matched.Level
		}
		t.mu.Lock()
		if len(t.variantCache) >= variantCacheMax {
			t.variantCache = make(map[int]resolvedTarget)
		}
		t.variantCache[*id] = r
		t.mu.Unlock()
	}

	t.applyResolved(name, isCorpse, r)
}

// descFor returns d when it describes the target named lookupName (the
// corpse-stripped display name), else nil. Guards against the name label and
// the MsgPlayer snapshot briefly describing different targets across a
// retarget. A descriptor set without a name is trusted as-is.
func descFor(d *zealpipe.TargetDescriptors, lookupName string) *zealpipe.TargetDescriptors {
	if d == nil || lookupName == "" {
		return nil
	}
	if d.Name == "" {
		return d
	}
	descName, _ := stripCorpseSuffix(d.Name)
	if !strings.EqualFold(descName, lookupName) {
		return nil
	}
	return d
}

// applyLive stamps the live level/class/type onto the current target (or
// clears them when d is nil), broadcasting only on a change.
func (t *NPCTracker) applyLive(lookupName string, d *zealpipe.TargetDescriptors) {
	var level, class, typ *int
	if d != nil {
		level, class, typ = &d.Level, &d.Class, &d.Type
	}
	t.mu.Lock()
	curName, _ := stripCorpseSuffix(t.st.TargetName)
	if !t.st.HasTarget || curName != lookupName ||
		(eqIntPtr(t.st.LiveLevel, level) && eqIntPtr(t.st.LiveClass, class) && eqIntPtr(t.st.TargetType, typ)) {
		t.mu.Unlock()
		return
	}
	t.st.LiveLevel, t.st.LiveClass, t.st.TargetType = copyInt(level), copyInt(class), copyInt(typ)
	t.st.LastUpdated = time.Now()
	snap := t.st
	t.mu.Unlock()
	t.broadcast(snap)
}

func copyInt(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// updateDistance publishes the player→target distance on its own lightweight
// event: on any change in availability immediately, otherwise only when the
// rounded distance moved and distanceMinInterval has passed since the last
// send.
func (t *NPCTracker) updateDistance(hasDesc bool, dist float64, ok bool) {
	next := TargetDistance{HasDescriptors: hasDesc}
	if ok {
		d := math.Round(dist)
		next.Distance = &d
	}
	now := time.Now()
	t.mu.Lock()
	prev := t.dist
	availability := !t.distSentOnce || prev.HasDescriptors != next.HasDescriptors ||
		(prev.Distance == nil) != (next.Distance == nil)
	moved := prev.Distance != nil && next.Distance != nil && *prev.Distance != *next.Distance
	if !availability && !(moved && now.Sub(t.distSentAt) >= distanceMinInterval) {
		t.mu.Unlock()
		return
	}
	t.dist = next
	t.distSentAt = now
	t.distSentOnce = true
	t.mu.Unlock()
	t.hub.Broadcast(ws.Event{Type: WSEventNPCTargetDistance, Data: next})
}

// GetDistance returns the most recently published target distance.
func (t *NPCTracker) GetDistance() TargetDistance {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dist
}

// cachedResolve returns the memoised resolution for spawn id *id when one
// exists and its stored name still matches lookupName. Safe for a nil id.
func (t *NPCTracker) cachedResolve(id *int, lookupName string) (resolvedTarget, bool) {
	if id == nil {
		return resolvedTarget{}, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.variantCache[*id]
	if !ok || r.name != lookupName {
		return resolvedTarget{}, false
	}
	return r, true
}

// Resolution is an NPC row resolved for a live spawn — the overlay's variant
// pick, exposed for other consumers (the pet overlay). Variants > 1 means the
// row could not be narrowed to one; NPC is then the deterministic first pick.
// Level is the spawn's exact live level, or 0 when unknown.
type Resolution struct {
	NPC       *db.NPC
	Abilities []db.SpecialAbility
	Summary   *db.NPCCasterSummary
	Variants  int
	Level     int
}

func (r resolvedTarget) export() Resolution {
	return Resolution{
		NPC:       r.npc,
		Abilities: r.abilities,
		Summary:   r.summary,
		Variants:  len(r.variants),
		Level:     r.level,
	}
}

// ResolvedForID returns the resolution memoised for a spawn id, if the player
// has targeted that spawn in the current zone. The player's pet is almost
// always targeted while it is charmed (and whenever the player selects it
// afterwards), so the pet overlay reads its row here by pet_id instead of
// re-running the same-name disambiguation.
func (t *NPCTracker) ResolvedForID(id int) (Resolution, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.variantCache[id]
	if !ok || r.npc == nil {
		return Resolution{}, false
	}
	return r.export(), true
}

// ResolveNPCID loads one npc_types row with its abilities and caster summary,
// for a pet identified some other way than its name (a summoned pet, by its
// summoning spell or live attributes). The level is unknown (0).
func (t *NPCTracker) ResolveNPCID(id int) (Resolution, bool) {
	if t.db == nil {
		return Resolution{}, false
	}
	npc, err := t.db.GetNPC(id)
	if err != nil || npc == nil {
		return Resolution{}, false
	}
	return Resolution{
		NPC:       npc,
		Abilities: mergeInvisFlags(db.ParseSpecialAbilities(npc.SpecialAbilities), npc),
		Summary:   t.casterSummary(npc.ID),
	}, true
}

// ResolveByName resolves a display name against the current zone the same way
// a log-inferred target is — the fallback for a pet whose spawn id was never
// targeted (or on a Zeal build without ids). No descriptors are available, so
// the level is unknown.
func (t *NPCTracker) ResolveByName(displayName string) Resolution {
	t.mu.RLock()
	zoneShort := t.pipeZoneShort
	pos := playerPos{known: t.pipePlayerKnown, x: t.pipePlayerX, y: t.pipePlayerY}
	t.mu.RUnlock()
	npc, abs, sum, vars := t.lookupNPCVariants(displayName, zoneShort, pos, nil)
	return resolvedTarget{npc: npc, abilities: abs, summary: sum, variants: vars}.export()
}

// applyResolved swaps a resolved NPC / variant set onto the current target,
// preserving the live name / HP / corpse fields, and broadcasts only when the
// headline NPC or the variant count actually changed (setTarget's first-look
// resolution usually already matches, so this is a no-op in the common case).
func (t *NPCTracker) applyResolved(lookupName string, isCorpse bool, r resolvedTarget) {
	t.mu.Lock()
	curName, _ := stripCorpseSuffix(t.st.TargetName)
	if !t.st.HasTarget || curName != lookupName {
		t.mu.Unlock()
		return // target changed out from under us
	}
	if npcID(t.st.NPCData) == npcID(r.npc) && len(t.st.Variants) == len(r.variants) {
		t.mu.Unlock()
		return // nothing the overlay renders differently
	}
	t.st.NPCData = r.npc
	t.st.SpecialAbilities = r.abilities
	t.st.CasterSummary = r.summary
	t.st.Variants = r.variants
	t.st.LastUpdated = time.Now()
	snap := t.st
	t.mu.Unlock()
	t.broadcast(snap)
}

// eqIntPtr reports whether two *int hold the same value (both nil counts as
// equal).
func eqIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// npcID returns an NPC's id, or 0 for a nil NPC.
func npcID(n *db.NPC) int {
	if n == nil {
		return 0
	}
	return n.ID
}

// SetPipeConnected records whether the Zeal pipe is currently live. Called
// from the pipe supervisor's OnConnect/OnDisconnect hooks. While connected,
// Handle() defers entirely to pipe-sourced targeting (see isPipeConnected);
// once disconnected, log-driven inference resumes as the fallback.
func (t *NPCTracker) SetPipeConnected(connected bool) {
	t.mu.Lock()
	t.pipeConnected = connected
	t.mu.Unlock()
}

func (t *NPCTracker) isPipeConnected() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pipeConnected
}

// SetPipeHPPercent updates the current target's HP percentage from a Zeal
// LabelTargetHPPerc payload. Values outside 0-100 are clamped; identical
// repeats are de-duped (the pipe resends at ~10 Hz). A no-op when no target
// is set so transient HP labels can't resurrect a cleared overlay.
func (t *NPCTracker) SetPipeHPPercent(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	t.mu.Lock()
	if !t.st.HasTarget || t.st.HPPercent == percent {
		t.mu.Unlock()
		return
	}
	t.st.HPPercent = percent
	t.st.LastUpdated = time.Now()
	snap := t.st
	t.mu.Unlock()
	t.broadcast(snap)
}

// ResetPipeFields drops pipe-only state (HP%, pet owner, player snapshot)
// from the current target without changing the target itself. Called when
// the Zeal pipe disconnects so the overlay HP bar disappears rather than
// freezing at a stale value — log-driven targeting continues regardless.
func (t *NPCTracker) ResetPipeFields() {
	t.mu.Lock()
	// Drop the player snapshot unconditionally — once Zeal is gone, position
	// is stale and shouldn't be used for variant disambiguation. Zone short
	// name from the pipe is also dropped; log-driven CurrentZone is retained.
	t.pipeZoneIDNumber = 0
	t.pipeZoneShort = ""
	t.pipePlayerKnown = false
	// Pipe gone: the spawn-id feed and the variant cache it keys are stale.
	t.flushVariantCacheLocked()
	distStale := t.dist.HasDescriptors || t.dist.Distance != nil
	t.dist = TargetDistance{}
	t.distSentOnce = false
	stateStale := t.st.HPPercent != -1 || t.st.PetOwner != "" || t.st.LiveLevel != nil ||
		t.st.LiveClass != nil || t.st.TargetType != nil
	t.st.HPPercent = -1
	t.st.PetOwner = ""
	t.st.LiveLevel, t.st.LiveClass, t.st.TargetType = nil, nil, nil
	if stateStale {
		t.st.LastUpdated = time.Now()
	}
	snap := t.st
	t.mu.Unlock()
	if distStale {
		t.hub.Broadcast(ws.Event{Type: WSEventNPCTargetDistance, Data: TargetDistance{}})
	}
	if stateStale {
		t.broadcast(snap)
	}
}

// SetPipePlayerSnapshot records the player's current zone and world position
// from a Zeal MsgPlayer frame. Stored for later disambiguation; doesn't
// trigger a re-lookup or broadcast on its own — that happens at next target
// change. Zone-id lookups are cached: only the first snapshot at a given
// zoneidnumber hits the DB.
//
// Called at ~10 Hz on the pipe message goroutine.
func (t *NPCTracker) SetPipePlayerSnapshot(zoneIDNumber int, x, y, z float64) {
	t.mu.Lock()
	// Update position every tick — cheap, no DB cost.
	t.pipePlayerX = x
	t.pipePlayerY = y
	t.pipePlayerZ = z
	t.pipePlayerKnown = true
	if zoneIDNumber == t.pipeZoneIDNumber {
		t.mu.Unlock()
		return
	}
	// Zone changed — resolve to short_name, then save. Drop the lock for the
	// DB call so concurrent readers aren't held up.
	t.pipeZoneIDNumber = zoneIDNumber
	t.pipeZoneShort = ""
	t.flushVariantCacheLocked()
	t.mu.Unlock()

	if t.db == nil || zoneIDNumber == 0 {
		return
	}
	z2, err := t.db.GetZoneByZoneIDNumber(zoneIDNumber)
	if err != nil {
		slog.Debug("overlay: zone lookup miss for pipe snapshot",
			"zoneidnumber", zoneIDNumber, "err", err)
		return
	}
	t.mu.Lock()
	// Re-check under the lock: another snapshot may have changed the zone
	// out from under us between unlock and lock. Only store if we're still
	// the latest known zone.
	if t.pipeZoneIDNumber == zoneIDNumber {
		t.pipeZoneShort = z2.ShortName
	}
	t.mu.Unlock()
}

// SetPipePetOwner records the owner name when the current target is a pet.
// An empty value clears any previously-set owner (e.g. when the player
// switches from a pet to a non-pet target before the next TargetName fires).
func (t *NPCTracker) SetPipePetOwner(owner string) {
	owner = strings.TrimSpace(owner)
	t.mu.Lock()
	if !t.st.HasTarget || t.st.PetOwner == owner {
		t.mu.Unlock()
		return
	}
	t.st.PetOwner = owner
	t.st.LastUpdated = time.Now()
	snap := t.st
	t.mu.Unlock()
	t.broadcast(snap)
}

// GetState returns a point-in-time snapshot of the current target state.
func (t *NPCTracker) GetState() TargetState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.st
}

// PreviewState builds a TargetState for an arbitrary NPC id, shaped like a
// live single-variant target, so Settings can preview the overlay against any
// NPC. It never touches the tracker's live state.
func (t *NPCTracker) PreviewState(npcID int) (TargetState, error) {
	if t.db == nil {
		return TargetState{}, fmt.Errorf("preview npc %d: no database", npcID)
	}
	npc, err := t.db.GetNPC(npcID)
	if err != nil {
		return TargetState{}, fmt.Errorf("preview npc %d: %w", npcID, err)
	}
	abilities := mergeInvisFlags(db.ParseSpecialAbilities(npc.SpecialAbilities), npc)
	return TargetState{
		HasTarget:        true,
		TargetName:       strings.ReplaceAll(strings.Trim(npc.Name, "#_"), "_", " "),
		NPCData:          npc,
		SpecialAbilities: abilities,
		CasterSummary:    t.casterSummary(npc.ID),
		HPPercent:        -1,
		LastUpdated:      time.Now(),
	}, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// isOwnPet reports whether name matches the EQMac generated-pet naming shape
// and hasn't been separately confirmed as a real player. See
// combat.IsGeneratedPetName for the naming model and its accuracy caveats.
func (t *NPCTracker) isOwnPet(name string) bool {
	if !combat.IsGeneratedPetName(name) {
		return false
	}
	t.mu.RLock()
	verified := t.verifiedPlayers[name]
	t.mu.RUnlock()
	return !verified
}

func (t *NPCTracker) setZone(zoneName string) {
	t.mu.Lock()
	t.st.CurrentZone = zoneName
	t.raidVariantMemo = nil // zone-scoped; deadVariants deliberately survives
	t.flushVariantCacheLocked()
	t.mu.Unlock()
}

// flushVariantCacheLocked drops the spawn-id variant cache and the tracked
// target id. Called on every zone change — the zone server recycles spawn ids
// on a reset, so a cached entry for id N in the old zone must never satisfy a
// lookup for id N in the new one. Caller holds t.mu.
func (t *NPCTracker) flushVariantCacheLocked() {
	if len(t.variantCache) > 0 {
		t.variantCache = make(map[int]resolvedTarget)
	}
	t.lastPipeTargetID = nil
	t.lastDesc = nil
}

func (t *NPCTracker) setTarget(displayName string) {
	// Avoid redundant DB lookups when the same target is already tracked.
	t.mu.RLock()
	same := t.st.HasTarget && t.st.TargetName == displayName
	zone := t.st.CurrentZone
	// Snapshot the pipe-sourced disambiguation inputs while we hold the lock.
	zoneShort := t.pipeZoneShort
	pos := playerPos{known: t.pipePlayerKnown, x: t.pipePlayerX, y: t.pipePlayerY}
	targetID := t.lastPipeTargetID
	lastDesc := t.lastDesc
	t.mu.RUnlock()
	if same {
		return
	}
	// Guard: reject names that exactly match the current zone name.  Zone
	// entry lines should never produce a target update, but this provides a
	// belt-and-suspenders defence against any false-positive from the parser.
	if zone != "" && displayName == zone {
		return
	}

	// Corpses target as "X's corpse" via ZealPipes — strip the suffix for DB
	// lookup but keep the original name for display, and flag is_corpse so
	// the overlay pins HP to 0%.
	lookupName, isCorpse := stripCorpseSuffix(displayName)
	// Usually nil here: on a retarget the name label lands before the
	// MsgPlayer carrying the new target's descriptors, and SetPipeTargetSnapshot
	// narrows the result a tick later. Used when they're already current.
	desc := descFor(lastDesc, lookupName)

	// If the Zeal pipe has already given this exact spawn an id and we resolved
	// it earlier in this zone, reuse that result rather than re-running the
	// position/strength disambiguation — which would otherwise be free to land
	// on a different same-name candidate now that the player (or the mob) has
	// moved. Read-only here; SetPipeTargetID owns writing the cache.
	var primary *db.NPC
	var primaryAbilities []db.SpecialAbility
	var primarySummary *db.NPCCasterSummary
	var variants []TargetVariant
	if r, ok := t.cachedResolve(targetID, lookupName); ok {
		primary, primaryAbilities, primarySummary, variants = r.npc, r.abilities, r.summary, r.variants
	} else {
		primary, primaryAbilities, primarySummary, variants = t.lookupNPCVariants(lookupName, zoneShort, pos, desc)
	}

	hpPercent := -1
	if isCorpse {
		hpPercent = 0
	}
	var liveLevel, liveClass, targetType *int
	if desc != nil {
		liveLevel, liveClass, targetType = copyInt(&desc.Level), copyInt(&desc.Class), copyInt(&desc.Type)
	}

	t.mu.Lock()
	t.st = TargetState{
		HasTarget:        true,
		TargetName:       displayName,
		NPCData:          primary,
		SpecialAbilities: primaryAbilities,
		CasterSummary:    primarySummary,
		Variants:         variants,
		CurrentZone:      t.st.CurrentZone,
		HPPercent:        hpPercent,
		IsCorpse:         isCorpse,
		LiveLevel:        liveLevel,
		LiveClass:        liveClass,
		TargetType:       targetType,
		LastUpdated:      time.Now(),
	}
	snap := t.st
	t.mu.Unlock()

	t.broadcast(snap)
}

// stripCorpseSuffix detects an "X's corpse" target name (Zeal sends this when
// the player has a corpse selected) and returns the underlying NPC name plus
// a flag. Both space and underscore variants are accepted since the pipe may
// deliver either depending on the EQ build.
func stripCorpseSuffix(name string) (string, bool) {
	const spaceSuffix = "'s corpse"
	const underscoreSuffix = "'s_corpse"
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, spaceSuffix) {
		return strings.TrimSpace(name[:len(name)-len(spaceSuffix)]), true
	}
	if strings.HasSuffix(lower, underscoreSuffix) {
		return strings.TrimSpace(name[:len(name)-len(underscoreSuffix)]), true
	}
	return name, false
}

func (t *NPCTracker) clearTarget() {
	t.mu.Lock()
	if !t.st.HasTarget {
		t.mu.Unlock()
		return
	}
	t.st = TargetState{
		HasTarget:   false,
		CurrentZone: t.st.CurrentZone,
		HPPercent:   -1,
		LastUpdated: time.Now(),
	}
	snap := t.st
	t.mu.Unlock()

	t.broadcast(snap)
}

// lookupNPCVariants converts the log display name (spaces) to the DB name
// format (underscores) and returns:
//   - primary: the single best-pick NPC (always the first variant by id when
//     ambiguous, so single-variant consumers still render something).
//   - primaryAbilities: parsed special abilities for primary.
//   - variants: non-nil and len>=2 only when the tracker can't reduce the
//     candidates to one — in which case the frontend should render the set
//     instead of pretending primary is correct.
//
// Disambiguation cascade:
//  1. Try GetNPCVariantsByNameInZone(name, zoneShort) — restricts to NPCs
//     that actually spawn in the player's current zone.
//  2. If that finds nothing, retry with placeholder prefixes ("## ", etc.)
//     against the same zone, then fall back to a global name search.
//  3. With >1 candidate variants in zone, a known player position, AND none
//     of the candidates flagged raid_target, keep only variants whose nearest
//     spawn point is within tieToleranceYards of the closest. This picks
//     ordinary same-name mobs apart by location while keeping shared-
//     spawngroup variants (shissar necro/SK at identical coords) bundled.
//     Skipped entirely when a candidate is a raid boss (see step 3a) because
//     raid mobs are routinely dragged far from their spawn2 coordinates
//     before anyone else in the raid targets them, making "distance to
//     spawn point" meaningless.
//  3a. Raid bosses (Kaas Thox Xi Aten Ha Ra, Thall Va Xakra, Cazic Thule,
//     the Fear adds) are frequently pulled hundreds of yards from their
//     spawn2 row to a raid's preferred fight spot before most of the raid
//     ever targets them. At that point the player's live position is close
//     to neither variant's static spawn coordinate, so picking whichever
//     spawn point happens to be nearer is a coin flip, not disambiguation —
//     and a wrong pick hides the other variant's loot table (and any
//     wishlist highlight on it) entirely. Safer to always surface the full
//     variant set for raid bosses and let the frontend render every
//     candidate's loot table.
//  4. With no player position (or the raid_target skip above), keep all
//     zone matches as the variant set — honest about not knowing, lets the
//     UI surface alternatives.
//
// When Zeal reports target descriptors (desc non-nil, PR #239), two steps
// get sharper:
//   - Before step 3, candidates are narrowed to rows whose race and class
//     match and whose level range covers the live level (see
//     filterVariantsByDescriptors). This alone separates rows that share a
//     spawn point, like ssratemple's necro/SK Shissar Revenant.
//   - Step 3 uses the *target's* own position when Zeal reported it (only
//     within 250 units of the player), and then runs for raid bosses too:
//     the 3a skip exists because the player's position says nothing about
//     where a pulled boss spawned, but the boss's own position does. Raid
//     candidates use the looser raidTargetLocTolerance so a boss dragged
//     toward the midpoint between spawns stays honestly ambiguous.
//
// A PC target (desc.Type PC / PC corpse) never resolves to an NPC row.
func (t *NPCTracker) lookupNPCVariants(
	displayName, zoneShort string,
	pos playerPos, desc *zealpipe.TargetDescriptors,
) (*db.NPC, []db.SpecialAbility, *db.NPCCasterSummary, []TargetVariant) {
	if t.db == nil {
		return nil, nil, nil, nil
	}
	if desc != nil && (desc.Type == zealpipe.TargetTypePC || desc.Type == zealpipe.TargetTypePCCorpse) {
		return nil, nil, nil, nil
	}
	dbName := strings.ReplaceAll(displayName, " ", "_")

	candidates := t.fetchVariants(dbName, zoneShort)
	if len(candidates) == 0 {
		// Zone-scoped query found nothing. Fall back to global name search
		// (loses position-based disambiguation but preserves the prior
		// behaviour of "still find the NPC even outside its usual zone" —
		// useful for moved/quest-spawned mobs).
		candidates = t.fetchVariants(dbName, "")
	}
	if len(candidates) == 0 {
		slog.Debug("overlay: npc lookup miss", "display_name", displayName, "db_name", dbName)
		return nil, nil, nil, nil
	}

	// Live descriptors first: they identify the row outright where rows
	// differ in race/class/level, independent of anyone's position.
	if desc != nil && len(candidates) > 1 {
		candidates = filterVariantsByDescriptors(candidates, desc)
	}
	// Same-name raid bosses already killed this session can't be the one being
	// fought (see variant_elimination.go).
	candidates = t.dropDeadVariants(zoneShort, dbName, candidates, desc)

	// Position-based disambiguation only applies when multiple variants in
	// the same zone are still in play and we have a position to compare.
	// The target's own position (Zeal, within 250 units) is trusted for raid
	// bosses too, with a looser tolerance. The *player's* position is not:
	// raid targets get pulled far from their spawn2 coordinates before most
	// of the raid targets them, so "nearest to the player" stops being a
	// meaningful signal (see the doc comment) and every candidate flows
	// through as a variant instead of risking a coin-flip pick that hides a
	// loot table. Always skip when a db.ScriptSpawnedNPCOverrides candidate is
	// in play — it has no spawn2 coordinates by definition (see
	// fetchVariants), so filterVariantsByPosition would drop the real
	// encounter in favor of a decoy row purely for having spawn coordinates.
	scriptSpawned := len(db.ScriptSpawnedNPCOverrides[dbName]) > 0
	switch {
	case len(candidates) < 2 || zoneShort == "" || scriptSpawned:
		// Nothing to separate, or no reliable coordinates to separate by.
	case desc != nil && desc.LocKnown:
		tol := tieToleranceYards
		if anyRaidTarget(candidates) {
			tol = raidTargetLocTolerance
		}
		candidates = filterVariantsByPosition(candidates, desc.GameX, desc.GameY, tol)
	case pos.known && !anyRaidTarget(candidates):
		candidates = filterVariantsByPosition(candidates, pos.x, pos.y, tieToleranceYards)
	}

	// Order the survivors strongest-first so the headline is the row the player
	// is actually fighting. Same-name rows in Quarm are frequently a raid boss
	// plus low-HP siblings that all spawn in the same zone (e.g. Cazic Thule
	// 450k vs 32k, A Dracoliche 175k vs 32k) — picking the lowest id arbitrarily
	// headlined the weak version. Prefer raid_target, then HP, then id so the
	// real boss wins and the rest fall under the "other versions" disclosure.
	sortVariantsByStrength(candidates)

	primaryVariant := candidates[0]
	primary := primaryVariant.NPC
	primaryAbilities := db.ParseSpecialAbilities(primary.SpecialAbilities)
	primaryAbilities = mergeInvisFlags(primaryAbilities, &primary)
	primarySummary := t.casterSummary(primary.ID)

	if len(candidates) == 1 {
		return &primary, primaryAbilities, primarySummary, nil
	}

	// 2+ candidates remained after filtering — return the variant set.
	out := make([]TargetVariant, 0, len(candidates))
	for _, c := range candidates {
		npc := c.NPC
		abs := db.ParseSpecialAbilities(npc.SpecialAbilities)
		abs = mergeInvisFlags(abs, &npc)
		// An NPC with no special abilities and no see-invis flags parses to a
		// nil slice; keep it an empty one so the field marshals as [].
		if abs == nil {
			abs = []db.SpecialAbility{}
		}
		out = append(out, TargetVariant{
			NPC:              npc,
			SpecialAbilities: abs,
			CasterSummary:    t.casterSummary(npc.ID),
		})
	}
	t.annotateVariants(out, zoneShort)
	return &primary, primaryAbilities, primarySummary, out
}

// casterSummary fetches the distilled caster-AI summary for an NPC, swallowing
// errors (logged at debug) so a summary failure never breaks target tracking.
func (t *NPCTracker) casterSummary(npcID int) *db.NPCCasterSummary {
	if t.db == nil {
		return nil
	}
	s, err := t.db.SummarizeNPCCaster(npcID)
	if err != nil {
		slog.Debug("overlay: caster summary failed", "npc_id", npcID, "err", err)
		return nil
	}
	return s
}

// fetchVariants gathers every same-name npc_types row for the target — the
// bare display name plus each placeholder-decorated form ("#Foo", "Foo_",
// "#Foo_", …, see db.NPCNameVariantCandidates) — and returns them as one
// deduplicated candidate set. Returns nil on a total miss (caller treats as
// no DB record).
//
// Project Quarm frequently splits one logical NPC across a bare row and a
// decorated row that live in different zones/versions and carry *different*
// stats or special_abilities. "Venril Sathir", for instance, has a plain row
// with no Rampage and a "#Venril_Sathir" row that does; "Emperor Ssraeshza"
// has a "#Emperor_Ssraeshza" row with no loot table alongside the real
// "Emperor_Ssraeshza_" encounter row. The old code returned the bare match
// and only consulted prefixes when the bare name found nothing, so whenever
// both forms existed one was never surfaced — and any ability or stat that
// lived only on it silently vanished from the overlay even though the DB
// search page still listed it. Unioning every form keeps all variants in
// play; zone/position disambiguation downstream narrows them when Zeal data
// is available, and the frontend renders the full set otherwise.
func (t *NPCTracker) fetchVariants(dbName, zoneShort string) []db.NPCVariant {
	seen := make(map[int]struct{})
	var out []db.NPCVariant
	add := func(vs []db.NPCVariant) {
		for _, v := range vs {
			if _, dup := seen[v.NPC.ID]; dup {
				continue
			}
			seen[v.NPC.ID] = struct{}{}
			out = append(out, v)
		}
	}

	for _, candidate := range db.NPCNameVariantCandidates(dbName) {
		vs, err := t.db.GetNPCVariantsByNameInZone(candidate, zoneShort)
		if err != nil {
			slog.Debug("overlay: variant lookup error", "db_name", candidate, "zone", zoneShort, "err", err)
			continue
		}
		add(vs)
	}
	for _, id := range db.ScriptSpawnedNPCOverrides[dbName] {
		if _, dup := seen[id]; dup {
			continue
		}
		npc, err := t.db.GetNPC(id)
		if err != nil {
			slog.Debug("overlay: script-spawned override lookup failed", "npc_id", id, "err", err)
			continue
		}
		seen[id] = struct{}{}
		out = append(out, db.NPCVariant{NPC: *npc})
	}
	if len(out) == 0 {
		return nil
	}
	t.fillAppearance(out, zoneShort)
	return out
}

// sortVariantsByStrength orders candidates so the most boss-like row comes
// first: raid_target rows ahead of normal ones, then higher HP, then lower id
// as a stable tiebreak. This drives the overlay's primary (headline) pick. It
// deliberately does NOT drop any variant — siblings that share a name and HP
// (legit RNG spawn pairs) stay in the set and remain reachable behind the
// frontend's "other versions" disclosure.
func sortVariantsByStrength(variants []db.NPCVariant) {
	sort.SliceStable(variants, func(i, j int) bool {
		a, b := variants[i].NPC, variants[j].NPC
		if a.RaidTarget != b.RaidTarget {
			return a.RaidTarget > b.RaidTarget
		}
		if a.HP != b.HP {
			return a.HP > b.HP
		}
		return a.ID < b.ID
	})
}

// anyRaidTarget reports whether any candidate is flagged raid_target in the
// DB. Used to skip position-based variant filtering — see lookupNPCVariants.
func anyRaidTarget(variants []db.NPCVariant) bool {
	for _, v := range variants {
		if v.NPC.RaidTarget == 1 {
			return true
		}
	}
	return false
}

// tieToleranceYards is how close two variants' nearest-spawn distances must
// be (after sqrt) for them to count as indistinguishable. 25 yards is loose
// enough to forgive minor variance/rounding in spawn coordinates while still
// resolving distinct raid placements (Kaas Thox variants are 600+ yards
// apart) and tagging shared-spawngroup variants (distance delta ≈ 0).
const tieToleranceYards = 25.0

// raidTargetLocTolerance is the tie tolerance used when separating raid-boss
// candidates by the boss's own reported position. Raid bosses are routinely
// pulled away from their spawn point, so only a decisive lead counts: Kaas
// Thox's two rows spawn ~640 apart, which resolves at spawn (0 vs 640) and
// partway through a pull (150 vs 490) but keeps both near the midpoint.
const raidTargetLocTolerance = 200.0

// filterVariantsByDescriptors keeps the candidates consistent with the live
// spawn Zeal describes: same race, same class, and a level range covering
// the live level. If nothing survives (DB drift, an illusioned NPC, a level
// the DB range doesn't cover), the full set is returned unchanged — a
// descriptor mismatch must never make the overlay lose the NPC.
func filterVariantsByDescriptors(variants []db.NPCVariant, d *zealpipe.TargetDescriptors) []db.NPCVariant {
	out := make([]db.NPCVariant, 0, len(variants))
	for _, v := range variants {
		n := v.NPC
		maxLevel := n.MaxLevel
		if maxLevel < n.Level {
			maxLevel = n.Level
		}
		if n.Race == d.Race && n.Class == d.Class && d.Level >= n.Level && d.Level <= maxLevel {
			out = append(out, v)
		}
	}
	if len(out) > 1 && d.HasAppearance {
		out = narrowByAppearance(out, d)
	}
	if len(out) == 0 {
		slog.Debug("overlay: target descriptors matched no candidate; keeping all",
			"level", d.Level, "class", d.Class, "race", d.Race, "candidates", len(variants))
		return variants
	}
	return out
}

// filterVariantsByPosition keeps only the variants whose nearest spawn point
// to (x, y) is within tol of the closest variant's nearest spawn. (x, y) is
// the target's own reported position when Zeal provides it, else the
// player's. Variants with no spawn points are dropped — without coordinates
// we can't position-match them, and a sibling variant that does have spawns
// is the better pick. Falls back to returning all candidates if every
// variant lacks spawn points (preserves the variant set so callers still
// see them).
func filterVariantsByPosition(variants []db.NPCVariant, x, y, tol float64) []db.NPCVariant {
	dists := make([]float64, len(variants))
	minDist := math.Inf(1)
	anyWithSpawns := false
	for i, v := range variants {
		d := nearestSpawnDistance(v.SpawnPoints, x, y)
		dists[i] = d
		if !math.IsInf(d, 1) {
			anyWithSpawns = true
			if d < minDist {
				minDist = d
			}
		}
	}
	if !anyWithSpawns {
		return variants
	}
	out := make([]db.NPCVariant, 0, len(variants))
	for i, v := range variants {
		if dists[i]-minDist <= tol {
			out = append(out, v)
		}
	}
	return out
}

// nearestSpawnDistance returns the 2D distance from the player to the closest
// spawn point of this variant. +Inf when there are no spawn points (variant
// is script-spawned or otherwise unrouted through spawn2).
func nearestSpawnDistance(spawnPoints []db.SpawnPoint, px, py float64) float64 {
	minD2 := math.Inf(1)
	for _, sp := range spawnPoints {
		dx, dy := sp.X-px, sp.Y-py
		d2 := dx*dx + dy*dy
		if d2 < minD2 {
			minD2 = d2
		}
	}
	if math.IsInf(minD2, 1) {
		return minD2
	}
	return math.Sqrt(minD2)
}

// mergeInvisFlags appends synthetic SpecialAbility entries for the dedicated
// see_invis / see_invis_undead columns so the overlay surfaces them like any
// other ability badge. The columns are the authoritative source — these
// flags aren't represented in Quarm's special_abilities string at all
// (codes 26/28 are CastingFromRangeImmunity/TauntImmunity in Quarm).
// Sentinel codes above SpecialAbility::Max (55) keep them from colliding.
func mergeInvisFlags(abilities []db.SpecialAbility, npc *db.NPC) []db.SpecialAbility {
	add := func(code int, name string) {
		for _, sa := range abilities {
			if sa.Code == code {
				return
			}
		}
		abilities = append(abilities, db.SpecialAbility{Code: code, Value: 1, Name: name})
	}
	if npc.SeeInvis != 0 {
		add(enums.SyntheticSeeInvis, "See Invis")
	}
	if npc.SeeInvisUndead != 0 {
		add(enums.SyntheticSeeInvisUndead, "See Invis vs Undead")
	}
	return abilities
}

func (t *NPCTracker) broadcast(state TargetState) {
	t.hub.Broadcast(ws.Event{
		Type: WSEventNPCTarget,
		Data: state,
	})
}

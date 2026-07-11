// Package overlay implements stateful game-overlay trackers that consume parsed
// log events, enrich them with database lookups, and broadcast typed WebSocket
// events to connected frontend overlay windows.
package overlay

import (
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// WSEventNPCTarget is the WebSocket event type broadcast when the inferred
// combat target changes or is lost.
const WSEventNPCTarget = "overlay:npc_target"

// TargetVariant is one possible interpretation of an ambiguous target. About
// 24% of npc_types rows share a name with at least one other row; when more
// than one variant fits the player's zone + position (e.g. shared-spawngroup
// rows like ssratemple's necro/SK shissar revenant), the overlay surfaces
// the full set rather than guessing.
type TargetVariant struct {
	NPC              db.NPC               `json:"npc"`
	SpecialAbilities []db.SpecialAbility  `json:"special_abilities"`
	CasterSummary    *db.NPCCasterSummary `json:"caster_summary,omitempty"`
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
}

// NewNPCTracker returns an initialised NPCTracker. Inject the WebSocket hub
// and database so the tracker can broadcast and look up NPC data.
func NewNPCTracker(hub *ws.Hub, database *db.DB) *NPCTracker {
	return &NPCTracker{hub: hub, db: database, st: TargetState{HPPercent: -1}}
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
		// Only update when the player is the attacker; ignore NPC→player hits.
		// Skip when the pipe is connected: DoT ticks keep generating "You hit
		// <mob>" lines against the original DoT target even after the player
		// retargets elsewhere, which would fight the pipe's authoritative
		// SetPipeTarget updates.
		if !t.isPipeConnected() && data.Actor == "You" && data.Target != "" && data.Target != "You" {
			t.setTarget(data.Target)
		}

	// ── Player misses NPC → still implies a target. ────────────────────────────
	case logparser.EventCombatMiss:
		data, ok := ev.Data.(logparser.CombatMissData)
		if !ok {
			return
		}
		if !t.isPipeConnected() && data.Actor == "You" && data.Target != "" && data.Target != "You" {
			t.setTarget(data.Target)
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

	// ── Kill → clear target only if the slain mob is the current target. ─────
	case logparser.EventKill:
		data, ok := ev.Data.(logparser.KillData)
		if !ok {
			return
		}
		t.mu.RLock()
		match := t.st.HasTarget && t.st.TargetName == data.Target
		t.mu.RUnlock()
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
	if t.st.HPPercent == -1 && t.st.PetOwner == "" {
		t.mu.Unlock()
		return
	}
	t.st.HPPercent = -1
	t.st.PetOwner = ""
	t.st.LastUpdated = time.Now()
	snap := t.st
	t.mu.Unlock()
	t.broadcast(snap)
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

// ── internal helpers ──────────────────────────────────────────────────────────

func (t *NPCTracker) setZone(zoneName string) {
	t.mu.Lock()
	t.st.CurrentZone = zoneName
	t.mu.Unlock()
}

func (t *NPCTracker) setTarget(displayName string) {
	// Avoid redundant DB lookups when the same target is already tracked.
	t.mu.RLock()
	same := t.st.HasTarget && t.st.TargetName == displayName
	zone := t.st.CurrentZone
	// Snapshot the pipe-sourced disambiguation inputs while we hold the lock.
	zoneShort := t.pipeZoneShort
	playerKnown := t.pipePlayerKnown
	px, py := t.pipePlayerX, t.pipePlayerY
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
	primary, primaryAbilities, primarySummary, variants := t.lookupNPCVariants(lookupName, zoneShort, playerKnown, px, py)

	hpPercent := -1
	if isCorpse {
		hpPercent = 0
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

// placeholderPrefixes are the leading tokens Project Quarm uses on
// "placeholder" npc_types rows (templates spawned by quest scripts that
// share base stats with the named version). The /con line never includes
// the prefix, so an exact match against the bare display name misses every
// time. Both spaced ("## Foo" → "##_Foo") and unspaced ("#Foo") forms exist
// in the DB — try the longest variants first so the most-specific match wins.
var placeholderPrefixes = []string{"###_", "###", "##_", "##", "#_", "#"}

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
func (t *NPCTracker) lookupNPCVariants(
	displayName, zoneShort string,
	playerKnown bool, px, py float64,
) (*db.NPC, []db.SpecialAbility, *db.NPCCasterSummary, []TargetVariant) {
	if t.db == nil {
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

	// Position-based disambiguation only applies when multiple variants in
	// the same zone are still in play, we actually have a player position
	// from Zeal, AND none of the candidates is a raid boss — raid targets
	// get pulled far from their spawn2 coordinates before most of the raid
	// targets them, so "nearest to the player's current position" stops
	// being a meaningful signal (see lookupNPCVariants doc comment). In that
	// case we skip filtering and let every candidate flow through as a
	// variant instead of risking a coin-flip pick that hides a loot table.
	if len(candidates) > 1 && playerKnown && zoneShort != "" && !anyRaidTarget(candidates) {
		candidates = filterVariantsByPlayerPosition(candidates, px, py)
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
		out = append(out, TargetVariant{
			NPC:              npc,
			SpecialAbilities: abs,
			CasterSummary:    t.casterSummary(npc.ID),
		})
	}
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

// fetchVariants gathers every same-name npc_types row for the target — both
// the bare display name and each placeholder-prefixed form ("#Foo", "##_Foo",
// …) — and returns them as one deduplicated candidate set. Returns nil on a
// total miss (caller treats as no DB record).
//
// Project Quarm frequently splits one logical NPC across a bare row and a
// "#"-prefixed row that live in different zones/versions and carry *different*
// special_abilities. "Venril Sathir", for instance, has a plain row with no
// Rampage and a "#Venril_Sathir" row that does. The old code returned the
// bare match and only consulted the prefixes when the bare name found nothing,
// so whenever both forms existed the "#"-row was never surfaced — and any
// ability that lived only on it (Rampage being the common one) silently
// vanished from the overlay even though the DB search page still listed it.
// Unioning both sets keeps every variant in play; zone/position
// disambiguation downstream narrows them when Zeal data is available, and the
// frontend renders the full set otherwise.
//
// Bare matches are added first so the deterministic primary pick (lowest id
// of the bare form) is unchanged for the common single-row case.
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

	bare, err := t.db.GetNPCVariantsByNameInZone(dbName, zoneShort)
	if err != nil {
		slog.Debug("overlay: variant lookup error", "db_name", dbName, "zone", zoneShort, "err", err)
	} else {
		add(bare)
	}
	for _, p := range placeholderPrefixes {
		alt := p + dbName
		c2, err := t.db.GetNPCVariantsByNameInZone(alt, zoneShort)
		if err != nil {
			slog.Debug("overlay: variant lookup error", "db_name", alt, "zone", zoneShort, "err", err)
			continue
		}
		add(c2)
	}
	if len(out) == 0 {
		return nil
	}
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

// filterVariantsByPlayerPosition keeps only the variants whose nearest spawn
// point is within tieToleranceYards of the closest variant's nearest spawn.
// Variants with no spawn points are dropped — without coordinates we can't
// position-match them, and a sibling variant that does have spawns is the
// better pick. Falls back to returning all candidates if every variant
// lacks spawn points (preserves the variant set so callers still see them).
func filterVariantsByPlayerPosition(variants []db.NPCVariant, px, py float64) []db.NPCVariant {
	dists := make([]float64, len(variants))
	minDist := math.Inf(1)
	anyWithSpawns := false
	for i, v := range variants {
		d := nearestSpawnDistance(v.SpawnPoints, px, py)
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
		if dists[i]-minDist <= tieToleranceYards {
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

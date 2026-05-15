// Package overlay implements stateful game-overlay trackers that consume parsed
// log events, enrich them with database lookups, and broadcast typed WebSocket
// events to connected frontend overlay windows.
package overlay

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// WSEventNPCTarget is the WebSocket event type broadcast when the inferred
// combat target changes or is lost.
const WSEventNPCTarget = "overlay:npc_target"

// TargetState is the payload for WSEventNPCTarget events and the REST
// response for GET /api/overlay/npc/target.
type TargetState struct {
	// HasTarget is true when a target is currently inferred from combat events.
	HasTarget bool `json:"has_target"`
	// TargetName is the display name as it appears in the log (spaces, not underscores).
	TargetName string `json:"target_name,omitempty"`
	// NPCData is the database record for the target, if a match was found.
	NPCData *db.NPC `json:"npc_data,omitempty"`
	// SpecialAbilities is the parsed special-abilities list from NPCData.
	SpecialAbilities []db.SpecialAbility `json:"special_abilities,omitempty"`
	// CurrentZone is the most recently seen zone name from the log.
	CurrentZone string `json:"current_zone,omitempty"`
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
}

// NewNPCTracker returns an initialised NPCTracker. Inject the WebSocket hub
// and database so the tracker can broadcast and look up NPC data.
func NewNPCTracker(hub *ws.Hub, database *db.DB) *NPCTracker {
	return &NPCTracker{hub: hub, db: database}
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
		if data.Actor == "You" && data.Target != "" && data.Target != "You" {
			t.setTarget(data.Target)
		}

	// ── Player misses NPC → still implies a target. ────────────────────────────
	case logparser.EventCombatMiss:
		data, ok := ev.Data.(logparser.CombatMissData)
		if !ok {
			return
		}
		if data.Actor == "You" && data.Target != "" && data.Target != "You" {
			t.setTarget(data.Target)
		}

	// ── /con result → target is whatever was considered. ─────────────────────
	case logparser.EventConsidered:
		data, ok := ev.Data.(logparser.ConsideredData)
		if !ok {
			return
		}
		if data.TargetName != "" {
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

	npcData, abilities := t.lookupNPC(displayName)

	t.mu.Lock()
	t.st = TargetState{
		HasTarget:        true,
		TargetName:       displayName,
		NPCData:          npcData,
		SpecialAbilities: abilities,
		CurrentZone:      t.st.CurrentZone,
		LastUpdated:      time.Now(),
	}
	snap := t.st
	t.mu.Unlock()

	t.broadcast(snap)
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

// lookupNPC converts the log display name (spaces) to the DB name format
// (underscores) and queries the database. When the bare name misses, retry
// with the placeholder prefixes — Project Quarm tags placeholder rows with
// "## ", "# ", or "### " in the database name (e.g. "## Diabo`Teka`Temariel"
// stored as "##_Diabo`Teka`Temariel"), which the /con phrase obviously omits.
func (t *NPCTracker) lookupNPC(displayName string) (*db.NPC, []db.SpecialAbility) {
	if t.db == nil {
		return nil, nil
	}
	dbName := strings.ReplaceAll(displayName, " ", "_")

	npc, err := t.db.GetNPCByName(dbName)
	if err != nil {
		// Try each placeholder-prefix variant. A miss on any single attempt is
		// silent (sql.ErrNoRows wrapped); only log if every attempt fails.
		for _, p := range placeholderPrefixes {
			alt := p + dbName
			n2, e2 := t.db.GetNPCByName(alt)
			if e2 == nil {
				slog.Debug("overlay: npc lookup matched via placeholder prefix",
					"display_name", displayName, "db_name", alt)
				npc = n2
				err = nil
				break
			}
		}
	}
	if err != nil {
		slog.Debug("overlay: npc lookup miss", "display_name", displayName, "db_name", dbName)
		return nil, nil
	}

	abilities := db.ParseSpecialAbilities(npc.SpecialAbilities)
	abilities = mergeInvisFlags(abilities, npc)
	return npc, abilities
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
		add(db.SyntheticSeeInvis, "See Invis")
	}
	if npc.SeeInvisUndead != 0 {
		add(db.SyntheticSeeInvisUndead, "See Invis vs Undead")
	}
	return abilities
}

func (t *NPCTracker) broadcast(state TargetState) {
	t.hub.Broadcast(ws.Event{
		Type: WSEventNPCTarget,
		Data: state,
	})
}

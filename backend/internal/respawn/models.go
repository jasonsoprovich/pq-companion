// Package respawn tracks NPC respawn ("death") timers. When a mob is killed,
// the engine looks up its respawn time from the spawn data (spawn2.respawntime,
// already reflecting Project Quarm's reduced timers) for the player's current
// zone and starts a countdown. State is broadcast over WebSocket and exposed
// via REST, mirroring the spelltimer overlay.
package respawn

import "time"

// WSEventRespawns is the WebSocket event type emitted when respawn-timer state
// changes.
const WSEventRespawns = "overlay:respawns"

// RespawnTimer represents one killed NPC counting down to its respawn.
type RespawnTimer struct {
	// ID is the unique key in the engine's timer map and the value the
	// frontend's per-row dismiss button sends back. Format:
	// "<zoneShort>|<npcName>|<labelIndex>".
	ID string `json:"id"`

	// NPCName is the display name exactly as it appeared in the kill line
	// (e.g. "a gnoll"), spaces preserved.
	NPCName string `json:"npc_name"`

	// LabelIndex disambiguates duplicate names within a zone (the "01", "02"
	// shown after the name). Starts at 1 and resets once every timer for that
	// zone+name has cleared.
	LabelIndex int `json:"label_index"`

	// NPCID is the npc_types.id the name resolved to (best-effort; 0 if
	// unresolved). Lets the frontend deep-link to the NPC page.
	NPCID int `json:"npc_id,omitempty"`

	Zone     string `json:"zone"`      // short_name the kill was scoped to
	ZoneName string `json:"zone_name"` // long_name, for the row's zone tag

	DiedAt    time.Time `json:"died_at"`
	RespawnAt time.Time `json:"respawn_at"`

	DurationSeconds  float64 `json:"duration_seconds"`
	RemainingSeconds float64 `json:"remaining_seconds"`

	// Ambiguous is true when the name maps to more than one distinct respawn
	// time in the zone — the bar still counts to the single best estimate, but
	// the UI flags it and shows the Min/Max range.
	Ambiguous  bool `json:"ambiguous"`
	MinSeconds int  `json:"min_seconds,omitempty"`
	MaxSeconds int  `json:"max_seconds,omitempty"`
}

// RespawnState is the full payload broadcast via WebSocket and returned by the
// REST API. Timers are sorted current-zone-first, then by remaining time
// ascending (most imminent respawn first).
type RespawnState struct {
	Timers []RespawnTimer `json:"timers"`
	// CurrentZone is the player's current zone short_name, so the UI can
	// emphasise rows for where the player is standing. Empty when unknown.
	CurrentZone string    `json:"current_zone"`
	LastUpdated time.Time `json:"last_updated"`
}

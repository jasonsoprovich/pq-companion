// Package combat implements a real-time combat tracker that consumes parsed
// log events, accumulates per-entity damage statistics, detects fight
// boundaries, and maintains session-level DPS aggregates.
package combat

import "time"

// WSEventCombat is the WebSocket event type broadcast when combat state changes.
const WSEventCombat = "overlay:combat"

// combatGap is how long with no incoming hits before a fight is considered over.
const combatGap = 6 * time.Second

// maxRecentFights is the number of completed fights retained in memory.
const maxRecentFights = 20

// EntityStats holds damage statistics for one combatant within a fight.
// Actor == "You" is the player's outgoing damage; any other name is incoming
// damage from that NPC hitting the player.
type EntityStats struct {
	Name        string  `json:"name"`
	TotalDamage int64   `json:"total_damage"`
	HitCount    int     `json:"hit_count"`
	MaxHit      int     `json:"max_hit"`
	DPS         float64 `json:"dps"`
}

// FightState describes the currently active fight.
type FightState struct {
	StartTime   time.Time      `json:"start_time"`
	Duration    float64        `json:"duration_seconds"`
	Combatants  []EntityStats  `json:"combatants"`
	TotalDamage int64          `json:"total_damage"` // player outgoing only
	TotalDPS    float64        `json:"total_dps"`
}

// FightSummary is an immutable snapshot of a completed fight.
type FightSummary struct {
	StartTime   time.Time      `json:"start_time"`
	EndTime     time.Time      `json:"end_time"`
	Duration    float64        `json:"duration_seconds"`
	Combatants  []EntityStats  `json:"combatants"`
	TotalDamage int64          `json:"total_damage"`
	TotalDPS    float64        `json:"total_dps"`
}

// CombatState is the full state payload sent over WebSocket and returned by
// GET /api/overlay/combat.
type CombatState struct {
	InCombat      bool           `json:"in_combat"`
	CurrentFight  *FightState    `json:"current_fight,omitempty"`
	RecentFights  []FightSummary `json:"recent_fights"`
	SessionDamage int64          `json:"session_damage"`
	SessionDPS    float64        `json:"session_dps"`
	LastUpdated   time.Time      `json:"last_updated"`
}

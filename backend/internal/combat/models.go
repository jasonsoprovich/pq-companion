// Package combat implements a real-time combat tracker that consumes parsed
// log events, accumulates per-entity damage statistics, detects fight
// boundaries, and maintains session-level DPS aggregates.
package combat

import "time"

// WSEventCombat is the WebSocket event type broadcast when combat state changes.
const WSEventCombat = "overlay:combat"

// fightExpiryWithDamage is the per-NPC inactivity window once a fight has
// recorded any damage. Matches EQLogParser's FightTimeout (30 s) so a brief
// lull within an encounter doesn't archive the fight prematurely.
const fightExpiryWithDamage = 30 * time.Second

// fightExpiryNoDamage is the per-NPC inactivity window before the fight
// records any damage (e.g. an NPC begins targeting a player without yet
// landing a hit). Matches EQLogParser's MaxTimeout (60 s).
const fightExpiryNoDamage = 60 * time.Second

// maxRecentFights is the number of completed fights retained in memory.
const maxRecentFights = 20

// activeGapWindow is the inactivity threshold used by per-combatant
// active-time accounting. A new damage event extends the current segment
// when it falls within this window of the previous event; otherwise it
// closes the current segment and starts a new one. 10s comfortably covers
// EQ's 3-second melee swing timer (with some misses) but still creates
// separate segments for a wizard nuking once every 20–30s, so the
// resulting "active DPS" reflects actual engagement rather than the full
// fight duration. Calibrated to feel like the typical raid-parser default.
const activeGapWindow = 10 * time.Second

// activeMinSegment is the minimum active duration credited to any
// combatant with at least one hit, in seconds. Without this floor a fight
// containing a single hit would divide damage by ~0 and produce absurd
// active DPS. EQLogParser uses the same +1 convention.
const activeMinSegment = 1.0

// EntityStats holds damage statistics for one combatant within a fight.
// Only outgoing damage dealers (actors whose target is not "You") appear here.
//
// Two DPS variants are emitted so the frontend can present whichever the
// user prefers without re-deriving from raw events:
//
//	DPS         — total damage divided by the fight's wall-clock duration.
//	              Same denominator for every combatant. "Contribution rate."
//	ActiveDPS   — total damage divided by the union of intervals during
//	              which this specific combatant was actually dealing damage,
//	              floored at activeMinSegment. Different denominator per
//	              combatant. "Throughput rate while engaged."
//
// ActiveSeconds is the denominator used for ActiveDPS, exposed for
// transparency so a UI column can show "engaged 42s / 90s" if desired.
type EntityStats struct {
	Name          string  `json:"name"`
	TotalDamage   int64   `json:"total_damage"`
	HitCount      int     `json:"hit_count"`
	MaxHit        int     `json:"max_hit"`
	DPS           float64 `json:"dps"`
	ActiveDPS     float64 `json:"active_dps"`
	ActiveSeconds float64 `json:"active_seconds"`
	// CritCount is the number of "Scores a critical hit!" announcements
	// matched to a damage event from this actor in the fight.
	CritCount int `json:"crit_count"`
	// CritDamage is the sum of damage from those matched crits — useful for
	// "X% of your damage was from crits" displays.
	CritDamage int64 `json:"crit_damage"`
	// OwnerName is the controlling player's name when this entity is a pet
	// (charmed NPC or summoned pet). Empty for player damage dealers and for
	// pets whose owner could not be identified.
	OwnerName string `json:"owner_name,omitempty"`
}

// HealerStats holds healing statistics for one healer within a fight.
// Mirrors EntityStats's two-flavour DPS exposure for HPS.
type HealerStats struct {
	Name          string  `json:"name"`
	TotalHeal     int64   `json:"total_heal"`
	HealCount     int     `json:"heal_count"`
	MaxHeal       int     `json:"max_heal"`
	HPS           float64 `json:"hps"`
	ActiveHPS     float64 `json:"active_hps"`
	ActiveSeconds float64 `json:"active_seconds"`
}

// FightState describes the currently active fight.
type FightState struct {
	StartTime     time.Time     `json:"start_time"`
	Duration      float64       `json:"duration_seconds"`
	PrimaryTarget string        `json:"primary_target,omitempty"` // most-hit NPC target
	Combatants    []EntityStats `json:"combatants"`                // outgoing damage dealers sorted by DPS desc (NPCs excluded)
	TotalDamage   int64         `json:"total_damage"`              // sum of all outgoing damage (all players)
	TotalDPS      float64       `json:"total_dps"`                 // total outgoing DPS (all players)
	YouDamage     int64         `json:"you_damage"`                // player personal outgoing damage
	YouDPS        float64       `json:"you_dps"`                   // player personal DPS
	Healers       []HealerStats `json:"healers"`                   // healers sorted by total heal desc
	TotalHeal     int64         `json:"total_heal"`                // sum of all healing done (all healers)
	TotalHPS      float64       `json:"total_hps"`                 // total HPS (all healers)
	YouHeal       int64         `json:"you_heal"`                  // player personal healing done
	YouHPS        float64       `json:"you_hps"`                   // player personal HPS
}

// FightSummary is an immutable snapshot of a completed fight.
type FightSummary struct {
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	Duration      float64       `json:"duration_seconds"`
	PrimaryTarget string        `json:"primary_target,omitempty"` // most-hit NPC target
	Combatants    []EntityStats `json:"combatants"`
	TotalDamage   int64         `json:"total_damage"`
	TotalDPS      float64       `json:"total_dps"`
	YouDamage     int64         `json:"you_damage"`
	YouDPS        float64       `json:"you_dps"`
	Healers       []HealerStats `json:"healers"`
	TotalHeal     int64         `json:"total_heal"`
	TotalHPS      float64       `json:"total_hps"`
	YouHeal       int64         `json:"you_heal"`
	YouHPS        float64       `json:"you_hps"`
}

// DeathRecord captures a single player death event.
type DeathRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Zone      string    `json:"zone"`
	SlainBy   string    `json:"slain_by"` // empty when "You died." (no named killer)
}

// CombatState is the full state payload sent over WebSocket and returned by
// GET /api/overlay/combat.
type CombatState struct {
	InCombat      bool           `json:"in_combat"`
	CurrentFight  *FightState    `json:"current_fight,omitempty"`
	RecentFights  []FightSummary `json:"recent_fights"`
	SessionDamage int64          `json:"session_damage"` // player personal only
	SessionDPS    float64        `json:"session_dps"`    // player personal only
	SessionHeal   int64          `json:"session_heal"`   // player personal healing only
	SessionHPS    float64        `json:"session_hps"`    // player personal HPS only
	Deaths        []DeathRecord  `json:"deaths"`
	DeathCount    int            `json:"death_count"`
	LastUpdated   time.Time      `json:"last_updated"`
}

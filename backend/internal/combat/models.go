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

// minPersonalSeconds is the floor applied to per-player active spans before
// dividing damage by them. A single-hit fight would otherwise divide by 0
// and produce Inf. EQLogParser uses the same "+1 per discrete event"
// convention to handle this.
const minPersonalSeconds = 1.0

// EntityStats holds damage statistics for one combatant within a fight.
// Only outgoing damage dealers (actors whose target is not "You") appear here.
//
// Three DPS variants are emitted so the frontend can present whichever the
// user prefers without re-deriving from raw events. They mirror the
// metrics EQLogParser surfaces:
//
//	DPS         — Encounter DPS. Total damage / fight wall-clock duration.
//	              Same denominator for every combatant in the fight.
//	              Useful for comparing whole fights to each other.
//	ActiveDPS   — Personal DPS. Total damage / this player's first-to-last
//	              span (no gap removal). EQLogParser's headline metric.
//	              Fair to the individual: a late-joiner or OOM caster
//	              isn't punished for time they weren't engaged.
//	RaidDPS     — Raid-relative DPS. Total damage / the raid's first-to-
//	              last span across every combatant in the fight (the same
//	              denominator for every player). EQLogParser's "Sdps".
//	              The right metric for ranking players within one fight.
//
// ActiveSeconds and RaidSeconds expose the denominators for the latter
// two so a UI column can show e.g. "engaged 42s / 90s".
type EntityStats struct {
	Name          string  `json:"name"`
	TotalDamage   int64   `json:"total_damage"`
	HitCount      int     `json:"hit_count"`
	MaxHit        int     `json:"max_hit"`
	DPS           float64 `json:"dps"`
	ActiveDPS     float64 `json:"active_dps"`
	ActiveSeconds float64 `json:"active_seconds"`
	RaidDPS       float64 `json:"raid_dps"`
	RaidSeconds   float64 `json:"raid_seconds"`
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
	// Class is the canonical base class name for this combatant (e.g.
	// "Warrior", "Shadow Knight"). For pets, this is the controlling
	// player's class so DPS bars colour-match the owner. Empty when the
	// class is unknown — the frontend falls back to the user's "Unknown"
	// palette colour.
	Class string `json:"class,omitempty"`
}

// HealerStats holds healing statistics for one healer within a fight.
// Mirrors EntityStats's three-flavour DPS exposure for HPS.
type HealerStats struct {
	Name          string  `json:"name"`
	TotalHeal     int64   `json:"total_heal"`
	HealCount     int     `json:"heal_count"`
	MaxHeal       int     `json:"max_heal"`
	HPS           float64 `json:"hps"`
	ActiveHPS     float64 `json:"active_hps"`
	ActiveSeconds float64 `json:"active_seconds"`
	RaidHPS       float64 `json:"raid_hps"`
	RaidSeconds   float64 `json:"raid_seconds"`
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

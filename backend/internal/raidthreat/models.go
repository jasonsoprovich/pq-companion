// Package raidthreat assembles an ESTIMATED, raid-wide per-mob hate view by
// combining the combat tracker's per-attacker damage (the same attribution the
// DPS meter uses) with the personal threat meter's high-fidelity "You" number.
//
// It is an approximation with the DPS meter's limitations plus threat-specific
// blind spots: other players' DoT ticks, utility/debuff casts, heals on others,
// taunts, and out-of-range damage are never in the local log, so DoT/healer/
// utility classes are understated and out-of-range players are missing. Tanks
// are understated too (taunt/disciplines/+hate gear are invisible), which the
// per-class hate adjustment partially offsets. The view is for cooperative
// situational awareness ("is a DPS catching the tank?"), not a precise gauge.
package raidthreat

import "time"

// WSEventRaidThreat is the WebSocket event type carrying a RaidThreatState.
const WSEventRaidThreat = "overlay:raidthreat"

// Confidence flags surfaced per player row so the viewer knows which estimates
// to distrust.
const (
	// ConfClassUnknown — class couldn't be resolved, so no hate adjustment was
	// applied (neutral).
	ConfClassUnknown = "class_unknown"
	// ConfDoTUndercount — a DoT-heavy class whose damage-over-time ticks are
	// never logged locally, so its hate is understated.
	ConfDoTUndercount = "dot_undercount"
	// ConfHealUndercount — a healing class whose heals on others are invisible
	// locally, so its hate is understated.
	ConfHealUndercount = "heal_undercount"
)

// RaidThreatState is the immutable snapshot broadcast to the overlay.
type RaidThreatState struct {
	// InCombat is true while at least one mob is tracked.
	InCombat bool `json:"in_combat"`
	// Mobs is every mob with estimated hate, sorted by top hate descending.
	Mobs        []RaidMob `json:"mobs"`
	LastUpdated time.Time `json:"last_updated"`
}

// RaidMob is one mob's ranked per-player estimated hate.
type RaidMob struct {
	// Name is the mob's display name (same key the DPS/personal meters use).
	Name string `json:"name"`
	// IsTarget marks the mob as the player's current Zeal target (highlighted).
	IsTarget bool `json:"is_target"`
	// TopHate is the highest player's estimated hate, the bar denominator.
	TopHate int64 `json:"top_hate"`
	// Players is every estimated hate-holder, sorted by hate descending.
	Players []RaidEntry `json:"players"`
}

// RaidEntry is one player's (or pet's) estimated hate on a mob.
type RaidEntry struct {
	Name      string `json:"name"`
	Class     string `json:"class,omitempty"`
	OwnerName string `json:"owner_name,omitempty"` // controlling player when IsPet
	IsYou     bool   `json:"is_you"`
	IsPet     bool   `json:"is_pet"`
	// Hate is the estimated accumulated hate. For "You" it is the personal
	// meter's full estimate; for others it is observed damage × class/player
	// adjustment.
	Hate int64 `json:"hate"`
	// HatePct is Hate / TopHate (0..1), the bar fill.
	HatePct float64 `json:"hate_pct"`
	// Confidence holds any caveat flags for this row (empty = high confidence).
	Confidence []string `json:"confidence,omitempty"`
}

// Package threat maintains a per-mob estimate of the active character's own
// hate ("threat") on Project Quarm, derived entirely from that character's own
// combat log lines plus a static spell→hate model.
//
// Why personal-only: a full raid threat meter is impossible from logs — out of
// range, other players' damage simply isn't written to your file. But your own
// character's lines are always complete regardless of range, and a personal
// hate total never needs visibility into anyone else's hate to be accurate for
// YOUR number. The intended use is cooperative raid callouts ("tank's at
// 200k", "I have 20k into the boss"), so each player runs the meter and shares
// the number — the comparison is reconstructed socially, not by one client.
//
// This is an ESTIMATE, not the server's exact value. The dominant hate source —
// damage dealt — is read directly from your damage lines, so DPS classes
// (wizards, the feature's original audience) track closely. Weaker spots:
//   - Taunt is inherently unknowable (it sets you to top-of-list, which depends
//     on everyone else's hate).
//   - Gear/AA hatemod isn't in the log; the user supplies a static % in
//     settings (matters mainly for tank aggro AAs).
//   - No-damage utility spells (mez/slow/tash) generate "standard" hate scaled
//     to the mob's HP — modelled in a later phase; for now only their instant
//     hate component (SE_InstantHate) is counted.
package threat

import "time"

// WSEventThreat is the WebSocket event type carrying a ThreatState snapshot to
// the overlay. Mirrors combat.WSEventCombat ("overlay:combat").
const WSEventThreat = "overlay:threat"

// MobThreat is the player's estimated hate into one mob, keyed by the mob's EQ
// display name (the same key the combat/DPS meter uses, so a multi-mob pull
// lines up between the two overlays).
type MobThreat struct {
	// Name is the mob's EQ display name (spaces, not underscores).
	Name string `json:"name"`
	// Hate is the estimated accumulated hate the active character holds on this
	// mob. Floored at zero — an aggro-shedding spell (e.g. Jolt) can drive the
	// running total negative, but a displayed "negative threat" is meaningless.
	Hate int64 `json:"hate"`
	// TPS is hate-per-second over the engagement span (first to last hate event
	// on this mob), the rough analogue of the DPS meter's personal DPS.
	TPS float64 `json:"tps"`
	// IsTarget marks the mob the overlay should highlight: the player's current
	// target (from the Zeal pipe) when available, otherwise the most recently
	// engaged mob.
	IsTarget bool `json:"is_target"`
}

// ThreatState is the immutable snapshot broadcast to the overlay on every
// change.
type ThreatState struct {
	// InCombat is true while at least one mob is being tracked.
	InCombat bool `json:"in_combat"`
	// Target is the highlighted mob (current Zeal target, else most-recently
	// engaged). Nil when no mob is tracked. Points at the same data as the
	// matching entry in Mobs.
	Target *MobThreat `json:"target,omitempty"`
	// Mobs is every tracked mob, sorted by hate descending.
	Mobs []MobThreat `json:"mobs"`
	// HatemodPct is the static gear/AA hate modifier currently applied to every
	// generated hate value, as a signed percentage (e.g. +15 for an aggro-AA
	// tank). Surfaced so the overlay can show that an adjustment is in effect.
	HatemodPct int `json:"hatemod_pct"`
	// LastUpdated is the snapshot time.
	LastUpdated time.Time `json:"last_updated"`
}

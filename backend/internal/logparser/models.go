// Package logparser implements a real-time EverQuest log file tailer and
// event parser. It watches the active character's log file, reads new lines
// as they appear, and dispatches typed LogEvent values to a caller-supplied
// handler function.
package logparser

import "time"

// EventType identifies the class of a parsed log event.
type EventType string

const (
	// EventZone is emitted when the player changes zones.
	EventZone EventType = "log:zone"

	// EventCombatHit is emitted when a hit lands (player → NPC or NPC → player).
	EventCombatHit EventType = "log:combat_hit"

	// EventCombatMiss is emitted when an attack misses, is dodged, or is parried.
	EventCombatMiss EventType = "log:combat_miss"

	// EventSpellCast is emitted when the player begins casting a spell.
	EventSpellCast EventType = "log:spell_cast"

	// EventSpellInterrupt is emitted when a spell cast is interrupted.
	EventSpellInterrupt EventType = "log:spell_interrupt"

	// EventSpellResist is emitted when the target resists a spell.
	EventSpellResist EventType = "log:spell_resist"

	// EventSpellFade is emitted when a spell effect wears off.
	EventSpellFade EventType = "log:spell_fade"

	// EventSpellFadeFrom is emitted when a spell effect fades from a specific
	// target (e.g. "Tashanian effect fades from Soandso.").
	EventSpellFadeFrom EventType = "log:spell_fade_from"

	// EventSpellDidNotTakeHold is emitted when a spell the player just cast
	// fails to take effect because a stronger version is already on the target
	// (e.g. casting Major Shielding when Greater Shielding is already up).
	// EQ does not include the spell name in this message — consumers must
	// correlate with the most recent EventSpellCast.
	EventSpellDidNotTakeHold EventType = "log:spell_did_not_take_hold"

	// EventDeath is emitted when the player is slain.
	EventDeath EventType = "log:death"

	// EventKill is emitted when a mob is slain by the player or a group member.
	EventKill EventType = "log:kill"

	// EventHeal is emitted when a heal lands (player → target or target → player).
	EventHeal EventType = "log:heal"

	// EventConsidered is emitted when the player /con's a target and EQ prints
	// the disposition message (e.g. "a gnoll regards you as an ally.").
	EventConsidered EventType = "log:considered"
)

// LogEvent is the parsed representation of a single EQ log line.
type LogEvent struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
}

// ZoneData is the structured payload for EventZone.
type ZoneData struct {
	ZoneName string `json:"zone_name"`
}

// CombatHitData is the structured payload for EventCombatHit.
type CombatHitData struct {
	// Actor is "You" for player-initiated hits, or the NPC display name.
	Actor string `json:"actor"`
	// Skill is the attack verb (slash, pierce, bash, hit, etc.).
	Skill string `json:"skill"`
	// Target is the entity that was hit.
	Target string `json:"target"`
	// Damage is the number of hit points dealt.
	Damage int `json:"damage"`
}

// CombatMissData is the structured payload for EventCombatMiss.
type CombatMissData struct {
	// Actor is "You" or the NPC name that attempted the attack.
	Actor string `json:"actor"`
	// Target is the entity that was targeted.
	Target string `json:"target"`
	// MissType describes how the attack failed (miss, dodge, parry, riposte, block).
	MissType string `json:"miss_type"`
}

// SpellCastData is the structured payload for EventSpellCast.
type SpellCastData struct {
	SpellName string `json:"spell_name"`
}

// SpellInterruptData is the structured payload for EventSpellInterrupt.
type SpellInterruptData struct {
	SpellName string `json:"spell_name"`
}

// SpellResistData is the structured payload for EventSpellResist.
type SpellResistData struct {
	SpellName string `json:"spell_name"`
}

// SpellFadeData is the structured payload for EventSpellFade.
type SpellFadeData struct {
	SpellName string `json:"spell_name"`
}

// SpellFadeFromData is the structured payload for EventSpellFadeFrom.
type SpellFadeFromData struct {
	SpellName  string `json:"spell_name"`
	TargetName string `json:"target_name"`
}

// SpellDidNotTakeHoldData is the structured payload for EventSpellDidNotTakeHold.
// EQ's "did not take hold" message carries no spell name — the empty struct
// signals the event type only and consumers correlate with the most recent
// cast event themselves.
type SpellDidNotTakeHoldData struct{}

// DeathData is the structured payload for EventDeath.
type DeathData struct {
	SlainBy string `json:"slain_by"`
}

// KillData is the structured payload for EventKill.
type KillData struct {
	Killer string `json:"killer"` // "You" or the player's name
	Target string `json:"target"` // the mob that was slain
}

// HealData is the structured payload for EventHeal.
type HealData struct {
	// Actor is "You" for player-cast heals, or the healer's name.
	Actor string `json:"actor"`
	// Target is the entity that was healed. "You" means the player was healed.
	Target string `json:"target"`
	// Amount is the number of hit points restored.
	Amount int `json:"amount"`
}

// ConsideredData is the structured payload for EventConsidered.
type ConsideredData struct {
	// TargetName is the NPC display name as it appeared in the /con output.
	TargetName string `json:"target_name"`
}

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

	// EventSpellLanded is emitted when a log line matches a spell's cast_on_you
	// or cast_on_other text — i.e. the spell actually took effect on a target.
	// This is the authoritative signal for starting a spell timer (the
	// EventSpellCast "begin casting" event only signals an attempt).
	EventSpellLanded EventType = "log:spell_landed"

	// EventDeath is emitted when the player is slain.
	EventDeath EventType = "log:death"

	// EventKill is emitted when a mob is slain by the player or a group member.
	EventKill EventType = "log:kill"

	// EventHeal is emitted when a heal lands (player → target or target → player).
	EventHeal EventType = "log:heal"

	// EventConsidered is emitted when the player /con's a target and EQ prints
	// the disposition message (e.g. "a gnoll regards you as an ally.").
	EventConsidered EventType = "log:considered"

	// EventPetOwner is emitted when a pet announces its owner. EQ prints this
	// canonical line when a charm spell binds an NPC to a player ("Kebartik
	// says 'My leader is Kildrey.'"). Consumers map the pet name to the owner
	// so that pet damage can be rolled up under the owning player.
	EventPetOwner EventType = "log:pet_owner"

	// EventIllusionFade is emitted when the active player's illusion buff
	// drops, via either of EQ's two end-of-illusion messages:
	//   "Your illusion fades."         (natural end / dispel)
	//   "You forget Illusion: <Race>." (buff cleared / removed)
	// EQ does not name the specific illusion in either message; consumers
	// remove every active "Illusion: …" timer on the active player.
	EventIllusionFade EventType = "log:illusion_fade"

	// EventCritHit is emitted when an actor scores a critical hit. Project
	// Quarm / EQMac log this on a separate line preceding the actual damage
	// line ("Sandrian Scores a critical hit!(62)" → "Sandrian slashes Zun
	// Thall for 62 points of damage."). Consumers correlate by actor + amount
	// to flag the matching CombatHit as a crit; a standalone count of crits
	// per actor is also useful for Phase 1 stats.
	EventCritHit EventType = "log:crit_hit"

	// EventCharmedPet is emitted when a charmed NPC sends a tell to its
	// owner naming itself as the pet — the canonical "Attacking X Master."
	// line that charmed pets produce when given an attack order. Unlike
	// summoned pets (which announce "X says 'My leader is Y.'"), charmed
	// pets never name their owner in plain text, but the "tells you …
	// Master." phrasing is unambiguous: that pet belongs to the active
	// character. Consumers bind the pet to the active character's name.
	EventCharmedPet EventType = "log:charmed_pet"

	// EventCharmBroken is emitted on the active character's "Your charm
	// spell has worn off." line — releases every currently-charmed pet
	// from the owner mapping so its post-break attacks don't keep rolling
	// up under the player.
	EventCharmBroken EventType = "log:charm_broken"

	// EventRollAnnounce is emitted when EQ logs the first line of a /random
	// result: "**A Magic Die is rolled by <Name>." It carries only the
	// roller's name — the range and value arrive on the next line as
	// EventRollResult.
	EventRollAnnounce EventType = "log:roll_announce"

	// EventRollResult is emitted when EQ logs the second line of a /random
	// result: "**It could have been any number from N to M, but this time
	// it turned up a V." Consumers correlate this with the immediately
	// preceding EventRollAnnounce (same timestamp, no other log lines
	// interleave between them).
	EventRollResult EventType = "log:roll_result"

	// EventVerifiedPlayer is emitted when a chat line proves an entity is
	// another player (`X tells the guild/raid/group/you, '…'`). Used by
	// the combat tracker to disambiguate single-word boss names from
	// player names when routing third-party damage events: if one side of
	// a hit is a verified player and the other isn't, the other is the
	// NPC. EQ NPCs never use guild/raid/group/tell channels, so this
	// signal is unambiguous.
	EventVerifiedPlayer EventType = "log:verified_player"
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
	// Skill is the attack verb (slash, pierce, bash, hit, etc.) or a generic
	// damage-source label ("spell", "dot") when the line is a non-melee form.
	Skill string `json:"skill"`
	// Target is the entity that was hit.
	Target string `json:"target"`
	// Damage is the number of hit points dealt.
	Damage int `json:"damage"`
	// SpellName is the originating spell for spell- or DoT-tick damage. Empty
	// for melee hits and for non-melee lines that omit the spell name (e.g.
	// the bare "X was hit by non-melee for N" form).
	SpellName string `json:"spell_name,omitempty"`
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

// SpellLandedKind discriminates the two cast-text variants:
//
//	"you"   — line matched cast_on_you (the active player is the target)
//	"other" — line matched cast_on_other (TargetName is the captured name)
type SpellLandedKind string

const (
	SpellLandedKindYou   SpellLandedKind = "you"
	SpellLandedKindOther SpellLandedKind = "other"
)

// SpellLandedCandidate is one possible (id, name) pair when a cast line is
// ambiguous — multiple spells share identical cast text (e.g. 17 distinct
// spells all log "Your eyes tingle." on land). The engine resolves these
// against its lastCastSpell record.
type SpellLandedCandidate struct {
	SpellID   int    `json:"spell_id"`
	SpellName string `json:"spell_name"`
}

// SpellLandedData is the structured payload for EventSpellLanded.
//
// When the cast text uniquely identifies a spell, SpellID and SpellName are
// populated and Candidates is empty. When multiple spells share the text,
// SpellID is 0, SpellName is empty, and every candidate is listed. TargetName
// is empty for SpellLandedKindYou (the active character is the implicit
// target) and the captured name for SpellLandedKindOther.
type SpellLandedData struct {
	Kind       SpellLandedKind        `json:"kind"`
	SpellID    int                    `json:"spell_id"`
	SpellName  string                 `json:"spell_name"`
	TargetName string                 `json:"target_name"`
	Candidates []SpellLandedCandidate `json:"candidates,omitempty"`
}

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

// PetOwnerData is the structured payload for EventPetOwner. The pet announced
// the binding by saying "My leader is <owner>." — Pet is the speaker, Owner
// is the named leader. Owner may be "You" if the active character charmed the
// pet (EQ does not differentiate; consumers can match Owner against the
// character name).
type PetOwnerData struct {
	Pet   string `json:"pet"`
	Owner string `json:"owner"`
}

// IllusionFadeData carries no data — both EQ illusion-end messages omit the
// race, so consumers handle this by removing every illusion timer on the
// active player.
type IllusionFadeData struct{}

// CritHitData is the structured payload for EventCritHit. PQ's "Scores a
// critical hit!(N)" line names only the actor and the crit damage amount —
// the target and skill are inferred from the immediately-following CombatHit
// from the same actor whose damage matches.
type CritHitData struct {
	Actor  string `json:"actor"`
	Damage int    `json:"damage"`
}

// CharmedPetData is the structured payload for EventCharmedPet. The pet
// name is what the log line names as the speaker of "Attacking X Master.";
// the owner is always the active character (charmed-pet tells are sent to
// the charmer, who is the player).
type CharmedPetData struct {
	Pet string `json:"pet"`
}

// RollAnnounceData is the structured payload for EventRollAnnounce — the
// name of the player who triggered a /random.
type RollAnnounceData struct {
	Roller string `json:"roller"`
}

// RollResultData is the structured payload for EventRollResult — the range
// and outcome of a /random. The roller is named on the preceding
// EventRollAnnounce; consumers pair the two by adjacency.
type RollResultData struct {
	Min   int `json:"min"`
	Max   int `json:"max"`
	Value int `json:"value"`
}

// VerifiedPlayerData is the structured payload for EventVerifiedPlayer.
// Carries the speaker name from a chat-channel line so the tracker can
// add it to its verified-player set.
type VerifiedPlayerData struct {
	Name string `json:"name"`
}

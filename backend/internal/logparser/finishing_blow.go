package logparser

// FinishingBlowMin is the melee damage at and above which a hit is treated as
// an instant-kill special (finishing blow, slay, assassinate) rather than a
// swing. These log as an ordinary melee line with a fixed huge number —
// "Playerone backstabs a mob for 32000 points of damage.", "Playertwo pierces
// a mob for 32011 points of damage." — far beyond the mob's
// remaining HP, and no real swing comes anywhere near it. Counted as damage, a
// single one can double an attacker's total for the fight.
const FinishingBlowMin = 32000

// IsFinishingBlow reports whether an outgoing hit is an instant-kill special
// that should be counted, not added to damage. Spell and DoT damage is never
// treated this way: only melee and ranged lines carry the fixed number.
func IsFinishingBlow(data CombatHitData) bool {
	if data.Damage < FinishingBlowMin {
		return false
	}
	return data.Skill != "spell" && data.Skill != "dot"
}

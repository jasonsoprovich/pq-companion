package enums

// SpecialAbilityMeta carries the display name and a one-line description
// for a single NPC special-ability code, used by the overlay and item
// detail tooltips. The Description is omitted from JSON when empty so
// the same shape can hold the bare-name synthetic flags below.
type SpecialAbilityMeta struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Synthetic ability codes used by the overlay layer for flags that are
// stored on dedicated NPC columns (see_invis, see_invis_undead) rather
// than encoded inside the caret-delimited `npc_types.special_abilities`
// string. They sit well above SpecialAbility::Max (55) so they cannot
// collide with a real Quarm code.
const (
	SyntheticSeeInvis       = 1001
	SyntheticSeeInvisUndead = 1002
)

// specialAbilities holds the canonical SpecialAbility code → metadata
// mapping for Project Quarm.
//
// Source: EQMacEmu/Server common/emu_constants.h — the SpecialAbility
// namespace. These codes match what the server itself uses when parsing
// the npc_types.special_abilities column and differ from modern EQEmu
// master numbering (which inserts additional codes mid-enum). Codes 1–54
// are canonical; 1001–1002 are PQ Companion-internal synthetics for the
// dedicated see-invis columns.
//
// Descriptions are PQ Companion display copy, written to match in-game
// behavior on Quarm. Edit them freely; the codes themselves must not
// drift from the EQMacEmu source.
var specialAbilities = map[int]SpecialAbilityMeta{
	1:  {Name: "Summon", Description: "Will summon players who run out of melee range."},
	2:  {Name: "Enrage", Description: "Randomly enrages at low HP, attacking all nearby players for a short duration."},
	3:  {Name: "Rampage", Description: "Randomly hits nearby players instead of only the current target."},
	4:  {Name: "Area Rampage", Description: "Hits every player within melee range on each rampage tick."},
	5:  {Name: "Flurry", Description: "Can strike multiple times in rapid succession on a single attack round."},
	6:  {Name: "Triple Attack", Description: "Attacks three times per combat round."},
	7:  {Name: "Dual Wield", Description: "Attacks with two weapons simultaneously."},
	8:  {Name: "Disallow Equip", Description: "Cannot equip items in this slot."},
	9:  {Name: "Bane Attack", Description: "NPC's melee counts as bane damage."},
	10: {Name: "Magical Attack", Description: "NPC's melee counts as magical and bypasses immune-to-melee."},
	11: {Name: "Ranged Attack", Description: "Performs ranged attacks (bow/throwing) at distance."},
	12: {Name: "Immune to Slow", Description: "Cannot be slowed by any spell or effect."},
	13: {Name: "Immune to Mez", Description: "Cannot be mesmerized."},
	14: {Name: "Immune to Charm", Description: "Cannot be charmed by any spell or effect."},
	15: {Name: "Immune to Stun", Description: "Cannot be stunned."},
	16: {Name: "Immune to Snare", Description: "Cannot be snared or rooted."},
	17: {Name: "Immune to Fear", Description: "Cannot be feared."},
	18: {Name: "Immune to Dispel", Description: "Buffs and effects cannot be removed by dispel spells."},
	19: {Name: "Immune to Melee", Description: "Cannot be damaged by ordinary melee attacks."},
	20: {Name: "Immune to Magic", Description: "Immune to all spell damage."},
	21: {Name: "Immune to Fleeing", Description: "Does not flee when health drops low."},
	22: {Name: "Immune to Non-Bane Melee", Description: "Only takes damage from melee weapons with bane damage."},
	23: {Name: "Immune to Non-Magical Melee", Description: "Only takes damage from magical melee weapons."},
	24: {Name: "Immune to Aggro", Description: "Cannot generate aggro on other NPCs."},
	25: {Name: "Immune to Being Aggro'd", Description: "Other NPCs cannot generate aggro on this mob."},
	26: {Name: "Immune to Ranged Spells", Description: "Spells cast from outside melee range have no effect."},
	27: {Name: "Immune to Feign Death", Description: "Will not be fooled by Feign Death."},
	28: {Name: "Immune to Taunt", Description: "Cannot be taunted off its current target."},
	29: {Name: "Tunnel Vision", Description: "Sticks to its current target until it dies or zones."},
	30: {Name: "Won't Heal/Buff Allies", Description: "Will not heal or buff other NPCs in its faction."},
	31: {Name: "Immune to Pacify", Description: "Cannot be pacified or lulled."},
	32: {Name: "Leashed", Description: "Returns to spawn point if pulled too far."},
	33: {Name: "Tethered", Description: "Resets to full HP if pulled out of its tether range."},
	34: {Name: "Permaroot Flee", Description: "Flees in place when low HP — does not move."},
	35: {Name: "Immune to Harm from Client", Description: "Players cannot damage this NPC directly."},
	36: {Name: "Always Flees", Description: "Always tries to flee, regardless of HP."},
	37: {Name: "Flee Percent", Description: "Flees at a custom HP percentage."},
	38: {Name: "Allows Beneficial Spells", Description: "Will accept beneficial spells from players."},
	39: {Name: "Melee Disabled", Description: "Will not perform melee attacks."},
	40: {Name: "Chase Distance", Description: "Custom maximum chase distance from spawn."},
	41: {Name: "Allowed to Tank", Description: "Can be the primary target for charmed pets/swarm pets."},
	42: {Name: "Proximity Aggro", Description: "Aggros on any player entering its proximity, regardless of faction."},
	43: {Name: "Always Calls for Help", Description: "Always calls nearby allies into combat."},
	44: {Name: "Use Warrior Skills", Description: "Performs warrior-class melee specials regardless of NPC class."},
	45: {Name: "Always Flee on Low Con", Description: "Always flees from gray-con players."},
	46: {Name: "No Loitering", Description: "Does not loiter — returns to spawn or despawns immediately."},
	47: {Name: "Block Handin on Bad Faction", Description: "Refuses quest hand-ins from players with bad faction."},
	48: {Name: "PC Deathblow Corpse", Description: "Corpse can be deathblown for the killing PC."},
	49: {Name: "Corpse Camper", Description: "Lingers near corpses after kills."},
	50: {Name: "Reverse Slow", Description: "Slows applied to this NPC instead haste it."},
	51: {Name: "Immune to Haste", Description: "Cannot be hasted."},
	52: {Name: "Immune to Disarm", Description: "Cannot be disarmed."},
	53: {Name: "Immune to Riposte", Description: "Melee attacks against this NPC cannot be riposted."},
	54: {Name: "Proximity Aggro 2", Description: "Secondary proximity-aggro variant."},

	SyntheticSeeInvis:       {Name: "See Invis", Description: "Can see invisible players and pets."},
	SyntheticSeeInvisUndead: {Name: "See Invis vs Undead", Description: "Can see players hidden with Invisibility vs. Undead."},
}

// SpecialAbilityName returns the display name for a SpecialAbility code,
// or an empty string when the code is unknown. The empty-string fallback
// matches the legacy behavior of the previous db package map.
func SpecialAbilityName(code int) string {
	return specialAbilities[code].Name
}

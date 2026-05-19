package enums

// charRaces maps the 1-based PC race index used by the character-
// creation UI and any EQ API that takes a "playable race ID" to its
// display name.
//
// Important: this is *not* the npc_types.race enum, which is the full
// EQMacEmu Race namespace (where 13 = Aviak, 14 = Werewolf, etc.).
// The PC race index is a compact 1-14 mapping that the EQ classic
// client uses internally for character selection. The two enums
// overlap at IDs 1-12 (the original playable races stored the same
// way in both schemas) but diverge at 13 and 14.
//
// No DB column reflects this enum directly (PC characters are tracked
// in user.db, not quarm.db), so it has no AuditDef.
//
// Source: EQMacEmu/Server common/races.h RaceIndex namespace.
var charRaces = map[int]string{
	1:  "Human",
	2:  "Barbarian",
	3:  "Erudite",
	4:  "Wood Elf",
	5:  "High Elf",
	6:  "Dark Elf",
	7:  "Half Elf",
	8:  "Dwarf",
	9:  "Troll",
	10: "Ogre",
	11: "Halfling",
	12: "Gnome",
	13: "Iksar",
	14: "Vah Shir",
}

// CharRaceName returns the display name for a PC race index.
func CharRaceName(id int) string {
	return charRaces[id]
}

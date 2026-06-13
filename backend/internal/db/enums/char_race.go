package enums

// charRaces maps the raw EQ race id used for PCs to a display name.
//
// This is the same namespace as npc_types.race (EQMacEmu Race), filtered
// to the player-eligible races. Zeal's quarmy.txt writes the raw race id
// (e.g. Iksar = 128, Vah Shir = 130, Froglok = 330) — NOT the compact 1–14
// "RaceIndex" the classic client uses for character creation — so we store
// and look up by the raw id. The original 12 races share the same id in
// both schemes, which is why earlier code that conflated them only broke
// for Iksar / Vah Shir / Froglok.
//
// No DB column reflects this enum directly (PC characters are tracked
// in user.db, not quarm.db), so it has no AuditDef.
//
// Source: EQMacEmu/Server common/races.h Race namespace.
var charRaces = map[int]string{
	1:   "Human",
	2:   "Barbarian",
	3:   "Erudite",
	4:   "Wood Elf",
	5:   "High Elf",
	6:   "Dark Elf",
	7:   "Half Elf",
	8:   "Dwarf",
	9:   "Troll",
	10:  "Ogre",
	11:  "Halfling",
	12:  "Gnome",
	128: "Iksar",
	130: "Vah Shir",
	330: "Froglok",
}

// CharRaceName returns the display name for a PC race index.
func CharRaceName(id int) string {
	return charRaces[id]
}

// charRaceToItemBit maps a raw EQ PC race id to the bit it occupies in an
// item's items.races bitmask. The 12 original races share id == bit-position+1
// in order; Iksar/Vah Shir use raw ids 128/130 but bits 0x1000/0x2000.
// Froglok (330) has no Quarm-era item race bit and maps to 0 (no restriction).
var charRaceToItemBit = map[int]int{
	1: 0x0001, 2: 0x0002, 3: 0x0004, 4: 0x0008, 5: 0x0010,
	6: 0x0020, 7: 0x0040, 8: 0x0080, 9: 0x0100, 10: 0x0200,
	11: 0x0400, 12: 0x0800, 128: 0x1000, 130: 0x2000,
}

// RaceBitForCharRace returns the items.races bitmask bit for a PC race id, or
// 0 when the race has no era-relevant bit (treat as unrestricted).
func RaceBitForCharRace(id int) int {
	return charRaceToItemBit[id]
}

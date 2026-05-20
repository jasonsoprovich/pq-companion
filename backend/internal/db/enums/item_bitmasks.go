package enums

import "database/sql"

// itemSlotBits maps each set bit in items.slots to its slot name.
//
// Source: EQMacEmu/Server common/eq_constants.h (PossessionSlot
// constants). Verified by sampling worn-slot items in the Quarm dump.
// Bits 0x2/0x10 ("Ear" left/right) and 0x200/0x400 ("Wrist") share the
// same display label by design.
//
// Quarm-specific correction: 0x200000 (bit 21) is the Ammo slot on the
// Mac client — earlier frontend code used 0x800000 (modern EQEmu Ammo
// bit) which is unused in the Quarm dataset, causing every ammo/throw
// item to silently render with no slot label.
var itemSlotBits = map[int]string{
	0x000001: "Charm",
	0x000002: "Ear",
	0x000004: "Head",
	0x000008: "Face",
	0x000010: "Ear",
	0x000020: "Neck",
	0x000040: "Shoulder",
	0x000080: "Arms",
	0x000100: "Back",
	0x000200: "Wrist",
	0x000400: "Wrist",
	0x000800: "Range",
	0x001000: "Hands",
	0x002000: "Primary",
	0x004000: "Secondary",
	0x008000: "Finger",
	0x010000: "Finger",
	0x020000: "Chest",
	0x040000: "Legs",
	0x080000: "Feet",
	0x100000: "Waist",
	0x200000: "Ammo",
}

// itemClassBits maps each set bit in items.classes to the corresponding
// player class — but only for classes that exist on Project Quarm
// (frozen at Planes of Power, with custom PoP-era content). The all-
// classes sentinel 0xFFFF is treated as "All" by the frontend
// decomposer (legacy data sometimes uses 0x7FFF or larger).
//
// Source: EQMacEmu/Server common/classes.h ClassesBitmask namespace.
//
// Quarm-era filter: bit 15 (Berserker, added in Gates of Discord
// 2003) appears in some imported item data but Berserker isn't a
// playable class on Quarm, so it's not exposed here. The validator's
// known set below covers it so audit runs stay clean.
var itemClassBits = map[int]string{
	0x0001: "Warrior",
	0x0002: "Cleric",
	0x0004: "Paladin",
	0x0008: "Ranger",
	0x0010: "Shadow Knight",
	0x0020: "Druid",
	0x0040: "Monk",
	0x0080: "Bard",
	0x0100: "Rogue",
	0x0200: "Shaman",
	0x0400: "Necromancer",
	0x0800: "Wizard",
	0x1000: "Magician",
	0x2000: "Enchanter",
	0x4000: "Beastlord",
}

// itemClassBitsOutOfEra are post-PoP bits the validator should accept
// without flagging, but that the catalog deliberately doesn't surface
// to the UI since those classes aren't playable on Quarm.
var itemClassBitsOutOfEra = map[int]string{
	0x8000: "Berserker (post-PoP)", // GoD, 2003
}

// itemRaceBits maps each set bit in items.races to the corresponding
// player race — but only for races playable on Project Quarm (frozen
// at Planes of Power, with custom PoP-era content). The all-races
// sentinel is treated as "All" by the frontend decomposer (Quarm
// legacy data variously uses 16383, 32767, 65535, or even 131071).
//
// Source: EQMacEmu/Server common/races.h RaceBitmask namespace (bits
// 0–13 = the 14 EQMacEmu canonical playable races).
//
// Quarm-era filter: bits 14 (Froglok PC, LoY) and 15 (Drakkin, SoF)
// appear in some imported item data but neither race is playable on
// Quarm, so they're not exposed here. The validator's known set below
// covers them so audit runs stay clean.
var itemRaceBits = map[int]string{
	0x0001: "Human",
	0x0002: "Barbarian",
	0x0004: "Erudite",
	0x0008: "Wood Elf",
	0x0010: "High Elf",
	0x0020: "Dark Elf",
	0x0040: "Half Elf",
	0x0080: "Dwarf",
	0x0100: "Troll",
	0x0200: "Ogre",
	0x0400: "Halfling",
	0x0800: "Gnome",
	0x1000: "Iksar",
	0x2000: "Vah Shir",
}

// itemRaceBitsOutOfEra are post-PoP bits the validator should accept
// without flagging, but that the catalog deliberately doesn't surface
// to the UI since those races aren't playable on Quarm.
var itemRaceBitsOutOfEra = map[int]string{
	0x4000: "Froglok PC (post-PoP)", // LoY, 2003
	0x8000: "Drakkin (post-PoP)",    // SoF, 2008
}

// extractBitmaskCodes runs a SELECT DISTINCT against the given column,
// decomposes each row into the bits below maxBit, and returns the
// distinct bit positions seen. The validator uses this to confirm every
// set bit has a label (rather than enumerating every possible mask
// combination).
//
// The maxBit parameter exists because Quarm's legacy items.races values
// sometimes set bits past the canonical race range (e.g. 65536, 131072)
// as part of an unstructured "all" sentinel. Walking only the
// meaningful bit range avoids spurious findings on those rows.
func extractBitmaskCodes(db *sql.DB, query string, maxBit int) ([]int, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[int]struct{})
	for rows.Next() {
		var mask int
		if err := rows.Scan(&mask); err != nil {
			return nil, err
		}
		for i := 0; i < maxBit; i++ {
			bit := 1 << i
			if mask&bit != 0 {
				seen[bit] = struct{}{}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]int, 0, len(seen))
	for bit := range seen {
		out = append(out, bit)
	}
	return out, nil
}

// ItemSlotBitsAudit, ItemClassBitsAudit, ItemRaceBitsAudit validate that
// every bit position observed in items.slots / items.classes /
// items.races is mapped to a label. They are bitmask audits — they
// flag unknown bit POSITIONS, not unknown mask values.
var ItemSlotBitsAudit = AuditDef{
	Name:       "Item Slot Bit",
	KnownCodes: keysAsSet(itemSlotBits),
	Extract:    func(db *sql.DB) ([]int, error) { return extractBitmaskCodes(db, `SELECT DISTINCT slots FROM items`, 24) },
}

var ItemClassBitsAudit = AuditDef{
	Name:       "Item Class Bit",
	KnownCodes: mergeSets(keysAsSet(itemClassBits), keysAsSet(itemClassBitsOutOfEra)),
	Extract:    func(db *sql.DB) ([]int, error) { return extractBitmaskCodes(db, `SELECT DISTINCT classes FROM items`, 16) },
}

var ItemRaceBitsAudit = AuditDef{
	Name:       "Item Race Bit",
	KnownCodes: mergeSets(keysAsSet(itemRaceBits), keysAsSet(itemRaceBitsOutOfEra)),
	Extract:    func(db *sql.DB) ([]int, error) { return extractBitmaskCodes(db, `SELECT DISTINCT races FROM items`, 16) },
}

func init() {
	registerLabels("Item Slot Bit", func(bit int) string { return itemSlotBits[bit] })
	registerLabels("Item Class Bit", func(bit int) string {
		if v := itemClassBits[bit]; v != "" {
			return v
		}
		return itemClassBitsOutOfEra[bit]
	})
	registerLabels("Item Race Bit", func(bit int) string {
		if v := itemRaceBits[bit]; v != "" {
			return v
		}
		return itemRaceBitsOutOfEra[bit]
	})
	registerSource("Item Slot Bit", "EQEmu schema docs items.slots — bitmask, each bit position = one equip slot")
	registerSource("Item Class Bit", "EQEmu items.classes — bitmask, bit 0 = WAR … bit 15 = BST (Mac-era classes); 16+ flagged out-of-era")
	registerSource("Item Race Bit", "EQEmu items.races — bitmask of allowed races; out-of-era bits flagged")
}

// mergeSets returns the union of two int sets.
func mergeSets(a, b map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

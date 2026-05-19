package enums

// charClasses maps the 0-based PC class index used in the EQ spell-table
// APIs (e.g. /api/spells/class/{classIndex}) and character-creation
// dropdowns to a display label of the form "ABBR — Full Name".
//
// Important: this enum is *not* the npc_types.class enum (which is
// 1-based and includes Unknown / GM variants / service NPCs — see
// npcClasses). It exists separately because the spell-class API and
// the UI dropdowns use 0 = Warrior, 1 = Cleric, ..., 14 = Beastlord.
//
// No DB column reflects this enum directly, so it has no AuditDef —
// the values are stable across Quarm releases.
//
// Source: EQMacEmu/Server common/classes.h (Class namespace minus the
// "+ 1" offset used in npc_types.class). Three-letter abbreviations
// follow the classic EQ /who output convention.
var charClasses = map[int]string{
	0:  "WAR — Warrior",
	1:  "CLR — Cleric",
	2:  "PAL — Paladin",
	3:  "RNG — Ranger",
	4:  "SHD — Shadow Knight",
	5:  "DRU — Druid",
	6:  "MNK — Monk",
	7:  "BRD — Bard",
	8:  "ROG — Rogue",
	9:  "SHM — Shaman",
	10: "NEC — Necromancer",
	11: "WIZ — Wizard",
	12: "MAG — Magician",
	13: "ENC — Enchanter",
	14: "BST — Beastlord",
}

// CharClassLabel returns the display label for a 0-based PC class index.
func CharClassLabel(id int) string {
	return charClasses[id]
}

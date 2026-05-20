package enums

import "database/sql"

// npcBodyTypes maps the npc_types.bodytype column value to a display
// name.
//
// Source: EQMacEmu/Server common/bodytypes.h — the BodyType namespace
// is authoritative. The previous frontend table for this column had
// every label off by one or more slots (it labeled bodytype 1 as
// "Biped" when canonical is "Humanoid", bodytype 2 as "Giant" when
// canonical is "Lycanthrope", and so on for nearly every entry).
//
// Quarm-display choices:
//   - The internal Summoned 2 / Summoned 3 variants (27/28) and the
//     Dragon 2 / Dragon 3 variants (29/32) collapse to "Summoned" and
//     "Dragon" since the numbered distinction is meaningless to
//     players inspecting an NPC.
//   - Bodytype 65 isn't in the EQMacEmu enum but appears on environment
//     trigger objects in the Quarm dump; surfaced as "Untargetable"
//     alongside the canonical untargetable types (11, 60).
//   - Bodytype 0 covers 90 NPCs with no explicit type set; rendered as
//     "Unknown".
var npcBodyTypes = map[int]string{
	0:  "Unknown",
	1:  "Humanoid",
	2:  "Lycanthrope",
	3:  "Undead",
	4:  "Giant",
	5:  "Construct",
	6:  "Extraplanar",
	7:  "Magical",
	8:  "Summoned Undead",
	9:  "Raid Giant",
	10: "Raid Coldain",
	11: "Untargetable",
	12: "Vampire",
	13: "Aten Ha Ra",
	14: "Akheva",
	15: "Khati Sha",
	16: "Seru",
	17: "Grieg Veneficus",
	18: "Draz Nurakk",
	19: "Zek",
	20: "Luggald",
	21: "Animal",
	22: "Insect",
	23: "Monster",
	24: "Summoned",
	25: "Plant",
	26: "Dragon",
	27: "Summoned",
	28: "Summoned",
	29: "Dragon",
	30: "Velious Dragon",
	31: "Familiar",
	32: "Dragon",
	33: "Boxes",
	34: "Muramite",
	60: "Untargetable",
	63: "Swarm Pet",
	64: "Monster Summon",
	65: "Untargetable",
	66: "Invisible",
	67: "Special",
}

// NPCBodyTypeName returns the display name for an npc_types.bodytype
// value, or the empty string when unknown.
func NPCBodyTypeName(id int) string {
	return npcBodyTypes[id]
}

// NPCBodyTypesAudit validates that every distinct npc_types.bodytype
// seen in the DB is mapped above.
var NPCBodyTypesAudit = AuditDef{
	Name:       "NPC Body Type",
	KnownCodes: keysAsSet(npcBodyTypes),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT bodytype FROM npc_types`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	},
}

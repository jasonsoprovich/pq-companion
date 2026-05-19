package enums

import "database/sql"

// npcClasses maps the npc_types.class column value to a display name.
//
// Source: EQMacEmu/Server common/classes.h — the Class enum, 1-based.
// Codes 1–16 are the canonical EQ classes; 20–35 are the in-zone GM
// trainer variants (each PC class + 19); 40–41 cover the service NPC
// roles (banker, merchant). This is *not* the 0-based "class index"
// used by the spell APIs — see CharClasses for that.
var npcClasses = map[int]string{
	0:  "Unknown",
	1:  "Warrior",
	2:  "Cleric",
	3:  "Paladin",
	4:  "Ranger",
	5:  "Shadow Knight",
	6:  "Druid",
	7:  "Monk",
	8:  "Bard",
	9:  "Rogue",
	10: "Shaman",
	11: "Necromancer",
	12: "Wizard",
	13: "Magician",
	14: "Enchanter",
	15: "Beastlord",
	16: "Berserker",
	20: "Warrior GM",
	21: "Cleric GM",
	22: "Paladin GM",
	23: "Ranger GM",
	24: "Shadow Knight GM",
	25: "Druid GM",
	26: "Monk GM",
	27: "Bard GM",
	28: "Rogue GM",
	29: "Shaman GM",
	30: "Necromancer GM",
	31: "Wizard GM",
	32: "Magician GM",
	33: "Enchanter GM",
	34: "Beastlord GM",
	35: "Berserker GM",
	40: "Banker",
	41: "Merchant",
}

// NPCClassName returns the display name for an npc_types.class value,
// or the empty string when unknown.
func NPCClassName(id int) string {
	return npcClasses[id]
}

// NPCClassesAudit validates that every distinct npc_types.class seen in
// the DB is mapped above.
var NPCClassesAudit = AuditDef{
	Name:       "NPC Class",
	KnownCodes: keysAsSet(npcClasses),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT class FROM npc_types`)
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

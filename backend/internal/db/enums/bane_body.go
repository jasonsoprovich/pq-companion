package enums

import "database/sql"

// baneBodyTypes maps the items.banedmgbody column value to the body
// type a weapon's bane damage applies to. Same value space as
// npc_types.bodytype.
//
// Source: EQMacEmu/Server common/eq_constants.h BodyType enum, with
// labels verified by sampling actual bane-damage items in the Quarm
// dump. "Humanoid (alt)" at 14 and "Summoned (alt)" at 28 are bodytype
// values that overlap conceptually with codes 1 / 13 but appear in the
// dataset.
var baneBodyTypes = map[int]string{
	1:  "Humanoid",
	2:  "Lycanthrope",
	3:  "Undead",
	4:  "Giant",
	5:  "Construct",
	6:  "Extraplanar",
	7:  "Magical",
	8:  "Summoned Undead",
	10: "Vampire",
	12: "Dragon",
	13: "Summoned",
	14: "Humanoid (alt)",
	16: "Plant",
	17: "Animal",
	18: "Insect",
	19: "Muramite",
	24: "Animation", // appears on Scimitar of Oak; matches bodytype 24 on Animation NPCs
	25: "Chest",
	26: "Amphibian",
	28: "Summoned (alt)",
}

// BaneBodyName returns the display name for an items.banedmgbody value.
func BaneBodyName(id int) string {
	return baneBodyTypes[id]
}

func init() {
	registerLabels("Bane Body", BaneBodyName)
	registerSource("Bane Body", "EQMacEmu/Server common/eq_constants.h BodyType enum (shares value space with NPC Body Type)")
}

// BaneBodiesAudit validates that every distinct items.banedmgbody (other
// than 0 = no bane) seen in the DB is mapped above.
var BaneBodiesAudit = AuditDef{
	Name:       "Bane Body",
	KnownCodes: keysAsSet(baneBodyTypes),
	Sample: func(db *sql.DB, code, limit int) ([]SampleRow, error) {
		return sampleRows(db, "items", "banedmgbody", code, limit)
	},
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT banedmgbody FROM items WHERE banedmgbody != 0`)
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

package enums

import "database/sql"

// tradeskills maps the tradeskill_recipe.tradeskill column value to a
// display name.
//
// Source: EQMacEmu/Server common/skills.h — the SkillType enum. Project
// Quarm's dump uses the same numeric IDs as EQMacEmu for ids 55–69. ID 75
// is a Quarm-only convention covering nofail/zero-skill combines (poison
// dose stacking, quest combines, etc.); EQMacEmu's SkillType enum stops
// at SkillTaunt = 73 and modern EQEmu uses 75 for the unrelated
// SkillRemoveTraps, so neither upstream applies. We surface it as
// "Common Combine" — the same label as ID 0 — because that's the
// observed behavior on Quarm.
//
// Quarm override: 75 → "Common Combine".
var tradeskills = map[int]string{
	0:  "Common Combine",
	55: "Fishing",
	56: "Make Poison",
	57: "Tinkering",
	58: "Research",
	59: "Alchemy",
	60: "Baking",
	61: "Tailoring",
	62: "Sense Traps",
	63: "Blacksmithing",
	64: "Fletching",
	65: "Brewing",
	66: "Alcohol Tolerance",
	67: "Begging",
	68: "Jewelry Making",
	69: "Pottery",
	75: "Common Combine",
}

// TradeskillName returns the display name for a tradeskill ID. Unknown
// IDs return the empty string so callers can decide between a generic
// fallback and a numeric-id stub.
func TradeskillName(id int) string {
	return tradeskills[id]
}

// TradeskillsAudit drives the enum-audit CLI against a live quarm.db:
// every distinct tradeskill_recipe.tradeskill value (enabled recipes
// only) should be present in the canonical map above.
var TradeskillsAudit = AuditDef{
	Name:       "Tradeskill",
	KnownCodes: keysAsSet(tradeskills),
	Sample: func(db *sql.DB, code, limit int) ([]SampleRow, error) {
		return sampleRows(db, "tradeskill_recipe", "tradeskill", code, limit)
	},
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT tradeskill FROM tradeskill_recipe WHERE enabled = 1`)
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

func init() {
	registerLabels("Tradeskill", TradeskillName)
	registerSource("Tradeskill", "EQMacEmu/Server common/skills.h SkillType enum (Quarm-only ID 75 → Common Combine)")
}

package enums

import "database/sql"

// containerTypes maps a tradeskill combine-container code to a display name.
//
// In tradeskill_recipe_entries, a row with iscontainer = 1 whose item_id has
// no matching items row is NOT an item id — it's a bagtype / world-container
// code. It means "this combine can be done in any container of this type"
// (e.g. a Forge, an Oven, an Enchanter's Lexicon) rather than one specific
// inventory container. Recipes that also accept a specific named container
// (e.g. "Tome of Endless Enchantments") list that item separately.
//
// Source: EQMacEmu/Server common/item_data.h — the BagTypes enum. Code 30
// ("Always Works" upstream) is surfaced as "Any Container" since it denotes a
// common combine that works in any container.
var containerTypes = map[int]string{
	0:  "Small Bag",
	1:  "Large Bag",
	2:  "Quiver",
	3:  "Belt Pouch",
	4:  "Wrist Pouch",
	5:  "Back Pack",
	6:  "Small Chest",
	7:  "Large Chest",
	8:  "Bandolier",
	9:  "Medicine Bag",
	10: "Tool Box",
	11: "Lexicon",
	12: "Mortar",
	13: "Self Dusting",
	14: "Mixing Bowl",
	15: "Oven",
	16: "Sewing Kit",
	17: "Forge",
	18: "Fletching Kit",
	19: "Brew Barrel",
	20: "Jewelers Kit",
	21: "Pottery Wheel",
	22: "Kiln",
	23: "Keymaker",
	24: "Wizards Lexicon",
	25: "Mages Lexicon",
	26: "Necromancers Lexicon",
	27: "Enchanters Lexicon",
	28: "Container Type 28",
	29: "Concordance of Research",
	30: "Any Container",
	31: "Koada'Dal Forge",
	32: "Teir'Dal Forge",
	33: "Oggok Forge",
	34: "Stormguard Forge",
	35: "Ak'Anon Forge",
	36: "Northman Forge",
	37: "Container Type 37",
	38: "Cabilis Forge",
	39: "Freeport Forge",
	40: "Royal Qeynos Forge",
	41: "Halfling Tailoring Kit",
	42: "Erudite Tailoring Kit",
	43: "Fier'Dal Tailoring Kit",
	44: "Fier'Dal Fletching Kit",
	45: "Iksar Pottery Wheel",
	46: "Tackle Box",
	47: "Troll Forge",
	48: "Fier'Dal Forge",
	49: "Vale Forge",
	50: "Erudite Forge",
	51: "Traders Satchel",
}

// ContainerTypeName returns the display name for a combine-container code.
// Unknown codes fall back to a generic label so the recipe view never shows a
// raw number.
func ContainerTypeName(code int) string {
	if name, ok := containerTypes[code]; ok {
		return name
	}
	return "Combine Container"
}

// ContainerTypesAudit drives the enum-audit CLI: every distinct combine-
// container code referenced by a recipe entry (iscontainer rows whose item_id
// has no items row) should be present in the canonical map above.
var ContainerTypesAudit = AuditDef{
	Name:       "ContainerType",
	KnownCodes: keysAsSet(containerTypes),
	Sample: func(db *sql.DB, code, limit int) ([]SampleRow, error) {
		return sampleRows(db, "tradeskill_recipe_entries", "item_id", code, limit)
	},
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`
			SELECT DISTINCT tre.item_id
			FROM tradeskill_recipe_entries tre
			LEFT JOIN items i ON i.id = tre.item_id
			WHERE tre.iscontainer = 1 AND i.id IS NULL`)
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
	registerLabels("ContainerType", ContainerTypeName)
	registerSource("ContainerType", "EQMacEmu/Server common/item_data.h BagTypes enum")
}

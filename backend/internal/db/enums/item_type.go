package enums

import "database/sql"

// itemTypes maps the items.itemtype column value to a display name.
//
// Source: EQMacEmu/Server common/item_data.h — the ItemType enum. The
// Mac client's enum diverges from modern EQEmu master starting at
// value 34 (modern EQEmu inserted additional types mid-enum), so any
// PR-Companion display label past 33 must be sourced from the Mac fork
// rather than the EQEmu schema docs.
//
// Gaps (6, 13, 28, 41, 43, 44, 46–51) are intentional — values either
// unused in the Mac era or not represented in the Quarm dump. The
// enum-audit CLI confirms no unknown itemtype appears in items.
var itemTypes = map[int]string{
	0:  "1H Slashing",
	1:  "2H Slashing",
	2:  "1H Piercing",
	3:  "1H Blunt",
	4:  "2H Blunt",
	5:  "Bow",
	7:  "Large Throwing",
	8:  "Shield",
	9:  "Scroll",
	10: "Armor",
	11: "Miscellaneous",
	12: "Lock Picks",
	14: "Food",
	15: "Drink",
	16: "Light",
	17: "Combinable",
	18: "Bandages",
	19: "Small Throwing",
	20: "Spell",
	21: "Potion",
	22: "Fletched Arrows",
	23: "Wind Instrument",
	24: "Stringed Instrument",
	25: "Brass Instrument",
	26: "Percussion Instrument",
	27: "Arrow",
	29: "Jewelry",
	30: "Skull",
	31: "Book",
	32: "Note",
	33: "Key",
	34: "Coin",
	35: "2H Piercing",
	36: "Fishing Pole",
	37: "Fishing Bait",
	38: "Alcohol",
	39: "Key (Alt)",
	40: "Compass",
	42: "Poison",
	45: "Hand to Hand",
	52: "Container Token",
}

// ItemTypeName returns the display name for an items.itemtype value, or
// the empty string when unknown.
func ItemTypeName(id int) string {
	return itemTypes[id]
}

// ItemTypesAudit validates that every distinct items.itemtype seen in
// the DB is mapped above.
var ItemTypesAudit = AuditDef{
	Name:       "Item Type",
	KnownCodes: keysAsSet(itemTypes),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT itemtype FROM items`)
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

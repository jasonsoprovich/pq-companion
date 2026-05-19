package enums

import "database/sql"

// baneRaces maps the items.banedmgrace column to a display name. Same
// value space as npc_types.race — the bane-vs-race column tells the
// game which race a weapon's bane damage applies against.
//
// Source: EQMacEmu/Server common/races.h Race namespace (same as
// npcRaces). Derived directly from npcRaces below so the two stay in
// lockstep — if Quarm ever changed a race ID's name, we update one
// place. (Distinct map kept for now in case bane-vs-race ever needs
// labels that diverge from NPC display labels.)
var baneRaces = func() map[int]string {
	out := make(map[int]string, len(npcRaces))
	for k, v := range npcRaces {
		out[k] = v
	}
	return out
}()

// BaneRaceName returns the display name for an items.banedmgrace value.
func BaneRaceName(id int) string {
	return baneRaces[id]
}

// BaneRacesAudit validates that every distinct items.banedmgrace (other
// than 0 = no bane) seen in the DB is mapped above.
var BaneRacesAudit = AuditDef{
	Name:       "Bane Race",
	KnownCodes: keysAsSet(baneRaces),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT banedmgrace FROM items WHERE banedmgrace != 0`)
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

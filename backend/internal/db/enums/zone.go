package enums

import "database/sql"

// zoneExpansions maps the zone.expansion column to its release name.
//
// Source: EQ release timeline (Sony / Verant / Daybreak). Values 0–7
// are everything through Omens of War; 8–14 are post-EQMacEmu eras
// that Quarm shouldn't normally surface but are present in the zone
// table because the upstream EQEmu data carries them.
var zoneExpansions = map[int]string{
	-1: "Hidden",       // loading/dev zones, not part of any expansion
	99: "Quarm Custom", // Quarm-added content (Aviak Village, Marauders Mire, etc.)
	0:  "Classic",
	1:  "Ruins of Kunark",
	2:  "Scars of Velious",
	3:  "Shadows of Luclin",
	4:  "Planes of Power",
	5:  "Legacy of Ykesha",
	6:  "Lost Dungeons of Norrath",
	7:  "Gates of Discord",
	8:  "Omens of War",
	9:  "Dragons of Norrath",
	10: "Depths of Darkhollow",
	11: "Prophecy of Ro",
	12: "The Serpent's Spine",
	13: "The Buried Sea",
	14: "Secrets of Faydwer",
}

// ZoneExpansionName returns the display name for a zone.expansion value.
func ZoneExpansionName(id int) string {
	return zoneExpansions[id]
}

// ZoneExpansionsAudit validates that every distinct zone.expansion seen
// in the DB is mapped above.
var ZoneExpansionsAudit = AuditDef{
	Name:       "Zone Expansion",
	KnownCodes: keysAsSet(zoneExpansions),
	Extract: func(db *sql.DB) ([]int, error) {
		rows, err := db.Query(`SELECT DISTINCT expansion FROM zone`)
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

// zoneTypes maps a spell's zonetype restriction (spells_new.zonetype)
// to its readable category.
//
// Source: EQEmu schema docs — these are the four scenery categories the
// zone-restriction spell flag uses. 0 means "no restriction" so it has
// no label of its own; callers should treat zoneType(0) as "any".
var zoneTypes = map[int]string{
	1: "Outdoor",
	2: "Indoor",
	3: "Outdoor & Underground",
	4: "City",
}

// ZoneTypeName returns the display name for a spells_new.zonetype value.
// Returns the empty string for 0 (no restriction) and unknown values.
func ZoneTypeName(id int) string {
	return zoneTypes[id]
}

// ZoneTypesAudit validates that every distinct spells_new.zonetype seen
// in the DB is mapped above. 0 (no restriction) is excluded since it
// has no label and isn't meant to be displayed.
var ZoneTypesAudit = AuditDef{
	Name:       "Zone Type",
	KnownCodes: keysAsSet(zoneTypes),
	Extract: func(db *sql.DB) ([]int, error) {
		// -1 is treated as "no restriction" same as 0; only audit the
		// positive zone-type values that actually need labels.
		rows, err := db.Query(`SELECT DISTINCT zonetype FROM spells_new WHERE zonetype > 0`)
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

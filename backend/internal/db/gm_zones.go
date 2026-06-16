package db

import "log/slog"

// gmZoneShortNames are the GM/non-playable "home" zones whose merchants normal
// players can never reach. Sunset Home (cshome) is the EQ guide/GM zone; cshome2
// is its sibling instance (no long_name in the zone table). Items obtainable
// only from merchants here are GM items and must never be offered as upgrades.
// Extend this list if more GM-only zones surface.
var gmZoneShortNames = []string{"cshome", "cshome2"}

// IsGMZoneItem reports whether an item is obtainable only from GM-zone merchants
// — sold by a merchant spawned in a GM zone, never sold by any non-GM merchant,
// and never dropped by an NPC. Such items are GM-only and the upgrade finder
// must exclude them. Safe default: an item we can't classify is treated as
// obtainable (false).
func (db *DB) IsGMZoneItem(itemID int) bool {
	db.ensureGMZoneIndex()
	return db.gmZoneItems[itemID]
}

// ensureGMZoneIndex computes the GM-zone-only item set once. On any error it
// installs an empty set so a failure degrades to "exclude nothing" rather than
// hiding legitimate gear.
func (db *DB) ensureGMZoneIndex() {
	db.gmOnce.Do(func() {
		set, err := db.buildGMZoneItems()
		if err != nil {
			slog.Warn("db: failed to build GM-zone item index; GM gear will not be hidden", "err", err)
			db.gmZoneItems = map[int]bool{}
			return
		}
		db.gmZoneItems = set
	})
}

// buildGMZoneItems derives the GM-zone-only set: items a GM-zone merchant sells
// that are neither sold by any non-GM merchant nor dropped by any NPC. The
// merchant→npc→spawn→zone joins are all indexed, so this is a sub-second pass.
func (db *DB) buildGMZoneItems() (map[int]bool, error) {
	// Build the IN-list of GM zones for both the "in GM" and "not in GM" sides.
	in := ""
	args := make([]any, 0, len(gmZoneShortNames)*2)
	for i, z := range gmZoneShortNames {
		if i > 0 {
			in += ","
		}
		in += "?"
		args = append(args, z)
	}
	// The query repeats the GM-zone list twice (gm CTE, then nongm CTE), so the
	// args are appended a second time below.
	for _, z := range gmZoneShortNames {
		args = append(args, z)
	}

	q := `
WITH gm AS (
  SELECT DISTINCT ml.item AS item_id
  FROM merchantlist ml
  JOIN npc_types n   ON n.merchant_id = ml.merchantid
  JOIN spawnentry se ON se.npcID = n.id
  JOIN spawn2 s2     ON s2.spawngroupID = se.spawngroupID
  WHERE s2.zone IN (` + in + `) AND ml.item > 0
),
nongm AS (
  SELECT DISTINCT ml.item AS item_id
  FROM merchantlist ml
  JOIN npc_types n   ON n.merchant_id = ml.merchantid
  JOIN spawnentry se ON se.npcID = n.id
  JOIN spawn2 s2     ON s2.spawngroupID = se.spawngroupID
  WHERE s2.zone NOT IN (` + in + `) AND ml.item > 0
),
dropped AS (
  SELECT DISTINCT lde.item_id
  FROM lootdrop_entries lde
  JOIN loottable_entries lte ON lte.lootdrop_id = lde.lootdrop_id
  JOIN npc_types n           ON n.loottable_id = lte.loottable_id
  WHERE n.loottable_id <> 0
)
SELECT item_id FROM gm
WHERE item_id NOT IN (SELECT item_id FROM nongm)
  AND item_id NOT IN (SELECT item_id FROM dropped)`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		set[id] = true
	}
	return set, rows.Err()
}

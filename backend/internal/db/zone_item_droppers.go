package db

import (
	"fmt"
	"strings"
)

// ZoneDropper is an NPC placed in a zone whose loot table can drop a given
// item.
type ZoneDropper struct {
	ID         int
	Name       string
	RaidTarget int
}

// ZoneNPCsDroppingItem returns every NPC placed in zoneShort (via spawn2) whose
// loot table can drop an item named itemName (any case, any item id with that
// name, since loot tables don't always reference the canonical id).
func (db *DB) ZoneNPCsDroppingItem(zoneShort, itemName string) ([]ZoneDropper, error) {
	itemName = strings.TrimSpace(itemName)
	if zoneShort == "" || itemName == "" {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT n.id, n.name, n.raid_target
		FROM spawn2 s
		JOIN spawnentry se ON se.spawngroupID = s.spawngroupID
		JOIN npc_types n ON n.id = se.npcID
		JOIN loottable_entries lte ON lte.loottable_id = n.loottable_id
		JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		WHERE s.zone = ?
		  AND lde.item_id IN (SELECT id FROM items WHERE Name = ? COLLATE NOCASE)`,
		zoneShort, itemName)
	if err != nil {
		return nil, fmt.Errorf("zone item droppers: %w", err)
	}
	defer rows.Close()
	var out []ZoneDropper
	for rows.Next() {
		var d ZoneDropper
		if err := rows.Scan(&d.ID, &d.Name, &d.RaidTarget); err != nil {
			return nil, fmt.Errorf("zone item droppers: scan: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

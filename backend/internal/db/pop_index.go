package db

import "log/slog"

// popExpansion is the zoneCatalog bucket id for Planes of Power. Items whose
// only sources live in PoP zones are not yet obtainable on Project Quarm.
const popExpansion = 4

// IsPoPGated reports whether an item is Planes-of-Power-gated — i.e. not
// obtainable in the current (pre-PoP) era — so the upgrade finder can hide it
// while the pop_enabled flag is off. Safe default: an item we can't classify
// is treated as available (false).
func (db *DB) IsPoPGated(itemID int) bool {
	db.ensurePoPIndex()
	return db.popGated[itemID]
}

// WarmPoPIndex builds the PoP item index ahead of first use. Call it in a
// background goroutine at startup so the first upgrade request doesn't pay the
// one-time build cost (a few bulk source-join passes).
func (db *DB) WarmPoPIndex() { db.ensurePoPIndex() }

// ensurePoPIndex builds the PoP item set once. On failure it logs and installs
// an empty set, degrading to "hide nothing" rather than breaking queries.
func (db *DB) ensurePoPIndex() {
	db.popOnce.Do(func() {
		gated, err := db.buildPoPGated()
		if err != nil {
			slog.Warn("db: failed to build PoP item index; PoP gear will not be hidden", "err", err)
			db.popGated = map[int]bool{}
			return
		}
		db.popGated = gated
	})
}

// buildPoPGated computes the set of items that are PoP-only. The per-item /
// per-zone expansion columns in quarm.db are unreliable, so membership is
// derived from where each item actually comes from:
//
//   - explicitly tagged items (items.min_expansion >= 4), to catch PoP items
//     with no recorded source (e.g. quest rewards), and
//   - items whose every known source zone (drop, vendor, forage, ground spawn)
//     is a PoP zone per the curated zoneCatalog.
//
// An item with at least one pre-PoP source is kept (it's obtainable now).
func (db *DB) buildPoPGated() (map[int]bool, error) {
	gated := map[int]bool{}

	// Explicit tag: trust an item flagged as PoP era even when it has no source.
	if rows, err := db.Query(`SELECT id FROM items WHERE min_expansion >= ?`, popExpansion); err == nil {
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err == nil {
				gated[id] = true
			}
		}
		rows.Close()
	} else {
		return nil, err
	}

	// Per-item source-zone classification.
	hasPoP := map[int]bool{}
	hasNonPoP := map[int]bool{}
	classify := func(itemID int, zoneShort string) {
		if zoneCatalog[zoneShort] >= popExpansion {
			hasPoP[itemID] = true
		} else {
			// Zones absent from the catalog are treated as available (non-PoP)
			// so we never over-hide an item with an unknown source.
			hasNonPoP[itemID] = true
		}
	}

	// Each source query yields (item_id, zone_short_name).
	sourceQueries := []string{
		// NPC drops
		`SELECT lde.item_id, s2.zone
		   FROM loottable_entries lte
		   JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		   JOIN npc_types n ON n.loottable_id = lte.loottable_id
		   JOIN spawnentry se ON se.npcid = n.id
		   JOIN spawngroup sg ON sg.id = se.spawngroupid
		   JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		  WHERE n.loottable_id > 0`,
		// Vendor sales
		`SELECT ml.item, s2.zone
		   FROM merchantlist ml
		   JOIN npc_types n ON n.merchant_id = ml.merchantid
		   JOIN spawnentry se ON se.npcid = n.id
		   JOIN spawngroup sg ON sg.id = se.spawngroupid
		   JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		  WHERE n.merchant_id > 0`,
		// Forage
		`SELECT f.Itemid, z.short_name FROM forage f JOIN zone z ON z.zoneidnumber = f.zoneid`,
		// Ground spawns
		`SELECT g.item, z.short_name FROM ground_spawns g JOIN zone z ON z.zoneidnumber = g.zoneid`,
	}
	for _, q := range sourceQueries {
		rows, err := db.Query(q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var itemID int
			var zone string
			if err := rows.Scan(&itemID, &zone); err != nil {
				rows.Close()
				return nil, err
			}
			if itemID > 0 && zone != "" {
				classify(itemID, zone)
			}
		}
		rows.Close()
	}

	// Gate items whose every known source is a PoP zone.
	for id := range hasPoP {
		if !hasNonPoP[id] {
			gated[id] = true
		}
	}
	return gated, nil
}

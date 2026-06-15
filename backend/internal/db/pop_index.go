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
//   - items whose every known source zone (drop, vendor, forage, ground spawn,
//     or quest reward) is a PoP zone per the curated zoneCatalog. Drops are
//     read both from where the NPC spawns and — for NPCs with no spawn entry
//     (PoP mobs, which aren't spawned while PoP is locked) — from the NPC's
//     home zone derived from its id, so their drops aren't invisible.
//
// An item with at least one pre-PoP source is kept (it's obtainable now).
//
// Quest rewards are essential here: EQEmu scripts quests in Lua/Perl, not DB
// tables, so a quest-reward item has no drop/vendor/forage row and would
// otherwise be unclassifiable. Folding the quest-source zones in both keeps
// current-era quest gear visible (Sigil Earring of Veracity → Katta/Luclin)
// and correctly hides PoP-era quest rewards (Jade Hoop of Speed → Plane of
// Knowledge). See quest_sources.go.
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
		// NPC drops (from where the NPC actually spawns).
		`SELECT lde.item_id, s2.zone
		   FROM loottable_entries lte
		   JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		   JOIN npc_types n ON n.loottable_id = lte.loottable_id
		   JOIN spawnentry se ON se.npcid = n.id
		   JOIN spawngroup sg ON sg.id = se.spawngroupid
		   JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		  WHERE n.loottable_id > 0`,
		// NPC drops from NPCs that have NO spawn entry. PoP NPCs aren't spawned
		// while PoP is locked, so the spawn2 join above misses their drops
		// entirely and the items leak through. Fall back to the NPC's home zone
		// derived from its id: EQEmu numbers zone NPCs as zoneidnumber*1000+n
		// (holds for ~95% of spawned NPCs; only used here where there's no
		// spawn2 truth to contradict it).
		`SELECT lde.item_id, z.short_name
		   FROM loottable_entries lte
		   JOIN lootdrop_entries lde ON lde.lootdrop_id = lte.lootdrop_id
		   JOIN npc_types n ON n.loottable_id = lte.loottable_id
		   JOIN zone z ON z.zoneidnumber = n.id / 1000
		  WHERE n.loottable_id > 0 AND n.id >= 1000
		    AND NOT EXISTS (
		      SELECT 1 FROM spawnentry se
		      JOIN spawngroup sg ON sg.id = se.spawngroupid
		      JOIN spawn2 s2 ON s2.spawngroupID = sg.id
		      WHERE se.npcid = n.id
		    )`,
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

	// Quest-reward sources (from the embedded quest scripts, not the DB). An
	// item rewarded by a quest is obtainable wherever that quest's NPC lives.
	loadQuestSources()
	for itemID, zones := range questRewardZoneSet {
		for zone := range zones {
			classify(itemID, zone)
		}
	}

	// Gate items whose every known source is a PoP zone.
	for id := range hasPoP {
		if !hasNonPoP[id] {
			gated[id] = true
		}
	}
	return gated, nil
}

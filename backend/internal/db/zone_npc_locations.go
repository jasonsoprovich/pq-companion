package db

import (
	"fmt"
	"sort"
	"strings"
)

// ZoneNPCLocation is one NPC's spawn point in a zone, in map space.
//
// This exists for map search. The generated POI set only covers NPCs that are
// interesting for a *category* — vendors, raid targets — which leaves out the
// large majority of a zone's population. Searching the Bazaar for "Ward
// Pungill" found nothing, not because the NPC is missing from the database but
// because a guard is not a POI, and the map had no way to say "here is the one
// you asked for".
type ZoneNPCLocation struct {
	NPCID int    `json:"npc_id"`
	Name  string `json:"name"`
	// X and Y are map space (negated from spawn2), matching every other
	// coordinate the renderer receives, so search results and POIs can be drawn
	// through exactly the same path.
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
	// Level and RaidTarget let the search list say something useful about a hit
	// beyond its name, which matters when a zone has six similar guards.
	Level      int  `json:"level"`
	RaidTarget bool `json:"raid_target"`
}

// zoneNPCLocationLimit caps the result set.
//
// A cap rather than pagination because this is a search index, not a listing:
// it is fetched whole, once per zone, and held in memory for substring
// matching. The busiest zone in the database is comfortably inside this, so the
// limit is a guard against a pathological row rather than a real constraint.
const zoneNPCLocationLimit = 4000

// GetZoneNPCLocations returns every NPC spawn point in a zone.
//
// One row per spawn point, not per NPC: a wandering guard placed at four points
// is four answers to "where is he", and collapsing them to one would send the
// player to an arbitrary quarter of the zone.
func (db *DB) GetZoneNPCLocations(shortName string) ([]ZoneNPCLocation, error) {
	rows, err := db.Query(`
		SELECT DISTINCT n.id, n.name, s.x, s.y, s.z, n.level, n.raid_target
		FROM spawn2 s
		JOIN spawnentry se ON se.spawngroupID = s.spawngroupID
		JOIN npc_types n ON n.id = se.npcID
		WHERE s.zone = ?
		  -- '#' names are utility/script rows with no physical presence.
		  AND n.name NOT LIKE '#%'
		ORDER BY n.name
		LIMIT ?`, shortName, zoneNPCLocationLimit)
	if err != nil {
		return nil, fmt.Errorf("get zone npc locations %q: %w", shortName, err)
	}
	defer rows.Close()

	out := []ZoneNPCLocation{}
	for rows.Next() {
		var (
			l          ZoneNPCLocation
			x, y, z    float64
			raidTarget int
		)
		if err := rows.Scan(&l.NPCID, &l.Name, &x, &y, &z, &l.Level, &raidTarget); err != nil {
			return nil, fmt.Errorf("scan zone npc location: %w", err)
		}
		// The same negation the geometry pipeline and every POI applies.
		l.X, l.Y, l.Z = int(-x), int(-y), int(z)
		l.RaidTarget = raidTarget != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// RaidTargetInZone is one raid-target NPC known to spawn in a zone, keyed by
// its display name (the form the game and `/sll` print — see
// GetNPCIDByName) so it can be matched directly against lockout entries.
type RaidTargetInZone struct {
	NPCID int    `json:"npc_id"`
	Name  string `json:"name"`
}

// RaidTargetOverrides force-includes npc_types ids as raid-target bosses for
// a zone even though they fail the normal spawn2+raid_target=1+race!=127
// query in GetRaidTargetsByZone — either the encounter is spawned purely by
// quest script with no spawn2 row at all (see ScriptSpawnedNPCOverrides), or
// raid_target is 0 on the row that actually carries the loot table (both
// confirmed gaps in the upstream Quarm dump — query-side correction, see
// project_quarm_data_corrections memory), or the only raid_target=1 row
// sharing the name is a race=127/bodytype=65 untargetable decoy (see the
// GetNPCVariantsByNameInZone doc comment) that the base query's race filter
// now excludes outright, leaving the real encounter unreachable without an
// override. Keyed by zone short_name. Confirm future additions the same way
// these were found: the boss's real target_name appears in a captured
// `/sll` snapshot (lockout_entries) but GetRaidTargetsByZone doesn't surface
// it — or, for the race=127 shape, both rows exist with a shared name but
// opposite raid_target/loottable_id/race signatures.
//
// ssratemple, all confirmed against real captured `/sll` data (character
// Osui) or the documented decoy pattern for this exact zone:
//   - Emperor Ssraeshza: real row 162491 "Emperor_Ssraeshza_" has
//     raid_target=0 and no spawn2 row; decoy 162065 "#Emperor_Ssraeshza" has
//     spawn2 but loottable_id=0.
//   - Blood of Ssraeshza: real row 162189 "#Blood_of_Ssraeshza" (loottable
//     12840) has raid_target=0, no other row shares the name.
//   - Vyzh`dra the Exiled (162039) and Vyzh`dra the Cursed (162042): both
//     raid_target=0 with no spawn2 row anywhere (script-spawned, no decoy).
//   - a glyph covered serpent (162037): same shape as the Vyzh`dra pair.
//   - Rhag`Mozdezh (162192), Rhag`Zhezum (162178), Arch Lich Rhag`Zadune
//     (162030): each has a same-named race=127/bodytype=65 decoy
//     (162264/162241/162244, ~20k HP, loottable_id=0) that carries
//     raid_target=1 while the real encounter (race 217, 200k–825k HP, a
//     real loottable_id) has raid_target=0 — the exact pattern already
//     documented on GetNPCVariantsByNameInZone for the overlay's lookup
//     path, not previously needed here since this query didn't exist yet.
//
// akheva and griegsend: NOT /sll-confirmed (no character has captured a
// lockout there) — added on structural confidence alone from a game-wide
// sweep for the same shape as the ssratemple script-spawned cases, since a
// wrong id here only costs a mismatched NPC detail-page link (lockout
// status matching is by name, not id) rather than an incorrect lockout
// status. Re-derive/adjust if a real `/sll` capture ever disagrees.
//   - Shei Vinitras (akheva): real row 179017 "Shei_Vinitras_" (loottable
//     13262, 650k HP) has raid_target=0 and no spawn2 row; decoy 179032
//     "#Shei_Vinitras" has spawn2 in akheva and raid_target=1 but
//     loottable_id=0 — a near-exact mirror of the Emperor Ssraeshza shape.
//   - Grieg Veneficus (griegsend): weaker match. "#Grieg_Veneficus" (163156,
//     325k HP) has the spawn2 point and raid_target=1 but loottable_id=0;
//     the only loot-bearing row, 163389 "Grieg_Veneficus" (loottable
//     91472), is HP 200 / race 1 — reads like a scripted "loot corpse" NPC
//     rather than the actual combat model, so it's less certain this is the
//     id `/sll` would reference, but it's the best candidate available.
var RaidTargetOverrides = map[string][]int{
	"ssratemple": {
		162491, // Emperor Ssraeshza
		162189, // Blood of Ssraeshza
		162039, // Vyzh`dra the Exiled
		162042, // Vyzh`dra the Cursed
		162037, // a glyph covered serpent
		162192, // Rhag`Mozdezh
		162178, // Rhag`Zhezum
		162030, // Arch Lich Rhag`Zadune
	},
	"akheva":    {179017}, // Shei Vinitras
	"griegsend": {163389}, // Grieg Veneficus
}

// stripNPCDecoration reverses the placeholder decoration NPCNameVariantCandidates
// adds when going display-name → db-name: strips at most one matching leading
// PlaceholderPrefixes entry and any trailing PlaceholderSuffixes entries, so a
// raw npc_types name like "#Emperor_Ssraeshza_" or "Emperor_Ssraeshza_"
// reduces to the same bare form before the underscore→space render.
func stripNPCDecoration(name string) string {
	for _, p := range PlaceholderPrefixes {
		if strings.HasPrefix(name, p) {
			name = strings.TrimPrefix(name, p)
			break
		}
	}
	for _, s := range PlaceholderSuffixes {
		name = strings.TrimSuffix(name, s)
	}
	return name
}

// GetRaidTargetsByZone returns every distinct raid-target NPC (raid_target =
// 1, plus RaidTargetOverrides) with at least one spawn point in the zone (or
// forced in via RaidTargetOverrides), ordered by name. Same-named variants
// collapse to one row keyed by their lowest npc_types id, mirroring
// GetNPCIDByName's convention so the id links to the same detail page a
// lockout row's resolver would land on. Used by the Zones tab's Lockouts
// sub-tab to know which bosses in a zone to check a character's lockout
// status against.
func (db *DB) GetRaidTargetsByZone(shortName string) ([]RaidTargetInZone, error) {
	rows, err := db.Query(`
		SELECT MIN(n.id), n.name
		FROM spawn2 s
		JOIN spawnentry se ON se.spawngroupID = s.spawngroupID
		JOIN npc_types n ON n.id = se.npcID
		WHERE s.zone = ? AND n.raid_target = 1
		  -- A '#' name prefix does NOT reliably mean "decoy/utility row" for
		  -- raid encounters — Lord Inquisitor Seru's only row is
		  -- "#Lord_Inquisitor_Seru" and it's the real, lootable boss. What
		  -- actually distinguishes a lockout-capable encounter from a
		  -- combat-only add sharing its raid_target flag (e.g. ssratemple's
		  -- "#Ssraeshzian_Blood_Golem", raid_target=1 but loottable_id=0) is
		  -- whether it has a loot table at all: /sll lockouts are inherently
		  -- loot-driven, so a boss with none can never produce one.
		  AND n.loottable_id != 0
		  -- race 127 / bodytype 65 is Quarm's untargetable-decoy convention;
		  -- several raid bosses have a same-named decoy row that wrongly
		  -- carries raid_target=1 while the real encounter doesn't (see
		  -- RaidTargetOverrides) — exclude it so the decoy never wins by
		  -- default instead of surfacing as a gap to override.
		  AND n.race != 127
		GROUP BY n.name
		ORDER BY n.name`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get raid targets by zone %q: %w", shortName, err)
	}
	defer rows.Close()

	out := []RaidTargetInZone{}
	seen := map[int]bool{}
	for rows.Next() {
		var t RaidTargetInZone
		if err := rows.Scan(&t.NPCID, &t.Name); err != nil {
			return nil, fmt.Errorf("scan raid target in zone: %w", err)
		}
		t.Name = strings.TrimSpace(strings.ReplaceAll(stripNPCDecoration(t.Name), "_", " "))
		out = append(out, t)
		seen[t.NPCID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, id := range RaidTargetOverrides[shortName] {
		if seen[id] {
			continue
		}
		var name string
		if err := db.QueryRow(`SELECT name FROM npc_types WHERE id = ?`, id).Scan(&name); err != nil {
			continue
		}
		out = append(out, RaidTargetInZone{
			NPCID: id,
			Name:  strings.TrimSpace(strings.ReplaceAll(stripNPCDecoration(name), "_", " ")),
		})
		seen[id] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

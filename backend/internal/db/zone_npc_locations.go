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
// a zone even though they fail the normal spawn2+raid_target=1 query in
// GetRaidTargetsByZone — either the encounter is spawned purely by quest
// script with no spawn2 row at all (see ScriptSpawnedNPCOverrides), or
// raid_target is 0 on the row that actually carries the loot table, both
// confirmed gaps in the upstream Quarm dump rather than something to
// hand-edit in quarm.db (query-side correction — see
// project_quarm_data_corrections memory). Keyed by zone short_name. Confirm
// future additions the same way these two were found: the boss's real
// target_name appears in a captured `/sll` snapshot (lockout_entries) but
// GetRaidTargetsByZone doesn't surface it.
//
// ssratemple: Emperor Ssraeshza's real encounter row (162491,
// "Emperor_Ssraeshza_") has raid_target=0 and no spawn2 row (see
// ScriptSpawnedNPCOverrides, npc 162065 "#Emperor_Ssraeshza" is a decoy).
// Blood of Ssraeshza's real row (162189, "#Blood_of_Ssraeshza", loottable
// 12840) also has raid_target=0.
var RaidTargetOverrides = map[string][]int{
	"ssratemple": {162491, 162189},
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
		  -- '#' names are utility/script rows with no physical presence.
		  AND n.name NOT LIKE '#%'
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

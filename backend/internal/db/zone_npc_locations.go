package db

import "fmt"

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

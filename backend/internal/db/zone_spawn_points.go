package db

import "fmt"

// defaultAggroRadius is the aggro range the server gives an NPC whose
// aggroradius is 0 (EQMacEmu Mob::AI_Init).
const defaultAggroRadius = 70

// ZoneSpawnPoint is one spawn2 point in a zone with the NPCs it can roll, for
// the map's spawn and aggro-ring layer.
type ZoneSpawnPoint struct {
	SpawnID int `json:"spawn_id"`
	// X, Y and Z are map space (negated from spawn2), like every other
	// coordinate the renderer receives.
	X    int             `json:"x"`
	Y    int             `json:"y"`
	Z    int             `json:"z"`
	NPCs []SpawnPointNPC `json:"npcs"`
}

// SpawnPointNPC is one NPC a spawn point can roll.
type SpawnPointNPC struct {
	NPCID int    `json:"npc_id"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	// MaxLevel is the top of the NPC's spawn level range (0 = fixed level).
	MaxLevel int `json:"max_level,omitempty"`
	// AggroRadius is the effective aggro range: npc_types.aggroradius, or
	// the server's default when that is 0.
	AggroRadius int `json:"aggro_radius"`
	// SeeInvis and SeeInvisUndead mirror npc_types: 0 = no, 1 = always, and
	// a higher value is the percent chance, rolled at spawn, that it does.
	SeeInvis       int  `json:"see_invis"`
	SeeInvisUndead int  `json:"see_invis_undead"`
	Undead         bool `json:"undead"`
	AggroPC        bool `json:"aggro_pc"`
	// Pct is the NPC's share of the spawn group's roll.
	Pct float64 `json:"pct"`
}

// bodyTypeUndead is EQMacEmu BodyType::Undead.
const bodyTypeUndead = 3

// GetZoneSpawnPoints returns every spawn point active in the current era,
// each with the NPCs it can roll (chance > 0). Placeholder and Invisible Man
// rows are left out (nonPlayerNPCClause): they are script anchors and
// decoys, not mobs a player meets.
func (db *DB) GetZoneSpawnPoints(shortName string) ([]ZoneSpawnPoint, error) {
	rows, err := db.Query(`
		WITH pts AS (
			SELECT s2.id, s2.spawngroupID, s2.x, s2.y, s2.z
			FROM spawn2 s2
			WHERE s2.zone = ? AND `+activeContentClause("s2")+`
		),
		ents AS (
			SELECT se.spawngroupID, se.npcID, se.chance
			FROM spawnentry se
			WHERE se.spawngroupID IN (SELECT spawngroupID FROM pts)
			  AND se.chance > 0
			  AND `+activeContentClause("se")+`
		),
		tot AS (
			SELECT spawngroupID, SUM(chance) AS t FROM ents GROUP BY spawngroupID
		)
		SELECT p.id, p.x, p.y, p.z,
		       n.id, n.name, n.level, n.maxlevel, n.aggroradius,
		       n.see_invis, n.see_invis_undead, n.bodytype, n.aggro_pc,
		       e.chance * 100.0 / tot.t
		FROM pts p
		JOIN ents e ON e.spawngroupID = p.spawngroupID
		JOIN tot ON tot.spawngroupID = p.spawngroupID
		JOIN npc_types n ON n.id = e.npcID
		WHERE 1 = 1`+nonPlayerNPCClause+`
		ORDER BY p.id, e.chance DESC, n.id`, shortName)
	if err != nil {
		return nil, fmt.Errorf("get zone spawn points %q: %w", shortName, err)
	}
	defer rows.Close()

	out := []ZoneSpawnPoint{}
	for rows.Next() {
		var (
			id                int
			x, y, z           float64
			n                 SpawnPointNPC
			bodyType, aggroPC int
		)
		if err := rows.Scan(&id, &x, &y, &z,
			&n.NPCID, &n.Name, &n.Level, &n.MaxLevel, &n.AggroRadius,
			&n.SeeInvis, &n.SeeInvisUndead, &bodyType, &aggroPC, &n.Pct); err != nil {
			return nil, fmt.Errorf("scan zone spawn point: %w", err)
		}
		if n.AggroRadius <= 0 {
			n.AggroRadius = defaultAggroRadius
		}
		n.Undead = bodyType == bodyTypeUndead
		n.AggroPC = aggroPC != 0
		if len(out) == 0 || out[len(out)-1].SpawnID != id {
			// The same negation the geometry pipeline and every POI applies.
			out = append(out, ZoneSpawnPoint{SpawnID: id, X: int(-x), Y: int(-y), Z: int(z)})
		}
		p := &out[len(out)-1]
		p.NPCs = append(p.NPCs, n)
	}
	return out, rows.Err()
}

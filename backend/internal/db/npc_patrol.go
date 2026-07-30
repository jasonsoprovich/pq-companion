package db

import "fmt"

// PatrolRoute is one NPC spawn point's waypoint path.
//
// Server data, not client geometry: waypoints live in the emulator's grid tables
// and are not in any .s3d, which is why no static map pack has them. That makes
// this something we can show and a downloaded map set cannot.
type PatrolRoute struct {
	Zone   string        `json:"zone"`
	GridID int           `json:"grid_id"`
	Points []PatrolPoint `json:"points"`
	// Ordered reports whether the waypoints are walked in sequence.
	//
	// Load-bearing for how this is drawn. Most grids are ordered — 18,504 of
	// 22,824 are type 3 (patrol) and 2,469 are type 0 (circular) — but ~1,349
	// are random types where the NPC picks a waypoint at will. Joining those
	// with a line would draw a route it never walks, stated as confidently as a
	// real one, so they are shown as loose points instead.
	Ordered bool `json:"ordered"`
	// Closed reports whether the path loops back to its start. Circular grids do;
	// patrol grids walk to the end and reverse, and one-way grids do not return
	// at all, so closing those would invent a leg.
	Closed bool `json:"closed"`
}

// PatrolPoint is one waypoint, in map space so the renderer needs no transform.
type PatrolPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
	// Pause is seconds the NPC waits here. Surfaced because a long pause is
	// where a roamer is actually findable.
	Pause int `json:"pause"`
}

// GetNPCPatrols returns every patrol route for an NPC.
//
// One route per spawn point, not one per NPC: the same mob can be placed at
// several points with different grids, and collapsing them would draw a path it
// never walks.
// EQEmu grid types. Only the ones whose ordering we rely on are named.
const (
	gridCircular    = 0 // walk the waypoints in order, then loop to the start
	gridPatrol      = 3 // walk to the end, then reverse back along the same path
	gridOneWayRepop = 4
	gridOneWayDepop = 6
)

// gridTypeOrdered reports whether a grid's waypoints are walked in sequence.
//
// The random types (1, 2, 5, 8, 9) pick a destination rather than following the
// list, so their waypoint order carries no meaning — drawing a line through it
// would be an invention.
func gridTypeOrdered(t int) bool {
	switch t {
	case gridCircular, gridPatrol, gridOneWayRepop, gridOneWayDepop:
		return true
	default:
		return false
	}
}

func (db *DB) GetNPCPatrols(npcID int) ([]PatrolRoute, error) {
	rows, err := db.Query(`
		SELECT DISTINCT s.zone, s.pathgrid, z.zoneidnumber, COALESCE(g.type, 3)
		FROM spawnentry se
		JOIN spawn2 s ON s.spawngroupID = se.spawngroupID
		JOIN zone z ON z.short_name = s.zone
		LEFT JOIN grid g ON g.id = s.pathgrid AND g.zoneid = z.zoneidnumber
		WHERE se.npcID = ? AND s.pathgrid > 0`, npcID)
	if err != nil {
		return nil, fmt.Errorf("list patrol grids for %d: %w", npcID, err)
	}
	defer rows.Close()

	type gridRef struct {
		zone     string
		grid     int
		zoneID   int
		gridType int
	}
	var refs []gridRef
	for rows.Next() {
		var g gridRef
		if err := rows.Scan(&g.zone, &g.grid, &g.zoneID, &g.gridType); err != nil {
			return nil, fmt.Errorf("scan patrol grid: %w", err)
		}
		refs = append(refs, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []PatrolRoute{}
	for _, g := range refs {
		wps, err := db.Query(`
			SELECT x, y, z, pause FROM grid_entries
			WHERE gridid = ? AND zoneid = ? ORDER BY number`, g.grid, g.zoneID)
		if err != nil {
			return nil, fmt.Errorf("read grid %d: %w", g.grid, err)
		}
		route := PatrolRoute{
			Zone: g.zone, GridID: g.grid, Points: []PatrolPoint{},
			Ordered: gridTypeOrdered(g.gridType),
			// Only circular grids return to their start. A patrol grid reverses
			// along the same waypoints, and a one-way grid ends where it ends.
			Closed: g.gridType == gridCircular,
		}
		for wps.Next() {
			var x, y, z float64
			var pause int
			if err := wps.Scan(&x, &y, &z, &pause); err != nil {
				wps.Close()
				return nil, fmt.Errorf("scan waypoint: %w", err)
			}
			// Same negation as every other coordinate on the map side.
			route.Points = append(route.Points, PatrolPoint{
				X: int(-x), Y: int(-y), Z: int(z), Pause: pause,
			})
		}
		wps.Close()
		// A grid with one point is a facing marker, not a patrol.
		if len(route.Points) > 1 {
			out = append(out, route)
		}
	}
	return out, nil
}

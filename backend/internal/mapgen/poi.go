package mapgen

import (
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
)

// POI categories. These become the renderer's layer toggles, so they are named
// for what a player is looking for rather than for the table they came from.
const (
	CategoryZoneLine    = "zone_line"
	CategoryGroundSpawn = "ground_spawn"
	CategoryTrap        = "trap"
	CategoryTradeskill  = "tradeskill"
	CategorySuccor      = "succor"
	CategoryVendor      = "vendor"
	CategoryRaidTarget  = "raid_target"
	CategoryDoor        = "door"
)

// POI is one labelled point on a zone map.
type POI struct {
	Zone     string
	X, Y, Z  int
	Category string
	Label    string
	// Source records where the row came from, so a quarm.db data release can
	// regenerate the db:* rows without destroying hand-researched or
	// community-contributed ones. See docs/maps-feasibility.md 7.3.
	Source string
	// RefID points back into the originating table where one exists, so the UI
	// can deep-link (an NPC id, an item id).
	RefID int
}

// coordSentinel marks "leave this coordinate unchanged" in zone_points. It is a
// control value, not a position, and lands ~999,999 units off the map if drawn.
const coordSentinel = 999999

// GeneratePOIs builds every POI derivable from quarm.db.
//
// Everything here is source="db:<table>" and is rebuilt from scratch on each
// run. Rows sourced from hand research or the community live alongside these
// with a different source and must survive regeneration untouched.
func GeneratePOIs(qdb *sql.DB) ([]POI, error) {
	zoneByID, err := zoneShortNames(qdb)
	if err != nil {
		return nil, err
	}

	var all []POI
	for _, gen := range []struct {
		name string
		fn   func(*sql.DB, map[int]string) ([]POI, error)
	}{
		{"zone lines", poiZoneLines},
		{"ground spawns", poiGroundSpawns},
		{"traps", poiTraps},
		{"tradeskill containers", poiTradeskillObjects},
		{"succor points", poiSuccor},
		{"vendors", poiVendors},
		{"raid targets", poiRaidTargets},
		{"doors", poiDoors},
	} {
		got, err := gen.fn(qdb, zoneByID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", gen.name, err)
		}
		all = append(all, got...)
	}
	return all, nil
}

func zoneShortNames(qdb *sql.DB) (map[int]string, error) {
	rows, err := qdb.Query(`SELECT zoneidnumber, short_name FROM zone`)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// toMapPoint converts game coordinates to map space, matching the geometry
// transform (map_f1 = -game_x, map_f2 = -game_y). Reports ok=false for the
// zone_points sentinel and for unset 0,0 rows.
func toMapPoint(gx, gy, gz float64) (x, y, z int, ok bool) {
	if math.Abs(gx) >= coordSentinel || math.Abs(gy) >= coordSentinel {
		return 0, 0, 0, false
	}
	if gx == 0 && gy == 0 {
		return 0, 0, 0, false
	}
	return int(math.Round(-gx)), int(math.Round(-gy)), int(math.Round(gz)), true
}

// cleanName turns a DB name into a label: underscores to spaces, and the
// leading '#' that marks a script-spawned NPC stripped.
func cleanName(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.TrimPrefix(s, "#"), "_", " "))
}

func poiZoneLines(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	// zone_points.target_zone_id names the destination; join to label the pin
	// with where it goes, which is the only thing a player wants from it.
	rows, err := qdb.Query(`
		SELECT zp.zone, zp.x, zp.y, zp.z, COALESCE(z.long_name, z.short_name, ''), zp.target_zone_id
		FROM zone_points zp
		LEFT JOIN zone z ON z.zoneidnumber = zp.target_zone_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zone, target string
		var gx, gy, gz float64
		var targetID int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &target, &targetID); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}
		label := "to " + target
		if target == "" {
			label = "zone line"
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategoryZoneLine, Label: label, Source: "db:zone_points", RefID: targetID})
	}
	return out, rows.Err()
}

func poiGroundSpawns(qdb *sql.DB, zoneByID map[int]string) ([]POI, error) {
	rows, err := qdb.Query(`
		SELECT gs.zoneid, gs.max_x, gs.max_y, gs.max_z, COALESCE(i.Name, gs.name), gs.item
		FROM ground_spawns gs
		LEFT JOIN items i ON i.id = gs.item`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zoneID, itemID int
		var gx, gy, gz float64
		var name string
		if err := rows.Scan(&zoneID, &gx, &gy, &gz, &name, &itemID); err != nil {
			return nil, err
		}
		zone, known := zoneByID[zoneID]
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || !known {
			continue
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategoryGroundSpawn, Label: cleanName(name),
			Source: "db:ground_spawns", RefID: itemID})
	}
	return out, rows.Err()
}

func poiTraps(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	// The traps table is the server's own definitions — more authoritative than
	// the hand-placed trap markers on existing community maps.
	rows, err := qdb.Query(`SELECT zone, x, y, z, COALESCE(message, ''), id FROM traps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zone, msg string
		var gx, gy, gz float64
		var id int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &msg, &id); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}
		label := "Trap"
		if msg != "" {
			label = "Trap: " + msg
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategoryTrap, Label: label, Source: "db:traps", RefID: id})
	}
	return out, rows.Err()
}

func poiTradeskillObjects(qdb *sql.DB, zoneByID map[int]string) ([]POI, error) {
	rows, err := qdb.Query(`SELECT zoneid, xpos, ypos, zpos, type FROM object`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zoneID, typ int
		var gx, gy, gz float64
		if err := rows.Scan(&zoneID, &gx, &gy, &gz, &typ); err != nil {
			return nil, err
		}
		zone, known := zoneByID[zoneID]
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || !known {
			continue
		}
		// Reuse the existing bagtype enum rather than a second copy that could
		// drift from it.
		name := enums.ContainerTypeName(typ)
		if name == "" {
			continue // not a tradeskill container
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategoryTradeskill, Label: name, Source: "db:object", RefID: typ})
	}
	return out, rows.Err()
}

func poiSuccor(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	rows, err := qdb.Query(`SELECT short_name, safe_x, safe_y, safe_z FROM zone`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zone string
		var gx, gy, gz float64
		if err := rows.Scan(&zone, &gx, &gy, &gz); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategorySuccor, Label: "Succor", Source: "db:zone"})
	}
	return out, rows.Err()
}

func poiVendors(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	// One pin per spawn point, not per NPC: a vendor spawning in three places
	// should show three pins. DISTINCT collapses the spawngroup fan-out that
	// would otherwise stack duplicates on one spot.
	rows, err := qdb.Query(`
		SELECT DISTINCT s2.zone, s2.x, s2.y, s2.z, n.name, n.id
		FROM spawn2 s2
		JOIN spawnentry se ON se.spawngroupID = s2.spawngroupID
		JOIN npc_types n   ON n.id = se.npcID
		WHERE n.merchant_id > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNPCPOIs(rows, CategoryVendor, "db:spawn2")
}

func poiRaidTargets(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	// Deliberately narrower than "named NPCs": that heuristic matches 9,018
	// NPCs, most of them guards and vendors, which would bury the map. The
	// raid_target flag is the server's own designation.
	rows, err := qdb.Query(`
		SELECT DISTINCT s2.zone, s2.x, s2.y, s2.z, n.name, n.id
		FROM spawn2 s2
		JOIN spawnentry se ON se.spawngroupID = s2.spawngroupID
		JOIN npc_types n   ON n.id = se.npcID
		WHERE n.raid_target = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNPCPOIs(rows, CategoryRaidTarget, "db:spawn2")
}

func scanNPCPOIs(rows *sql.Rows, category, source string) ([]POI, error) {
	var out []POI
	for rows.Next() {
		var zone, name string
		var gx, gy, gz float64
		var npcID int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &name, &npcID); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: category, Label: cleanName(name), Source: source, RefID: npcID})
	}
	return out, rows.Err()
}

func poiDoors(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	// Only doors worth pinning: ones that need a key, or that lead somewhere.
	// All 8,205 doors would be noise — most are ordinary interior doors a
	// player walks through without thinking.
	rows, err := qdb.Query(`
		SELECT zone, pos_x, pos_y, pos_z, COALESCE(dest_zone, ''), keyitem, id
		FROM doors
		WHERE keyitem > 0
		   OR (dest_zone IS NOT NULL AND dest_zone != '' AND dest_zone != 'NONE')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zone, dest string
		var gx, gy, gz float64
		var keyItem, id int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &dest, &keyItem, &id); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}
		label := "Door"
		switch {
		case keyItem > 0 && dest != "" && dest != "NONE":
			label = "Locked door to " + dest
		case keyItem > 0:
			label = "Locked door"
		case dest != "" && dest != "NONE":
			label = "Door to " + dest
		}
		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: CategoryDoor, Label: label, Source: "db:doors", RefID: id})
	}
	return out, rows.Err()
}

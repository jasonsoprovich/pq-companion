package mapgen

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
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
	// CategoryLocked is a door that needs a key or a lockpick. Split from
	// CategoryDoor because it answers a different question — not "where can I
	// go" but "what do I need to get in", which is the thing worth a layer of
	// its own and worth naming the actual key for.
	CategoryLocked = "locked"
	// CategorySwitch is something to operate: a lever, a plate, an elevator.
	CategorySwitch = "switch"
	// CategoryTeleport is a door whose destination is inside the same zone —
	// the unmarked ports that make zones like Sanctus Seru and Plane of Justice
	// navigable, and which no static map set records.
	CategoryTeleport = "teleport"
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
		{"trap NPCs", poiTrapNPCs},
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

// switchNamePattern matches door names that are controls rather than doors.
//
// Needed because `triggerdoor` is not the signal it looks like. 667 doors carry
// it, but it only means "opening this opens that", and the overwhelming majority
// are ordinary double doors wired to their own other half — Paineel's PADOOR101
// and PADOOR102 account for 372 rows between them. Pinning those would bury the
// ~15 genuine levers and plates in door-pair noise. Names are crude but they are
// the only thing that separates a lever from a hinge here.
const switchNamePattern = `(
	   upper(d.name) LIKE '%SWITCH%' OR upper(d.name) LIKE '%SWTCH%'
	OR upper(d.name) LIKE '%LEVER%'  OR upper(d.name) LIKE '%LEVR%'
	OR upper(d.name) LIKE '%PULL%'   OR upper(d.name) LIKE '%CHAIN%'
	OR upper(d.name) LIKE '%BUTTON%' OR upper(d.name) LIKE '%PLATE%'
	OR upper(d.name) LIKE '%HANDLE%' OR upper(d.name) LIKE '%CRANK%'
	OR upper(d.name) LIKE '%WHEEL%'
)`

// poiDoors turns the doors table into annotations.
//
// Of 8,205 doors, most are ordinary interior doors a player walks through
// without thinking, and pinning them all would be pure noise. What is worth a
// pin is a door that stops you or moves you: one that needs a key or a pick, one
// that leads somewhere, one that is really a lever or a lift.
//
// This is the derive-first half of the annotation layer (§10 phase 4a). The same
// facts that hand-drawn maps mark by hand — locked doors, hidden ports, levers —
// are sitting in the server's own data, and derived beats hand-entered: it is
// verifiable, it regenerates with a data release, and it carries detail an
// annotation cannot. A hand-marked map says "locked"; this says which key, and
// links to it.
func poiDoors(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	rows, err := qdb.Query(`
		SELECT d.zone, d.pos_x, d.pos_y, d.pos_z,
		       COALESCE(d.dest_zone, ''), d.dest_x, d.dest_y, d.dest_z,
		       d.keyitem, d.altkeyitem, d.lockpick, d.islift,
		       COALESCE(k.Name, ''), COALESCE(a.Name, ''), d.id
		FROM doors d
		LEFT JOIN items k ON k.id = d.keyitem
		LEFT JOIN items a ON a.id = d.altkeyitem
		WHERE d.keyitem > 0
		   OR d.altkeyitem > 0
		   OR d.lockpick > 0
		   OR d.islift = 1
		   OR (d.dest_zone IS NOT NULL AND d.dest_zone != '' AND d.dest_zone != 'NONE')
		   OR (d.triggerdoor > 0 AND ` + switchNamePattern + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []POI
	for rows.Next() {
		var zone, dest, keyName, altKeyName string
		var gx, gy, gz, dx, dy, dz float64
		var keyItem, altKeyItem, lockpick, islift, id int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &dest, &dx, &dy, &dz,
			&keyItem, &altKeyItem, &lockpick, &islift,
			&keyName, &altKeyName, &id); err != nil {
			return nil, err
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok || zone == "" {
			continue
		}

		// A destination equal to the zone itself is an in-zone port, not an exit.
		// Guarded on a non-zero destination because plenty of rows name their own
		// zone with no coordinates, which means nothing.
		inZonePort := dest == zone && (dx != 0 || dy != 0 || dz != 0)
		leadsAway := dest != "" && dest != "NONE" && !inZonePort

		// Most specific wins. A locked door to another zone is filed under what
		// blocks you, since that is the part you have to solve.
		var category, label, source string
		var refID int
		switch {
		case keyItem > 0 || altKeyItem > 0:
			category, source = CategoryLocked, "db:doors-locked"
			label = "Locked"
			if leadsAway {
				label = "Locked door to " + dest
			}
			switch {
			case keyName != "" && altKeyName != "":
				label += " — needs " + keyName + " or " + altKeyName
			case keyName != "":
				label += " — needs " + keyName
			case altKeyName != "":
				label += " — needs " + altKeyName
			}
			// Point at the key, not the door: the useful next click from "what do
			// I need to get in" is the key's own page.
			refID = keyItem
			if refID == 0 {
				refID = altKeyItem
			}
		case lockpick > 0:
			category, source = CategoryLocked, "db:doors-locked"
			label = fmt.Sprintf("Pickable — lockpicking %d", lockpick)
			if leadsAway {
				label = fmt.Sprintf("Pickable door to %s — lockpicking %d", dest, lockpick)
			}
		case islift == 1:
			category, source = CategorySwitch, "db:doors-switch"
			label = "Elevator"
		case inZonePort:
			category, source = CategoryTeleport, "db:doors-teleport"
			label = "Teleport within zone"
		case leadsAway:
			category, source = CategoryDoor, "db:doors"
			label = "Door to " + dest
		default:
			// Reached only via the switch-name clause.
			category, source = CategorySwitch, "db:doors-switch"
			label = "Switch or lever"
		}

		out = append(out, POI{Zone: zone, X: x, Y: y, Z: z,
			Category: category, Label: label, Source: source, RefID: refID})
	}
	return out, rows.Err()
}

// ── Trap NPCs ─────────────────────────────────────────────────────────────────

// Many zones implement traps as invisible NPCs rather than rows in the traps
// table. Grokii's Zeal-maps write-up is entirely about this: Maiden's Eye traps
// spawn as "The ground" (npc 173034), and Akheva Ruins has 318 trap spawns —
// "You_trip", "Shadows", "You_can_feel", "The_stones" — while the traps table
// holds none for either zone. Generating from that table alone left the two
// most trap-heavy zones in the game showing nothing.
//
// There is no clean structural signal. The invisible race (127) with "immune to
// harm from client" also covers script utilities and invisible decoy copies of
// real mobs, and the abilities that would exclude those (immune to melee/magic)
// also exclude The_ground itself. So this is a prefilter plus a name denylist,
// deliberately conservative, and tagged with its own source so a later
// correction can replace exactly these rows.
const trapNPCRace = 127

// utilityNamePatterns mark script plumbing rather than a trap a player can
// trigger: spawn controllers, counters, encounter state machines.
var utilityNamePatterns = []string{
	"spawner", "spawn_", "trigger", "placeholder", "counter", "listener",
	"stopper", "marker", "timer", "controller", "invis", "_test", "dummy",
	"beacon", "actor", "anim", "raid", "encounter", "event",
}

func looksLikeUtility(name string) bool {
	l := strings.ToLower(name)
	for _, p := range utilityNamePatterns {
		if strings.Contains(l, p) {
			return true
		}
	}
	// Trailing underscore marks a script-spawned override variant, not a
	// distinct trap.
	if strings.HasSuffix(name, "_") {
		return true
	}
	// Programmer-style CamelCase with no underscores: real EQ names are
	// underscore-separated ("A_musty_smell"), so "AMTrigger" or "AnimMan" is
	// plumbing.
	if !strings.Contains(name, "_") {
		caps := 0
		for _, r := range name[1:] {
			if r >= 'A' && r <= 'Z' {
				caps++
			}
		}
		if caps > 0 {
			return true
		}
	}
	return false
}

func poiTrapNPCs(qdb *sql.DB, _ map[int]string) ([]POI, error) {
	rows, err := qdb.Query(`
		SELECT DISTINCT s2.zone, s2.x, s2.y, s2.z, n.name, n.id
		FROM spawn2 s2
		JOIN spawnentry se ON se.spawngroupID = s2.spawngroupID
		JOIN npc_types n   ON n.id = se.npcID
		WHERE n.race = ?
		  AND n.name NOT LIKE '#%'
		  AND ('^' || n.special_abilities || '^') LIKE '%^35,%'
		  -- Decoys share a name with a real, visible NPC; traps do not.
		  AND NOT EXISTS (
		        SELECT 1 FROM npc_types v WHERE v.race <> ? AND v.name = n.name)`,
		trapNPCRace, trapNPCRace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collapse by position. A trap spawn point is one spawngroup that rolls
	// between several flavours — akheva's group 179148 spawns A_rock (33%),
	// Shadows (34%) or The_stones (33%) at one spot. Emitting a POI per NPC
	// counted one trap three times (318 rows for 129 real locations) and stacked
	// three labels on a single pixel, which rendered as unreadable glyph soup.
	type spot struct {
		zone    string
		x, y, z int
	}
	names := map[spot][]string{}
	refs := map[spot]int{}
	var order []spot

	for rows.Next() {
		var zone, name string
		var gx, gy, gz float64
		var npcID int
		if err := rows.Scan(&zone, &gx, &gy, &gz, &name, &npcID); err != nil {
			return nil, err
		}
		if zone == "" || looksLikeUtility(name) {
			continue
		}
		x, y, z, ok := toMapPoint(gx, gy, gz)
		if !ok {
			continue
		}
		k := spot{zone, x, y, z}
		if _, seen := names[k]; !seen {
			order = append(order, k)
			refs[k] = npcID
		}
		names[k] = append(names[k], cleanName(name))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]POI, 0, len(order))
	for _, k := range order {
		v := names[k]
		sort.Strings(v)
		// Name every flavour: which one is up tells a player what to expect, and
		// at one label per spot there is room to say so.
		out = append(out, POI{Zone: k.zone, X: k.x, Y: k.y, Z: k.z,
			Category: CategoryTrap, Label: "Trap: " + strings.Join(v, " / "),
			Source: "db:spawn2-trap", RefID: refs[k]})
	}
	return out, nil
}

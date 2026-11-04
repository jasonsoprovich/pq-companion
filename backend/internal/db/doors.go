package db

// ZoneDoor is a door the server places in a zone, for the 3D map. Name is
// its model (an actor in the zone's _obj archive); X, Y, Z are game
// coordinates; Heading and Incline are in 1/512ths of a turn; Size is a
// percentage (100 is the model's own size).
type ZoneDoor struct {
	Name    string
	X, Y, Z float64
	Heading float64
	Incline int
	Size    int
}

// ZoneDoors returns the doors active in a zone in the current era: doors
// are expansion-gated like spawns, and an era's replacement door would
// otherwise draw on top of the one it replaced.
func (db *DB) ZoneDoors(zone string) ([]ZoneDoor, error) {
	rows, err := db.Query(`
		SELECT d.name, d.pos_x, d.pos_y, d.pos_z, d.heading,
		       COALESCE(d.incline, 0), d.size
		FROM doors d
		WHERE d.zone = ? AND `+activeContentClause("d"), zone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ZoneDoor
	for rows.Next() {
		var d ZoneDoor
		if err := rows.Scan(&d.Name, &d.X, &d.Y, &d.Z, &d.Heading, &d.Incline, &d.Size); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

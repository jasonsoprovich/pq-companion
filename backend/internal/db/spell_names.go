package db

// SpellMeta is the slice of a spells_new row the Pet overlay needs to label a
// `/pet health` effect line.
type SpellMeta struct {
	ID         int
	Icon       int  // new_icon
	Beneficial bool // goodEffect != 0
	Charm      bool // has SPA 22 (Charm) in its effect list
	LandText   string // cast_on_other, e.g. " yawns."
}

// SpellMetaByName returns every spell in spells_new keyed by exact name. The
// Pet overlay uses it to recognise the bare spell-name lines of a `/pet health`
// reply, tell a charmed pet from a summoned one, and colour/icon each effect.
// Names are exact (Quarm stores some with a backtick, e.g. "Boltran`s
// Agacerie", exactly as the client logs them). About 170 names are shared by
// several rows; the lowest id wins. Read-only data, loaded once at startup.
func (d *DB) SpellMetaByName() (map[string]SpellMeta, error) {
	const q = `
		SELECT id, name, new_icon, goodEffect != 0,
		       22 IN (effectid1, effectid2, effectid3, effectid4, effectid5,
		              effectid6, effectid7, effectid8, effectid9, effectid10,
		              effectid11, effectid12),
		       COALESCE(cast_on_other, '')
		FROM spells_new
		WHERE name IS NOT NULL AND name != ''
		ORDER BY id
	`
	rows, err := d.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]SpellMeta)
	for rows.Next() {
		var name string
		var m SpellMeta
		if err := rows.Scan(&m.ID, &name, &m.Icon, &m.Beneficial, &m.Charm, &m.LandText); err != nil {
			return nil, err
		}
		if _, dup := out[name]; !dup {
			out[name] = m
		}
	}
	return out, rows.Err()
}

// SpellLandTexts returns every spell's non-empty cast_on_other keyed by id.
// The Pet overlay uses it to judge a trigger timer (whose name needn't be a
// spell) by the spell id it carries. Read-only data, loaded once at startup.
func (d *DB) SpellLandTexts() (map[int]string, error) {
	rows, err := d.Query(`SELECT id, cast_on_other FROM spells_new
		WHERE cast_on_other IS NOT NULL AND cast_on_other != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]string)
	for rows.Next() {
		var id int
		var land string
		if err := rows.Scan(&id, &land); err != nil {
			return nil, err
		}
		out[id] = land
	}
	return out, rows.Err()
}

package db

// CastMessage holds the cast-text fields for a single spell — the text EQ
// writes to the log when a spell lands on the player (CastOnYou) or on
// somebody else (CastOnOther). Used by the logparser's CastIndex to detect
// spell-landed events. Either field may be empty; rows with both empty are
// excluded by LoadCastMessages.
type CastMessage struct {
	SpellID     int
	SpellName   string
	CastOnYou   string
	CastOnOther string
}

// LoadCastMessages returns one CastMessage for every spell with at least one
// non-empty cast text. Called once at server startup to seed the spell-landed
// detection index — the underlying data is read-only.
func (d *DB) LoadCastMessages() ([]CastMessage, error) {
	const q = `
		SELECT id,
		       COALESCE(name, ''),
		       COALESCE(cast_on_you, ''),
		       COALESCE(cast_on_other, '')
		FROM spells_new
		WHERE name IS NOT NULL AND name != ''
		  AND ((cast_on_you   IS NOT NULL AND cast_on_you   != '')
		    OR (cast_on_other IS NOT NULL AND cast_on_other != ''))
	`
	rows, err := d.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []CastMessage
	for rows.Next() {
		var m CastMessage
		if err := rows.Scan(&m.SpellID, &m.SpellName, &m.CastOnYou, &m.CastOnOther); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

package db

// SpellNameSets returns every spell name in spells_new, and the subset whose
// effect list contains SPA 22 (Charm). The Pet overlay uses them to recognise
// the bare spell-name lines of a `/pet health` reply and to tell a charmed pet
// from a summoned one. Names are exact (Quarm stores some with a backtick,
// e.g. "Boltran`s Agacerie", exactly as the client logs them). Read-only data,
// loaded once at startup.
func (d *DB) SpellNameSets() (all, charm map[string]bool, err error) {
	const q = `
		SELECT name,
		       22 IN (effectid1, effectid2, effectid3, effectid4, effectid5,
		              effectid6, effectid7, effectid8, effectid9, effectid10,
		              effectid11, effectid12)
		FROM spells_new
		WHERE name IS NOT NULL AND name != ''
	`
	rows, err := d.Query(q)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	all = make(map[string]bool)
	charm = make(map[string]bool)
	for rows.Next() {
		var name string
		var isCharm bool
		if err := rows.Scan(&name, &isCharm); err != nil {
			return nil, nil, err
		}
		all[name] = true
		if isCharm {
			charm[name] = true
		}
	}
	return all, charm, rows.Err()
}

package db

import "log/slog"

// RecourseParent returns the name of the spell whose recourse is spellID
// (spells_new.RecourseLink), or "" when spellID isn't a recourse. A recourse
// is the caster-side buff a spell like Aura of Darkness lands on the caster
// (or the caster's group) alongside its main effect. No player class can cast
// one directly, so the spell row alone can't say whose it is; its parent can.
// Built once on first use: 44 rows in quarm.db.
func (db *DB) RecourseParent(spellID int) string {
	db.recourseOnce.Do(db.loadRecourseParents)
	return db.recourseParents[spellID]
}

func (db *DB) loadRecourseParents() {
	db.recourseParents = map[int]string{}
	rows, err := db.Query(`SELECT RecourseLink, name FROM spells_new
		WHERE RecourseLink > 0 AND name IS NOT NULL AND name != '' ORDER BY id`)
	if err != nil {
		slog.Warn("db: load recourse links", "err", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Warn("db: scan recourse link", "err", err)
			return
		}
		if _, ok := db.recourseParents[id]; !ok {
			db.recourseParents[id] = name
		}
	}
}

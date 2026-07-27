package db

// SpellEmoteDefault holds the five emote text columns quarm.db carries per
// spell — the same canonical text the server originally shipped in
// spells_en.txt, independent of anything a player has since hand-edited into
// their local copy of that file.
type SpellEmoteDefault struct {
	Name        string
	YouCast     string
	OtherCasts  string
	CastOnYou   string
	CastOnOther string
	SpellFades  string
}

// LoadSpellEmoteDefaults returns every spell's canonical emote text, keyed by
// id. Used by the Spell Emote Customizer to derive a true pristine default —
// never a snapshot of whatever happens to be on disk — and to detect emotes a
// user already hand-edited into spells_en.txt before ever using the feature.
func (d *DB) LoadSpellEmoteDefaults() (map[int]SpellEmoteDefault, error) {
	const q = `
		SELECT id,
		       COALESCE(name, ''),
		       COALESCE(you_cast, ''),
		       COALESCE(other_casts, ''),
		       COALESCE(cast_on_you, ''),
		       COALESCE(cast_on_other, ''),
		       COALESCE(spell_fades, '')
		FROM spells_new
	`
	rows, err := d.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]SpellEmoteDefault)
	for rows.Next() {
		var id int
		var def SpellEmoteDefault
		if err := rows.Scan(&id, &def.Name, &def.YouCast, &def.OtherCasts, &def.CastOnYou, &def.CastOnOther, &def.SpellFades); err != nil {
			return nil, err
		}
		out[id] = def
	}
	return out, rows.Err()
}

package emote

import "github.com/jasonsoprovich/pq-companion/backend/internal/db"

// deriveDefaultContent rewrites content's emote columns to quarm.db's
// canonical text for every spell it has a row for, leaving every other
// column and every spell quarm.db doesn't know about untouched. This is the
// pristine default the rest of the package rebuilds from — never a raw
// snapshot of whatever happened to already be on disk, which could itself
// carry hand-edits (see detectDivergence).
func deriveDefaultContent(content string, defaults map[int]db.SpellEmoteDefault) string {
	canonical := make(map[int]OverrideRow, len(defaults))
	for id, def := range defaults {
		def := def // fresh copy per iteration so these pointers don't alias
		canonical[id] = OverrideRow{
			SpellID:     id,
			YouCast:     &def.YouCast,
			OtherCasts:  &def.OtherCasts,
			CastOnYou:   &def.CastOnYou,
			CastOnOther: &def.CastOnOther,
			SpellFades:  &def.SpellFades,
		}
	}
	return applyOverrides(content, canonical)
}

// detectDivergence compares liveContent against the freshly-derived
// canonical content and reports every spell whose emote text already
// differed — almost always a customization the user hand-edited into
// spells_en.txt before ever using the Spell Emote Customizer. Reused as both
// the pending-import list and its diff display: Old is the canonical/default
// text, New is what the user already had.
func detectDivergence(liveContent, canonicalContent string) []SpellDiff {
	canonicalByID := make(map[int][]string)
	for _, line := range splitLines(canonicalContent) {
		fields := parseFields(line)
		if len(fields) < minEmoteFields {
			continue
		}
		if id, ok := lineSpellID(fields); ok {
			canonicalByID[id] = fields
		}
	}

	idxs := [5]int{idxYouCast, idxOtherCasts, idxCastOnYou, idxCastOnOther, idxSpellFades}
	var out []SpellDiff
	for _, line := range splitLines(liveContent) {
		liveFields := parseFields(line)
		if len(liveFields) < minEmoteFields {
			continue
		}
		id, ok := lineSpellID(liveFields)
		if !ok {
			continue
		}
		canonicalFields, ok := canonicalByID[id]
		if !ok {
			continue
		}
		var fields []FieldDiff
		for i, idx := range idxs {
			if liveFields[idx] != canonicalFields[idx] {
				fields = append(fields, FieldDiff{
					Field: columnField(i),
					Label: columnLabels[i],
					Old:   canonicalFields[idx],
					New:   liveFields[idx],
				})
			}
		}
		if len(fields) > 0 {
			out = append(out, SpellDiff{SpellID: id, Name: liveFields[1], Fields: fields})
		}
	}
	return out
}

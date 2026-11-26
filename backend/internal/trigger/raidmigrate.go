package trigger

import "strings"

// raidCausticOnYou is the shared Caustic Mist / Putrefy Flesh trigger's
// on-you branch: both spells' on-you lines and resists (only the on-other
// line is shared). raidCausticOldPattern is the pattern Raid Alerts
// shipped, with Caustic Mist's on-you line only.
const (
	raidCausticOnYou      = `Your skin begins to rot\.|Your flesh begins to liquefy\.|You resist the (?:Caustic Mist|Putrefy Flesh) spell!`
	raidCausticOldPattern = `^(?:Your skin begins to rot\.|[A-Z][a-zA-Z']{2,14}'s flesh begins to liquefy\.)$`
)

// raidAlertsPackName is the built-in pack the Ring War signature-spell
// triggers originally shipped in.
const raidAlertsPackName = "Raid Alerts"

// installedSignatureRows returns the installed rows that came from the
// shipped Raid Alerts signature-spell triggers, paired with the shipped
// definition each one matches. Rows are found by install origin
// (SourcePack, so a row the user moved to another category still counts)
// and stable identity (PackKey, so a renamed row still counts). Legacy rows
// written before those columns existed fall back to the "Raid Alerts"
// category, matched by pattern and then by name.
func installedSignatureRows(store *Store) ([]*Trigger, []*Trigger, error) {
	defs := raidSignatureSpellAlerts()
	byKey := make(map[string]*Trigger, len(defs))
	byPattern := make(map[string]*Trigger, len(defs))
	for i := range defs {
		d := &defs[i]
		byKey[packKeyOf(d)] = d
		byPattern[d.Pattern] = d
		if strings.HasPrefix(d.Name, "Caustic Mist") {
			byPattern[raidCausticOldPattern] = d
		}
	}

	all, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	var rows, matched []*Trigger
	for _, t := range all {
		var def *Trigger
		switch {
		case t.SourcePack == raidAlertsPackName && t.PackKey != "":
			def = byKey[t.PackKey]
		case t.SourcePack == raidAlertsPackName || (t.SourcePack == "" && t.PackName == raidAlertsPackName):
			if d, ok := byPattern[t.Pattern]; ok {
				def = d
			} else {
				def = byKey[strings.TrimSpace(t.Name)]
			}
		}
		if def != nil {
			rows = append(rows, t)
			matched = append(matched, def)
		}
	}
	return rows, matched, nil
}

// migrateRaidSignatureRows prepares installed Raid Alerts signature-spell
// rows for their move to the raid packs. For each row it stamps the shipped
// BossCasters (when the row has none) and the shared DedupKey, fills a
// missing SourcePack/PackKey on legacy rows, and makes sure Raid Alerts has
// a baseline for it: rows added by the old one-off inserts never got one,
// and without it the Raid Alerts update couldn't list the row as moved.
// Every user customization is left untouched.
//
// The raid packs ship in the same build, so this runs before any of them
// can be installed; with the DedupKey owned by Raid Alerts, installing one
// later skips these five instead of duplicating them, and applying the Raid
// Alerts update re-homes them (ApplyPackUpdate).
func migrateRaidSignatureRows(store *Store) (bool, error) {
	rows, defs, err := installedSignatureRows(store)
	if err != nil {
		return false, err
	}
	baselines, err := store.PackBaselines(raidAlertsPackName)
	if err != nil {
		return false, err
	}
	changed := false
	for i, t := range rows {
		def := defs[i]
		key := packKeyOf(def)
		dirty := false
		if len(t.BossCasters) == 0 {
			t.BossCasters = append([]BossCaster(nil), def.BossCasters...)
			dirty = true
		}
		if t.DedupKey == "" {
			t.DedupKey = def.DedupKey
			dirty = true
		}
		if t.SourcePack == "" {
			t.SourcePack = raidAlertsPackName
			dirty = true
		}
		if t.PackKey == "" {
			t.PackKey = key
			dirty = true
		}
		// A pattern the user never edited follows the new definition.
		if t.Pattern == raidCausticOldPattern && def.Pattern != t.Pattern {
			t.Pattern = def.Pattern
			dirty = true
		}
		if dirty {
			if err := store.Update(t); err != nil {
				return changed, err
			}
			changed = true
		}
		if baselines[t.PackKey] == nil {
			b := *def
			b.PackName = raidAlertsPackName
			if err := store.UpsertPackBaseline(raidAlertsPackName, &b); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

// raidSignatureDedupKey is the shared DedupKey for a signature-spell
// trigger, derived from its (stable) shipped name.
func raidSignatureDedupKey(name string) string {
	k := strings.ToLower(name)
	k = strings.NewReplacer(" / ", "_", " ", "_").Replace(k)
	return "raid_sig_" + k
}

// raidSignatureShared returns the shared signature-spell definitions with
// the given names, for a raid pack to ship.
func raidSignatureShared(names ...string) []Trigger {
	var out []Trigger
	for _, t := range raidSignatureSpellAlerts() {
		if containsString(names, t.Name) {
			out = append(out, t)
		}
	}
	return out
}

// bc is shorthand for a BossCaster with no duration override.
func bc(name string) BossCaster { return BossCaster{Name: name} }

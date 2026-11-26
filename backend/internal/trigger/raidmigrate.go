package trigger

import "strings"

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

// stampRaidSignatureCasters copies each shipped signature trigger's
// BossCasters onto its installed row when the row has none yet. Every other
// field — the user's customizations — is left untouched.
func stampRaidSignatureCasters(store *Store) (bool, error) {
	rows, defs, err := installedSignatureRows(store)
	if err != nil {
		return false, err
	}
	changed := false
	for i, t := range rows {
		if len(t.BossCasters) > 0 {
			continue
		}
		t.BossCasters = append([]BossCaster(nil), defs[i].BossCasters...)
		if err := store.Update(t); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}

// bc is shorthand for a BossCaster with no duration override.
func bc(name string) BossCaster { return BossCaster{Name: name} }

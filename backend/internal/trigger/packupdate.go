package trigger

import (
	"encoding/json"
	"fmt"
	"time"
)

// Pack updates
//
// Built-in pack definitions are compiled into the app, so a new release can
// change them under an existing install. The pack_baselines table snapshots
// each trigger's definition as it was last installed/updated, giving a
// three-way view:
//
//	baseline ≠ shipped definition → the developer changed it (update available)
//	baseline ≠ user's row         → the user customized it
//
// ComputePackDiff turns that into a per-trigger changelist; ApplyPackUpdate
// applies it in one of two modes:
//
//	UpdateModePreserve — per-field merge: fields the user customized keep
//	  their values (actions, characters, enabled included), everything else
//	  takes the new definition.
//	UpdateModeReset — the trigger is reset wholesale to the new definition,
//	  including actions, default category, and default characters.

const (
	UpdateModePreserve = "preserve"
	UpdateModeReset    = "reset"
)

// packKeyOf resolves a pack definition trigger's stable identity: an explicit
// PackKey when the definition sets one (used to carry identity across a
// developer rename), otherwise the definition name.
func packKeyOf(t *Trigger) string {
	if t.PackKey != "" {
		return t.PackKey
	}
	return t.Name
}

// packField is one diffable definition field: how to read it (normalized so
// DB rows and in-code definitions compare equal) and how to copy it from one
// trigger to another during a preserve-mode merge.
type packField struct {
	name  string
	label string
	get   func(*Trigger) any
	set   func(dst, src *Trigger)
}

// packFields lists every field the pack-update diff considers. Characters and
// SortOrder are deliberately absent: characters are per-user context (reset
// mode re-derives them from the class defaults), and sort order is purely a
// user arrangement.
var packFields = []packField{
	{"name", "Name",
		func(t *Trigger) any { return t.Name },
		func(d, s *Trigger) { d.Name = s.Name }},
	{"pattern", "Pattern",
		func(t *Trigger) any { return t.Pattern },
		func(d, s *Trigger) { d.Pattern = s.Pattern }},
	{"extra_patterns", "Additional patterns",
		func(t *Trigger) any { return emptyIfNilExtra(t.ExtraPatterns) },
		func(d, s *Trigger) { d.ExtraPatterns = s.ExtraPatterns }},
	{"exclude_patterns", "Exclude patterns",
		func(t *Trigger) any { return emptyIfNilStr(t.ExcludePatterns) },
		func(d, s *Trigger) { d.ExcludePatterns = s.ExcludePatterns }},
	{"category", "Category",
		func(t *Trigger) any { return t.PackName },
		func(d, s *Trigger) { d.PackName = s.PackName }},
	{"enabled", "Enabled",
		func(t *Trigger) any { return t.Enabled },
		func(d, s *Trigger) { d.Enabled = s.Enabled }},
	{"source", "Match source",
		func(t *Trigger) any {
			if t.Source == "" {
				return SourceLog
			}
			return t.Source
		},
		func(d, s *Trigger) { d.Source = s.Source }},
	{"pipe_condition", "Pipe condition",
		func(t *Trigger) any { return t.PipeCondition },
		func(d, s *Trigger) { d.PipeCondition = s.PipeCondition }},
	{"timer_type", "Timer type",
		func(t *Trigger) any {
			if t.TimerType == "" {
				return TimerTypeNone
			}
			return t.TimerType
		},
		func(d, s *Trigger) { d.TimerType = s.TimerType }},
	{"timer_duration_secs", "Timer duration",
		func(t *Trigger) any { return t.TimerDurationSecs },
		func(d, s *Trigger) { d.TimerDurationSecs = s.TimerDurationSecs }},
	{"worn_off_pattern", "Worn-off pattern",
		func(t *Trigger) any { return t.WornOffPattern },
		func(d, s *Trigger) { d.WornOffPattern = s.WornOffPattern }},
	{"spell_id", "Linked spell",
		func(t *Trigger) any { return t.SpellID },
		func(d, s *Trigger) { d.SpellID = s.SpellID }},
	{"timer_alerts", "Timer alerts",
		func(t *Trigger) any { return emptyIfNilAlerts(t.TimerAlerts) },
		func(d, s *Trigger) { d.TimerAlerts = s.TimerAlerts }},
	{"timer_duration_capture", "Duration capture",
		func(t *Trigger) any { return t.TimerDurationCapture },
		func(d, s *Trigger) { d.TimerDurationCapture = s.TimerDurationCapture }},
	{"timer_key_capture", "Timer key capture",
		func(t *Trigger) any { return t.TimerKeyCapture },
		func(d, s *Trigger) { d.TimerKeyCapture = s.TimerKeyCapture }},
	{"timer_target_capture", "Timer target capture",
		func(t *Trigger) any { return t.TimerTargetCapture },
		func(d, s *Trigger) { d.TimerTargetCapture = s.TimerTargetCapture }},
	{"display_threshold_secs", "Display threshold",
		func(t *Trigger) any { return t.DisplayThresholdSecs },
		func(d, s *Trigger) { d.DisplayThresholdSecs = s.DisplayThresholdSecs }},
	{"cooldown_secs", "Reuse cooldown",
		func(t *Trigger) any { return t.CooldownSecs },
		func(d, s *Trigger) { d.CooldownSecs = s.CooldownSecs }},
	{"refire_cooldown_secs", "Refire cooldown",
		func(t *Trigger) any { return t.RefireCooldownSecs },
		func(d, s *Trigger) { d.RefireCooldownSecs = s.RefireCooldownSecs }},
	{"bar_color", "Timer bar color",
		func(t *Trigger) any { return t.BarColor },
		func(d, s *Trigger) { d.BarColor = s.BarColor }},
	{"dedup_key", "Dedup key",
		func(t *Trigger) any { return t.DedupKey },
		func(d, s *Trigger) { d.DedupKey = s.DedupKey }},
	{"actions", "Actions",
		func(t *Trigger) any { return emptyIfNilActions(t.Actions) },
		func(d, s *Trigger) { d.Actions = s.Actions }},
}

func emptyIfNilStr(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func emptyIfNilExtra(v []ExtraPattern) []ExtraPattern {
	if v == nil {
		return []ExtraPattern{}
	}
	return v
}

func emptyIfNilAlerts(v []TimerAlert) []TimerAlert {
	if v == nil {
		return []TimerAlert{}
	}
	return v
}

func emptyIfNilActions(v []Action) []Action {
	if v == nil {
		return []Action{}
	}
	return v
}

// fieldJSON renders a field value for comparison and display. Plain strings
// come back unquoted; everything else as compact JSON.
func fieldJSON(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func fieldEqual(a, b any) bool {
	return fieldJSON(a) == fieldJSON(b)
}

// FieldDiff is one field-level difference between the baseline (what shipped
// when the user installed the pack), the new shipped definition, and the
// user's current row.
type FieldDiff struct {
	Field string `json:"field"`
	Label string `json:"label"`
	// Old is the baseline value; New the value in the current build.
	Old string `json:"old"`
	New string `json:"new"`
	// Current is the user's row value; UserCustomized is true when it
	// differs from the baseline — a preserve-mode update keeps Current for
	// this field.
	Current        string `json:"current"`
	UserCustomized bool   `json:"user_customized"`
}

// ChangedTrigger is an installed pack trigger whose shipped definition
// changed since it was installed.
type ChangedTrigger struct {
	PackKey string `json:"pack_key"`
	Name    string `json:"name"`
	// InstalledName is the user's current name for the row (differs from
	// Name after a user or developer rename).
	InstalledName string      `json:"installed_name"`
	Fields        []FieldDiff `json:"fields"`
}

// AddedTrigger is a definition present in the current build but not in the
// user's install baseline — a trigger the developer added to the pack.
type AddedTrigger struct {
	PackKey string `json:"pack_key"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// RemovedTrigger is an installed trigger whose definition no longer exists
// in the current build — applying the update deletes it.
type RemovedTrigger struct {
	PackKey string `json:"pack_key"`
	Name    string `json:"name"`
}

// DeletedLocalTrigger is a definition the user deleted from their install
// but the pack still ships. Never re-added unless explicitly selected.
type DeletedLocalTrigger struct {
	PackKey string `json:"pack_key"`
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// PackDiff is the full changelist between an installed pack and the pack
// definition compiled into the current build.
type PackDiff struct {
	PackName       string                `json:"pack_name"`
	Changed        []ChangedTrigger      `json:"changed"`
	Added          []AddedTrigger        `json:"added"`
	Removed        []RemovedTrigger      `json:"removed"`
	DeletedLocally []DeletedLocalTrigger `json:"deleted_locally"`
	UpToDate       int                   `json:"up_to_date"`
}

// HasUpdates reports whether applying the diff would change anything the
// user hasn't opted out of (locally-deleted triggers don't count — they're
// offered, not pending).
func (d *PackDiff) HasUpdates() bool {
	return len(d.Changed) > 0 || len(d.Added) > 0 || len(d.Removed) > 0
}

// PackUpdateSummary is the per-pack badge payload for the Packs tab.
type PackUpdateSummary struct {
	PackName       string `json:"pack_name"`
	Changed        int    `json:"changed"`
	Added          int    `json:"added"`
	Removed        int    `json:"removed"`
	DeletedLocally int    `json:"deleted_locally"`
}

// ComputePackDiff compares the shipped definition of pack against the stored
// baselines and installed rows. Rows installed before the baseline system
// that couldn't be matched back to a definition (pack_key = ”) are left
// alone and never reported.
func ComputePackDiff(store *Store, pack TriggerPack) (*PackDiff, error) {
	baselines, err := store.PackBaselines(pack.PackName)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListBySourcePack(pack.PackName)
	if err != nil {
		return nil, err
	}
	rowsByKey := make(map[string]*Trigger, len(rows))
	for _, r := range rows {
		if r.PackKey != "" {
			rowsByKey[r.PackKey] = r
		}
	}

	diff := &PackDiff{PackName: pack.PackName}
	defKeys := make(map[string]bool, len(pack.Triggers))
	for i := range pack.Triggers {
		def := &pack.Triggers[i]
		key := packKeyOf(def)
		defKeys[key] = true
		base := baselines[key]
		row := rowsByKey[key]
		if base == nil {
			if row != nil {
				// Installed but never baselined (pre-feature edge case);
				// nothing to compare against, treat as current.
				diff.UpToDate++
				continue
			}
			// Not in the baseline and not installed → the developer added
			// it — unless another installed pack owns its dedup_key, in
			// which case this pack doesn't ship it for this user.
			if def.DedupKey != "" {
				owner, err := store.FindByDedupKey(def.DedupKey)
				if err != nil {
					return nil, err
				}
				if owner != nil && owner.SourcePack != pack.PackName {
					continue
				}
			}
			diff.Added = append(diff.Added, AddedTrigger{PackKey: key, Name: def.Name, Pattern: def.Pattern})
			continue
		}
		if row == nil {
			diff.DeletedLocally = append(diff.DeletedLocally, DeletedLocalTrigger{PackKey: key, Name: def.Name, Pattern: def.Pattern})
			continue
		}
		fields := diffFields(base, def, row)
		if len(fields) == 0 {
			diff.UpToDate++
			continue
		}
		diff.Changed = append(diff.Changed, ChangedTrigger{
			PackKey:       key,
			Name:          def.Name,
			InstalledName: row.Name,
			Fields:        fields,
		})
	}

	// Baselines with no matching definition: the developer removed the
	// trigger from the pack. Only report ones the user still has installed;
	// stale baseline rows with no row are purged on the next apply.
	for key := range baselines {
		if defKeys[key] {
			continue
		}
		if row := rowsByKey[key]; row != nil {
			diff.Removed = append(diff.Removed, RemovedTrigger{PackKey: key, Name: row.Name})
		}
	}
	return diff, nil
}

// diffFields returns one FieldDiff per field where the shipped definition
// diverged from the baseline, annotated with whether the user's row also
// diverged from the baseline (a preserve-mode update keeps the user's value
// there).
func diffFields(base, def, row *Trigger) []FieldDiff {
	var out []FieldDiff
	for _, f := range packFields {
		bv, dv := f.get(base), f.get(def)
		if fieldEqual(bv, dv) {
			continue
		}
		rv := f.get(row)
		out = append(out, FieldDiff{
			Field:          f.name,
			Label:          f.label,
			Old:            fieldJSON(bv),
			New:            fieldJSON(dv),
			Current:        fieldJSON(rv),
			UserCustomized: !fieldEqual(rv, bv),
		})
	}
	return out
}

// ComputeUpdateSummaries returns a summary for every installed built-in pack
// that has pending updates or locally-deleted definitions to offer.
func ComputeUpdateSummaries(store *Store) ([]PackUpdateSummary, error) {
	installed, err := store.InstalledPackNames()
	if err != nil {
		return nil, err
	}
	var out []PackUpdateSummary
	for _, p := range AllPacks() {
		if !installed[p.PackName] {
			continue
		}
		diff, err := ComputePackDiff(store, p)
		if err != nil {
			return nil, err
		}
		if !diff.HasUpdates() && len(diff.DeletedLocally) == 0 {
			continue
		}
		out = append(out, PackUpdateSummary{
			PackName:       p.PackName,
			Changed:        len(diff.Changed),
			Added:          len(diff.Added),
			Removed:        len(diff.Removed),
			DeletedLocally: len(diff.DeletedLocally),
		})
	}
	return out, nil
}

// PackUpdateResult reports what an ApplyPackUpdate call actually did.
type PackUpdateResult struct {
	Updated int `json:"updated"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// ApplyPackUpdate applies the pending diff for def (the pack definition in
// the current build) to the user's installed triggers.
//
// defaulted must be the same pack with per-user character defaults applied
// (the API handler's applyDefaultCharacters) — it supplies Characters and
// Enabled for triggers inserted or reset by the update.
//
// keys selects which triggers (by pack_key) to touch; nil/empty means every
// pending change except locally-deleted definitions, which are only ever
// re-added when explicitly selected. Baselines advance only for applied
// triggers, so anything deselected stays flagged for next time.
func ApplyPackUpdate(store *Store, def, defaulted TriggerPack, mode string, keys []string) (*PackUpdateResult, error) {
	if mode != UpdateModePreserve && mode != UpdateModeReset {
		return nil, fmt.Errorf("unknown pack update mode %q", mode)
	}
	diff, err := ComputePackDiff(store, def)
	if err != nil {
		return nil, err
	}
	baselines, err := store.PackBaselines(def.PackName)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListBySourcePack(def.PackName)
	if err != nil {
		return nil, err
	}
	rowsByKey := make(map[string]*Trigger, len(rows))
	for _, r := range rows {
		if r.PackKey != "" {
			rowsByKey[r.PackKey] = r
		}
	}
	defsByKey := make(map[string]*Trigger, len(def.Triggers))
	for i := range def.Triggers {
		defsByKey[packKeyOf(&def.Triggers[i])] = &def.Triggers[i]
	}
	defaultedByKey := make(map[string]*Trigger, len(defaulted.Triggers))
	for i := range defaulted.Triggers {
		defaultedByKey[packKeyOf(&defaulted.Triggers[i])] = &defaulted.Triggers[i]
	}

	selected := func(key string) bool { return true }
	explicit := make(map[string]bool, len(keys))
	if len(keys) > 0 {
		for _, k := range keys {
			explicit[k] = true
		}
		selected = func(key string) bool { return explicit[key] }
	}

	res := &PackUpdateResult{}
	now := time.Now().UTC()

	insertDef := func(key string) error {
		d := defsByKey[key]
		if d == nil {
			return nil
		}
		if d.DedupKey != "" {
			owner, err := store.FindByDedupKey(d.DedupKey)
			if err != nil {
				return err
			}
			if owner != nil && owner.SourcePack != def.PackName {
				return nil
			}
		}
		t := *d
		if dd := defaultedByKey[key]; dd != nil {
			t.Characters = dd.Characters
			t.Enabled = dd.Enabled
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		t.ID = id
		t.CreatedAt = now
		t.SourcePack = def.PackName
		t.PackKey = key
		if so, err := store.NextTriggerSortOrder(t.PackName); err == nil {
			t.SortOrder = so
		}
		if err := store.Insert(&t); err != nil {
			return err
		}
		res.Added++
		return store.UpsertPackBaseline(def.PackName, d)
	}

	for _, c := range diff.Changed {
		if !selected(c.PackKey) {
			continue
		}
		row := rowsByKey[c.PackKey]
		d := defsByKey[c.PackKey]
		base := baselines[c.PackKey]
		if row == nil || d == nil || base == nil {
			continue
		}
		if mode == UpdateModeReset {
			fresh := *d
			if dd := defaultedByKey[c.PackKey]; dd != nil {
				fresh.Characters = dd.Characters
				fresh.Enabled = dd.Enabled
			}
			fresh.ID = row.ID
			fresh.CreatedAt = row.CreatedAt
			fresh.SourcePack = def.PackName
			fresh.PackKey = c.PackKey
			fresh.SortOrder = row.SortOrder
			if err := store.Update(&fresh); err != nil {
				return res, err
			}
		} else {
			for _, f := range packFields {
				// Field untouched by the user → follow the new definition;
				// customized → keep theirs.
				if fieldEqual(f.get(row), f.get(base)) {
					f.set(row, d)
				}
			}
			if err := store.Update(row); err != nil {
				return res, err
			}
		}
		res.Updated++
		if err := store.UpsertPackBaseline(def.PackName, d); err != nil {
			return res, err
		}
	}

	for _, a := range diff.Added {
		if !selected(a.PackKey) {
			continue
		}
		if err := insertDef(a.PackKey); err != nil {
			return res, err
		}
	}

	// Locally-deleted definitions are opt-in only: never resurrected by a
	// blanket "apply all".
	for _, dl := range diff.DeletedLocally {
		if !explicit[dl.PackKey] {
			continue
		}
		if err := insertDef(dl.PackKey); err != nil {
			return res, err
		}
	}

	for _, r := range diff.Removed {
		if !selected(r.PackKey) {
			continue
		}
		if row := rowsByKey[r.PackKey]; row != nil {
			if err := store.Delete(row.ID); err != nil {
				return res, err
			}
			res.Removed++
		}
		if err := store.DeletePackBaseline(def.PackName, r.PackKey); err != nil {
			return res, err
		}
	}

	// Purge stale baselines: definition gone from the pack and no installed
	// row left (the user already deleted theirs).
	for key := range baselines {
		if defsByKey[key] != nil || rowsByKey[key] != nil {
			continue
		}
		if err := store.DeletePackBaseline(def.PackName, key); err != nil {
			return res, err
		}
	}
	return res, nil
}

package trigger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists trigger definitions in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) user.db at path and runs schema migrations.
//
// Three packages (trigger, character, backup) each open their own *sql.DB
// against the same user.db file. WAL mode lets readers and a single writer
// coexist, but concurrent writers still hit SQLITE_BUSY — the busy_timeout
// is how long SQLite will retry before giving up. 30s comfortably covers
// startup bursts like zeal.RefreshAllPersonas writing every character's AAs
// while the user clicks "Install trigger pack".
//
// modernc.org/sqlite expects PRAGMAs via the _pragma=NAME(VALUE) URI form;
// the mattn-style _journal_mode/_busy_timeout query params are silently
// ignored, which previously left the DB in default (DELETE) journal mode
// with a 0 busy_timeout — surfacing SQLITE_BUSY at the slightest contention.
func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open user.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping user.db: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate user.db: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS triggers (
			id                     TEXT    NOT NULL PRIMARY KEY,
			name                   TEXT    NOT NULL,
			enabled                INTEGER NOT NULL DEFAULT 1,
			pattern                TEXT    NOT NULL,
			actions                TEXT    NOT NULL DEFAULT '[]',
			pack_name              TEXT    NOT NULL DEFAULT '',
			created_at             INTEGER NOT NULL,
			timer_type             TEXT    NOT NULL DEFAULT 'none',
			timer_duration_secs    INTEGER NOT NULL DEFAULT 0,
			worn_off_pattern       TEXT    NOT NULL DEFAULT '',
			spell_id               INTEGER NOT NULL DEFAULT 0,
			display_threshold_secs INTEGER NOT NULL DEFAULT 0,
			characters             TEXT    NOT NULL DEFAULT '[]',
			timer_alerts           TEXT    NOT NULL DEFAULT '[]',
			exclude_patterns       TEXT    NOT NULL DEFAULT '[]',
			extra_patterns         TEXT    NOT NULL DEFAULT '[]',
			timer_duration_capture TEXT    NOT NULL DEFAULT '',
			timer_key_capture      TEXT    NOT NULL DEFAULT '',
			timer_target_capture   TEXT    NOT NULL DEFAULT '',
			source                 TEXT    NOT NULL DEFAULT 'log',
			pipe_condition         TEXT    NOT NULL DEFAULT '',
			dedup_key              TEXT    NOT NULL DEFAULT '',
			cooldown_secs          INTEGER NOT NULL DEFAULT 0,
			sort_order             INTEGER NOT NULL DEFAULT 0,
			source_pack            TEXT    NOT NULL DEFAULT '',
			bar_color              TEXT    NOT NULL DEFAULT '',
			refire_cooldown_secs   INTEGER NOT NULL DEFAULT 0,
			pack_key               TEXT    NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}

	// Snapshot of each built-in pack trigger's definition as it was last
	// installed/updated, keyed by (source_pack, pack_key). The pack-update
	// diff compares these against the definitions compiled into the current
	// build: baseline≠shipped = "update available", baseline≠user's row =
	// "user customized" — so user edits never show up as phantom updates.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pack_baselines (
			source_pack TEXT    NOT NULL,
			pack_key    TEXT    NOT NULL,
			definition  TEXT    NOT NULL,
			updated_at  INTEGER NOT NULL,
			PRIMARY KEY (source_pack, pack_key)
		)
	`); err != nil {
		return err
	}

	// Named, reusable Actions lists (see ActionTemplate). is_default marks
	// at most one template whose actions prefill newly created triggers.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS action_templates (
			id         TEXT    NOT NULL PRIMARY KEY,
			name       TEXT    NOT NULL,
			actions    TEXT    NOT NULL DEFAULT '[]',
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}

	// Tracks one-time additive pack default updates so each runs at most
	// once. See ApplyDefaultUpdates / DefaultUpdates for the migration list.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pack_default_updates (
			key        TEXT    NOT NULL PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}

	// Persists user-created custom categories so an empty, freshly-created
	// group survives a restart. The category key is the triggers.pack_name
	// column; built-in (class) and imported packs are NOT recorded here —
	// they're derived from in-use pack_name values. See category.go.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trigger_categories (
			name       TEXT    NOT NULL PRIMARY KEY,
			created_at INTEGER NOT NULL,
			explicit   INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}

	// Idempotently add columns for databases created before each feature.
	addColumns := []string{
		`ALTER TABLE triggers ADD COLUMN timer_type TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE triggers ADD COLUMN timer_duration_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN worn_off_pattern TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN spell_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN display_threshold_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN characters TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN timer_alerts TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN exclude_patterns TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN extra_patterns TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN timer_duration_capture TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN timer_key_capture TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN timer_target_capture TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN source TEXT NOT NULL DEFAULT 'log'`,
		`ALTER TABLE triggers ADD COLUMN pipe_condition TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN dedup_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN cooldown_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN source_pack TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN bar_color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN refire_cooldown_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN pack_key TEXT NOT NULL DEFAULT ''`,
		// trigger_categories columns for databases created before category
		// ordering. Existing rows were all user-created → explicit defaults 1.
		`ALTER TABLE trigger_categories ADD COLUMN explicit INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE trigger_categories ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column: %w", err)
		}
	}
	if err := s.backfillSourcePack(); err != nil {
		return err
	}
	if err := s.backfillPackKeysAndBaselines(); err != nil {
		return err
	}
	return nil
}

// backfillPackKeysAndBaselines bootstraps the pack-update diff system for
// installs that predate it: stamps pack_key on existing pack triggers (matched
// against the current build's definitions by name, then by pattern as a
// fallback for user-renamed rows) and seeds pack_baselines with the current
// definitions. Seeding baseline = shipped-def means no phantom "updates" show
// immediately after upgrading, and customizations made before this release
// read as baseline — i.e. they're preserved, never flagged or overwritten.
// One-time, ledger-guarded; the install/update paths maintain both from here on.
func (s *Store) backfillPackKeysAndBaselines() error {
	const key = "PackBaselines:Seed:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	installed, err := s.InstalledPackNames()
	if err != nil {
		return err
	}
	for _, p := range AllPacks() {
		if !installed[p.PackName] {
			continue
		}
		rows, err := s.ListBySourcePack(p.PackName)
		if err != nil {
			return err
		}
		byName := make(map[string]*Trigger, len(rows))
		byPattern := make(map[string]*Trigger, len(rows))
		for _, r := range rows {
			byName[r.Name] = r
			byPattern[r.Pattern] = r
		}
		claimed := make(map[string]bool, len(rows))
		for i := range p.Triggers {
			def := &p.Triggers[i]
			row := byName[def.Name]
			if row == nil {
				row = byPattern[def.Pattern]
			}
			if row == nil || row.PackKey != "" || claimed[row.ID] {
				continue
			}
			claimed[row.ID] = true
			if _, err := s.db.Exec(
				`UPDATE triggers SET pack_key = ? WHERE id = ?`,
				packKeyOf(def), row.ID,
			); err != nil {
				return fmt.Errorf("backfill pack_key for %s/%s: %w", p.PackName, def.Name, err)
			}
		}
		if err := s.writePackBaselines(p); err != nil {
			return err
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// writePackBaselines upserts a baseline row for every trigger in the pack's
// shipped definition, except those whose dedup_key is currently owned by a
// different installed pack (those aren't installed from this pack, so they
// must not count as this pack's baseline or they'd read as deleted forever).
func (s *Store) writePackBaselines(pack TriggerPack) error {
	for i := range pack.Triggers {
		def := pack.Triggers[i]
		if def.DedupKey != "" {
			owner, err := s.FindByDedupKey(def.DedupKey)
			if err != nil {
				return err
			}
			if owner != nil && owner.SourcePack != pack.PackName {
				continue
			}
		}
		if err := s.UpsertPackBaseline(pack.PackName, &def); err != nil {
			return err
		}
	}
	return nil
}

// UpsertPackBaseline records def (a shipped pack definition, not a user row)
// as the baseline for (sourcePack, packKeyOf(def)). Identity/user fields are
// blanked so the stored JSON is a pure definition.
func (s *Store) UpsertPackBaseline(sourcePack string, def *Trigger) error {
	d := *def
	d.ID = ""
	d.CreatedAt = time.Time{}
	d.SourcePack = ""
	d.Characters = nil
	d.PackKey = packKeyOf(def)
	blob, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal baseline %s/%s: %w", sourcePack, d.PackKey, err)
	}
	_, err = s.db.Exec(
		`INSERT INTO pack_baselines (source_pack, pack_key, definition, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(source_pack, pack_key) DO UPDATE SET definition = excluded.definition, updated_at = excluded.updated_at`,
		sourcePack, d.PackKey, string(blob), time.Now().UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert baseline %s/%s: %w", sourcePack, d.PackKey, err)
	}
	return nil
}

// PackBaselines returns the stored baseline definitions for a pack, keyed by
// pack_key. Rows whose JSON fails to parse are skipped (they'll be rewritten
// on the next update apply).
func (s *Store) PackBaselines(sourcePack string) (map[string]*Trigger, error) {
	rows, err := s.db.Query(
		`SELECT pack_key, definition FROM pack_baselines WHERE source_pack = ?`, sourcePack,
	)
	if err != nil {
		return nil, fmt.Errorf("list baselines for %s: %w", sourcePack, err)
	}
	defer rows.Close()
	out := make(map[string]*Trigger)
	for rows.Next() {
		var key, blob string
		if err := rows.Scan(&key, &blob); err != nil {
			return nil, err
		}
		var t Trigger
		if err := json.Unmarshal([]byte(blob), &t); err != nil {
			continue
		}
		out[key] = &t
	}
	return out, rows.Err()
}

// DeletePackBaseline removes a single baseline row.
func (s *Store) DeletePackBaseline(sourcePack, packKey string) error {
	if _, err := s.db.Exec(
		`DELETE FROM pack_baselines WHERE source_pack = ? AND pack_key = ?`,
		sourcePack, packKey,
	); err != nil {
		return fmt.Errorf("delete baseline %s/%s: %w", sourcePack, packKey, err)
	}
	return nil
}

// DeletePackBaselines removes every baseline row for a pack (uninstall, or
// the wipe half of a reinstall).
func (s *Store) DeletePackBaselines(sourcePack string) error {
	if _, err := s.db.Exec(
		`DELETE FROM pack_baselines WHERE source_pack = ?`, sourcePack,
	); err != nil {
		return fmt.Errorf("delete baselines for %s: %w", sourcePack, err)
	}
	return nil
}

// ListBySourcePack returns every trigger installed from the named pack,
// regardless of what display category it has been moved to.
func (s *Store) ListBySourcePack(sourcePack string) ([]*Trigger, error) {
	rows, err := s.db.Query(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition, dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key
		 FROM triggers WHERE source_pack = ? ORDER BY created_at ASC`, sourcePack,
	)
	if err != nil {
		return nil, fmt.Errorf("list triggers for pack %s: %w", sourcePack, err)
	}
	defer rows.Close()
	var triggers []*Trigger
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}

// backfillSourcePack stamps source_pack for built-in pack triggers created
// before the column existed (empty source_pack but a built-in pack_name).
// One-time, ledger-guarded: it must NOT re-run, or a user trigger later
// assigned to a built-in pack's category would be wrongly tagged as
// pack-originated and deleted on uninstall. Going forward only the pack-install
// path sets source_pack.
func (s *Store) backfillSourcePack() error {
	const key = "SourcePack:Backfill:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	for _, p := range AllPacks() {
		if _, err := s.db.Exec(
			`UPDATE triggers SET source_pack = ? WHERE source_pack = '' AND pack_name = ?`,
			p.PackName, p.PackName,
		); err != nil {
			return fmt.Errorf("backfill source_pack for %s: %w", p.PackName, err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateGroupAwarenessToGeneralTriggers is a one-time migration that
// renames any installed "Group Awareness" pack to "General Triggers" and
// inserts the two new class-agnostic triggers (Spell Resist, Spell
// Interrupt) shipped with the rename. Idempotent via the
// pack_default_updates ledger.
//
// Why: the Triggers page used to host a separate "Global Alerts" tab with
// hardcoded death/zone/resist/interrupt event handlers. That subsystem
// was removed; the resist/interrupt cases now live as regular triggers
// inside the renamed pack so users get one unified surface.
func (s *Store) MigrateGroupAwarenessToGeneralTriggers() error {
	const key = "GroupAwareness:RenameAndAddSpellTriggers:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	if _, err := s.db.Exec(
		`UPDATE triggers SET pack_name = 'General Triggers', source_pack = 'General Triggers'
		 WHERE pack_name = 'Group Awareness'`,
	); err != nil {
		return fmt.Errorf("rename Group Awareness pack: %w", err)
	}
	hasPack, err := s.packHasAnyTrigger("General Triggers")
	if err != nil {
		return err
	}
	if hasPack {
		additions := []Trigger{
			{
				Name:       "Spell Resist",
				Enabled:    true,
				Pattern:    `Your target resisted the (.+) spell\.`,
				PackName:   "General Triggers",
				SourcePack: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "RESISTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
			{
				Name:       "Spell Interrupt",
				Enabled:    true,
				Pattern:    `Your(?: (.+))? spell is interrupted\.`,
				PackName:   "General Triggers",
				SourcePack: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INTERRUPTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
		}
		for i := range additions {
			t := &additions[i]
			existing, err := s.FindByPackAndName(t.PackName, t.Name)
			if err != nil {
				return err
			}
			if existing != nil {
				continue
			}
			id, err := NewID()
			if err != nil {
				return err
			}
			t.ID = id
			t.CreatedAt = time.Now().UTC()
			if err := s.Insert(t); err != nil {
				return fmt.Errorf("insert %s: %w", t.Name, err)
			}
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateMezBrokeTTSPronunciation rewrites the TTS text on the installed
// "Mez Broke" triggers from "Mez broke" to "Mezz broke" so Windows SAPI
// reads the EQ term correctly. Idempotent via pack_default_updates.
//
// Built-in pack definitions ship the corrected spelling; this migration
// only matters for users who installed the Enchanter / Bard pack before
// the rename.
func (s *Store) MigrateMezBrokeTTSPronunciation() error {
	const key = "MezBroke:TTSPronunciation:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	for _, packName := range []string{"Enchanter", "Bard"} {
		t, err := s.FindByPackAndName(packName, "Mez Broke")
		if err != nil {
			return err
		}
		if t == nil {
			continue
		}
		changed := false
		for i := range t.Actions {
			if t.Actions[i].Type == ActionTextToSpeech && t.Actions[i].Text == "Mez broke" {
				t.Actions[i].Text = "Mezz broke"
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := s.Update(t); err != nil {
			return fmt.Errorf("update %s mez broke: %w", packName, err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateRemoveDuplicateClassPackTriggers deletes triggers that used to ship
// in class packs but were duplicates of triggers in the General Triggers
// pack — installing both packs produced two overlays per event. As of this
// migration the canonical location for generic resist/interrupt overlays is
// the General Triggers pack; class packs only carry their class-specific
// alerts.
//
// We only delete the specific (PackName, Name) pairs listed below and only
// when the row matches the exact built-in pattern. That avoids clobbering a
// user who manually renamed/customized the trigger — in that case the
// pattern won't match and the row is left alone. Idempotent via
// pack_default_updates.
func (s *Store) MigrateRemoveDuplicateClassPackTriggers() error {
	const key = "ClassPackDupes:RemoveResistInterrupt:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	dupes := []struct {
		packName    string
		triggerName string
		pattern     string // exact pattern of the original built-in row
	}{
		{"Enchanter", "Spell Resisted", `Your target resisted the .+ spell\.`},
		{"Enchanter", "Spell Interrupted", `Your spell is interrupted\.`},
	}
	for _, d := range dupes {
		t, err := s.FindByPackAndName(d.packName, d.triggerName)
		if err != nil {
			return err
		}
		if t == nil {
			continue
		}
		if t.Pattern != d.pattern {
			// User customized the pattern — leave it alone.
			continue
		}
		if err := s.Delete(t.ID); err != nil && err != ErrNotFound {
			return fmt.Errorf("delete duplicate %s/%s: %w", d.packName, d.triggerName, err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateBroadenDebuffPatternsAndBardDurations brings already-installed
// built-in packs up to parity with two corrections shipped after the packs
// were first authored:
//
//   - Every detrimental trigger's cast_on_other branch was broadened from an
//     uppercase-only, single-word target-name class to npcNameClass, so the
//     timer fires on the articled / lowercase / multi-word mobs that make up
//     the vast majority of real targets ("a sand giant has been crippled.").
//     (commits c34016f, a289611)
//   - Eight Bard buff/debuff songs had their hardcoded 54s timer corrected to
//     their true 18s base duration. (commit c34016f)
//
// These edits live in the pack definitions, which only reach user.db at
// import time — so a user who imported a pack before the fix keeps the stale
// pattern/duration in their installed row. This migration patches those rows
// in place so existing installs don't need a destructive re-import.
//
// Only the pattern and timer_duration_secs columns are touched, and only when
// the row still holds the exact pre-fix built-in value. A row a user has
// customized (different pattern or duration) is left untouched, and the user's
// overlay text, TTS actions, and timer alerts are never read or modified
// regardless. Idempotent via pack_default_updates.
func (s *Store) MigrateBroadenDebuffPatternsAndBardDurations() error {
	const key = "Packs:BroadenDebuffPatterns+BardDurations:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	// correction is a guarded in-place fix to one installed pack trigger. An
	// empty newPattern (or zero newDuration) means "leave that column alone".
	type correction struct {
		packName    string
		triggerName string
		oldPattern  string
		newPattern  string
		oldDuration float64
		newDuration float64
	}
	byKey := map[string]*correction{}
	get := func(pack, name string) *correction {
		k := pack + "\x00" + name
		c := byKey[k]
		if c == nil {
			c = &correction{packName: pack, triggerName: name}
			byKey[k] = c
		}
		return c
	}

	// Pattern broadening: find every detrimental trigger whose current
	// built-in pattern embeds npcNameClass and reconstruct the pre-broadening
	// pattern by reversing the substitution. This auto-covers Bard plus the
	// ten class packs touched by a289611 without hand-listing each trigger,
	// and will pick up any future trigger broadened the same way.
	const oldNameClass = `[A-Z][a-zA-Z']{2,14}`
	for _, pack := range AllPacks() {
		for _, tr := range pack.Triggers {
			if !strings.Contains(tr.Pattern, npcNameClass) {
				continue
			}
			c := get(pack.PackName, tr.Name)
			c.oldPattern = strings.ReplaceAll(tr.Pattern, npcNameClass, oldNameClass)
			c.newPattern = tr.Pattern
		}
	}

	// Bard song duration corrections (54s → 18s base). These can't be derived
	// from the current definition, so the affected songs are listed explicitly.
	for _, name := range []string{
		"Cantata of Replenishment",
		"Warsong of Zek",
		"Niv's Melody of Preservation",
		"Psalm of Veeshan",
		"Elemental Rhythms",
		"Guardian Rhythms",
		"Kelin's Lugubrious Lament",
		"Largo's Absonant Binding",
	} {
		c := get("Bard", name)
		c.oldDuration = 54
		c.newDuration = 18
	}

	for _, c := range byKey {
		t, err := s.FindByPackAndName(c.packName, c.triggerName)
		if err != nil {
			return err
		}
		if t == nil {
			// Pack not installed or the trigger was removed — nothing to do.
			continue
		}
		changed := false
		if c.newPattern != "" && t.Pattern == c.oldPattern {
			t.Pattern = c.newPattern
			changed = true
		}
		if c.newDuration != 0 && t.TimerDurationSecs == c.oldDuration {
			t.TimerDurationSecs = c.newDuration
			changed = true
		}
		if !changed {
			// Row missing or user-customized in both columns — leave it alone.
			continue
		}
		if err := s.Update(t); err != nil {
			return fmt.Errorf("update %s/%s: %w", c.packName, c.triggerName, err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateBroadenAssistCallPattern upgrades the Raid Alerts "Raid Assist Call"
// trigger installed before it learned the "kill" calling style (e.g.
// "'< --- Kill Qua Zethon Xakra -->'") and the dash-arrow decorations. Packs
// only reach user.db at import time, so a user who imported Raid Alerts earlier
// keeps the assist-only pattern. This rewrites that one row in place when it
// still holds the exact pre-fix built-in value; a user-customized pattern is
// left untouched, and the row's actions are never read or modified. Idempotent
// via pack_default_updates.
func (s *Store) MigrateBroadenAssistCallPattern() error {
	const key = "Packs:BroadenAssistCallPattern:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	// Verbatim pre-fix and current built-in patterns (see RaidAlertsPack).
	const oldPattern = "(?i)^(\\w+) tells the raid,\\s*'.*?assist\\W*([A-Za-z][A-Za-z`' ]*?)(?:\\s*[>|<!']|$)"
	const newPattern = "(?i)^(\\w+) tells the raid,\\s*'.*?\\b(?:assist|kill)\\b\\W*([A-Za-z][A-Za-z`' ]*?)(?:\\s*[-<>|!']|$)"

	t, err := s.FindByPackAndName("Raid Alerts", "Raid Assist Call")
	if err != nil {
		return err
	}
	if t != nil && t.Pattern == oldPattern {
		t.Pattern = newPattern
		if err := s.Update(t); err != nil {
			return fmt.Errorf("update Raid Alerts/Raid Assist Call: %w", err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateAddBuffTargetCapture upgrades buff triggers installed before the
// target-capture feature so they show the "on <target>" overlay suffix when a
// group buff lands on someone. The current built-in pattern (from AllPacks,
// post-applyBuffTargetCapture) wraps the cast-on-other name in a (?P<target>…)
// group and sets timer_target_capture="target"; the pre-feature row has the
// same pattern with the bare name class and an empty capture. We derive that
// old form by unwrapping and patch only rows that still hold it (and haven't
// already got a target capture), so a user's customized pattern is left alone.
// Idempotent via the pack_default_updates ledger.
func (s *Store) MigrateAddBuffTargetCapture() error {
	const key = "Packs:BuffTargetCapture:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	for _, pack := range AllPacks() {
		for _, tr := range pack.Triggers {
			if tr.TimerTargetCapture == "" || !strings.Contains(tr.Pattern, targetCaptureGroup) {
				continue
			}
			oldPattern := strings.Replace(tr.Pattern, targetCaptureGroup, playerNameClass, 1)
			installed, err := s.FindByPackAndName(pack.PackName, tr.Name)
			if err != nil {
				return err
			}
			if installed == nil {
				continue // pack not installed or trigger removed
			}
			// Only touch an untouched built-in row: still the pre-feature
			// pattern, no target capture yet. A user-edited pattern or an
			// already-set capture is left alone.
			if installed.Pattern != oldPattern || installed.TimerTargetCapture != "" {
				continue
			}
			installed.Pattern = tr.Pattern
			installed.TimerTargetCapture = "target"
			if err := s.Update(installed); err != nil {
				return fmt.Errorf("update %s/%s: %w", pack.PackName, tr.Name, err)
			}
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

func (s *Store) packHasAnyTrigger(pack string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM triggers WHERE pack_name = ?`, pack).Scan(&n); err != nil {
		return false, fmt.Errorf("count pack %s: %w", pack, err)
	}
	return n > 0, nil
}

// BackfillCharactersIfNeeded is a one-time migration that populates the
// characters list of every existing trigger with the supplied character
// names. Triggered by PRAGMA user_version: runs only when the version is
// below 1, then bumps it. Safe to call on every startup. Skips entirely
// when names is empty so we don't lock in "no characters" before any have
// been recorded.
func (s *Store) BackfillCharactersIfNeeded(names []string) error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= 1 {
		return nil
	}
	if len(names) == 0 {
		// No characters yet — defer migration to a future startup so we don't
		// permanently lock existing triggers into an empty (= all) list.
		return nil
	}
	payload, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("marshal names: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE triggers SET characters = ? WHERE characters = '[]' OR characters = '' OR characters IS NULL`, string(payload)); err != nil {
		return fmt.Errorf("backfill characters: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("bump user_version: %w", err)
	}
	return nil
}

// execer is the subset of *sql.DB / *sql.Tx that insertWith needs, so a
// single insert body serves both the standalone Insert and the transactional
// InsertMany.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Insert saves a new trigger to the database.
func (s *Store) Insert(t *Trigger) error { return s.insertWith(s.db, t) }

// InsertMany inserts every trigger in one transaction: either all land or none
// do. Used by the import wizard so a mid-batch failure can't leave a partial
// import behind (a retry would otherwise re-insert the survivors under fresh
// IDs, duplicating them).
func (s *Store) InsertMany(ts []*Trigger) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, t := range ts {
		if err := s.insertWith(tx, t); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) insertWith(ex execer, t *Trigger) error {
	actJSON, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	charJSON, err := json.Marshal(t.Characters)
	if err != nil {
		return fmt.Errorf("marshal characters: %w", err)
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	alertJSON, err := json.Marshal(t.TimerAlerts)
	if err != nil {
		return fmt.Errorf("marshal timer_alerts: %w", err)
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	excludeJSON, err := json.Marshal(t.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("marshal exclude_patterns: %w", err)
	}
	if t.ExtraPatterns == nil {
		t.ExtraPatterns = []ExtraPattern{}
	}
	extraJSON, err := json.Marshal(t.ExtraPatterns)
	if err != nil {
		return fmt.Errorf("marshal extra_patterns: %w", err)
	}
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	source, pipeJSON := normalizeSourceAndCondition(t)
	_, err = ex.Exec(
		`INSERT INTO triggers (id, name, enabled, pattern, actions, pack_name, created_at,
		                       timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		                       display_threshold_secs, characters, timer_alerts, exclude_patterns,
		                       extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition,
		                       dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName, t.CreatedAt.Unix(),
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
		string(extraJSON), t.TimerDurationCapture, t.TimerKeyCapture, t.TimerTargetCapture, source, pipeJSON, t.DedupKey, t.CooldownSecs, t.SortOrder, t.SourcePack, t.BarColor, t.RefireCooldownSecs, t.PackKey,
	)
	if err != nil {
		return fmt.Errorf("insert trigger: %w", err)
	}
	return nil
}

// normalizeSourceAndCondition resolves the persisted Source value (defaults
// to "log" when empty) and marshals the optional PipeCondition to JSON. A
// nil PipeCondition serializes as the empty string so the column reads back
// cleanly. Pipe-source triggers must have a non-nil PipeCondition; the
// engine ignores any whose JSON fails to parse.
func normalizeSourceAndCondition(t *Trigger) (string, string) {
	src := t.Source
	if src == "" {
		src = SourceLog
	}
	if t.PipeCondition == nil {
		return src, ""
	}
	b, err := json.Marshal(t.PipeCondition)
	if err != nil {
		return src, ""
	}
	return src, string(b)
}

// List returns all triggers ordered by creation time ascending.
func (s *Store) List() ([]*Trigger, error) {
	rows, err := s.db.Query(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition, dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key
		 FROM triggers ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	defer rows.Close()

	var triggers []*Trigger
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}

// Get returns the trigger with the given ID, or ErrNotFound.
func (s *Store) Get(id string) (*Trigger, error) {
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition, dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key
		 FROM triggers WHERE id = ?`, id,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get trigger %s: %w", id, err)
	}
	return t, nil
}

// Update saves changes to an existing trigger.
func (s *Store) Update(t *Trigger) error {
	actJSON, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	charJSON, err := json.Marshal(t.Characters)
	if err != nil {
		return fmt.Errorf("marshal characters: %w", err)
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	alertJSON, err := json.Marshal(t.TimerAlerts)
	if err != nil {
		return fmt.Errorf("marshal timer_alerts: %w", err)
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	excludeJSON, err := json.Marshal(t.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("marshal exclude_patterns: %w", err)
	}
	if t.ExtraPatterns == nil {
		t.ExtraPatterns = []ExtraPattern{}
	}
	extraJSON, err := json.Marshal(t.ExtraPatterns)
	if err != nil {
		return fmt.Errorf("marshal extra_patterns: %w", err)
	}
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	source, pipeJSON := normalizeSourceAndCondition(t)
	res, err := s.db.Exec(
		`UPDATE triggers SET name=?, enabled=?, pattern=?, actions=?, pack_name=?,
		                     timer_type=?, timer_duration_secs=?, worn_off_pattern=?, spell_id=?,
		                     display_threshold_secs=?, characters=?, timer_alerts=?, exclude_patterns=?,
		                     extra_patterns=?, timer_duration_capture=?, timer_key_capture=?, timer_target_capture=?, source=?, pipe_condition=?,
		                     dedup_key=?, cooldown_secs=?, sort_order=?, source_pack=?, bar_color=?, refire_cooldown_secs=?, pack_key=?
		 WHERE id=?`,
		t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName,
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
		string(extraJSON), t.TimerDurationCapture, t.TimerKeyCapture, t.TimerTargetCapture, source, pipeJSON, t.DedupKey, t.CooldownSecs, t.SortOrder, t.SourcePack, t.BarColor, t.RefireCooldownSecs, t.PackKey,
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("update trigger: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the trigger with the given ID.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM triggers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete trigger: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// FindByPackAndName returns the (single) trigger with the given pack name
// and trigger name, or (nil, nil) if no match exists. Used by the default-
// updates pipeline to locate built-in pack triggers without iterating the
// whole list at the call site. Returns the first match if duplicates somehow
// exist; the trigger UI prevents duplicate names within a pack.
func (s *Store) FindByPackAndName(packName, name string) (*Trigger, error) {
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition, dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key
		 FROM triggers WHERE pack_name = ? AND name = ? LIMIT 1`,
		packName, name,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find trigger %s/%s: %w", packName, name, err)
	}
	return t, nil
}

// FindByDedupKey returns the trigger that currently owns the given
// dedup_key, or (nil, nil) when no trigger has claimed it. Used by
// InstallPack to decide whether to skip a pack trigger because another
// pack already provides the same conceptual entry (e.g. Root shared by
// Wizard and Enchanter packs), and by DeleteByPack's promote-on-uninstall
// path to detect orphaned keys.
func (s *Store) FindByDedupKey(key string) (*Trigger, error) {
	if key == "" {
		return nil, nil
	}
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        extra_patterns, timer_duration_capture, timer_key_capture, timer_target_capture, source, pipe_condition, dedup_key, cooldown_secs, sort_order, source_pack, bar_color, refire_cooldown_secs, pack_key
		 FROM triggers WHERE dedup_key = ? LIMIT 1`, key,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find trigger by dedup_key %s: %w", key, err)
	}
	return t, nil
}

// IsDefaultUpdateApplied reports whether the named default-update key has
// already run. ApplyDefaultUpdates uses this to keep one-time additive
// migrations idempotent across restarts.
func (s *Store) IsDefaultUpdateApplied(key string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pack_default_updates WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check default update %s: %w", key, err)
	}
	return n > 0, nil
}

// MarkDefaultUpdateApplied records that the named default-update key has
// run. Idempotent (INSERT OR IGNORE) so a duplicate call from a parallel
// startup path doesn't error.
func (s *Store) MarkDefaultUpdateApplied(key string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO pack_default_updates (key, applied_at) VALUES (?, ?)`,
		key, time.Now().UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("mark default update %s: %w", key, err)
	}
	return nil
}

// InstalledPackNames returns the set of distinct source_pack values that
// currently have at least one trigger in the store. Keyed on source_pack (the
// install origin), not pack_name (the display category), so a pack still reads
// as installed after its triggers are moved into custom categories. Empty
// source_pack (user-authored triggers) is excluded. Used by UninstallPack to
// know which other packs are candidates for promoting orphaned dedup_keys.
func (s *Store) InstalledPackNames() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT DISTINCT source_pack FROM triggers WHERE source_pack <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list installed packs: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// DeleteByPack removes every trigger installed from the named pack, keyed on
// source_pack so triggers moved into other categories are still removed when
// the pack is deactivated.
func (s *Store) DeleteByPack(packName string) error {
	_, err := s.db.Exec(`DELETE FROM triggers WHERE source_pack = ?`, packName)
	if err != nil {
		return fmt.Errorf("delete pack %s: %w", packName, err)
	}
	return nil
}

// DeleteAll removes every trigger in one statement. Used by the Clear All
// flow on the Triggers page so the frontend doesn't have to fan out N
// per-id deletes (each of which would otherwise trigger its own engine
// reload on the API side).
func (s *Store) DeleteAll() error {
	if _, err := s.db.Exec(`DELETE FROM triggers`); err != nil {
		return fmt.Errorf("delete all triggers: %w", err)
	}
	return nil
}

// NextTriggerSortOrder returns one past the highest sort_order among triggers
// in the given category (empty packName = Uncategorized), so a freshly created
// or moved trigger appends to the end of that category's manual order.
func (s *Store) NextTriggerSortOrder(packName string) (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRow(
		`SELECT MAX(sort_order) FROM triggers WHERE pack_name = ?`, packName,
	).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("next trigger sort order for %q: %w", packName, err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// ReorderTriggers rewrites the manual sort_order of the given trigger IDs to
// match their position in ids (0-based). Used by the Manual sort mode's
// drag-to-reorder. IDs are updated in a single transaction; unknown IDs are
// no-ops.
func (s *Store) ReorderTriggers(ids []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder triggers: %w", err)
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE triggers SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return fmt.Errorf("reorder trigger %s: %w", id, err)
		}
	}
	return tx.Commit()
}

type scanner interface {
	Scan(...any) error
}

func scanTrigger(row scanner) (*Trigger, error) {
	var t Trigger
	var enabledInt int
	var actJSON, charJSON, alertJSON, excludeJSON, extraJSON, source, pipeJSON string
	var unixSec int64
	var timerType string
	if err := row.Scan(
		&t.ID, &t.Name, &enabledInt, &t.Pattern, &actJSON, &t.PackName, &unixSec,
		&timerType, &t.TimerDurationSecs, &t.WornOffPattern, &t.SpellID,
		&t.DisplayThresholdSecs, &charJSON, &alertJSON, &excludeJSON,
		&extraJSON, &t.TimerDurationCapture, &t.TimerKeyCapture, &t.TimerTargetCapture, &source, &pipeJSON, &t.DedupKey, &t.CooldownSecs, &t.SortOrder, &t.SourcePack, &t.BarColor, &t.RefireCooldownSecs, &t.PackKey,
	); err != nil {
		return nil, err
	}
	t.Source = source
	if t.Source == "" {
		t.Source = SourceLog
	}
	if pipeJSON != "" {
		var pc PipeCondition
		if err := json.Unmarshal([]byte(pipeJSON), &pc); err == nil && pc.Kind != "" {
			t.PipeCondition = &pc
		}
	}
	t.Enabled = enabledInt != 0
	t.CreatedAt = time.Unix(unixSec, 0).UTC()
	t.TimerType = TimerType(timerType)
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	if err := json.Unmarshal([]byte(actJSON), &t.Actions); err != nil {
		t.Actions = []Action{}
	}
	if t.Actions == nil {
		t.Actions = []Action{}
	}
	if charJSON != "" {
		if err := json.Unmarshal([]byte(charJSON), &t.Characters); err != nil {
			t.Characters = []string{}
		}
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	if alertJSON != "" {
		if err := json.Unmarshal([]byte(alertJSON), &t.TimerAlerts); err != nil {
			t.TimerAlerts = []TimerAlert{}
		}
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	if excludeJSON != "" {
		if err := json.Unmarshal([]byte(excludeJSON), &t.ExcludePatterns); err != nil {
			t.ExcludePatterns = []string{}
		}
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	if extraJSON != "" {
		if err := json.Unmarshal([]byte(extraJSON), &t.ExtraPatterns); err != nil {
			t.ExtraPatterns = []ExtraPattern{}
		}
	}
	if t.ExtraPatterns == nil {
		t.ExtraPatterns = []ExtraPattern{}
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

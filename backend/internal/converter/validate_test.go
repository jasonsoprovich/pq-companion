package converter

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T, stmts ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return db
}

// Schema used by the referential / spot-check tests — minimal but matches real column names.
const testSchemaSQL = `
CREATE TABLE items (id INTEGER PRIMARY KEY, Name TEXT, lore TEXT);
CREATE TABLE zone (id INTEGER PRIMARY KEY, short_name TEXT, long_name TEXT);
CREATE TABLE spells_new (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE npc_types (id INTEGER PRIMARY KEY, name TEXT, loottable_id INTEGER, npc_spells_id INTEGER);
CREATE TABLE spawn2 (id INTEGER PRIMARY KEY, zone TEXT);
CREATE TABLE spawnentry (spawngroupID INTEGER, npcID INTEGER);
CREATE TABLE spawngroup (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE lootdrop (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE lootdrop_entries (lootdrop_id INTEGER, item_id INTEGER);
CREATE TABLE loottable (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE loottable_entries (loottable_id INTEGER, lootdrop_id INTEGER);
CREATE TABLE npc_spells (id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE npc_spells_entries (npc_spells_id INTEGER, spellid INTEGER);
CREATE TABLE skill_caps (skill_id INTEGER, class_id INTEGER, level INTEGER, cap INTEGER);
`

func TestValidate_EmptyDatabase(t *testing.T) {
	db := newTestDB(t, testSchemaSQL)

	rep, err := Validate(context.Background(), Config{}, db)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Every row count should fail because every table is empty. Spot checks fail too.
	if rep.Errors == 0 {
		t.Fatalf("expected errors on empty database, got none (report=%+v)", rep)
	}
}

func TestValidate_OrphansBelowThresholdWarn(t *testing.T) {
	// Set up: one valid and one orphan spawn entry. Orphans (1) are below the 500 threshold,
	// so the referential check should report a warning, not an error.
	stmts := []string{
		testSchemaSQL,
		`INSERT INTO npc_types (id, name) VALUES (1, 'Fippy')`,
		`INSERT INTO spawngroup (id, name) VALUES (10, 'sg')`,
		`INSERT INTO spawnentry (spawngroupID, npcID) VALUES (10, 1)`,   // valid
		`INSERT INTO spawnentry (spawngroupID, npcID) VALUES (10, 999)`, // orphan npcID
	}
	db := newTestDB(t, stmts...)

	rep, err := Validate(context.Background(), Config{}, db)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Locate the FK check result.
	var got *CheckResult
	for i := range rep.Checks {
		if rep.Checks[i].Name == "spawnentry.npcID → npc_types.id" {
			got = &rep.Checks[i]
			break
		}
	}
	if got == nil {
		t.Fatal("referential check result missing from report")
	}
	if got.Severity != SeverityWarning {
		t.Errorf("severity = %q, want %q", got.Severity, SeverityWarning)
	}
	if got.Count != 1 {
		t.Errorf("count = %d, want 1", got.Count)
	}
}

func TestValidate_IgnoreZeroFK(t *testing.T) {
	// npc_types.loottable_id = 0 means "no loot" and must not be counted as an orphan.
	stmts := []string{
		testSchemaSQL,
		`INSERT INTO loottable (id, name) VALUES (5, 'lt5')`,
		`INSERT INTO npc_types (id, name, loottable_id) VALUES (1, 'a', 0)`, // sentinel, not orphan
		`INSERT INTO npc_types (id, name, loottable_id) VALUES (2, 'b', 5)`, // valid
	}
	db := newTestDB(t, stmts...)

	rep, err := Validate(context.Background(), Config{}, db)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for _, c := range rep.Checks {
		if c.Name == "npc_types.loottable_id → loottable.id" {
			if c.Severity != SeverityOK {
				t.Errorf("loottable_id FK check severity = %q, want %q (count=%d)",
					c.Severity, SeverityOK, c.Count)
			}
			return
		}
	}
	t.Fatal("loottable_id FK check missing from report")
}

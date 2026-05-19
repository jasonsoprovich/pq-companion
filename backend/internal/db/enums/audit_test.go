package enums

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// makeTestDB builds an in-memory SQLite database with the subset of
// quarm.db schema the audit relies on, seeded with rows that exercise
// both clean and unknown-code paths.
func makeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE tradeskill_recipe (id INTEGER PRIMARY KEY, tradeskill INTEGER NOT NULL, enabled INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE npc_types (id INTEGER PRIMARY KEY, special_abilities TEXT, class INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE items (id INTEGER PRIMARY KEY, itemtype INTEGER NOT NULL DEFAULT 0)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	return db
}

func TestRunAudit_CleanDatabase(t *testing.T) {
	db := makeTestDB(t)
	mustExec(t, db, `INSERT INTO tradeskill_recipe (id, tradeskill) VALUES (1, 55), (2, 61), (3, 75)`)
	mustExec(t, db, `INSERT INTO npc_types (id, special_abilities) VALUES (1, '1,1^18,1'), (2, '54,2')`)

	findings, err := RunAudit(db)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	if len(findings) != len(Defs()) {
		t.Fatalf("got %d findings, want %d", len(findings), len(Defs()))
	}
	for _, f := range findings {
		if len(f.Unknown) != 0 {
			t.Errorf("%s: unexpected unknowns %v", f.Name, f.Unknown)
		}
	}
}

func TestRunAudit_FlagsUnknownTradeskill(t *testing.T) {
	db := makeTestDB(t)
	mustExec(t, db, `INSERT INTO tradeskill_recipe (id, tradeskill) VALUES (1, 55), (2, 999)`)

	findings, err := RunAudit(db)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	ts := findingByName(findings, "Tradeskill")
	if ts == nil {
		t.Fatal("no Tradeskill finding")
	}
	if len(ts.Unknown) != 1 || ts.Unknown[0] != 999 {
		t.Errorf("Tradeskill.Unknown = %v, want [999]", ts.Unknown)
	}
}

func TestRunAudit_FlagsUnknownSpecialAbility(t *testing.T) {
	db := makeTestDB(t)
	mustExec(t, db, `INSERT INTO npc_types (id, special_abilities) VALUES (1, '1,1^77,1^200,3')`)

	findings, err := RunAudit(db)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	sa := findingByName(findings, "Special Ability")
	if sa == nil {
		t.Fatal("no Special Ability finding")
	}
	wantUnknown := []int{77, 200}
	if len(sa.Unknown) != len(wantUnknown) {
		t.Fatalf("Special Ability.Unknown = %v, want %v", sa.Unknown, wantUnknown)
	}
	for i, code := range wantUnknown {
		if sa.Unknown[i] != code {
			t.Errorf("Special Ability.Unknown[%d] = %d, want %d", i, sa.Unknown[i], code)
		}
	}
}

func TestRunAudit_IgnoresDisabledRecipes(t *testing.T) {
	db := makeTestDB(t)
	// 999 is unknown but disabled — must not be flagged.
	mustExec(t, db, `INSERT INTO tradeskill_recipe (id, tradeskill, enabled) VALUES (1, 55, 1), (2, 999, 0)`)

	findings, err := RunAudit(db)
	if err != nil {
		t.Fatalf("RunAudit: %v", err)
	}
	ts := findingByName(findings, "Tradeskill")
	if len(ts.Unknown) != 0 {
		t.Errorf("Tradeskill.Unknown = %v, want empty (disabled recipe ignored)", ts.Unknown)
	}
}

func TestRunAudit_SyntheticSpecialAbilityCodesNotInKnownSet(t *testing.T) {
	// Synthetics (1001+) live only in overlay logic; if a DB row ever
	// carried one we want it flagged, not silently accepted.
	if _, ok := SpecialAbilitiesAudit.KnownCodes[SyntheticSeeInvis]; ok {
		t.Errorf("SyntheticSeeInvis (%d) should not be in KnownCodes", SyntheticSeeInvis)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func findingByName(findings []AuditFinding, name string) *AuditFinding {
	for i := range findings {
		if findings[i].Name == name {
			return &findings[i]
		}
	}
	return nil
}

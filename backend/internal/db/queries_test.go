package db_test

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// dbPath returns the path to the test database relative to this file.
func dbPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// This file is at backend/internal/db/; quarm.db is at backend/data/
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db")
}

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(dbPath(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// ─── Items ────────────────────────────────────────────────────────────────────

func TestGetItem_Found(t *testing.T) {
	d := openTestDB(t)
	// ID 1000 is a well-known item in EQ data; use a search to find a valid ID first.
	res, err := d.SearchItems(db.ItemFilter{Query: "Sword", ItemType: -1, Limit: 1})
	if err != nil {
		t.Fatalf("search items: %v", err)
	}
	if len(res.Items) == 0 {
		t.Skip("no items named Sword in DB")
	}
	id := res.Items[0].ID
	item, err := d.GetItem(id)
	if err != nil {
		t.Fatalf("GetItem(%d): %v", id, err)
	}
	if item.ID != id {
		t.Errorf("got id %d, want %d", item.ID, id)
	}
	if item.Name == "" {
		t.Errorf("item name is empty")
	}
}

func TestGetItem_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetItem(-1)
	if err == nil {
		t.Fatal("expected error for missing item, got nil")
	}
}

func TestSearchItems(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantMin int // at least this many results
	}{
		{"broad search", "a", 1},
		{"sword", "Sword", 1},
		{"no results", "xyzzy_notanitem_zyx", 0},
	}
	d := openTestDB(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := d.SearchItems(db.ItemFilter{Query: tt.query, ItemType: -1, Limit: 20})
			if err != nil {
				t.Fatalf("SearchItems(%q): %v", tt.query, err)
			}
			if res.Total < tt.wantMin {
				t.Errorf("total=%d, want >= %d", res.Total, tt.wantMin)
			}
			if len(res.Items) > 20 {
				t.Errorf("returned %d items, limit is 20", len(res.Items))
			}
		})
	}
}

func TestSearchItems_Pagination(t *testing.T) {
	d := openTestDB(t)
	page1, err := d.SearchItems(db.ItemFilter{Query: "a", ItemType: -1, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) == 0 {
		t.Skip("no items for pagination test")
	}
	page2, err := d.SearchItems(db.ItemFilter{Query: "a", ItemType: -1, Limit: 5, Offset: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Items) > 0 && len(page2.Items) > 0 {
		if page1.Items[0].ID == page2.Items[0].ID {
			t.Error("page1 and page2 returned the same first item")
		}
	}
}

// ─── NPCs ─────────────────────────────────────────────────────────────────────

func TestGetNPC_Found(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchNPCs("gnoll", 1, 0, false)
	if err != nil {
		t.Fatalf("search npcs: %v", err)
	}
	if len(res.Items) == 0 {
		t.Skip("no gnoll NPCs in DB")
	}
	id := res.Items[0].ID
	npc, err := d.GetNPC(id)
	if err != nil {
		t.Fatalf("GetNPC(%d): %v", id, err)
	}
	if npc.ID != id {
		t.Errorf("got id %d, want %d", npc.ID, id)
	}
	if npc.Name == "" {
		t.Errorf("npc name is empty")
	}
}

func TestGetNPC_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetNPC(-1)
	if err == nil {
		t.Fatal("expected error for missing npc, got nil")
	}
}

func TestSearchNPCs(t *testing.T) {
	d := openTestDB(t)
	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{"gnoll", "gnoll", 1},
		{"no results", "xyzzy_notnpc_zyx", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := d.SearchNPCs(tt.query, 20, 0, false)
			if err != nil {
				t.Fatalf("SearchNPCs(%q): %v", tt.query, err)
			}
			if res.Total < tt.wantMin {
				t.Errorf("total=%d, want >= %d", res.Total, tt.wantMin)
			}
		})
	}
}

// ─── Spells ───────────────────────────────────────────────────────────────────

func TestGetSpell_Found(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchSpells("Fire", -1, 0, 0, 1, 0)
	if err != nil {
		t.Fatalf("search spells: %v", err)
	}
	if len(res.Items) == 0 {
		t.Skip("no Fire spells in DB")
	}
	id := res.Items[0].ID
	sp, err := d.GetSpell(id)
	if err != nil {
		t.Fatalf("GetSpell(%d): %v", id, err)
	}
	if sp.ID != id {
		t.Errorf("got id %d, want %d", sp.ID, id)
	}
	if sp.Name == "" {
		t.Errorf("spell name is empty")
	}
}

func TestGetSpell_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetSpell(-1)
	if err == nil {
		t.Fatal("expected error for missing spell, got nil")
	}
}

func TestSearchSpells(t *testing.T) {
	d := openTestDB(t)
	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{"mesmerize", "Mesmer", 1},
		{"no results", "xyzzy_notspell_zyx", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := d.SearchSpells(tt.query, -1, 0, 0, 20, 0)
			if err != nil {
				t.Fatalf("SearchSpells(%q): %v", tt.query, err)
			}
			if res.Total < tt.wantMin {
				t.Errorf("total=%d, want >= %d", res.Total, tt.wantMin)
			}
		})
	}
}

// ─── Zones ────────────────────────────────────────────────────────────────────

func TestGetZoneByShortName(t *testing.T) {
	d := openTestDB(t)
	// "qeynos" is a classic EQ zone that should always be present.
	z, err := d.GetZoneByShortName("qeynos")
	if err == sql.ErrNoRows {
		t.Skip("qeynos zone not found in DB")
	}
	if err != nil {
		t.Fatalf("GetZoneByShortName: %v", err)
	}
	if z.ShortName != "qeynos" {
		t.Errorf("got short_name %q, want %q", z.ShortName, "qeynos")
	}
	// Verify zone attribute fields are scanned into the correct struct fields.
	// qeynos has hotzone=1, outdoor=1, exp_mod=1.0 in the DB.
	// A misaligned scanZone would silently produce wrong values here.
	if z.Hotzone != 1 {
		t.Errorf("hotzone: got %d, want 1", z.Hotzone)
	}
	if z.Outdoor != 1 {
		t.Errorf("outdoor: got %d, want 1", z.Outdoor)
	}
	if z.ExpMod != 1.0 {
		t.Errorf("exp_mod: got %f, want 1.0", z.ExpMod)
	}
}

func TestGetZone_NotFound(t *testing.T) {
	d := openTestDB(t)
	_, err := d.GetZone(-1)
	if err == nil {
		t.Fatal("expected error for missing zone, got nil")
	}
}

func TestSearchZones(t *testing.T) {
	d := openTestDB(t)
	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{"karana", "Karana", 1},
		{"no results", "xyzzy_notzone_zyx", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := d.SearchZones(tt.query, 20, 0)
			if err != nil {
				t.Fatalf("SearchZones(%q): %v", tt.query, err)
			}
			if res.Total < tt.wantMin {
				t.Errorf("total=%d, want >= %d", res.Total, tt.wantMin)
			}
		})
	}
}

// ─── Special Abilities ────────────────────────────────────────────────────────

func TestParseSpecialAbilities(t *testing.T) {
	tests := []struct {
		raw     string
		wantLen int
		wantFirst db.SpecialAbility
	}{
		{
			raw:     "1,1^18,1^19,1",
			wantLen: 3,
			wantFirst: db.SpecialAbility{Code: 1, Value: 1, Name: "Summon"},
		},
		{
			raw:     "",
			wantLen: 0,
		},
		{
			raw:     "2,1^4,1",
			wantLen: 2,
			wantFirst: db.SpecialAbility{Code: 2, Value: 1, Name: "Enrage"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := db.ParseSpecialAbilities(tt.raw)
			if len(got) != tt.wantLen {
				t.Errorf("len=%d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 {
				if got[0] != tt.wantFirst {
					t.Errorf("first=%+v, want %+v", got[0], tt.wantFirst)
				}
			}
		})
	}
}

func TestHasSpecialAbility(t *testing.T) {
	raw := "1,1^18,1^19,1"
	if !db.HasSpecialAbility(raw, 1) {
		t.Error("expected Summon (1) to be present")
	}
	if db.HasSpecialAbility(raw, 2) {
		t.Error("expected Enrage (2) to be absent")
	}
}

func TestItemIcons(t *testing.T) {
	d := openTestDB(t)
	// Empty input → empty result, no error.
	got, err := d.ItemIcons(nil)
	if err != nil {
		t.Fatalf("ItemIcons(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ItemIcons(nil) = %v, want empty map", got)
	}
	// Pick a few known IDs from the items table that have non-zero icons.
	rows, err := d.Query("SELECT id, icon FROM items WHERE icon > 0 LIMIT 5")
	if err != nil {
		t.Fatalf("seed query: %v", err)
	}
	want := map[int]int{}
	var ids []int
	for rows.Next() {
		var id, icon int
		if err := rows.Scan(&id, &icon); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		want[id] = icon
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		t.Skip("no items with icons in DB")
	}
	// Add a non-existent ID — it should be silently omitted.
	ids = append(ids, -1)
	got, err = d.ItemIcons(ids)
	if err != nil {
		t.Fatalf("ItemIcons: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("got %d icons, want %d", len(got), len(want))
	}
	for id, icon := range want {
		if got[id] != icon {
			t.Errorf("ItemIcons[%d] = %d, want %d", id, got[id], icon)
		}
	}
}

func TestNPCSpecialAbilities_RealDB(t *testing.T) {
	d := openTestDB(t)
	// Find any NPC that has special abilities set.
	var raw string
	err := d.QueryRow(
		"SELECT COALESCE(special_abilities,'') FROM npc_types WHERE special_abilities != '' AND special_abilities IS NOT NULL LIMIT 1",
	).Scan(&raw)
	if err == sql.ErrNoRows {
		t.Skip("no NPCs with special_abilities in DB")
	}
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	abilities := db.ParseSpecialAbilities(raw)
	if len(abilities) == 0 {
		t.Errorf("ParseSpecialAbilities(%q) returned empty for non-empty raw value", raw)
	}
}

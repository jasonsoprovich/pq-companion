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

// Kaas Thox Xi Aten Ha Ra is a canonical same-zone collision: two distinct
// raid bosses in Vex Thal sharing one name, differing only in loottable_id
// and physical spawn point. Disambiguating the two requires both rows plus
// their spawn coordinates — exactly what GetNPCVariantsByNameInZone returns.
func TestGetNPCVariantsByNameInZone_KaasThoxBothVariants(t *testing.T) {
	d := openTestDB(t)
	variants, err := d.GetNPCVariantsByNameInZone("Kaas_Thox_Xi_Aten_Ha_Ra", "vexthal")
	if err != nil {
		t.Fatalf("GetNPCVariantsByNameInZone: %v", err)
	}
	if len(variants) != 2 {
		t.Fatalf("got %d variants, want 2", len(variants))
	}
	for _, v := range variants {
		if len(v.SpawnPoints) == 0 {
			t.Errorf("variant id=%d has no spawn points", v.NPC.ID)
		}
	}
	// The two variants are at y=318 and y=-321 respectively — distinct enough
	// that a player-position match will reliably pick one.
	ys := []float64{variants[0].SpawnPoints[0].Y, variants[1].SpawnPoints[0].Y}
	if (ys[0] > 0) == (ys[1] > 0) {
		t.Errorf("expected variants on opposite sides of y=0, got y=%v, y=%v", ys[0], ys[1])
	}
	// Loot tables must differ — that's the whole reason this collision matters.
	if variants[0].NPC.LootTableID == variants[1].NPC.LootTableID {
		t.Errorf("variants have same loottable %d, expected divergent tables", variants[0].NPC.LootTableID)
	}
}

// Shissar revenant in ssraeshza_temple is the other canonical case: two
// variants (necromancer + shadow knight) that share a spawngroup. They come
// back as separate npc_types rows but their SpawnPoints overlap, so the
// caller must detect the shared-spawngroup case and surface both rather than
// pick one.
func TestGetNPCVariantsByNameInZone_ShissarSharedSpawngroup(t *testing.T) {
	d := openTestDB(t)
	variants, err := d.GetNPCVariantsByNameInZone("A_Shissar_Revenant", "ssratemple")
	if err != nil {
		t.Fatalf("GetNPCVariantsByNameInZone: %v", err)
	}
	if len(variants) != 2 {
		t.Fatalf("got %d variants, want 2", len(variants))
	}
	// The class field is what differs: necro=11 vs SK=5. If both come back
	// with the same class the join collapsed the variants somehow.
	if variants[0].NPC.Class == variants[1].NPC.Class {
		t.Errorf("variants have same class %d, expected different classes", variants[0].NPC.Class)
	}
	// Both variants must occupy spawngroup 162197 — that's what makes this
	// case truly ambiguous from position alone.
	gotShared := false
	for _, sp1 := range variants[0].SpawnPoints {
		for _, sp2 := range variants[1].SpawnPoints {
			if sp1.SpawngroupID == sp2.SpawngroupID {
				gotShared = true
			}
		}
	}
	if !gotShared {
		t.Error("expected variants to share at least one spawngroup, got none")
	}
}

// When no zone filter is given, the query should still return all variants
// matching the name globally — but without spawn points. This is the
// "no Zeal data, name only" fallback path used when the pipe is offline.
func TestGetNPCVariantsByNameInZone_NoZoneReturnsAllNoSpawns(t *testing.T) {
	d := openTestDB(t)
	variants, err := d.GetNPCVariantsByNameInZone("A_Shissar_Revenant", "")
	if err != nil {
		t.Fatalf("GetNPCVariantsByNameInZone: %v", err)
	}
	if len(variants) < 2 {
		t.Fatalf("got %d variants, want at least 2 (necro + SK in ssratemple)", len(variants))
	}
	for _, v := range variants {
		if len(v.SpawnPoints) != 0 {
			t.Errorf("variant id=%d has %d spawn points, want 0 (no zone filter)", v.NPC.ID, len(v.SpawnPoints))
		}
	}
}

// Empty result is not an error — the caller treats it like "no DB record",
// the same as GetNPCByName returning sql.ErrNoRows today.
func TestGetNPCVariantsByNameInZone_NoMatch(t *testing.T) {
	d := openTestDB(t)
	variants, err := d.GetNPCVariantsByNameInZone("a_nonexistent_mob_xyzzy", "qeynos")
	if err != nil {
		t.Fatalf("GetNPCVariantsByNameInZone: %v", err)
	}
	if len(variants) != 0 {
		t.Errorf("got %d variants, want 0", len(variants))
	}
}

func TestGetRespawnTimesInZone(t *testing.T) {
	d := openTestDB(t)

	// a_skeleton spawns in nektulos with known respawn timing.
	infos, err := d.GetRespawnTimesInZone("a_skeleton", "nektulos")
	if err != nil {
		t.Fatalf("GetRespawnTimesInZone: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("got 0 respawn rows for a_skeleton in nektulos, want >0")
	}
	for _, ri := range infos {
		if ri.RespawnTime <= 0 {
			t.Errorf("non-positive respawn time: %+v", ri)
		}
		if ri.NPCID <= 0 {
			t.Errorf("missing npc id: %+v", ri)
		}
	}

	// A name with no spawn data returns an empty slice, not an error.
	none, err := d.GetRespawnTimesInZone("a_nonexistent_mob_xyzzy", "qeynos")
	if err != nil {
		t.Fatalf("GetRespawnTimesInZone (none): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("got %d rows for unknown name, want 0", len(none))
	}
}

func TestGetZoneShortNameByLongName(t *testing.T) {
	d := openTestDB(t)
	tests := []struct {
		long string
		want string
	}{
		{"Northern Plains of Karana", "northkarana"},
		{"The Feerrott", "feerrott"},
		{"Plane of Fear (Instanced)", "fearplane"}, // parenthetical stripped
		{"Not A Real Zone Name", ""},               // no match → empty, no error
	}
	for _, tc := range tests {
		got, err := d.GetZoneShortNameByLongName(tc.long)
		if err != nil {
			t.Fatalf("GetZoneShortNameByLongName(%q): %v", tc.long, err)
		}
		if got != tc.want {
			t.Errorf("GetZoneShortNameByLongName(%q) = %q, want %q", tc.long, got, tc.want)
		}
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
		{"space matches underscore", "a gnoll", 1},
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
	res, err := d.SearchSpells("Fire", -1, 0, 0, 1, 0, false)
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
			res, err := d.SearchSpells(tt.query, -1, 0, 0, 20, 0, false)
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
			res, err := d.SearchZones(tt.query, db.ZoneSearchFilters{}, 20, 0)
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
		raw       string
		wantLen   int
		wantFirst db.SpecialAbility
	}{
		{
			raw:       "1,1^18,1^19,1",
			wantLen:   3,
			wantFirst: db.SpecialAbility{Code: 1, Value: 1, Name: "Summon"},
		},
		{
			raw:     "",
			wantLen: 0,
		},
		{
			raw:       "2,1^4,1",
			wantLen:   2,
			wantFirst: db.SpecialAbility{Code: 2, Value: 1, Name: "Enrage"},
		},
		{
			// Regression: Thall_Va_Xakra's row is "1,1^2,1^3,1,30^...".
			// The "3,1,30" entry is Rampage with a range arg in the third
			// field; the old SplitN(",", 2) dropped it silently. Confirm
			// it's now parsed with code=3, value=1 and the third field
			// ignored.
			raw:       "1,1^3,1,30^10,1",
			wantLen:   3,
			wantFirst: db.SpecialAbility{Code: 1, Value: 1, Name: "Summon"},
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

func TestSkillCaps_RealDB(t *testing.T) {
	d := openTestDB(t)

	// Defense (skill_id 15) — caster cap is 145, plate cap 252. These pin the
	// Mac-era enum that the AC/ATK math relies on.
	if c, _ := d.DefenseSkillCap(14, 60); c != 145 { // Enchanter
		t.Errorf("DefenseSkillCap(Enchanter,60) = %d, want 145", c)
	}
	if c, _ := d.DefenseSkillCap(1, 60); c != 252 { // Warrior
		t.Errorf("DefenseSkillCap(Warrior,60) = %d, want 252", c)
	}

	// Offense (skill_id 22) — Warrior 245; pure casters have no Offense row → 0.
	if c, _ := d.OffenseSkillCap(1, 60); c != 245 { // Warrior
		t.Errorf("OffenseSkillCap(Warrior,60) = %d, want 245", c)
	}
	if c, _ := d.OffenseSkillCap(14, 60); c != 0 { // Enchanter
		t.Errorf("OffenseSkillCap(Enchanter,60) = %d, want 0", c)
	}

	// Best melee weapon skill cap — Warrior 250, Enchanter 110 (Hand to Hand /
	// 1H Blunt floor).
	if c, _ := d.BestWeaponSkillCap(1, 60); c != 250 { // Warrior
		t.Errorf("BestWeaponSkillCap(Warrior,60) = %d, want 250", c)
	}
	if c, _ := d.BestWeaponSkillCap(14, 60); c != 110 { // Enchanter
		t.Errorf("BestWeaponSkillCap(Enchanter,60) = %d, want 110", c)
	}

	// Zero/invalid inputs are safe.
	if c, _ := d.OffenseSkillCap(0, 0); c != 0 {
		t.Errorf("OffenseSkillCap(0,0) = %d, want 0", c)
	}
}

// ─── Alternate Advancement ──────────────────────────────────────────────────────

// Fletching Mastery is a Ranger archery AA whose `classes` mask is corrupt in
// the source dump (65534 = all classes); aaClassMaskOverrides corrects it to
// Ranger only. Druids must not see it; Rangers must (issue #134).
func TestListAvailableAAs_FletchingMasteryRangerOnly(t *testing.T) {
	d := openTestDB(t)
	const (
		ranger = 4 // 1-indexed EQ class
		druid  = 6
	)

	has := func(class int, name string) bool {
		t.Helper()
		aas, err := d.ListAvailableAAs(class)
		if err != nil {
			t.Fatalf("ListAvailableAAs(%d): %v", class, err)
		}
		for _, a := range aas {
			if a.Name == name {
				return true
			}
		}
		return false
	}

	if has(druid, "Fletching Mastery") {
		t.Error("druid should NOT have Fletching Mastery (issue #134)")
	}
	if !has(ranger, "Fletching Mastery") {
		t.Error("ranger SHOULD have Fletching Mastery")
	}
	// Sanity: a genuine all-class AA still shows for druid.
	if !has(druid, "Innate Strength") {
		t.Error("druid should still have the all-class Innate Strength AA")
	}
}

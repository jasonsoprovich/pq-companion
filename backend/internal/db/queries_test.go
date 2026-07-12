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

// NPCNameVariantCandidates must try every leading-prefix, trailing-suffix,
// and combined form — the trailing "_" convention was missed entirely until
// the Emperor Ssraeshza report showed "Emperor_Ssraeshza_" was unreachable
// via prefix-only candidates.
func TestNPCNameVariantCandidates(t *testing.T) {
	got := db.NPCNameVariantCandidates("Foo")
	want := map[string]bool{
		"Foo": true, "#Foo": true, "##Foo": true, "###Foo": true,
		"#_Foo": true, "##_Foo": true, "###_Foo": true,
		"Foo_": true, "#Foo_": true, "##Foo_": true, "###Foo_": true,
		"#_Foo_": true, "##_Foo_": true, "###_Foo_": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d: %v", len(got), len(want), got)
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected candidate %q", c)
		}
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("missing candidates: %v", want)
	}
	if got[0] != "Foo" {
		t.Errorf("first candidate = %q, want bare name %q first", got[0], "Foo")
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

// TestGetSpellsByClass_EraLevelCap drives the era-dependent level cap (see
// internal/era): pre-PoP the enchanter list stops at 60; with the PoP cap of
// 65 the list grows and includes 61+ spells (e.g. Greater Fetter at 61).
func TestGetSpellsByClass_EraLevelCap(t *testing.T) {
	d := openTestDB(t)
	const enchanter = 13 // 0-based class index

	pre, err := d.GetSpellsByClass(enchanter, 60, 2000, 0)
	if err != nil {
		t.Fatalf("GetSpellsByClass(maxLevel=60): %v", err)
	}
	pop, err := d.GetSpellsByClass(enchanter, 65, 2000, 0)
	if err != nil {
		t.Fatalf("GetSpellsByClass(maxLevel=65): %v", err)
	}
	if pop.Total <= pre.Total {
		t.Errorf("PoP total=%d, want > pre-PoP total=%d", pop.Total, pre.Total)
	}
	for _, sp := range pre.Items {
		if lvl := sp.ClassLevels[enchanter]; lvl > 60 {
			t.Errorf("pre-PoP list contains %q at level %d", sp.Name, lvl)
		}
	}
	found61Plus := false
	for _, sp := range pop.Items {
		if lvl := sp.ClassLevels[enchanter]; lvl > 60 && lvl <= 65 {
			found61Plus = true
			break
		}
	}
	if !found61Plus {
		t.Error("PoP list has no spells between 61 and 65")
	}
}

// TestGetSpellsByClass_ExcludesMassCraftDuplicates verifies that Quarm's
// non-scribable "Mass X" tradeskill mirrors (e.g. Mass Enchant Clay/Silver,
// which have no teaching scroll and duplicate a real scribable spell at the
// same level) are dropped from the class list, while the plain scribable
// spells a player actually owns remain. See GetSpellsByClass.
func TestGetSpellsByClass_ExcludesMassCraftDuplicates(t *testing.T) {
	d := openTestDB(t)
	const enchanter = 13 // 0-based class index

	res, err := d.GetSpellsByClass(enchanter, 60, 2000, 0)
	if err != nil {
		t.Fatalf("GetSpellsByClass: %v", err)
	}
	byID := make(map[int]string, len(res.Items))
	for _, sp := range res.Items {
		byID[sp.ID] = sp.Name
	}

	// Non-scribable "Mass X" duplicates must be gone.
	for _, id := range []int{3986, 3991, 3987} { // Mass Enchant Clay/Silver/Electrum
		if name, ok := byID[id]; ok {
			t.Errorf("Mass-craft duplicate %d (%q) should be excluded", id, name)
		}
	}
	// The real, scribable spells the player owns must remain.
	for _, id := range []int{1359, 667, 668} { // Enchant Clay/Silver/Electrum
		if _, ok := byID[id]; !ok {
			t.Errorf("scribable spell %d should remain in the class list", id)
		}
	}
}

// TestGetSpellsByClass_ExcludesNotPlayerSpell verifies that spells_new rows
// flagged not_player_spell (innate class abilities and AA-granted spells that
// list a class/level but were never scribable) are dropped from the class
// list. See GetSpellsByClass.
func TestGetSpellsByClass_ExcludesNotPlayerSpell(t *testing.T) {
	d := openTestDB(t)
	const druid = 5 // 0-based class index

	res, err := d.GetSpellsByClass(druid, 60, 2000, 0)
	if err != nil {
		t.Fatalf("GetSpellsByClass: %v", err)
	}
	byID := make(map[int]string, len(res.Items))
	for _, sp := range res.Items {
		byID[sp.ID] = sp.Name
	}

	// AA-granted / innate spells with no teaching scroll must be gone.
	notPlayerIDs := []int{
		3693,             // Pure Blood
		3277, 3278, 3279, // Spirit of the Wood (AA #548)
		3255, 3256, 3257, // Wrath of the Wild (AA #510)
		3695, // Frost Zephyr
		3792, // Circle of Stonebrunt
		3834, // Healing Water
	}
	for _, id := range notPlayerIDs {
		if name, ok := byID[id]; ok {
			t.Errorf("not_player_spell %d (%q) should be excluded", id, name)
		}
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

func TestRechargeableMaxCharges(t *testing.T) {
	d := openTestDB(t)
	// Empty input → empty result, no error.
	got, err := d.RechargeableMaxCharges(nil)
	if err != nil {
		t.Fatalf("RechargeableMaxCharges(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("RechargeableMaxCharges(nil) = %v, want empty map", got)
	}

	// Seed: a genuine multi-charge clicky, an unlimited clicky (-1 sentinel),
	// and a single-charge item — only the first should come back.
	limited := seedRechargeID(t, d, "clickeffect > 0 AND maxcharges > 1")
	unlimited := seedRechargeID(t, d, "clickeffect > 0 AND maxcharges = -1")
	single := seedRechargeID(t, d, "clickeffect > 0 AND maxcharges = 1")
	if limited.id == 0 {
		t.Skip("no multi-charge clicky in DB")
	}

	ids := []int{limited.id, -1}
	if unlimited.id != 0 {
		ids = append(ids, unlimited.id)
	}
	if single.id != 0 {
		ids = append(ids, single.id)
	}
	got, err = d.RechargeableMaxCharges(ids)
	if err != nil {
		t.Fatalf("RechargeableMaxCharges: %v", err)
	}
	if got[limited.id] != limited.maxCharges {
		t.Errorf("limited[%d] = %d, want %d", limited.id, got[limited.id], limited.maxCharges)
	}
	if unlimited.id != 0 {
		if _, ok := got[unlimited.id]; ok {
			t.Errorf("unlimited clicky %d should be excluded, got %d", unlimited.id, got[unlimited.id])
		}
	}
	if single.id != 0 {
		if _, ok := got[single.id]; ok {
			t.Errorf("single-charge item %d should be excluded, got %d", single.id, got[single.id])
		}
	}
	if _, ok := got[-1]; ok {
		t.Errorf("non-existent id -1 should be omitted")
	}
}

type rechargeSeed struct {
	id         int
	maxCharges int
}

func seedRechargeID(t *testing.T, d *db.DB, where string) rechargeSeed {
	t.Helper()
	var s rechargeSeed
	err := d.QueryRow("SELECT id, maxcharges FROM items WHERE "+where+" LIMIT 1").
		Scan(&s.id, &s.maxCharges)
	if err == sql.ErrNoRows {
		return rechargeSeed{}
	}
	if err != nil {
		t.Fatalf("seed query (%s): %v", where, err)
	}
	return s
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

	// Offense (skill_id 33) — universal skill: Warrior 252, casters ~140.
	if c, _ := d.OffenseSkillCap(1, 60); c != 252 { // Warrior
		t.Errorf("OffenseSkillCap(Warrior,60) = %d, want 252", c)
	}
	if c, _ := d.OffenseSkillCap(14, 60); c != 140 { // Enchanter
		t.Errorf("OffenseSkillCap(Enchanter,60) = %d, want 140", c)
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

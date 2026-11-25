package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/upgrade"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// TestIsTwoHander checks the items.itemtype classifier used to flag a
// candidate as a two-handed weapon. itemtype is the only reliable signal —
// items.slots does NOT distinguish 1H from 2H (verified against live
// quarm.db: e.g. Facesmasher, a confirmed 2H Blunt weapon, has slots=Primary
// only, while Serrated Dragon Tooth, a confirmed 1H Piercing weapon, has
// slots=Primary|Secondary|Range — the opposite of what an earlier
// slots-bitmask heuristic assumed).
func TestIsTwoHander(t *testing.T) {
	cases := []struct {
		name     string
		itemType int
		want     bool
	}{
		{"2H Slashing", 1, true},
		{"2H Blunt", 4, true},
		{"2H Piercing", 35, true},
		{"1H Slashing", 0, false},
		{"1H Piercing", 2, false},
		{"1H Blunt", 3, false},
		{"Hand to Hand (monk fist weapon)", 45, false},
		{"Shield", 8, false},
		{"unrelated type (armor)", 10, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTwoHander(c.itemType); got != c.want {
				t.Errorf("isTwoHander(%d) = %v, want %v", c.itemType, got, c.want)
			}
		})
	}
}

// TestSumStatLine checks the field-by-field baseline combiner, including the
// best-of behaviour for Haste (worn haste doesn't stack, so combining two
// worn items should keep the higher one, not add them).
func TestSumStatLine(t *testing.T) {
	a := upgrade.StatLine{HP: 100, AC: 5, Damage: 10, Delay: 20, Haste: 15}
	b := upgrade.StatLine{HP: 100, AC: 3, Damage: 8, Delay: 19, Haste: 20}
	got := sumStatLine(a, b)
	want := upgrade.StatLine{HP: 200, AC: 8, Damage: 18, Delay: 39, Haste: 20}
	if got != want {
		t.Errorf("sumStatLine(%+v, %+v) = %+v, want %+v", a, b, got, want)
	}
}

// TestScoreSlotCands_TwoHanderNetsOutOffhand is the regression test for the
// reported bug: a 2H weapon suggested for Primary must be scored against the
// combined stats of BOTH currently-worn hand items, not the primary weapon
// alone — otherwise the finder credits the 2H for offhand stats it actually
// takes away. Case from the bug report: +100 HP primary, +100 HP offhand,
// candidate 2H is +125 HP — the real net change is -75 HP, not +25.
func TestScoreSlotCands_TwoHanderNetsOutOffhand(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()

	byLoc := map[string][]zeal.InventoryEntry{
		"Primary":   {{Location: "Primary", ID: 1, Name: "Primary Weapon"}},
		"Secondary": {{Location: "Secondary", ID: 2, Name: "Offhand Weapon"}},
	}
	worn := map[int]*db.Item{
		1: {ID: 1, Name: "Primary Weapon", HP: 100, Slots: 0x002000},
		2: {ID: 2, Name: "Offhand Weapon", HP: 100, Slots: 0x004000},
	}
	cands := []db.UpgradeCandidate{
		// itemtype 4 = 2H Blunt; real 2H weapons carry slots=Primary only
		// (see TestIsTwoHander) — items.slots plays no part in the check.
		{ID: 3, Name: "2H Weapon", HP: 125, Slots: 0x002000, ItemType: 4},
	}

	slot, ok := upgradeSlotByKey("primary")
	if !ok {
		t.Fatal("primary slot not found")
	}
	ctx := upgrade.Context{Level: 60}
	weights := upgrade.Weights{HP: 1}

	_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, wc, nil, cands)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	res := results[0]
	if res.Score != -75 {
		t.Errorf("score = %v, want -75 (125 HP candidate vs 200 HP combined primary+secondary baseline)", res.Score)
	}
	if !res.ReplacesSecondary {
		t.Error("ReplacesSecondary = false, want true for a 2H candidate in the Primary slot")
	}
	if res.SecondaryItemName != "Offhand Weapon" {
		t.Errorf("SecondaryItemName = %q, want %q", res.SecondaryItemName, "Offhand Weapon")
	}
}

// TestScoreSlotCands_OneHanderUnaffected checks that a normal 1H candidate in
// the Primary slot still scores against the primary weapon alone — the 2H
// baseline combination must not leak into ordinary single-slot comparisons.
func TestScoreSlotCands_OneHanderUnaffected(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()

	byLoc := map[string][]zeal.InventoryEntry{
		"Primary":   {{Location: "Primary", ID: 1, Name: "Primary Weapon"}},
		"Secondary": {{Location: "Secondary", ID: 2, Name: "Offhand Weapon"}},
	}
	worn := map[int]*db.Item{
		1: {ID: 1, Name: "Primary Weapon", HP: 100, Slots: 0x002000},
		2: {ID: 2, Name: "Offhand Weapon", HP: 100, Slots: 0x004000},
	}
	cands := []db.UpgradeCandidate{
		// itemtype 2 = 1H Piercing.
		{ID: 4, Name: "1H Weapon", HP: 140, Slots: 0x002000 | 0x004000, ItemType: 2},
	}

	slot, _ := upgradeSlotByKey("primary")
	ctx := upgrade.Context{Level: 60}
	weights := upgrade.Weights{HP: 1}

	_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, wc, nil, cands)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Score != 40 {
		t.Errorf("score = %v, want 40 (140 HP candidate vs 100 HP primary alone)", results[0].Score)
	}
	if results[0].ReplacesSecondary {
		t.Error("ReplacesSecondary = true, want false for a 1H candidate")
	}
}

// TestScoreSlotCands_TwoHanderExcludedFromSecondary checks that a 2H weapon
// is never offered as a candidate for the Secondary slot view — it can't be
// equipped "into" just the offhand. In practice a real 2H item's slots
// bitmask is Primary-only, so it wouldn't even be fetched as a Secondary
// candidate; this exercises the explicit itemtype-based guard directly as a
// defensive check against anomalous data (e.g. a miscategorized item whose
// slots bitmask does include Secondary).
func TestScoreSlotCands_TwoHanderExcludedFromSecondary(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()

	byLoc := map[string][]zeal.InventoryEntry{
		"Secondary": {{Location: "Secondary", ID: 2, Name: "Offhand Weapon"}},
	}
	worn := map[int]*db.Item{
		2: {ID: 2, Name: "Offhand Weapon", AC: 5, Slots: 0x004000},
	}
	cands := []db.UpgradeCandidate{
		{ID: 3, Name: "2H Weapon", HP: 125, Slots: 0x002000 | 0x004000, ItemType: 4},
	}

	slot, _ := upgradeSlotByKey("secondary")
	ctx := upgrade.Context{Level: 60}
	weights := upgrade.Weights{HP: 1, AC: 1}

	_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, wc, nil, cands)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0 (2H candidate must be excluded from the Secondary slot view)", len(results))
	}
}

// TestScoreSlotCands_WeaponStyleFilter checks the "dw"/"2h" hand-style filter
// on the Primary slot: a dual-wielder can exclude 2H candidates (even a
// statistically better one) and a 2H user can exclude 1H candidates, so the
// list only ever shows weapons they'd actually equip.
func TestScoreSlotCands_WeaponStyleFilter(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()

	byLoc := map[string][]zeal.InventoryEntry{
		"Primary": {{Location: "Primary", ID: 1, Name: "Primary Weapon"}},
	}
	worn := map[int]*db.Item{
		1: {ID: 1, Name: "Primary Weapon", HP: 50, Slots: 0x002000},
	}
	cands := []db.UpgradeCandidate{
		// itemtype 2 = 1H Piercing (slots=Primary|Secondary, like a real 1H
		// weapon); itemtype 4 = 2H Blunt (slots=Primary only, like a real 2H
		// weapon) — see TestIsTwoHander for why slots alone can't tell these
		// apart.
		{ID: 2, Name: "1H Weapon", HP: 60, Slots: 0x002000 | 0x004000, ItemType: 2},
		{ID: 3, Name: "2H Weapon", HP: 200, Slots: 0x002000, ItemType: 4},
	}

	slot, _ := upgradeSlotByKey("primary")
	ctx := upgrade.Context{Level: 60}
	weights := upgrade.Weights{HP: 1}

	t.Run("dw excludes 2H", func(t *testing.T) {
		_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "dw", 10, nil, nil, wc, nil, cands)
		if len(results) != 1 || results[0].ID != 2 {
			t.Fatalf("got %+v, want only the 1H candidate", results)
		}
	})

	t.Run("2h excludes 1H", func(t *testing.T) {
		_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "2h", 10, nil, nil, wc, nil, cands)
		if len(results) != 1 || results[0].ID != 3 {
			t.Fatalf("got %+v, want only the 2H candidate", results)
		}
	})

	t.Run("empty style includes both", func(t *testing.T) {
		_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, wc, nil, cands)
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}
	})
}

// TestEquippedLoreSet_CanonicalizesVariantIDs is the regression test for a
// real gear-upgrade-finder bug: quarm.db carries some LORE items under
// several duplicate-name rows with different ids (e.g. "Dull Pearl Necklace"
// exists as ids 13347-13350, all identical stats) — the item explorer and
// candidate query collapse these to one canonical row (see
// db.CanonicalItemID / variants.go), but a live character's Quarmy export
// reports whichever raw id the server actually assigned, which can be one of
// the non-canonical siblings. Comparing that raw id against the (always
// canonical) candidate id would silently fail to match, so the finder kept
// suggesting a LORE item the character already had equipped elsewhere.
func TestEquippedLoreSet_CanonicalizesVariantIDs(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()
	h := &charactersHandler{db: d}

	const wornVariantID = 13350 // a non-canonical sibling of "Dull Pearl Necklace"
	canon := d.CanonicalItemID(wornVariantID)
	if canon == wornVariantID {
		t.Skip("13350 is no longer a non-canonical variant in this quarm.db; test fixture stale")
	}

	worn := map[int]*db.Item{
		wornVariantID: {ID: wornVariantID, Name: "Dull Pearl Necklace", Lore: "*Jaylas Necklace"},
	}
	set := h.equippedLoreSet(worn)
	if !set[canon] {
		t.Errorf("equippedLoreSet(%+v) = %v, want the canonical id %d present so a candidate row (always canonical) is recognized as already worn", worn, set, canon)
	}
}

// TestStatLineFromBlock_UsesRawTotals guards the over-cap headroom bug: the
// scorer must see uncapped attribute/FT totals and gear-only ATK, or dropping
// an item's over-cap stat looks like a loss and AA attack eats worn-ATK room.
func TestStatLineFromBlock_UsesRawTotals(t *testing.T) {
	b := statBlock{WIS: 280, WISRaw: 300, STR: 280, STRRaw: 291, FT: 15, FTRaw: 18, Attack: 160}
	b.Breakdown.Attack = sourceSplit{Item: 100, AA: 60}
	got := statLineFromBlock(b)
	if got.WIS != 300 || got.STR != 291 {
		t.Errorf("attributes = WIS %d STR %d, want raw 300/291", got.WIS, got.STR)
	}
	if got.ManaRegen != 18 {
		t.Errorf("ManaRegen = %d, want raw FT 18", got.ManaRegen)
	}
	if got.Attack != 100 {
		t.Errorf("Attack = %d, want gear-only 100", got.Attack)
	}
}

// TestScore_OverCapRawTotals runs the reported case end to end through Score:
// a lvl-65 character over the WIS cap loses nothing by dropping a +20 WIS item,
// and a Planar Power cap raise gives real headroom.
func TestScore_OverCapRawTotals(t *testing.T) {
	w := upgrade.Weights{WIS: 1}
	worn := upgrade.StatLine{WIS: 20}
	ctx := upgrade.Context{Level: 65, Current: statLineFromBlock(statBlock{WISRaw: 300})}
	if res := upgrade.Score(ctx, w, worn, upgrade.StatLine{}); res.Score != 0 {
		t.Errorf("over-cap drop scored %v, want 0 (cap 280, raw stays 280)", res.Score)
	}
	// At exactly the base cap (raw 280 incl. the worn +20), Planar Power's +10
	// raises it to 290, so a +30 WIS item is worth 10 more than the worn +20.
	ctx.Current = statLineFromBlock(statBlock{WISRaw: 280})
	ctx.StatCapMod = upgrade.StatLine{WIS: 10}
	if res := upgrade.Score(ctx, w, worn, upgrade.StatLine{WIS: 30}); res.Score != 10 {
		t.Errorf("+30 vs +20 WIS under a raised 290 cap scored %v, want 10", res.Score)
	}
}

// TestScore_AAAttackLeavesWornHeadroom: 200 AA ATK plus 100 worn ATK must still
// credit a +50 worn ATK upgrade (worn total 150 → 200, under the 250 soft cap).
func TestScore_AAAttackLeavesWornHeadroom(t *testing.T) {
	b := statBlock{Attack: 300}
	b.Breakdown.Attack = sourceSplit{Item: 100, AA: 200}
	ctx := upgrade.Context{Level: 65, Current: statLineFromBlock(b)}
	res := upgrade.Score(ctx, upgrade.Weights{ATK: 1}, upgrade.StatLine{Attack: 0}, upgrade.StatLine{Attack: 50})
	if res.Score != 50 {
		t.Errorf("worn ATK upgrade scored %v, want 50", res.Score)
	}
}

// TestScoreSlotCands_PriorityContract pins the priority-focus results rules:
// wanted-focus items are always listed in priority (stat losses included, no
// limit), they never take a ranked row, a stat loss never enters the ranked
// list unless show_all asks for downgrades, and priority is the same either
// way.
func TestScoreSlotCands_PriorityContract(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()
	slot, ok := upgradeSlotByKey("arms")
	if !ok {
		t.Fatal("arms slot not found")
	}
	cands := []db.UpgradeCandidate{
		{ID: 1, Name: "Plain A", HP: 50, Slots: slot.Mask},
		{ID: 2, Name: "Plain B", HP: 40, Slots: slot.Mask},
		{ID: 3, Name: "Wanted loss", HP: -30, Slots: slot.Mask, FocusEffect: 500},
		{ID: 4, Name: "Wanted gain", HP: 60, Slots: slot.Mask, FocusEffect: 501},
		{ID: 5, Name: "Unwanted focus", HP: 10, Slots: slot.Mask, FocusEffect: 502},
	}
	// Unknown foci (no lookup entry) match by exact spell id.
	pf := &priorityFocus{selected: []int{500, 501}, top: 65,
		lookup: func(int) (buffmod.Modifier, bool) { return buffmod.Modifier{}, false }}
	ctx := upgrade.Context{Level: 65}
	weights := upgrade.Weights{HP: 1}
	ids := func(rs []upgradeResult) []int {
		out := []int{}
		for _, r := range rs {
			out = append(out, r.ID)
		}
		return out
	}
	eq := func(a, b []int) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	_, _, results, priority, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, nil, nil, false, "", 2, pf, nil, wc, nil, cands)
	if got := ids(results); !eq(got, []int{4, 1}) {
		t.Errorf("ranked = %v, want [4 1] (limit 2, positive only)", got)
	}
	if got := ids(priority); !eq(got, []int{4, 3}) {
		t.Errorf("priority = %v, want [4 3] (every wanted item, by score, no limit)", got)
	}
	if results[0].ID == 3 {
		t.Error("a stat-loss priority item became the best pick")
	}
	for _, r := range priority {
		if !r.PriorityFocus {
			t.Errorf("priority item %d not flagged", r.ID)
		}
	}
	if priority[1].Score != -30 {
		t.Errorf("wanted loss score = %v, want the plain -30", priority[1].Score)
	}

	_, _, all, priorityAll, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, nil, nil, true, "", 10, pf, nil, wc, nil, cands)
	if got := ids(all); !eq(got, []int{4, 1, 2, 5, 3}) {
		t.Errorf("show_all ranked = %v, want every candidate by score", got)
	}
	if !eq(ids(priorityAll), ids(priority)) {
		t.Errorf("priority changed with show_all: %v vs %v", ids(priorityAll), ids(priority))
	}

	// Already wearing the wanted focus 501 elsewhere: item 4 drops out of priority.
	pf.worn = []int{501}
	_, _, _, priorityWorn, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, nil, nil, false, "", 2, pf, nil, wc, nil, cands)
	if got := ids(priorityWorn); !eq(got, []int{3}) {
		t.Errorf("priority with 501 worn = %v, want [3]", got)
	}
}

// TestMergeItemPriority: the item-scores endpoint reports a wanted focus even
// as a stat loss, keeping the item's best slot.
func TestMergeItemPriority(t *testing.T) {
	m := map[int]itemPriorityFocus{}
	mergeItemPriority(m, []upgradeResult{{ID: 7, Score: -40, FocusName: "Affliction Haste IV"}}, "Ear 1")
	mergeItemPriority(m, []upgradeResult{{ID: 7, Score: -10, FocusName: "Affliction Haste IV"}}, "Ear 2")
	got, ok := m[7]
	if !ok {
		t.Fatal("negative-score priority item missing")
	}
	if got.Score != -10 || got.Slot != "Ear 2" {
		t.Errorf("got %+v, want the better slot (Ear 2, -10)", got)
	}
}

// TestPriorityFocusMatch_RealAfflictionHaste runs the rank rules on quarm.db's
// real Affliction Haste foci: picking III means III or better.
func TestPriorityFocusMatch_RealAfflictionHaste(t *testing.T) {
	d := openFocusTestDB(t)
	h := &charactersHandler{db: d}
	lookup := h.newFocusLookup()
	const (
		ah2 = 2361
		ah3 = 2362
		ah4 = 3534
	)
	for _, id := range []int{ah2, ah3, ah4} {
		if _, ok := lookup(id); !ok {
			t.Fatalf("focus spell %d didn't resolve to a categorised modifier", id)
		}
	}
	cases := []struct {
		name string
		cand int
		worn []int
		want bool
	}{
		{"IV satisfies a III pick", ah4, nil, true},
		{"II doesn't satisfy a III pick", ah2, nil, false},
		{"IV still wanted while wearing III", ah4, []int{ah3}, true},
		{"III not wanted while wearing IV", ah3, []int{ah4}, false},
	}
	for _, c := range cases {
		if got := priorityFocusMatch(c.cand, []int{ah3}, c.worn, lookup, 65); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestLoadEquipped_WornSlotsOnly: the Quarmy export also lists bags, bank and
// shared bank. The finder's "equipped" map must hold worn slots only, or a
// haste item or focus sitting in the bank counts as worn.
func TestLoadEquipped_WornSlotsOnly(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata")
	if _, err := os.Stat(filepath.Join(testdata, "Osui-Quarmy.txt")); err != nil {
		t.Skip("Osui-Quarmy.txt fixture not present")
	}
	h := &charactersHandler{}
	byLoc, ok := h.loadEquipped(testdata, "Osui")
	if !ok {
		t.Fatal("loadEquipped found no export")
	}
	for loc := range byLoc {
		if !equipSlots[loc] {
			t.Errorf("non-worn location %q in the equipped map", loc)
		}
	}
	if len(byLoc["Ear"]) == 0 || len(byLoc["Fingers"]) == 0 {
		t.Errorf("expected worn ear/finger entries, got %v", byLoc)
	}
}

// TestScoreSlotCands_FromMold: a PoP mold product carries the mold it is
// combined from (the thing that actually drops); ordinary items don't.
func TestScoreSlotCands_FromMold(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	slot, ok := upgradeSlotByKey("arms")
	if !ok {
		t.Fatal("arms slot not found")
	}
	cands := []db.UpgradeCandidate{
		{ID: 11446, Name: "Planestriders Vambraces", HP: 50, Slots: slot.Mask},
		{ID: 1, Name: "Plain", HP: 40, Slots: slot.Mask},
	}
	_, _, results, _, _ := h.scoreSlotCands(character.Character{}, upgrade.Context{Level: 65},
		upgrade.Weights{HP: 1}, slot, nil, nil, true, "", 10, nil, nil, h.newWornCache(), nil, cands)
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for _, r := range results {
		switch r.ID {
		case 11446:
			if r.FromMold == nil || r.FromMold.ID != 16292 || r.FromMold.NoDrop == 0 {
				t.Errorf("from_mold = %+v, want tradeable Ornate Chain Sleeve Pattern (16292)", r.FromMold)
			}
		case 1:
			if r.FromMold != nil {
				t.Errorf("plain item got from_mold %+v", r.FromMold)
			}
		}
	}
}

// TestZoneCandidates_ViaMold: a mold product joins a zone's candidates with
// its mold's drop sources when the mold drops there.
func TestZoneCandidates_ViaMold(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	moldSrc := []db.ZoneItemSource{{NPCID: 99, NPCName: "a mold dropper", Chance: 10}}
	sources := map[int][]db.ZoneItemSource{
		16292: moldSrc,                               // Ornate Chain Sleeve Pattern drops here
		2:     {{NPCID: 5, NPCName: "x", Chance: 1}}, // ordinary zone drop
	}
	all := []db.UpgradeCandidate{
		{ID: 11446}, // Planestriders Vambraces — product of 16292
		{ID: 11447}, // Askr's — mold 16364 doesn't drop here
		{ID: 2},
		{ID: 3}, // drops nowhere
	}
	got, via := h.zoneCandidates(all, sources)
	ids := map[int]bool{}
	for _, c := range got {
		ids[c.ID] = true
	}
	if !ids[11446] || !ids[2] || ids[11447] || ids[3] || len(got) != 2 {
		t.Errorf("zone candidates = %v, want 11446 and 2", ids)
	}
	if !via[11446] || via[2] {
		t.Errorf("viaMold = %v, want only 11446", via)
	}
	if len(sources[11446]) != 1 || sources[11446][0].NPCID != 99 {
		t.Errorf("product sources = %+v, want the mold's", sources[11446])
	}
}

// Backstab in a rogue's Primary comparison must use the actual worn Primary
// (12 damage), never the summed Primary+Secondary baseline (22), and a 2H
// candidate drops the offhand's Backstab skill mod.
func TestScoreSlotCands_BackstabUsesActualPrimary(t *testing.T) {
	h := &charactersHandler{}
	wc := h.newWornCache()
	byLoc := map[string][]zeal.InventoryEntry{
		"Primary":   {{Location: "Primary", ID: 1, Name: "Dagger"}},
		"Secondary": {{Location: "Secondary", ID: 2, Name: "Mod Dagger"}},
	}
	worn := map[int]*db.Item{
		1: {ID: 1, Name: "Dagger", ItemType: 2, Damage: 12, Slots: 0x006000},
		2: {ID: 2, Name: "Mod Dagger", ItemType: 2, Damage: 10, Slots: 0x006000,
			SkillModType: upgrade.SkillBackstab, SkillModValue: 12},
	}
	cands := []db.UpgradeCandidate{
		{ID: 3, Name: "Better Dagger", ItemType: 2, Damage: 13, Slots: 0x006000},
		{ID: 4, Name: "2H Spear", ItemType: 35, Damage: 30, Slots: 0x002000},
	}
	slot, _ := upgradeSlotByKey("primary")
	ctx := upgrade.Context{Level: 65, BackstabSkill: 225}
	weights := upgrade.Weights{Backstab: 1}

	_, _, results, _, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, wc, nil, cands)
	got := map[string]float64{}
	for _, r := range results {
		got[r.Name] = r.Score
	}
	// With the offhand's +12% mod (skill 252): 7.04×12 = 84 → 7.04×13 = 91.
	if got["Better Dagger"] != 7 {
		t.Errorf("Better Dagger score = %v, want 7 (84 → 91)", got["Better Dagger"])
	}
	// The 2H spear can't backstab and displaces the mod offhand: 84 → 0.
	if got["2H Spear"] != -84 {
		t.Errorf("2H Spear score = %v, want -84", got["2H Spear"])
	}
}

func TestBackfillNewWeights(t *testing.T) {
	def := upgrade.DefaultWeights(8) // rogue
	w := upgrade.Weights{SpellShield: 0}
	// A profile saved before the PoP stats, except spell_shield set to 0.
	backfillNewWeights(`{"hp":1,"spell_shield":0}`, &w, def)
	if w.Backstab != def.Backstab || w.Strikethrough != def.Strikethrough {
		t.Errorf("missing keys not backfilled: %+v", w)
	}
	if w.SpellShield != 0 {
		t.Errorf("saved spell_shield 0 overwritten with %v", w.SpellShield)
	}
}

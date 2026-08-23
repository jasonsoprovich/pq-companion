package api

import (
	"testing"

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

	_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, nil, wc, nil, cands)
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

	_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, nil, wc, nil, cands)
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

	_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, nil, wc, nil, cands)
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
		_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "dw", 10, nil, nil, nil, wc, nil, cands)
		if len(results) != 1 || results[0].ID != 2 {
			t.Fatalf("got %+v, want only the 1H candidate", results)
		}
	})

	t.Run("2h excludes 1H", func(t *testing.T) {
		_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "2h", 10, nil, nil, nil, wc, nil, cands)
		if len(results) != 1 || results[0].ID != 3 {
			t.Fatalf("got %+v, want only the 2H candidate", results)
		}
	})

	t.Run("empty style includes both", func(t *testing.T) {
		_, _, results, _ := h.scoreSlotCands(character.Character{}, ctx, weights, slot, byLoc, worn, true, "", 10, nil, nil, nil, wc, nil, cands)
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}
	})
}

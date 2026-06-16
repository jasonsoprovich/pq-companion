package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestUpgradeCandidates_HeadSlotEnchanter(t *testing.T) {
	d := openTestDB(t)

	const headBit = 0x000004
	const enchanterBit = 0x2000 // class index 13 -> 1<<13

	cands, err := d.UpgradeCandidates(db.CandidateFilter{
		SlotMask: headBit,
		ClassBit: enchanterBit,
		RaceBit:  0x0020, // Dark Elf
		MaxLevel: 60,
	})
	if err != nil {
		t.Fatalf("UpgradeCandidates: %v", err)
	}
	if len(cands) < 20 {
		t.Fatalf("expected a healthy set of head items, got %d", len(cands))
	}

	sawFocus := false
	for _, c := range cands {
		// Every candidate must actually fit the head slot.
		if c.Slots&headBit == 0 {
			t.Fatalf("%q (%d) does not fit head slot (slots=%d)", c.Name, c.ID, c.Slots)
		}
		// Must be usable by an enchanter (all-class sentinel or the bit set).
		if !(c.Classes == 0 || c.Classes >= 32767 || c.Classes&enchanterBit != 0) {
			t.Fatalf("%q (%d) not usable by enchanter (classes=%d)", c.Name, c.ID, c.Classes)
		}
		// Level gating: no item requiring above level 60.
		if c.ReqLevel > 60 {
			t.Fatalf("%q (%d) requires level %d", c.Name, c.ID, c.ReqLevel)
		}
		if c.FocusEffect > 0 && c.FocusName != "" {
			sawFocus = true
		}
	}
	if !sawFocus {
		t.Errorf("expected at least one head item with a named focus effect")
	}
	t.Logf("enchanter head candidates: %d", len(cands))
}

// TestUpgradeCandidates_ExcludesNoRent guards the norent filter direction. NO
// RENT items are encoded as norent=0 in this dataset (every rentable item is
// non-zero, typically -1), so the finder must drop only the norent=0 rows. A
// flipped comparison (norent=0 kept) collapsed every slot to a handful of
// summoned temporaries — the regression this test exists to catch.
func TestUpgradeCandidates_ExcludesNoRent(t *testing.T) {
	d := openTestDB(t)

	const headBit = 0x000004
	cands, err := d.UpgradeCandidates(db.CandidateFilter{
		SlotMask: headBit,
		ClassBit: 0x2000, // enchanter
		RaceBit:  0x0020, // Dark Elf
		MaxLevel: 60,
	})
	if err != nil {
		t.Fatalf("UpgradeCandidates: %v", err)
	}
	if len(cands) < 20 {
		t.Fatalf("filter dropped too much — got %d head items, expected the full rentable set", len(cands))
	}
	for _, c := range cands {
		if c.NoRent == 0 {
			t.Errorf("%q (%d) is NO RENT (norent=0) but was offered as an upgrade", c.Name, c.ID)
		}
	}
}

// TestUpgradeCandidates_CombinedMaskPartition verifies the overview endpoint's
// optimization: querying the union of every slot mask once and partitioning the
// result in Go by (slots & slotMask) yields exactly the same per-slot candidate
// set as a separate query per slot. If this holds, the 19-query sweep can be
// collapsed to one scan without changing results.
func TestUpgradeCandidates_CombinedMaskPartition(t *testing.T) {
	d := openTestDB(t)

	// A representative spread including the dual slots (ear/wrist/fingers) whose
	// masks OR two item bits — the partition must handle those correctly.
	slotMasks := []int{
		0x000002 | 0x000010, // ear
		0x000004,            // head
		0x000020,            // neck
		0x000200 | 0x000400, // wrist
		0x002000,            // primary
		0x004000,            // secondary
		0x008000 | 0x010000, // fingers
		0x020000,            // chest
	}
	const (
		enchanterBit = 0x2000
		darkElf      = 0x0020
		maxLevel     = 60
	)

	combined := 0
	for _, m := range slotMasks {
		combined |= m
	}
	all, err := d.UpgradeCandidates(db.CandidateFilter{
		SlotMask: combined, ClassBit: enchanterBit, RaceBit: darkElf, MaxLevel: maxLevel,
	})
	if err != nil {
		t.Fatalf("combined query: %v", err)
	}

	for _, mask := range slotMasks {
		perSlot, err := d.UpgradeCandidates(db.CandidateFilter{
			SlotMask: mask, ClassBit: enchanterBit, RaceBit: darkElf, MaxLevel: maxLevel,
		})
		if err != nil {
			t.Fatalf("per-slot query (mask=%#x): %v", mask, err)
		}
		want := map[int]bool{}
		for _, c := range perSlot {
			want[c.ID] = true
		}
		got := map[int]bool{}
		for _, c := range all {
			if c.Slots&mask != 0 {
				got[c.ID] = true
			}
		}
		if len(got) != len(want) {
			t.Fatalf("mask %#x: partition has %d items, per-slot query has %d", mask, len(got), len(want))
		}
		for id := range want {
			if !got[id] {
				t.Fatalf("mask %#x: partition missing item %d returned by per-slot query", mask, id)
			}
		}
	}
}

func TestUpgradeCandidates_LevelGate(t *testing.T) {
	d := openTestDB(t)

	const chestBit = 0x020000
	low, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: chestBit, MaxLevel: 10})
	if err != nil {
		t.Fatal(err)
	}
	high, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: chestBit, MaxLevel: 60})
	if err != nil {
		t.Fatal(err)
	}
	// A higher level cap can only ever unlock more (or equal) items.
	if len(high) < len(low) {
		t.Fatalf("level 60 returned fewer chest items (%d) than level 10 (%d)", len(high), len(low))
	}
	for _, c := range low {
		if c.ReqLevel > 10 {
			t.Fatalf("level-10 query leaked %q requiring level %d", c.Name, c.ReqLevel)
		}
	}
}

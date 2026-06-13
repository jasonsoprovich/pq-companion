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

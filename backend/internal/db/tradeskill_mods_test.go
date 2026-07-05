package db_test

import "testing"

// TestTradeskillModifiers checks the skill-mod catalog against the shipped
// quarm.db: Tinkering (skill 57) has boost items, they carry positive percent
// values, and results are ordered best-bonus-first. Fishing (55) is the
// densest and Sense Traps (62) has none.
func TestTradeskillModifiers(t *testing.T) {
	d := openTestDB(t)

	tinkering, err := d.TradeskillModifiers(57)
	if err != nil {
		t.Fatalf("TradeskillModifiers(57): %v", err)
	}
	if len(tinkering) == 0 {
		t.Fatal("expected Tinkering skill-mod items, got none")
	}
	for i, m := range tinkering {
		if m.Value <= 0 {
			t.Errorf("modifier %q has non-positive value %d", m.Name, m.Value)
		}
		if m.ItemID <= 0 || m.Name == "" {
			t.Errorf("modifier %d missing id/name: %+v", i, m)
		}
		if i > 0 && tinkering[i-1].Value < m.Value {
			t.Errorf("not sorted best-first: %d before %d", tinkering[i-1].Value, m.Value)
		}
	}

	// Sense Traps has no boost items in the dump.
	senseTraps, err := d.TradeskillModifiers(62)
	if err != nil {
		t.Fatalf("TradeskillModifiers(62): %v", err)
	}
	if len(senseTraps) != 0 {
		t.Errorf("expected no Sense Traps modifiers, got %d", len(senseTraps))
	}
}

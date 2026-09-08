package character

import "testing"

func TestSetUnspentAA(t *testing.T) {
	s, _ := openTestStore(t)

	// Fresh character: -1 sentinel = never observed.
	c, ok, err := s.GetByName("Testchar")
	if err != nil || !ok {
		t.Fatalf("get char: ok=%v err=%v", ok, err)
	}
	if c.UnspentAA != -1 || c.UnspentAAAt != 0 {
		t.Fatalf("new char unspent_aa=%d@%d, want -1@0", c.UnspentAA, c.UnspentAAAt)
	}

	// First observation lands.
	if err := s.SetUnspentAA("testchar", 12, 1_700_000_000); err != nil {
		t.Fatalf("set: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.UnspentAA != 12 || c.UnspentAAAt != 1_700_000_000 {
		t.Fatalf("after first set: %d@%d", c.UnspentAA, c.UnspentAAAt)
	}

	// A newer reading (points spent down to 3) overwrites.
	if err := s.SetUnspentAA("TESTCHAR", 3, 1_700_000_500); err != nil {
		t.Fatalf("set 2: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.UnspentAA != 3 || c.UnspentAAAt != 1_700_000_500 {
		t.Fatalf("after newer set: %d@%d", c.UnspentAA, c.UnspentAAAt)
	}

	// A stale reading (older timestamp, e.g. a log backfill replaying an old
	// ding) must NOT clobber the current value.
	if err := s.SetUnspentAA("Testchar", 99, 1_699_000_000); err != nil {
		t.Fatalf("stale set: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.UnspentAA != 3 || c.UnspentAAAt != 1_700_000_500 {
		t.Errorf("stale reading clobbered current value: %d@%d", c.UnspentAA, c.UnspentAAAt)
	}

	// Equal timestamp is allowed to write (same tick re-report).
	if err := s.SetUnspentAA("Testchar", 4, 1_700_000_500); err != nil {
		t.Fatalf("equal-ts set: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.UnspentAA != 4 {
		t.Errorf("equal-ts set didn't land: %d", c.UnspentAA)
	}

	// List() carries the fields.
	list, _ := s.List()
	if len(list) != 1 || list[0].UnspentAA != 4 {
		t.Errorf("List()[0].UnspentAA = %d (n=%d)", list[0].UnspentAA, len(list))
	}

	// Empty name, negative points, and unknown names are silent no-ops.
	if err := s.SetUnspentAA("", 5, 2_000_000_000); err != nil {
		t.Errorf("empty name should no-op: %v", err)
	}
	if err := s.SetUnspentAA("Testchar", -5, 2_000_000_000); err != nil {
		t.Errorf("negative points should no-op: %v", err)
	}
	if err := s.SetUnspentAA("Nobody", 5, 2_000_000_000); err != nil {
		t.Errorf("unknown character should no-op: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.UnspentAA != 4 {
		t.Errorf("no-op calls changed the row: %d", c.UnspentAA)
	}
}

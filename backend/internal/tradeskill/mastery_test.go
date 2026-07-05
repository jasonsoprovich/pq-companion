package tradeskill

import "testing"

func TestMasteryFor(t *testing.T) {
	// The three honored disciplines, with their eqmacids.
	for _, tc := range []struct {
		ts      int
		eqmacid int
		name    string
	}{
		{59, 49, "Alchemy Mastery"},
		{68, 56, "Jewelcraft Mastery"},
		{56, 103, "Poison Mastery"},
	} {
		m, ok := MasteryFor(tc.ts)
		if !ok {
			t.Errorf("tradeskill %d: expected a mastery AA", tc.ts)
			continue
		}
		if m.EqmacID != tc.eqmacid || m.Name != tc.name {
			t.Errorf("tradeskill %d: got %+v, want eqmacid %d %q", tc.ts, m, tc.eqmacid, tc.name)
		}
	}
	// Fletching (64) and Blacksmithing (63) are NOT honored by the combine code.
	for _, ts := range []int{64, 63, 0, 60, 61, 65, 69, 57} {
		if _, ok := MasteryFor(ts); ok {
			t.Errorf("tradeskill %d should not have a honored mastery AA", ts)
		}
	}
}

func TestFailReducePct(t *testing.T) {
	want := map[int]int{0: 0, 1: 10, 2: 25, 3: 50, 4: 0, -1: 0}
	for rank, pct := range want {
		if got := FailReducePct(rank); got != pct {
			t.Errorf("FailReducePct(%d)=%d, want %d", rank, got, pct)
		}
	}
}

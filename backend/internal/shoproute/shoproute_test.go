package shoproute

import (
	"reflect"
	"testing"
)

// avail is a tiny helper to build a SpellAvail from a list of zone names.
func avail(id int, zones ...string) SpellAvail {
	set := make(map[string]bool, len(zones))
	for _, z := range zones {
		set[z] = true
	}
	return SpellAvail{SpellID: id, Zones: set}
}

func TestSolve(t *testing.T) {
	tests := []struct {
		name      string
		spells    []SpellAvail
		wantStops []Stop
		wantUncov []int
	}{
		{
			name:      "empty list",
			spells:    nil,
			wantStops: nil,
			wantUncov: nil,
		},
		{
			name:   "single spell single zone is an anchor",
			spells: []SpellAvail{avail(1, "qeynos")},
			wantStops: []Stop{
				{Zone: "qeynos", Reason: ReasonAnchor, SpellIDs: []int{1}},
			},
		},
		{
			name: "anchor pulls in co-located spells for free",
			spells: []SpellAvail{
				avail(1, "highpass"),           // anchor: only source
				avail(2, "highpass", "qeynos"), // co-located, picked up free
				avail(3, "highpass", "freeport"),
			},
			wantStops: []Stop{
				{Zone: "highpass", Reason: ReasonAnchor, SpellIDs: []int{1, 2, 3}},
			},
		},
		{
			name: "two anchors visited in sorted order",
			spells: []SpellAvail{
				avail(1, "qeynos"),
				avail(2, "freeport"),
			},
			wantStops: []Stop{
				{Zone: "freeport", Reason: ReasonAnchor, SpellIDs: []int{2}},
				{Zone: "qeynos", Reason: ReasonAnchor, SpellIDs: []int{1}},
			},
		},
		{
			name: "greedy picks densest zone first",
			spells: []SpellAvail{
				avail(1, "nkarana", "qeynos"),
				avail(2, "nkarana", "freeport"),
				avail(3, "nkarana", "halas"),
				avail(4, "qeynos", "freeport"),
			},
			// nkarana covers 1,2,3. Then 4 remains, tie between qeynos and
			// freeport -> freeport wins on lexicographic order.
			wantStops: []Stop{
				{Zone: "nkarana", Reason: ReasonGreedy, SpellIDs: []int{1, 2, 3}},
				{Zone: "freeport", Reason: ReasonGreedy, SpellIDs: []int{4}},
			},
		},
		{
			name: "anchor first then greedy on remainder",
			spells: []SpellAvail{
				avail(1, "sol"),                 // anchor
				avail(2, "commons", "ecommons"), // greedy
				avail(3, "commons", "nektulos"), // greedy, co-located w/ 2
				avail(4, "sol", "commons"),      // free via sol anchor
			},
			wantStops: []Stop{
				{Zone: "sol", Reason: ReasonAnchor, SpellIDs: []int{1, 4}},
				{Zone: "commons", Reason: ReasonGreedy, SpellIDs: []int{2, 3}},
			},
		},
		{
			name: "unavailable spells reported, others still routed",
			spells: []SpellAvail{
				avail(1, "qeynos"),
				avail(2), // no zones -> unavailable
				avail(3), // no zones -> unavailable
			},
			wantStops: []Stop{
				{Zone: "qeynos", Reason: ReasonAnchor, SpellIDs: []int{1}},
			},
			wantUncov: []int{2, 3},
		},
		{
			name: "greedy tie broken lexicographically and deterministically",
			spells: []SpellAvail{
				avail(1, "zzz", "aaa"),
				avail(2, "zzz", "aaa"),
			},
			// Both zones cover both spells equally -> aaa wins.
			wantStops: []Stop{
				{Zone: "aaa", Reason: ReasonGreedy, SpellIDs: []int{1, 2}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Solve(tt.spells, nil)
			if !reflect.DeepEqual(got.Stops, tt.wantStops) {
				t.Errorf("stops:\n got %+v\nwant %+v", got.Stops, tt.wantStops)
			}
			if !reflect.DeepEqual(got.Uncovered, tt.wantUncov) {
				t.Errorf("uncovered: got %v want %v", got.Uncovered, tt.wantUncov)
			}
			// Every routed spell must be covered exactly once across all stops.
			seen := map[int]int{}
			for _, s := range got.Stops {
				for _, id := range s.SpellIDs {
					seen[id]++
				}
			}
			for id, n := range seen {
				if n != 1 {
					t.Errorf("spell %d credited to %d stops, want 1", id, n)
				}
			}
		})
	}
}

// TestSolveDeterministic confirms repeated runs over the same input (with map
// iteration order varying) yield identical routes.
func TestSolveDeterministic(t *testing.T) {
	spells := []SpellAvail{
		avail(1, "a", "b", "c"),
		avail(2, "b", "c"),
		avail(3, "c", "d"),
		avail(4, "d"),
		avail(5, "a", "e"),
	}
	first := Solve(spells, nil)
	for i := 0; i < 50; i++ {
		got := Solve(spells, nil)
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differed:\n got %+v\nwant %+v", i, got, first)
		}
	}
}

// TestSolveDistanceTiebreak covers the real bug: a single spell sold in several
// zones used to always go to the alphabetically-first zone, ignoring where the
// player starts. With distances, the nearest source wins; with no distances the
// old alphabetical tiebreak still holds.
func TestSolveDistanceTiebreak(t *testing.T) {
	// "strengthen" is sold in three zones; akanon sorts first alphabetically.
	spells := []SpellAvail{avail(1, "akanon", "poknowledge", "shadowhaven")}

	// No distance signal -> alphabetical tiebreak picks akanon (old behaviour).
	if got := Solve(spells, nil).Stops[0].Zone; got != "akanon" {
		t.Errorf("no-dist tiebreak: got %q, want akanon", got)
	}

	// Starting next to Shadow Haven -> it's the nearest source and should win,
	// even though akanon and poknowledge sort earlier.
	dist := map[string]int{"shadowhaven": 0, "nexus": 1, "akanon": 5, "poknowledge": 9}
	if got := Solve(spells, dist).Stops[0].Zone; got != "shadowhaven" {
		t.Errorf("dist tiebreak: got %q, want shadowhaven", got)
	}

	// A zone absent from the distance map is treated as far away, so a known-near
	// zone still beats it.
	dist2 := map[string]int{"akanon": 2} // shadowhaven/poknowledge unreachable
	if got := Solve(spells, dist2).Stops[0].Zone; got != "akanon" {
		t.Errorf("partial-dist tiebreak: got %q, want akanon", got)
	}
}

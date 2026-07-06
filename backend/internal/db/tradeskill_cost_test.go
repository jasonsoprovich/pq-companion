package db

import "testing"

// newTestResolver builds a costResolver over a hand-made recipe graph so the
// recursion, cheapest-of-buy-or-craft, cycle guard, and sub-combine detection
// can be checked deterministically.
//
//	item 1: vendor 10                (raw, bought)
//	item 2: vendor 5                 (raw, bought)
//	item 10: NOT sold; recipe 100 (triv 20) = 2x1+1x2, recipe 101 (triv 12) = 2x2
//	         -> craft via 100 = 25, via 101 = 10; cheapest 10; easiest = 101
//	item 20: vendor 100; recipe 200 = 3x2, yield 1         -> min(100, 15) = 15
//	item 30: NOT sold, not craftable                       -> unknown (farmed)
//	item 40: NOT sold; recipe 400 = 10x1, yield 4          -> 100/4 = 25 per unit
//	item 50 <-> item 51: mutually crafted, neither sold    -> cycle, unknown
//
// Cross-discipline items (target discipline 1) for obtainableWithin:
//
//	item 60: NOT sold; only recipe 600 (discipline 2)      -> needs discipline 2
//	item 70: NOT sold; recipe 700 (disc 2) or 701 (disc 1) -> stays in 1
//	item 80: NOT sold; recipe 800 (disc 1) but uses item60 -> transitively needs 2
//	item 90: NOT sold; recipe 900 (Common Combine, disc 0) -> no skill gate
func newTestResolver() *costResolver {
	return &costResolver{
		vendorPrice: map[int]int{1: 10, 2: 5, 20: 100},
		producedBy: map[int][]int{
			10: {100, 101}, 20: {200}, 40: {400}, 50: {500}, 51: {510},
			60: {600}, 70: {700, 701}, 80: {800}, 90: {900},
		},
		components: map[int][]recipeComp{
			100: {{itemID: 1, count: 2}, {itemID: 2, count: 1}},
			101: {{itemID: 2, count: 2}},
			200: {{itemID: 2, count: 3}},
			400: {{itemID: 1, count: 10}},
			500: {{itemID: 51, count: 1}},
			510: {{itemID: 50, count: 1}},
			600: {{itemID: 1, count: 1}},
			700: {{itemID: 1, count: 1}},
			701: {{itemID: 1, count: 1}},
			800: {{itemID: 60, count: 1}},
			900: {{itemID: 1, count: 1}},
		},
		yield: map[int]int{
			100: 1, 101: 1, 200: 1, 400: 4, 500: 1, 510: 1,
			600: 1, 700: 1, 701: 1, 800: 1, 900: 1,
		},
		trivial: map[int]int{
			100: 20, 101: 12, 200: 15, 400: 30, 500: 5, 510: 5,
			// item 70's cross producer 700 has a LOWER trivial than the
			// in-discipline 701, to prove in-discipline preference wins anyway.
			600: 10, 700: 6, 701: 8, 800: 10, 900: 5,
		},
		recipeTS: map[int]int{
			100: 1, 101: 1, 200: 1, 400: 1, 500: 1, 510: 1,
			600: 2, 700: 2, 701: 1, 800: 1, 900: 0,
		},
		memo:       map[int]itemAcquisition{},
		inProgress: map[int]bool{},
		withinMemo: map[int]bool{},
		withinProg: map[int]bool{},
	}
}

func TestCostResolver_Cost(t *testing.T) {
	r := newTestResolver()
	cases := []struct {
		item      int
		wantCop   int
		wantKnown bool
	}{
		{1, 10, true},  // vendor raw
		{10, 10, true}, // cheapest of two producers: recipe 101 (2*5) beats 100 (25)
		{20, 15, true}, // craft (3*5=15) beats vendor (100)
		{30, 0, false}, // farmed
		{40, 25, true}, // yield division: 10*10 / 4
		{50, 0, false}, // cycle
		{51, 0, false}, // cycle
	}
	for _, c := range cases {
		got := r.cost(c.item)
		if got.known != c.wantKnown || (c.wantKnown && got.copper != c.wantCop) {
			t.Errorf("cost(%d) = {%d, %v}, want {%d, %v}",
				c.item, got.copper, got.known, c.wantCop, c.wantKnown)
		}
	}
}

func TestCostResolver_ObtainableWithin(t *testing.T) {
	r := newTestResolver()
	const target = 1
	cases := []struct {
		item int
		want bool
	}{
		{1, true},   // vendor raw
		{10, true},  // craftable in discipline 1
		{30, true},  // farmed/raw (not craftable) — no skill needed
		{60, false}, // only craftable in discipline 2
		{70, true},  // has a discipline-1 producer alongside the discipline-2 one
		{80, false}, // discipline-1 recipe, but transitively needs item 60 (disc 2)
		{90, true},  // Common Combine producer — not skill-gated
	}
	for _, c := range cases {
		if got := r.obtainableWithin(c.item, target); got != c.want {
			t.Errorf("obtainableWithin(%d, %d) = %v, want %v", c.item, target, got, c.want)
		}
	}
	// From discipline 2's own perspective, item 60 is in-discipline. Uses a fresh
	// resolver — withinMemo is keyed by item for a single target discipline.
	if !newTestResolver().obtainableWithin(60, 2) {
		t.Error("obtainableWithin(60, 2) = false, want true (recipe 600 is discipline 2)")
	}
}

func TestCostResolver_CraftableSubcombine(t *testing.T) {
	r := newTestResolver()
	const target = 1
	cases := []struct {
		item      int
		consumer  int
		wantPid   int
		wantIsSub bool
	}{
		{10, 999, 101, true}, // not sold, two disc-1 producers -> lowest trivial (101, 12)
		{1, 999, 0, false},   // vendor raw -> just buy it
		{20, 999, 0, false},  // vendor-sold (even though craft is cheaper) -> not forced
		{30, 999, 0, false},  // not craftable
		{10, 101, 100, true}, // exclude the self recipe -> falls back to the other producer
		{70, 999, 701, true}, // in-discipline 701 preferred over lower-trivial cross 700
		{60, 999, 600, true}, // only a cross producer exists -> still returned
	}
	for _, c := range cases {
		pid, isSub := r.craftableSubcombine(c.item, c.consumer, target)
		if isSub != c.wantIsSub || (c.wantIsSub && pid != c.wantPid) {
			t.Errorf("craftableSubcombine(%d, consumer %d) = (%d, %v), want (%d, %v)",
				c.item, c.consumer, pid, isSub, c.wantPid, c.wantIsSub)
		}
	}
}

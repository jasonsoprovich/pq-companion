package db

import "testing"

func TestResolvePoPRoutes(t *testing.T) {
	set := func(ids ...int) map[int]bool {
		m := map[int]bool{}
		for _, id := range ids {
			m[id] = true
		}
		return m
	}
	// Items: 1 = PoP pattern, 2 = pre-PoP component, 3 = unknown component.
	recipes := []popRecipe{
		{components: []int{1, 2}, products: []int{10}}, // PoP combine
		{components: []int{2}, products: []int{11}},    // pre-PoP combine
		{components: []int{1}, products: []int{12}},    // PoP route for 12...
		{components: []int{2}, products: []int{12}},    // ...plus a pre-PoP one
		{components: []int{10}, products: []int{13}},   // chained off PoP 10
		{components: []int{3}, products: []int{14}},    // unknown component
		{components: []int{1}, products: []int{15}},    // PoP route, but 15...
	}
	hasPoP := set(1, 16)
	hasNonPoP := set(2, 15) // ...also drops pre-PoP
	recipes = append(recipes,
		popRecipe{components: []int{3}, products: []int{16}}, // unknown route can't clear a PoP drop
	)
	got := resolvePoPRoutes(set(), hasPoP, hasNonPoP, recipes)

	cases := []struct {
		id   int
		want bool
		why  string
	}{
		{1, true, "PoP-only drop"},
		{2, false, "pre-PoP drop"},
		{10, true, "combine needs a PoP component"},
		{11, false, "combine of pre-PoP components"},
		{12, false, "an alternative pre-PoP recipe keeps it available"},
		{13, true, "chained off a PoP-gated product"},
		{14, false, "unknown component doesn't hide a recipe-only item"},
		{15, false, "a pre-PoP drop beats a PoP recipe"},
		{16, true, "an unknown recipe route doesn't clear a PoP-only drop"},
	}
	for _, c := range cases {
		if got[c.id] != c.want {
			t.Errorf("item %d gated=%v, want %v (%s)", c.id, got[c.id], c.want, c.why)
		}
	}
}

func TestResolvePoPRoutes_TaggedNeverAvailable(t *testing.T) {
	tagged := map[int]bool{5: true}
	recipes := []popRecipe{{components: []int{5}, products: []int{6}}}
	got := resolvePoPRoutes(tagged, map[int]bool{}, map[int]bool{5: true}, recipes)
	if !got[5] || !got[6] {
		t.Errorf("tagged item and its product should be gated, got %v", got)
	}
}

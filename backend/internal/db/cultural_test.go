package db

import "testing"

func TestContainerRaceRestrict(t *testing.T) {
	cases := []struct {
		container string
		want      int
	}{
		{"Vale Sewing Kit", 11},       // Halfling
		{"Fier`Dal Sewing Kit", 4},    // Wood Elf (backtick spelling)
		{"Fierdal Sewing Kit", 4},     // Wood Elf (no-backtick spelling)
		{"Fierdal Fletching Kit", 4},  // Wood Elf
		{"Erudite Sewing Kit", 3},     // Erudite
		{"VALE SEWING KIT", 11},       // case-insensitive
		{"Collapsible Sewing Kit", 0}, // generic, open to all
		{"Forge", 0},                  // world container
		{"Coldain Tanners Kit", 0},    // faction-gated, intentionally NOT race-locked
		{"", 0},                       // empty
	}
	for _, tc := range cases {
		if got := ContainerRaceRestrict(tc.container); got != tc.want {
			t.Errorf("ContainerRaceRestrict(%q) = %d, want %d", tc.container, got, tc.want)
		}
	}
}

func TestContainersRaceRestrict(t *testing.T) {
	con := func(names ...string) []RecipeEntry {
		out := make([]RecipeEntry, len(names))
		for i, n := range names {
			out[i] = RecipeEntry{ItemName: n}
		}
		return out
	}
	cases := []struct {
		name       string
		containers []RecipeEntry
		want       int
	}{
		{"single cultural", con("Vale Sewing Kit"), 11},
		{"cultural + generic → open to all", con("Vale Sewing Kit", "Collapsible Sewing Kit"), 0},
		{"two kits same race", con("Fier`Dal Sewing Kit", "Fierdal Fletching Kit"), 4},
		{"two kits different races", con("Vale Sewing Kit", "Erudite Sewing Kit"), 0},
		{"only generic", con("Collapsible Sewing Kit"), 0},
		{"no containers", nil, 0},
	}
	for _, tc := range cases {
		if got := containersRaceRestrict(tc.containers); got != tc.want {
			t.Errorf("%s: containersRaceRestrict = %d, want %d", tc.name, got, tc.want)
		}
	}
}

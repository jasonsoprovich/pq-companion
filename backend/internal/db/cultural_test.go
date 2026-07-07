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

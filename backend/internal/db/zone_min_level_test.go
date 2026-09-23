package db

import "testing"

// TestApplyMinLevelOverride guards the query-side correction for EQMacEmu PR
// #386 ("Set minimum levels for Plane of Power zones") — see the comment on
// applyMinLevelOverride. Remove this override (and the test) once quarm.db is
// regenerated from a dump that already carries min_level=46 for these zones.
func TestApplyMinLevelOverride(t *testing.T) {
	cases := []struct {
		name         string
		zoneIDNumber int
		raw          int
		want         int
	}{
		{"codecay (200) raised from 0", 200, 0, 46},
		{"pojustice (201) already 15, still raised to 46", 201, 15, 46},
		{"poknowledge (202) excluded, kept at 0", 202, 0, 0},
		{"potranquility (203) raised from 0", 203, 0, 46},
		{"poearthb (222) raised from 0", 222, 0, 46},
		{"potimea (219) already above 46, untouched", 219, 64, 64},
		{"potimeb (223) already above 46, untouched", 223, 64, 64},
		{"below range untouched", 199, 0, 0},
		{"above range untouched", 224, 0, 0},
		{"already 46+ untouched", 210, 50, 50},
		{"non-PoP zone untouched", 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyMinLevelOverride(tc.zoneIDNumber, tc.raw); got != tc.want {
				t.Errorf("applyMinLevelOverride(%d, %d) = %d, want %d", tc.zoneIDNumber, tc.raw, got, tc.want)
			}
		})
	}
}

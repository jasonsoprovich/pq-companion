package respawn

import "testing"

func TestReduceRespawnTime(t *testing.T) {
	tests := []struct {
		name    string
		base    int
		level   int
		reduced bool
		dungeon bool
		want    int
	}{
		// The motivating case: netherbian drone (level 18) in a dungeon-flagged
		// reduced zone. Raw 1200s (20 min) -> dungeon higher bound 480s (8 min).
		{"netherbian drone dungeon higher", 1200, 18, true, true, 480},
		{"dungeon lower bound", 600, 30, true, true, 360},
		{"dungeon below all ranges unchanged", 300, 20, true, true, 300},
		{"dungeon above all ranges unchanged", 3000, 50, true, true, 3000},
		// Dungeon boundary exactness (ms compare avoids off-by-one): a 360s base
		// is 360000ms, just below the 360001ms lower bound -> unchanged.
		{"dungeon 360s just below lower bound", 360, 20, true, true, 360},
		{"dungeon 900s hits higher bound", 900, 20, true, true, 480},

		// Standard reduced zones only speed up newbie mobs (level 1..14).
		{"standard newbie higher bound", 350, 8, true, false, 60},
		{"standard newbie lower bound", 30, 5, true, false, 12},
		{"standard non-newbie unchanged", 350, 30, true, false, 350},
		{"standard level 0 unchanged", 350, 0, true, false, 350},
		{"standard level 14 still newbie", 350, 14, true, false, 60},
		{"standard level 15 not newbie", 350, 15, true, false, 350},

		// Reduction disabled / degenerate inputs.
		{"not a reduced zone", 1200, 18, false, true, 1200},
		{"zero base unchanged", 0, 18, true, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reduceRespawnTime(tc.base, tc.level, tc.reduced, tc.dungeon)
			if got != tc.want {
				t.Errorf("reduceRespawnTime(%d, lvl=%d, reduced=%v, dungeon=%v): got %d, want %d",
					tc.base, tc.level, tc.reduced, tc.dungeon, got, tc.want)
			}
		})
	}
}

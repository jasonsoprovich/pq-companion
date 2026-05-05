package spelltimer

import "testing"

func TestCalcDurationTicks(t *testing.T) {
	tests := []struct {
		name    string
		formula int
		base    int
		level   int
		want    int
	}{
		// Formula 0: instant, always 0
		{name: "instant", formula: 0, base: 10, level: 60, want: 0},
		// Formula 1: min(level/2, base)
		{name: "f1 capped by base", formula: 1, base: 20, level: 60, want: 20},
		{name: "f1 capped by level", formula: 1, base: 100, level: 60, want: 30},
		{name: "f1 low level", formula: 1, base: 100, level: 10, want: 5},
		// Formula 3: min(level*30, base) — used by some long mezzes
		{name: "f3 capped by base", formula: 3, base: 200, level: 60, want: 200},
		{name: "f3 low level", formula: 3, base: 1800, level: 5, want: 150},
		// Formula 8: Quarm-style fixed-base buff (e.g. Pacify, base=60).
		// EQEmu canonical "level + base capped at base*3" overshoots and
		// would yield 12-minute Pacify timers — verified against PQDI which
		// shows 6 minutes / 60 ticks for spell 45.
		{name: "f8 pacify 60", formula: 8, base: 60, level: 60, want: 60},
		{name: "f8 low level", formula: 8, base: 60, level: 5, want: 60},
		// Formula 9: min(level*2 + 10, base) — anchored on PQDI Min Duration
		// at each spell's minimum castable level.
		{name: "f9 lull lvl1", formula: 9, base: 20, level: 1, want: 12},   // PQDI Lull min
		{name: "f9 lull cap", formula: 9, base: 20, level: 60, want: 20},   // capped by base
		{name: "f9 tashanian lvl57", formula: 9, base: 140, level: 57, want: 124}, // PQDI Tashanian min
		{name: "f9 tashanian lvl60", formula: 9, base: 140, level: 60, want: 130},
		// Formula 10: min(level*3 + 10, base) — anchored on PQDI charm-line
		// Min Duration at each spell's minimum castable level.
		{name: "f10 charm lvl12", formula: 10, base: 205, level: 12, want: 46},     // PQDI Charm min
		{name: "f10 beguile lvl24", formula: 10, base: 205, level: 24, want: 82},   // PQDI Beguile min
		{name: "f10 cajoling lvl39", formula: 10, base: 205, level: 39, want: 127}, // PQDI Cajoling min
		{name: "f10 charm lvl60", formula: 10, base: 205, level: 60, want: 190},
		{name: "f10 cap reached", formula: 10, base: 205, level: 65, want: 205},
		// Formula 11: fixed base regardless of level
		{name: "f11 fixed", formula: 11, base: 72, level: 1, want: 72},
		{name: "f11 fixed high level", formula: 11, base: 72, level: 60, want: 72},
		// Formula 50: level/5
		{name: "f50", formula: 50, base: 0, level: 60, want: 12},
		{name: "f50 min 1", formula: 50, base: 0, level: 3, want: 1},
		// Formula 3600: treated as instant
		{name: "f3600", formula: 3600, base: 100, level: 60, want: 0},
		// Default/unknown formula falls back to base
		{name: "unknown formula", formula: 99, base: 40, level: 60, want: 40},
		// Level 0 guard: treated as level 1
		{name: "level 0", formula: 11, base: 30, level: 0, want: 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcDurationTicks(tc.formula, tc.base, tc.level)
			if got != tc.want {
				t.Errorf("CalcDurationTicks(formula=%d, base=%d, level=%d) = %d, want %d",
					tc.formula, tc.base, tc.level, got, tc.want)
			}
		})
	}
}

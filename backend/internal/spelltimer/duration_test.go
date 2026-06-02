package spelltimer

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// bardSpell returns a *db.Spell shaped like a bard song: classes8 (bard)
// learnable at the given level, all other classes locked at 255.
func bardSpell(formula, base int) *db.Spell {
	var cls [15]int
	for i := range cls {
		cls[i] = 255
	}
	cls[7] = 50 // bard learns at level 50
	return &db.Spell{
		BuffDurationFormula: formula,
		BuffDuration:        base,
		ClassLevels:         cls,
	}
}

// nonBardSpell returns a *db.Spell castable by chanter (classes12 = index 11)
// at the given level — used to confirm the bard clamp does NOT apply to
// non-song spells with the same formula/base.
func nonBardSpell(formula, base int) *db.Spell {
	var cls [15]int
	for i := range cls {
		cls[i] = 255
	}
	cls[11] = 50 // enchanter
	return &db.Spell{
		BuffDurationFormula: formula,
		BuffDuration:        base,
		ClassLevels:         cls,
	}
}

func TestSpellDurationTicks_BardSongUsesBase(t *testing.T) {
	// Selo's Accelerando: formula=5, base=3. EQMac formula 5 = min(2, base) =
	// 2 ticks. With the bard clamp we report the raw base of 3 ticks (18s)
	// instead — matches in-game fade after the bard stops singing.
	const level = 60
	bard := bardSpell(5, 3)
	if got := SpellDurationTicks(bard, level); got != 3 {
		t.Errorf("bard song SpellDurationTicks = %d ticks, want 3 (base clamp)", got)
	}

	// Non-bard spell with the same formula+base must still run the full
	// formula — the clamp is narrowly scoped to bard-only spells.
	nonBard := nonBardSpell(5, 3)
	if got := SpellDurationTicks(nonBard, level); got != 2 {
		t.Errorf("non-bard spell SpellDurationTicks = %d ticks, want 2 (formula 5 = min(2,base))", got)
	}
}

func TestIsBardSong_DisciplineExcluded(t *testing.T) {
	// Resistant Discipline (id 4585): listed in classes8 alongside warrior,
	// monk, rogue, etc. Not a song — must not trigger the bard clamp.
	var cls [15]int
	for i := range cls {
		cls[i] = 255
	}
	cls[0] = 30 // warrior
	cls[2] = 51 // monk
	cls[7] = 30 // bard
	disc := &db.Spell{ClassLevels: cls}
	if isBardSong(disc) {
		t.Errorf("isBardSong returned true for a cross-class discipline (multi-class)")
	}
}

func TestCalcDurationTicks(t *testing.T) {
	tests := []struct {
		name    string
		formula int
		base    int
		level   int
		want    int
	}{
		// Values are a faithful port of EQMacEmu CalcBuffDuration_formula and
		// were spot-checked against PQDI (Project Quarm's data site).
		//
		// Formula 0: instant, always 0
		{name: "instant", formula: 0, base: 10, level: 60, want: 0},
		// Formula 1: min(level/2, base)
		{name: "f1 capped by base", formula: 1, base: 20, level: 60, want: 20},
		{name: "f1 capped by level", formula: 1, base: 100, level: 60, want: 30},
		{name: "f1 low level", formula: 1, base: 100, level: 10, want: 5},
		// Formula 2: min(level<=1 ? 6 : level/2+5, base)
		{name: "f2 lvl1", formula: 2, base: 100, level: 1, want: 6},
		{name: "f2 lvl60", formula: 2, base: 100, level: 60, want: 35},
		{name: "f2 capped by base", formula: 2, base: 20, level: 60, want: 20},
		// Formula 3: min(level*30, base) — used by some long mezzes
		{name: "f3 capped by base", formula: 3, base: 200, level: 60, want: 200},
		{name: "f3 low level", formula: 3, base: 1800, level: 5, want: 150},
		// Formula 4: min(50, base) — fixed 50 ticks
		{name: "f4 fixed 50", formula: 4, base: 100, level: 60, want: 50},
		{name: "f4 capped by base", formula: 4, base: 30, level: 60, want: 30},
		// Formula 5: min(2, base) — verified against PQDI Invigor (base 3) = 2
		{name: "f5 invigor", formula: 5, base: 3, level: 60, want: 2},
		{name: "f5 base1", formula: 5, base: 1, level: 60, want: 1},
		// Formula 6: min(level/2+2, base) — verified against PQDI Forlorn Deeds
		// (id 1712, base 35, Enchanter L57): 30 ticks at L57, 32 at L60. The
		// previous modern-EQEmu formula reported ~105 ticks (issue #131).
		{name: "f6 forlorn lvl57", formula: 6, base: 35, level: 57, want: 30},
		{name: "f6 forlorn lvl60", formula: 6, base: 35, level: 60, want: 32},
		{name: "f6 capped by base", formula: 6, base: 20, level: 60, want: 20},
		// Formula 7: min(level, base) — verified against PQDI Berserker
		// Strength (id 21, base 30, Enchanter L20): 20 ticks at L20, caps at 30
		{name: "f7 berserker lvl20", formula: 7, base: 30, level: 20, want: 20},
		{name: "f7 berserker cap", formula: 7, base: 30, level: 60, want: 30},
		// Formula 8: min(level+10, base) — Pacify (id 45, base 60) = 60 at L60
		{name: "f8 pacify 60", formula: 8, base: 60, level: 60, want: 60},
		{name: "f8 low level", formula: 8, base: 60, level: 5, want: 15},
		// Formula 9: min(level*2 + 10, base) — anchored on PQDI Min Duration
		// at each spell's minimum castable level.
		{name: "f9 lull lvl1", formula: 9, base: 20, level: 1, want: 12},          // PQDI Lull min
		{name: "f9 lull cap", formula: 9, base: 20, level: 60, want: 20},          // capped by base
		{name: "f9 tashanian lvl57", formula: 9, base: 140, level: 57, want: 124}, // PQDI Tashanian min
		{name: "f9 tashanian lvl60", formula: 9, base: 140, level: 60, want: 130},
		// Formula 10: min(level*3 + 10, base) — anchored on PQDI charm-line
		// Min Duration at each spell's minimum castable level.
		{name: "f10 charm lvl12", formula: 10, base: 205, level: 12, want: 46},     // PQDI Charm min
		{name: "f10 beguile lvl24", formula: 10, base: 205, level: 24, want: 82},   // PQDI Beguile min
		{name: "f10 cajoling lvl39", formula: 10, base: 205, level: 39, want: 127}, // PQDI Cajoling min
		{name: "f10 charm lvl60", formula: 10, base: 205, level: 60, want: 190},
		{name: "f10 cap reached", formula: 10, base: 205, level: 65, want: 205},
		// Formula 11: min(level*30 + 90, base) — base-capped in practice
		{name: "f11 capped by base", formula: 11, base: 72, level: 1, want: 72},
		{name: "f11 high level", formula: 11, base: 72, level: 60, want: 72},
		// Formula 50: permanent buff — reported as 0 (no countdown timer)
		{name: "f50 permanent", formula: 50, base: 270, level: 60, want: 0},
		// Formula >= 200: literal tick count (the field IS the duration)
		{name: "f600 literal", formula: 600, base: 100, level: 60, want: 600},
		{name: "f3600 literal", formula: 3600, base: 100, level: 60, want: 3600},
		// Unknown formula (< 200, not in the table): no timer
		{name: "unknown formula", formula: 99, base: 40, level: 60, want: 0},
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

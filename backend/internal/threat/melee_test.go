package threat

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"
)

func TestMeleeSwingHate(t *testing.T) {
	cases := []struct {
		name                          string
		dmg, delay, itemType, lvl, cl int
		want                          int
	}{
		{
			// Pure caster: no damage bonus at any level → just the weapon damage.
			name: "caster 1H", dmg: 20, delay: 30, itemType: 0, lvl: 60, cl: eqstat.Wizard,
			want: 20,
		},
		{
			// Warrior under level 28: still no damage bonus.
			name: "warrior level 27", dmg: 20, delay: 30, itemType: 0, lvl: 27, cl: eqstat.Warrior,
			want: 20,
		},
		{
			// Warrior 60 with a 1H: base bonus 1 + (60-28)/3 = 1 + 10 = 11.
			name: "warrior 60 1H", dmg: 20, delay: 30, itemType: 0, lvl: 60, cl: eqstat.Warrior,
			want: 31,
		},
		{
			// Rogue 60 with a 1H piercer (itemType 2): warrior-class, base bonus 11.
			name: "rogue 60 1H pierce", dmg: 18, delay: 21, itemType: 2, lvl: 60, cl: eqstat.Rogue,
			want: 29,
		},
		{
			// Unknown / no weapon → 0 so the meter falls back to observed damage.
			name: "no weapon", dmg: 0, delay: 0, itemType: 0, lvl: 60, cl: eqstat.Warrior,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MeleeSwingHate(tc.dmg, tc.delay, tc.itemType, tc.lvl, tc.cl); got != tc.want {
				t.Errorf("MeleeSwingHate = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBackstabHate(t *testing.T) {
	cases := []struct {
		name       string
		dmg, level int
		want       int
	}{
		{
			// Level 60 rogue, ~14-dmg piercer: skill capped at 252 →
			// (252*0.02 + 2) * 14 = 7.04 * 14 = 98 (matches Quarmy's ~97).
			name: "level 60 piercer", dmg: 14, level: 60, want: 98,
		},
		{
			// Below the skill cap the multiplier is smaller: level 40 → skill 200 →
			// (200*0.02 + 2) * 14 = 6.0 * 14 = 84.
			name: "level 40 piercer", dmg: 14, level: 40, want: 84,
		},
		{
			// Unknown / no weapon → 0 so the meter falls back to observed damage.
			name: "no weapon", dmg: 0, level: 60, want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BackstabHate(tc.dmg, tc.level); got != tc.want {
				t.Errorf("BackstabHate = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMeleeSwingHate2HBonus(t *testing.T) {
	// A 2H weapon adds level- and delay-scaled bonus terms on top of the base.
	// Warrior 60, a 2H slasher (itemType 1) with delay 40:
	//   base       = 1 + (60-28)/3                 = 11
	//   levelBonus = (60-30)/5 + 1 = 7; +1 (>50)   = 8
	//                levelBonus2 = (60-50)=10, +4 (>59) = 14; 14*40/40 = 14 → +14 = 22
	//   delayBonus = (40-40)/3 + 1                 = 1
	//   bonus      = 11 + 22 + 1                   = 34
	// hate = weapon damage (40) + 34 = 74.
	got := MeleeSwingHate(40, 40, itemType2HSlash, 60, eqstat.Warrior)
	if got != 74 {
		t.Errorf("2H swing hate = %d, want 74 (40 dmg + 34 bonus)", got)
	}

	// The same 2H gives more hate than a 1H of equal damage (extra 2H terms).
	oneH := MeleeSwingHate(40, 40, 0, 60, eqstat.Warrior)
	if oneH >= got {
		t.Errorf("1H hate %d should be less than 2H hate %d", oneH, got)
	}
}

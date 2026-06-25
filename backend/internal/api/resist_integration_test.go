package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/resist"
)

// TestResistCheck_RealSpells exercises the db.Spell -> resist.Spell mapping and
// the classification against real quarm.db rows. The key case is Mesmerize:
// its no_partial_resist flag is 0, yet it must classify as binary because its
// first effect is Mez (not damage) — proving we honour the full
// IsPartialCapableSpell predicate, not just the column.
func TestResistCheck_RealSpells(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	cases := []struct {
		id         int
		name       string
		wantBinary bool
	}{
		{292, "Mesmerize", true},      // mez: no_partial_resist=0 but still binary
		{286, "Shallow Breath", true}, // snare: no_partial_resist=1
		{69, "Cinder Bolt", false},    // nuke: partial-capable
		{38, "Lightning Bolt", false}, // nuke: partial-capable
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sp, err := d.GetSpell(tc.id)
			if err != nil {
				t.Fatalf("GetSpell(%d): %v", tc.id, err)
			}
			in := resist.Input{
				Spell:        toResistSpell(sp),
				CasterLevel:  60,
				CasterClass:  13, // Enchanter (0-based)
				CasterCHA:    200,
				TargetLevel:  55,
				TargetResist: 100,
				Era:          resist.Era{LuclinEnabled: true},
			}
			got := resist.ComputeChances(in)
			if got.Binary != tc.wantBinary {
				t.Errorf("%s: Binary = %v, want %v (no_partial_resist=%d)",
					tc.name, got.Binary, tc.wantBinary, sp.NoPartialResist)
			}
			// Probabilities must form a valid distribution.
			sum := got.FullResist + got.Partial + got.FullDamage
			if sum < 0.99 || sum > 1.01 {
				t.Errorf("%s: probabilities sum to %v, want ~1", tc.name, sum)
			}
			if got.Binary && got.Partial != 0 {
				t.Errorf("%s: binary spell reported partials: %v", tc.name, got.Partial)
			}
		})
	}
}

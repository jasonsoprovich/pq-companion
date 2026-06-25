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

// TestResistCheck_CharmImmuneNPC reproduces the reported bug: Beguile on a Vex
// Thal NPC (charm-immune and well above the charm level cap) must report
// "cannot affect", not a ~33% chance.
func TestResistCheck_CharmImmuneNPC(t *testing.T) {
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

	// Eom Centien Xakra, Vex Thal: level 66, special_abilities include 14
	// (charm immunity). Beguile's charm effect caps at level 37.
	npcRes, err := d.SearchNPCs("Eom_Centien_Xakra", 5, 0, true)
	if err != nil || len(npcRes.Items) == 0 {
		t.Skip("Eom_Centien_Xakra not present in DB")
	}
	npc := npcRes.Items[0]

	beguile, err := d.GetSpell(182)
	if err != nil {
		t.Fatalf("GetSpell(Beguile): %v", err)
	}

	in := resist.Input{
		Spell:            toResistSpell(beguile),
		CasterLevel:      60,
		CasterClass:      13, // Enchanter
		CasterCHA:        95,
		TargetLevel:      max(npc.Level, npc.MaxLevel),
		TargetResist:     npc.MR,
		TargetImmunities: parseImmunities(npc.SpecialAbilities),
		Era:              resist.Era{LuclinEnabled: true},
	}
	got := resist.ComputeChances(in)
	if !got.CannotAffect {
		t.Fatalf("Beguile on charm-immune L%d NPC should be CannotAffect, got %+v",
			in.TargetLevel, got)
	}
	if got.LandChance != 0 {
		t.Errorf("LandChance = %v, want 0", got.LandChance)
	}
	t.Logf("reason: %s", got.Reason)
}

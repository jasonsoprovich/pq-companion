package buffmod

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestParseFocusDamageFocus locks in the fix for the user report that worn
// damage focus effects (e.g. Improved Damage III on Fedora Secundae) were
// missing from the Spell Modifiers tab. Improved Damage III (spell 2338) is
// SPA 124 (+20%) with a max-level 60 limit on detrimental spells.
func TestParseFocusDamageFocus(t *testing.T) {
	s := &db.Spell{ID: 2338, Name: "Improved Damage III"}
	s.EffectIDs = [12]int{SPAImprovedDamage, SPALimitMaxLevel, SPALimitSpellType}
	s.EffectBaseValues = [12]int{20, 60, SpellTypeDetrimental}

	mods := parseFocusSpell(s)
	if len(mods) != 1 {
		t.Fatalf("parseFocusSpell returned %d modifiers, want 1", len(mods))
	}
	m := mods[0]
	if m.SPA != SPAImprovedDamage {
		t.Errorf("SPA = %d, want %d (Improved Damage)", m.SPA, SPAImprovedDamage)
	}
	if m.Percent != 20 {
		t.Errorf("Percent = %d, want 20", m.Percent)
	}
	if m.Limits.MaxLevel != 60 {
		t.Errorf("MaxLevel = %d, want 60", m.Limits.MaxLevel)
	}
	if m.Limits.SpellType != SpellTypeDetrimental {
		t.Errorf("SpellType = %d, want %d (detrimental)", m.Limits.SpellType, SpellTypeDetrimental)
	}
}

// TestParseFocusZeroPercentSkipped ensures a placeholder 0% focus slot does not
// produce a noise "+0%" contributor row.
func TestParseFocusZeroPercentSkipped(t *testing.T) {
	s := &db.Spell{ID: 1, Name: "Empty Focus"}
	s.EffectIDs = [12]int{SPAImprovedHeal}
	s.EffectBaseValues = [12]int{0}

	if mods := parseFocusSpell(s); len(mods) != 0 {
		t.Errorf("parseFocusSpell returned %d modifiers for a 0%% slot, want 0", len(mods))
	}
}

// TestParseFocusAllWornTypes verifies every worn-focus SPA (124–132) is
// surfaced as a contributor, not just duration/cast-time.
func TestParseFocusAllWornTypes(t *testing.T) {
	want := []int{
		SPAImprovedDamage, SPAImprovedHeal, SPAResistReduction,
		SPACastTime, SPADuration, SPAIncreaseRange,
		SPASpellHate, SPAReagentConserve, SPAManaCost,
	}
	for _, spa := range want {
		s := &db.Spell{ID: spa, Name: "Focus"}
		s.EffectIDs = [12]int{spa}
		s.EffectBaseValues = [12]int{15}
		mods := parseFocusSpell(s)
		if len(mods) != 1 || mods[0].SPA != spa {
			t.Errorf("SPA %d not surfaced as a contributor (got %d mods)", spa, len(mods))
		}
	}
}

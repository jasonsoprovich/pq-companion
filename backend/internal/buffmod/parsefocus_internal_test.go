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

// TestParseFocusSpellHasteExcludesCompleteHeal locks in the corrected limit-SPA
// numbering. Every Spell Haste focus carries SPA 139:-13 (SE_LimitSpell,
// exclude spell 13 = Complete Healing) and SPA 143 (min cast time). 139 must
// populate ExcludeSpells, NOT MinLevel — the old code mislabeled 139 as min
// level, which both failed to exclude CH and (for clicky foci with a positive
// 139 spell-ID) produced a garbage "≥ L3325"-style min-level value.
func TestParseFocusSpellHasteExcludesCompleteHeal(t *testing.T) {
	// Mirrors Spell Haste III (2341): 127:15, 134:60, 139:-13, 143:3000.
	s := &db.Spell{ID: 2341, Name: "Spell Haste III"}
	s.EffectIDs = [12]int{SPACastTime, SPALimitMaxLevel, SPALimitSpellID, SPALimitCastTimeMin}
	s.EffectBaseValues = [12]int{15, 60, -13, 3000}

	mods := parseFocusSpell(s)
	if len(mods) != 1 {
		t.Fatalf("parseFocusSpell returned %d modifiers, want 1", len(mods))
	}
	l := mods[0].Limits
	if l.MinLevel != 0 {
		t.Errorf("MinLevel = %d, want 0 (139 is a spell-ID limit, not min level)", l.MinLevel)
	}
	if len(l.ExcludeSpells) != 1 || l.ExcludeSpells[0] != 13 {
		t.Errorf("ExcludeSpells = %v, want [13] (Complete Healing excluded from haste)", l.ExcludeSpells)
	}
	if l.MinCastTimeMs != 3000 {
		t.Errorf("MinCastTimeMs = %d, want 3000 (SPA 143)", l.MinCastTimeMs)
	}

	// Match must reject Complete Healing (spell 13) even on a ≥3s spell…
	if mods[0].Match(13, 50, 0, 4000, SpellTypeBeneficial, nil) {
		t.Error("haste focus should NOT apply to Complete Healing (spell 13)")
	}
	// …apply to an ordinary spell whose cast time meets the 143 threshold…
	if !mods[0].Match(999, 50, 0, 4000, SpellTypeBeneficial, nil) {
		t.Error("haste focus should apply to a non-excluded ≥3s spell")
	}
	// …and reject a fast spell below the SPA-143 min cast time.
	if mods[0].Match(999, 50, 0, 1500, SpellTypeBeneficial, nil) {
		t.Error("haste focus should NOT apply to a sub-3s spell (SPA 143)")
	}
}

// TestParseFocusInstantOnly verifies SPA 141 (SE_LimitInstant) sets InstantOnly
// and that Match rejects duration spells (DoTs/HoTs) for an instant-only focus.
func TestParseFocusInstantOnly(t *testing.T) {
	// Mirrors Improved Damage III (2338): 124:20, 134:60, 141:1, 138:0.
	s := &db.Spell{ID: 2338, Name: "Improved Damage III"}
	s.EffectIDs = [12]int{SPAImprovedDamage, SPALimitMaxLevel, SPALimitInstant, SPALimitSpellType}
	s.EffectBaseValues = [12]int{20, 60, 1, SpellTypeDetrimental}

	mods := parseFocusSpell(s)
	if len(mods) != 1 || !mods[0].Limits.InstantOnly {
		t.Fatalf("InstantOnly not set from SPA 141 (mods=%+v)", mods)
	}
	// Direct nuke (zero base duration) → applies.
	if !mods[0].Match(999, 50, 0, 0, SpellTypeDetrimental, nil) {
		t.Error("instant-only damage focus should apply to a direct nuke")
	}
	// DoT (non-zero duration) → excluded.
	if mods[0].Match(999, 50, 60, 0, SpellTypeDetrimental, nil) {
		t.Error("instant-only damage focus should NOT apply to a DoT")
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

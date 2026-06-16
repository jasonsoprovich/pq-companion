package spelltimer

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// allClasses255 builds a class table no player class can cast — the shape of
// an item-clicky / NPC-only spell (e.g. Shield of the Eighth, Fiery Might).
func allClasses255() [15]int {
	var cls [15]int
	for i := range cls {
		cls[i] = 255
	}
	return cls
}

// enchanterCastable builds a class table the enchanter (index 11) can cast.
func enchanterCastable() [15]int {
	cls := allClasses255()
	cls[11] = 50
	return cls
}

const enchanterClassIdx = 11

// classFilterAllowsBuff is the gate behind the "only show buffs my class can
// cast" filter. The load-bearing case is the regression that flooded the buff
// overlay: an all-classes-255 self-buff (clicky or NPC recourse) cast by
// SOMEONE ELSE must be dropped, while the user's own clicky (landing on them)
// stays exempt.
func TestClassFilterAllowsBuff(t *testing.T) {
	clicky := &db.Spell{ClassLevels: allClasses255(), GoodEffect: 1, TargetType: targetTypeSelf}
	offClassBuff := &db.Spell{ClassLevels: func() [15]int { c := allClasses255(); c[0] = 50; return c }(), GoodEffect: 1}
	onClassBuff := &db.Spell{ClassLevels: enchanterCastable(), GoodEffect: 1}

	tests := []struct {
		name         string
		spell        *db.Spell
		isSelfTarget bool
		enabled      bool
		classIdx     int
		want         bool
	}{
		{"my own clicky lands on me", clicky, true, true, enchanterClassIdx, true},
		// The regression: other player's clicky / NPC self-buff under scope=anyone.
		{"other player's clicky lands on them", clicky, false, true, enchanterClassIdx, false},
		{"npc recourse on other (all 255)", clicky, false, true, enchanterClassIdx, false},
		{"off-class buff on other", offClassBuff, false, true, enchanterClassIdx, false},
		{"on-class buff on other", onClassBuff, false, true, enchanterClassIdx, true},
		// Filter disabled: everything passes regardless of target/class.
		{"filter off, other player's clicky", clicky, false, false, enchanterClassIdx, true},
		{"filter off, off-class buff", offClassBuff, false, false, enchanterClassIdx, true},
		// Unknown class index must not hide everything.
		{"unknown class idx", offClassBuff, false, true, -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classFilterAllowsBuff(tc.spell, tc.isSelfTarget, tc.enabled, tc.classIdx)
			if got != tc.want {
				t.Errorf("classFilterAllowsBuff = %v, want %v", got, tc.want)
			}
		})
	}
}

// categorize must put a self-target item clicky that the source data mis-flags
// as detrimental (goodEffect=0) into the buff overlay — the Maelin's Magical
// Concoction case — while leaving real player debuffs detrimental.
func TestCategorize_ClickyAndDebuff(t *testing.T) {
	tests := []struct {
		name  string
		spell *db.Spell
		want  Category
	}{
		{
			// Maelin's Magical Concoction: goodEffect=0, self-target, all 255.
			name:  "mis-flagged self clicky buff",
			spell: &db.Spell{ClassLevels: allClasses255(), GoodEffect: 0, TargetType: targetTypeSelf},
			want:  CategoryBuff,
		},
		{
			// Shield of the Eighth / Fungal Regrowth: goodEffect=1 → buff.
			name:  "good-effect clicky buff",
			spell: &db.Spell{ClassLevels: allClasses255(), GoodEffect: 1, TargetType: targetTypeSelf},
			want:  CategoryBuff,
		},
		{
			// Real player debuff cast on an enemy: goodEffect=0, castable,
			// not self-target → stays detrimental (must NOT hit the override).
			name:  "real player debuff",
			spell: &db.Spell{ClassLevels: enchanterCastable(), GoodEffect: 0, TargetType: 5},
			want:  CategoryDebuff,
		},
		{
			// DoT: effect 0 (HP) with a negative base value.
			name: "dot via negative hp effect",
			spell: &db.Spell{
				ClassLevels:      enchanterCastable(),
				EffectIDs:        [12]int{0},
				EffectBaseValues: [12]int{-40},
			},
			want: CategoryDot,
		},
		{
			// Ancient: Master of Death — a self-target beneficial spell
			// (goodEffect=1) whose effect slot 7 is an HP cost (effect 0, base
			// -63). That negative HP must NOT classify it as a DoT; the self +
			// goodEffect flags win.
			name: "self buff with hp-cost slot is a buff, not a dot",
			spell: &db.Spell{
				GoodEffect:       1,
				TargetType:       targetTypeSelf,
				ClassLevels:      enchanterCastable(),
				EffectIDs:        [12]int{58, 15, 10, 66, 13, 10, 0},
				EffectBaseValues: [12]int{85, 44, 0, 1, 1, 0, -63},
			},
			want: CategoryBuff,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := categorize(tc.spell); got != tc.want {
				t.Errorf("categorize = %v, want %v", got, tc.want)
			}
		})
	}
}

// commonCandidateName lets an ambiguous land resolve when every candidate is
// the same spell by name (two "Fungal Regrowth" rows differing only by ID),
// without needing a "You begin casting" line — instant clickies emit none.
func TestCommonCandidateName(t *testing.T) {
	same := []logparser.SpellLandedCandidate{
		{SpellID: 1806, SpellName: "Fungal Regrowth"},
		{SpellID: 2008, SpellName: "Fungal Regrowth"},
	}
	if got := commonCandidateName(same); got != "Fungal Regrowth" {
		t.Errorf("same-name candidates: got %q, want %q", got, "Fungal Regrowth")
	}

	diff := []logparser.SpellLandedCandidate{
		{SpellID: 1796, SpellName: "Shield of the Ring"},
		{SpellID: 1963, SpellName: "Shield of the Eighth"},
	}
	if got := commonCandidateName(diff); got != "" {
		t.Errorf("different-name candidates: got %q, want empty", got)
	}

	if got := commonCandidateName(nil); got != "" {
		t.Errorf("no candidates: got %q, want empty", got)
	}
}

// resolveLandedSpellName must use the shared name for same-named ambiguous
// candidates even with no recent cast, but stay ambiguous (return "") for
// differently-named candidates with no recent cast.
func TestResolveLandedSpellName_SameNameFallback(t *testing.T) {
	e := newTestEngine()

	fungal := logparser.SpellLandedData{
		Kind: logparser.SpellLandedKindYou,
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 1806, SpellName: "Fungal Regrowth"},
			{SpellID: 2008, SpellName: "Fungal Regrowth"},
		},
	}
	if got := e.resolveLandedSpellName(fungal); got != "Fungal Regrowth" {
		t.Errorf("fungal: got %q, want %q", got, "Fungal Regrowth")
	}

	shield := logparser.SpellLandedData{
		Kind: logparser.SpellLandedKindYou,
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 1796, SpellName: "Shield of the Ring"},
			{SpellID: 1963, SpellName: "Shield of the Eighth"},
		},
	}
	if got := e.resolveLandedSpellName(shield); got != "" {
		t.Errorf("shield (no recent cast): got %q, want empty", got)
	}
}

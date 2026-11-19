package api

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
)

func haste(pct, minCastMs, spellType, maxLevel int) buffmod.Modifier {
	return buffmod.Modifier{
		SPA: 127, Category: "spell_haste", Percent: pct,
		Limits: buffmod.Limits{MinCastTimeMs: minCastMs, SpellType: spellType, MaxLevel: maxLevel},
	}
}

func TestFocusDominates_Directional(t *testing.T) {
	const top = 65
	any := buffmod.SpellTypeUnset
	cases := []struct {
		name string
		a, b buffmod.Modifier
		want bool
	}{
		{"3s+ covers 4s+ at equal strength", haste(20, 3000, any, 0), haste(20, 4000, any, 0), true},
		{"4s+ does not cover 3s+", haste(20, 4000, any, 0), haste(20, 3000, any, 0), false},
		{"weaker never dominates", haste(15, 0, any, 0), haste(20, 0, any, 0), false},
		{"beneficial vs detrimental are incomparable",
			haste(30, 0, buffmod.SpellTypeBeneficial, 0), haste(20, 0, buffmod.SpellTypeDetrimental, 0), false},
		{"any spell type covers detrimental-only",
			haste(20, 0, any, 0), haste(20, 0, buffmod.SpellTypeDetrimental, 0), true},
		{"a max-level-60 focus doesn't cover a lvl-65 caster's top spells",
			haste(30, 0, any, 60), haste(20, 0, any, 65), false},
		{"a max level past the top spells is the same as none",
			haste(20, 0, any, 70), haste(20, 0, any, 0), true},
		{"different effects never compare",
			buffmod.Modifier{SPA: 127, Category: "spell_haste", Percent: 30},
			buffmod.Modifier{SPA: 128, Category: "duration", Percent: 10}, false},
		{"broad spell haste covers a detrimental-only affliction haste",
			haste(30, 0, any, 0),
			buffmod.Modifier{SPA: 127, Category: "affliction_haste", Percent: 20,
				Limits: buffmod.Limits{SpellType: buffmod.SpellTypeDetrimental}}, true},
		{"hate foci are never ranked",
			buffmod.Modifier{SPA: 130, Category: "aggro_reduction", Percent: 20},
			buffmod.Modifier{SPA: 130, Category: "aggro_reduction", Percent: 10}, false},
	}
	for _, c := range cases {
		if got := focusDominates(c.a, c.b, top); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFocusCovers_Whitelists(t *testing.T) {
	broad := buffmod.Limits{SpellType: buffmod.SpellTypeUnset}
	narrow := buffmod.Limits{SpellType: buffmod.SpellTypeUnset, IncludeSpells: []int{1, 2}}
	wider := buffmod.Limits{SpellType: buffmod.SpellTypeUnset, IncludeSpells: []int{1, 2, 3}}
	if !focusCovers(broad, narrow, 65) {
		t.Error("an unrestricted focus should cover a whitelisted one")
	}
	if focusCovers(narrow, broad, 65) {
		t.Error("a whitelisted focus must not cover an unrestricted one")
	}
	if !focusCovers(wider, narrow, 65) || focusCovers(narrow, wider, 65) {
		t.Error("whitelist coverage should follow set inclusion")
	}
}

func TestPriorityFocusMatch(t *testing.T) {
	const top = 65
	any := buffmod.SpellTypeUnset
	mods := map[int]buffmod.Modifier{
		// Worn: a restricted 30%, a broad 25%; candidate: a broad 20%.
		1: haste(30, 0, buffmod.SpellTypeBeneficial, 0),
		2: haste(25, 0, any, 0),
		3: haste(20, 0, any, 0),
		// Ranks of one line: selected III (20%) and IV (30%).
		10: haste(20, 0, any, 0),
		11: haste(30, 0, any, 0),
		12: haste(15, 0, any, 0), // II, weaker than the selected rank
		// A max-level-60 30% worn focus vs a lvl-65 20% candidate.
		20: haste(30, 0, any, 60),
		21: haste(20, 0, any, 0),
	}
	lookup := func(id int) (buffmod.Modifier, bool) { m, ok := mods[id]; return m, ok }

	cases := []struct {
		name     string
		cand     int
		selected []int
		worn     []int
		want     bool
	}{
		{"the broad 25% worn focus hides a broad 20% even though the restricted 30% can't",
			3, []int{3}, []int{1, 2}, false},
		{"only the restricted 30% worn: the broad 20% still qualifies",
			3, []int{3}, []int{1}, true},
		{"a higher rank than the one picked qualifies", 11, []int{10}, nil, true},
		{"a lower rank than the one picked doesn't", 12, []int{10}, nil, false},
		{"the exact selected spell qualifies", 10, []int{10}, nil, true},
		{"already wearing the same spell", 10, []int{10}, []int{10}, false},
		{"a worn max-level-60 focus doesn't block a lvl-65 caster's focus",
			21, []int{21}, []int{20}, true},
		{"unknown foci fall back to exact id", 99, []int{99}, nil, true},
		{"unknown foci never match a different id", 99, []int{98}, nil, false},
		{"no focus on the candidate", 0, []int{10}, nil, false},
	}
	for _, c := range cases {
		if got := priorityFocusMatch(c.cand, c.selected, c.worn, lookup, top); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

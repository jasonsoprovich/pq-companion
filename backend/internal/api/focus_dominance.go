package api

import "github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"

// focusComparable reports whether "a bigger percent is better" holds for a
// focus category. Hate foci (aggro reduction, bash hate) are playstyle calls,
// so rank comparisons are never made for them.
func focusComparable(category string) bool {
	return category != "" && category != "aggro_reduction" && category != "bash_hate"
}

// focusDominates reports whether focus a is at least as good as focus b for a
// character casting spells up to topLevel: the same effect (SPA), at least the
// same strength, and applying to at least every spell b applies to. It's a
// partial order. Two foci that each cover spells the other doesn't (one
// beneficial-only, one detrimental-only) dominate neither way, and callers
// fall back to exact spell-id matching for them.
//
// Categories aren't required to match: they're derived from the SPA and
// limits, and the limits check is the real test. That lets a broad Spell Haste
// cover a narrower Affliction Haste of the same or lower percent.
func focusDominates(a, b buffmod.Modifier, topLevel int) bool {
	if a.SPA != b.SPA || !focusComparable(a.Category) || !focusComparable(b.Category) {
		return false
	}
	if a.Percent < b.Percent {
		return false
	}
	return focusCovers(a.Limits, b.Limits, topLevel)
}

// focusCovers reports whether a focus with limits a applies to every spell a
// focus with limits b applies to, among spells the character can cast (level
// ≤ topLevel). Every limit has to be no stricter on a's side.
func focusCovers(a, b buffmod.Limits, topLevel int) bool {
	// A max-level limit past the character's top spells is the same as none.
	effMax := func(m int) int {
		if m == 0 || m > topLevel {
			return topLevel
		}
		return m
	}
	if effMax(a.MaxLevel) < effMax(b.MaxLevel) {
		return false
	}
	if a.MinLevel > b.MinLevel {
		return false
	}
	if !spellTypeCovers(a.SpellType, b.SpellType) {
		return false
	}
	if a.ResistType != 0 && a.ResistType != b.ResistType {
		return false
	}
	if a.MinDurationSec > b.MinDurationSec || a.MinCastTimeMs > b.MinCastTimeMs {
		return false
	}
	if a.InstantOnly && !b.InstantOnly {
		return false
	}
	// Exclusions: everything a excludes, b must exclude too.
	if !subset(a.ExcludeEffects, b.ExcludeEffects) || !subset(a.ExcludeSpells, b.ExcludeSpells) {
		return false
	}
	// Whitelists: an empty list allows everything; otherwise a's list must
	// contain all of b's (and b can't be unrestricted where a is restricted).
	if !whitelistCovers(a.IncludeEffects, b.IncludeEffects) || !whitelistCovers(a.IncludeSpells, b.IncludeSpells) {
		return false
	}
	return true
}

func spellTypeCovers(a, b int) bool {
	if a == buffmod.SpellTypeUnset || a == buffmod.SpellTypeAny {
		return true
	}
	return a == b
}

func whitelistCovers(a, b []int) bool {
	if len(a) == 0 {
		return true
	}
	if len(b) == 0 {
		return false
	}
	return subset(b, a)
}

// subset reports whether every value in sub is also in super.
func subset(sub, super []int) bool {
	if len(sub) == 0 {
		return true
	}
	set := make(map[int]bool, len(super))
	for _, v := range super {
		set[v] = true
	}
	for _, v := range sub {
		if !set[v] {
			return false
		}
	}
	return true
}

// focusLookup resolves a focus spell id to its primary categorised modifier
// (ok false for an unknown or uncategorised focus).
type focusLookup func(spellID int) (buffmod.Modifier, bool)

// focusDominatesByID compares two focus spells through lookup; anything it
// can't resolve and compare confidently returns false.
func focusDominatesByID(a, b int, lookup focusLookup, topLevel int) bool {
	ma, okA := lookup(a)
	mb, okB := lookup(b)
	return okA && okB && focusDominates(ma, mb, topLevel)
}

// priorityFocusMatch reports whether a candidate's focus is one the player
// asked for and doesn't already have:
//
//   - it matches a selected focus: the same spell, or one that dominates it
//     (picking a rank means "this rank or better");
//   - no worn item focus is the same spell or dominates it. Coverage only
//     partly orders foci, so every worn focus is checked, not just the
//     strongest.
//
// worn holds item focus spell ids only. AA foci stack on top of the best item
// focus, so they never make an item focus redundant.
func priorityFocusMatch(cand int, selected, worn []int, lookup focusLookup, topLevel int) bool {
	if cand <= 0 {
		return false
	}
	wanted := false
	for _, s := range selected {
		if s == cand || focusDominatesByID(cand, s, lookup, topLevel) {
			wanted = true
			break
		}
	}
	if !wanted {
		return false
	}
	for _, w := range worn {
		if w == cand || focusDominatesByID(w, cand, lookup, topLevel) {
			return false
		}
	}
	return true
}

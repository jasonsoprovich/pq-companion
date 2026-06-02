package spelltimer

import "github.com/jasonsoprovich/pq-companion/backend/internal/db"

// bardSongUseBaseDuration controls how bard-song durations are computed.
//
// true  (default) — bard songs report their base duration (buffduration ×
//
//	6 seconds). Songs are pulse effects: the bard's client
//	re-applies the buff every tick, so the only time the
//	timer matters is right after the bard stops singing,
//	at which point the buff fades within ~base ticks.
//	18s for Selo's, not 54s, matches what the user sees.
//
// false — bard songs run through the full duration formula like any other
//
//	spell. Use only to revert if the base-duration mode causes
//	regressions; the result (54s for Selo's at L60) doesn't match
//	in-game fade behaviour.
//
// Detection is class-based, not formula- or skill-based: a song is a
// spell only the bard class (index 7) can cast. See SpellDurationTicks.
const bardSongUseBaseDuration = true

// SpellDurationTicks returns the buff duration in EQ server ticks for a
// landed spell, applying the bard-song base-duration clamp when enabled.
// Use this in preference to the lower-level CalcDurationTicks anywhere a
// full *db.Spell is available — only the parser-test fixtures call the
// raw formula form directly.
func SpellDurationTicks(spell *db.Spell, level int) int {
	if bardSongUseBaseDuration && isBardSong(spell) {
		return spell.BuffDuration
	}
	return CalcDurationTicks(spell.BuffDurationFormula, spell.BuffDuration, level)
}

// isBardSong reports whether spell is a bard song — a spell only the bard
// class (classes8, index 7) can cast. Excludes the two cross-class
// disciplines (Resistant/Fearless Discipline) which list bard in their
// class table alongside warriors/rangers/etc. and aren't pulse songs.
func isBardSong(spell *db.Spell) bool {
	const bardIdx = 7
	if spell.ClassLevels[bardIdx] >= 255 {
		return false
	}
	for i, lvl := range spell.ClassLevels {
		if i == bardIdx {
			continue
		}
		if lvl < 255 {
			return false
		}
	}
	return true
}

// CalcDurationTicks computes the actual buff duration in EQ server ticks
// (1 tick = 6 seconds) from the three spell DB fields:
//   - formula  — the buffduration_formula column
//   - base     — the buffduration column (the per-formula max tick count)
//   - level    — caster level (use defaultCasterLevel when unknown)
//
// This is a faithful port of EQMacEmu's CalcBuffDuration_formula
// (zone/spells.cpp) — the EQMac/Al'Kabor ruleset Project Quarm runs, whose
// formulas DIVERGE from modern EQEmu's. The earlier modern-EQEmu table here
// over-stated many durations (e.g. Forlorn Deeds, formula 6: it reported ~10m
// instead of the real 3m12s at level 60). Verified against PQDI:
//   - formula 5  Invigor (base 3)              → 2 ticks
//   - formula 6  Forlorn Deeds (base 35, L57)  → 30 ticks; L60 → 32
//   - formula 7  Berserker Strength (base 30)  → min(level, 30)
//   - formula 9/10 charm/CC spells             → level*2+10 / level*3+10
//
// Returns 0 for instant spells (no timer required).
func CalcDurationTicks(formula, base, level int) int {
	if level <= 0 {
		level = 1
	}

	// EQMacEmu: any formula value >= 200 is a literal tick count.
	if formula >= 200 {
		return formula
	}

	switch formula {
	case 0:
		return 0 // not a buff / instant
	case 1:
		return capAtBase(level/2, base)
	case 2:
		i := 6
		if level > 1 {
			i = level/2 + 5
		}
		return capAtBase(i, base)
	case 3:
		return capAtBase(level*30, base)
	case 4:
		return capToBase(50, base)
	case 5:
		return capToBase(2, base)
	case 6:
		return capToBase(level/2+2, base)
	case 7:
		return capToBase(level, base)
	case 8:
		return capAtBase(level+10, base)
	case 9:
		return capAtBase(level*2+10, base)
	case 10:
		return capAtBase(level*3+10, base)
	case 11:
		return capAtBase(level*30+90, base)
	case 12: // not used by any current spell, ported for completeness
		i := level / 4
		if i == 0 {
			i = 1
		}
		return capToBase(i, base)
	case 50:
		// Permanent buff. EQMacEmu returns 0xFFFF here; we return 0 so the
		// countdown overlay doesn't show a bogus multi-day timer for a buff
		// that never ticks down (these are NPC/event spells in practice).
		return 0
	default:
		return 0
	}
}

// capAtBase mirrors EQMacEmu's pattern for the level-scaling formulas:
//
//	return i < duration ? (i < 1 ? 1 : i) : duration
//
// i.e. min(i, base), but never below 1 when i is the smaller value.
func capAtBase(i, base int) int {
	if i < base {
		if i < 1 {
			return 1
		}
		return i
	}
	return base
}

// capToBase mirrors EQMacEmu's pattern for the fixed / short formulas:
//
//	return duration ? (i < duration ? i : duration) : i
//
// i.e. min(i, base), or i when base is 0.
func capToBase(i, base int) int {
	if base != 0 {
		if i < base {
			return i
		}
		return base
	}
	return i
}

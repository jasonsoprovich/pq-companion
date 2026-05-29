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
//   - base     — the buffduration column (base tick count from DB)
//   - level    — caster level (use defaultCasterLevel when unknown)
//
// Formula codes and their semantics match EQEmu's CalcBuffDuration_formula
// implementation for the classic-era ruleset used by Project Quarm.
// Returns 0 for instant spells (no timer required).
func CalcDurationTicks(formula, base, level int) int {
	if level <= 0 {
		level = 1
	}
	switch formula {
	case 0:
		return 0 // instant / no duration
	case 1:
		// min(level/2, base)
		d := level / 2
		if d > base {
			d = base
		}
		return d
	case 2:
		// min(30/level + base, base*2)
		d := base
		if level > 0 {
			d = 30/level + base
		}
		if cap := base * 2; d > cap {
			d = cap
		}
		return d
	case 3:
		// min(level*30, base)
		d := level * 30
		if d > base {
			d = base
		}
		return d
	case 4:
		// min(level*2 + base, base*3)
		d := level*2 + base
		if cap := base * 3; d > cap {
			d = cap
		}
		return d
	case 5:
		// min(level*5 + base, base*3)
		d := level*5 + base
		if cap := base * 3; d > cap {
			d = cap
		}
		return d
	case 6:
		// min(level*30 + base, base*3)
		d := level*30 + base
		if cap := base * 3; d > cap {
			d = cap
		}
		return d
	case 7:
		// min(level*5, base)
		d := level * 5
		if d > base {
			d = base
		}
		return d
	case 8:
		// Quarm/EQMacEmu treats formula 8 as a fixed-duration buff: just
		// `base` ticks regardless of level. The EQEmu-canonical "level + base
		// capped at base*3" doubles every formula-8 spell at level 60, which
		// produced the user-reported 12-minute Pacify (real value is 6m / 60
		// ticks per PQDI for spell id 45 — buffduration=60, formula=8). Aligns
		// with the documented formulas 1/5/7 in SCHEMA.md, which also just
		// return the base duration.
		return base
	case 9:
		// min(level*2 + 10, base) — verified against PQDI's "Min Duration"
		// at each spell's minimum castable level: Lull (id 208, base=20,
		// level 1) → 12, Tashanian (id 1702, base=140, level 57) → 124. Both
		// equal level*2 + 10 exactly. Was previously level*2 (no +10), which
		// undershoots every f9 spell by ten ticks at every caster level.
		d := level*2 + 10
		if d > base {
			d = base
		}
		return d
	case 10:
		// min(level*3 + 10, base) — verified against PQDI's "Min Duration":
		// Charm (id 300, base=205, level 12) → 46, Beguile (id 182, base=205,
		// level 24) → 82, Cajoling Whispers (id 183, base=205, level 39) →
		// 127. All equal level*3 + 10 exactly. Was previously `level` capped
		// at base, which dramatically underprices every f10 charm spell —
		// at level 60 our calc yielded 60 ticks (6 min) instead of the
		// game's 190 ticks (19 min) for charm.
		d := level*3 + 10
		if d > base {
			d = base
		}
		return d
	case 11:
		return base // fixed duration — base ticks regardless of level
	case 50:
		// level/5 ticks (used by some short crowd-control spells)
		d := level / 5
		if d < 1 {
			d = 1
		}
		return d
	case 3600:
		// Typically used for permanent / song-pulse effects; treated as no timer.
		return 0
	default:
		return base
	}
}

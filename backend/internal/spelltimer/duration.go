package spelltimer

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
		// min(level*2, base)
		d := level * 2
		if d > base {
			d = base
		}
		return d
	case 10:
		// min(level, base)
		d := level
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

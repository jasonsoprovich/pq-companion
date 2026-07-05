package db

// ServerLevelCap is the Project Quarm level cap. Level-scaling spell effect
// formulas are evaluated at this level for best-case ("at cap") display, the
// same way the frontend uses SERVER_LEVEL_CAP in spellHelpers.ts. Raid buffs
// are cast by max-level characters, so the cap value is what a level-60 main
// actually receives.
const ServerLevelCap = 60

// ApplyLevelFormula mirrors EQMacEmu Mob::CalcSpellEffectValue_formula (and the
// frontend applyLevelFormula in spellHelpers.ts): it returns a spell effect's
// value at a given caster level, applying the up/down sign and the post-formula
// max clamp. Unknown formulas fall back to the raw base value.
//
// Keep this byte-for-byte equivalent to the TypeScript port so the character
// buff list, the spell database page, and the derived-stat totals all agree.
func ApplyLevelFormula(formula, base, max, level int) int {
	ubase := base
	if ubase < 0 {
		ubase = -ubase
	}
	updownsign := 1
	if max != 0 && max < base {
		updownsign = -1
	}
	var result int
	switch formula {
	case 0, 100:
		result = ubase
	case 101:
		result = updownsign * (ubase + level/2)
	case 102:
		result = updownsign * (ubase + level)
	case 103:
		result = updownsign * (ubase + level*2)
	case 104:
		result = updownsign * (ubase + level*3)
	case 105:
		result = updownsign * (ubase + level*4)
	case 109:
		result = updownsign * (ubase + level/4)
	case 110:
		result = updownsign * (ubase + level/6)
	case 119:
		result = updownsign * (ubase + level/8)
	case 121:
		result = ubase + level/3
	default:
		return base
	}
	if max != 0 {
		if updownsign == 1 && result > max {
			result = max
		}
		if updownsign == -1 && result < max {
			result = max
		}
	}
	if base < 0 && result > 0 {
		result = -result
	}
	return result
}

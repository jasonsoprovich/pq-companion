package tradeskill

import "math"

// This file ports Project Quarm's (EQMacEmu) tradeskill skill-up mechanic —
// Client::CheckIncreaseTradeskill in zone/tradeskills.cpp — to estimate how many
// combines it takes to raise a skill. It complements the success-chance model in
// chance.go (which is the OTHER tradeskill formula).
//
// Per attempt, a skill-up requires passing two independent rolls:
//
//	statCheck = tradeStat * 10 / (difficulty * (combineSucceeded ? 1 : 2))
//	pass1: statCheck > random.Real(1, 1000)            // stat/difficulty roll
//	pass2: rawSkill <= 15 || random.Int(1,200) > min(190, rawSkill)
//
// Both must pass, and only while the combine is non-trivial — i.e. the raw
// (unmodified) skill is below the recipe's trivial AND below the class/level
// skill cap (CanIncreaseTradeskill). Item skill-mods do NOT change this: they
// raise effective skill for the success roll only, so they help indirectly
// (a successful combine gets the full statCheck, a failure only half).
//
// tradeStat and difficulty are inputs (resolved by the caller from character
// stats and quarm.db skill_difficulty); this keeps the math pure and testable.

// SkillUpResult estimates the combines needed to raise a tradeskill.
type SkillUpResult struct {
	CurrentSkill int     `json:"current_skill"`
	TargetSkill  int     `json:"target_skill"` // where skill-ups stop: min(trivial, cap)
	Trivial      int     `json:"trivial"`
	Cap          int     `json:"cap"`         // class/level skill cap (0 = unknown)
	Difficulty   float64 `json:"difficulty"`  // skill_difficulty for this tradeskill
	TradeStat    int     `json:"trade_stat"`  // effective governing stat used
	StatName     string  `json:"stat_name"`   // which attributes drive it, e.g. "WIS/INT"
	StatSource   string  `json:"stat_source"` // "base+gear" (base stats + equipped) or "base"

	PointsToGo       int     `json:"points_to_go"`
	AttemptsToNext   float64 `json:"attempts_to_next"`   // to gain the next single point
	AttemptsToTarget float64 `json:"attempts_to_target"` // to reach TargetSkill

	Maxed       bool `json:"maxed"`       // current >= target: no skill-ups from this recipe
	AtCap       bool `json:"at_cap"`      // target is the class/level cap, not trivial
	Impractical bool `json:"impractical"` // effectively can't skill up (stat/difficulty ~ 0)
}

// TradeStat returns the governing stat value and a label for a tradeskill, per
// the EQMacEmu CheckIncreaseTradeskill rules. The general case subtracts 15
// (skills with no alternate stat are slightly harder to raise).
func TradeStat(tradeskill, str, dex, intel, wis int) (int, string) {
	switch tradeskill {
	case 64, 56: // Fletching, Make Poison
		return max3(dex, intel, wis), "DEX/INT/WIS"
	case 63: // Blacksmithing
		return max3(str, intel, wis), "STR/INT/WIS"
	default:
		v := intel
		if wis > v {
			v = wis
		}
		v -= 15
		if v < 0 {
			v = 0
		}
		return v, "WIS/INT"
	}
}

// EstimateSkillUp computes the expected combines to raise rawSkill toward the
// point where skill-ups stop (the lower of trivial and the class/level cap).
// skillMod/aaReduce/nofail feed the per-attempt success chance, since a
// successful combine skills up at twice the rate of a failed one.
//
// skillupBonusPct is a skill-up-RATE multiplier from buffs — on Quarm this is
// Maelin's Magical Concoction (SPA 504, +75%). It scales the per-attempt
// skill-up probability by (100+pct)/100 (capped at certain success), which is
// the natural reading of a "skillup rate" multiplier; the exact server math for
// this custom effect isn't published, so treat the boosted number as an
// estimate.
func EstimateSkillUp(current, trivial, cap, tradeStat int, difficulty float64, skillMod, aaReduce, skillupBonusPct int, nofail bool) SkillUpResult {
	target := trivial
	atCap := false
	if cap > 0 && cap < target {
		target = cap
		atCap = true
	}

	res := SkillUpResult{
		CurrentSkill: current, TargetSkill: target, Trivial: trivial, Cap: cap,
		Difficulty: difficulty, TradeStat: tradeStat, AtCap: atCap,
	}

	if current >= target {
		res.Maxed = true
		return res
	}
	res.PointsToGo = target - current

	if difficulty <= 0 || tradeStat <= 0 {
		res.Impractical = true
		return res
	}

	total := 0.0
	for s := current; s < target; s++ {
		pUp := SkillUpChanceAt(s, trivial, skillMod, aaReduce, skillupBonusPct, tradeStat, difficulty, nofail)
		if pUp <= 0 {
			res.Impractical = true
			return res
		}
		attempts := 1.0 / pUp
		if s == current {
			res.AttemptsToNext = math.Round(attempts*10) / 10
		}
		total += attempts
	}
	res.AttemptsToTarget = math.Round(total)
	return res
}

// SkillUpChanceAt returns the probability that a single combine of a recipe with
// the given trivial, performed at raw skill `skill`, yields a +1 skill-up — the
// per-attempt atom of EstimateSkillUp (which sums 1/chance across a skill range).
// It reuses the same two-roll CheckIncreaseTradeskill model:
//
//	pUp = P2(skill) * ( success * P1succ + (1-success) * P1fail ) * (1 + bonus%)
//
// Returns 0 when the combine can't skill up: skill already at/above trivial
// (trivial combines never grant points), or degenerate stat/difficulty. The
// leveling planner (internal/tsplan) calls this per skill point to build a plan;
// keeping the math here means both callers stay pinned to the same formula.
func SkillUpChanceAt(skill, trivial, skillMod, aaReduce, skillupBonusPct, tradeStat int, difficulty float64, nofail bool) float64 {
	if skill >= trivial || difficulty <= 0 || tradeStat <= 0 {
		return 0
	}
	p1succ := realRollProb(float64(tradeStat) * 10.0 / (difficulty * 1.0))
	p1fail := realRollProb(float64(tradeStat) * 10.0 / (difficulty * 2.0))
	p := Chance(skill, trivial, skillMod, aaReduce, nofail).Success / 100.0
	rateMult := 1.0 + float64(skillupBonusPct)/100.0
	pUp := skillRollProb(skill) * (p*p1succ + (1-p)*p1fail) * rateMult
	if pUp > 1 {
		pUp = 1
	}
	if pUp < 0 {
		pUp = 0
	}
	return pUp
}

// realRollProb is P(random.Real(1,1000) < stat) — the stat/difficulty check.
func realRollProb(stat float64) float64 {
	if stat <= 1 {
		return 0
	}
	if stat >= 1000 {
		return 1
	}
	return (stat - 1) / 999.0
}

// skillRollProb is the second check: P2 = 1 for raw skill <= 15, else
// P(random.Int(1,200) > min(190, skill)) = (200 - min(190, skill)) / 200. This
// is what floors the skill-up rate at 5% as skill approaches the cap.
func skillRollProb(skill int) float64 {
	if skill <= 15 {
		return 1
	}
	m := skill
	if m > 190 {
		m = 190
	}
	return float64(200-m) / 200.0
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

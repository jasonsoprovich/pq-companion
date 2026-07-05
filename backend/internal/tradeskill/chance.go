// Package tradeskill ports Project Quarm's (EQMacEmu fork) tradeskill combine
// success-chance formula so the app can show a recipe's success/failure odds at
// a given skill, plus the skill needed to reach the 5% failure floor.
//
// Source: EQMacEmu/Server zone/tradeskills.cpp Client::TradeskillExecute and
// zone/bonuses.cpp (item SkillMod handling) / zone/client.cpp Client::GetSkill.
// These are historical EQMac constants — treat the numbers here as pinned to
// that fork, not modern EQEmu (whose curve differs). If Quarm ever changes the
// server formula, update this file and its tests together.
//
// The model, verbatim from the server:
//
//	effSkill = min(252, floor(rawSkill * (100 + skillMod%) / 100))   // GetSkill
//	trivial >= 68:  chance = effSkill - 0.75*trivial + 51.5
//	trivial <  68:  chance = effSkill - trivial + 66
//	chance = clamp(chance, 5, 95)                                    // 5% floor
//	if nofail: chance = 100
//	chance += (100 - chance) * aaFailReduce% / 100                   // tradeskill AA
//
// Stats (INT/WIS) do NOT affect combine success in this era (only skill-up
// chance, which is a separate mechanic not modeled here). Item skill-mods are a
// percentage of raw skill and, per bonuses.cpp, only the single highest worn
// mod applies — callers should pass the max, not a sum.
package tradeskill

import "math"

// hardSkillCap is the EQMac-era ceiling on effective skill (GetSkill clamps
// here after applying the item skill-mod percentage).
const hardSkillCap = 252

// Result is the computed outcome for one recipe at one skill/modifier setting.
type Result struct {
	RawSkill int `json:"raw_skill"` // skill echoed back from the request
	SkillMod int `json:"skill_mod"` // item skill-mod percentage applied (max, not sum)
	EffSkill int `json:"eff_skill"` // rawSkill after the mod%, capped at 252

	Success float64 `json:"success"` // combine success %, 0–100, one decimal
	Failure float64 `json:"failure"` // 100 - Success

	// SuccessAtTrivial is the success % once raw skill reaches the recipe's
	// trivial (same modifiers applied). Trivial marks where skill-ups STOP, not
	// where failures stop — for most recipes this is only ~66%, which is why
	// "trivial" is not the same as "guaranteed".
	SuccessAtTrivial float64 `json:"success_at_trivial"`

	AtTrivial bool `json:"at_trivial"` // effSkill >= trivial (no more skill-ups)
	NoFail    bool `json:"no_fail"`    // recipe cannot fail

	// FloorSkill is the RAW skill needed (with no modifiers) to reach the 5%
	// failure floor — the canonical, shareable breakpoint (e.g. 66 for a
	// trivial-37 recipe). FloorReachable is false when that breakpoint exceeds
	// the 252 hard cap, i.e. the recipe can never drop below its best failure %.
	FloorSkill     int  `json:"floor_skill"`
	FloorReachable bool `json:"floor_reachable"`
	AtFloor        bool `json:"at_floor"` // already at the 5% floor at this eff skill
}

// EffectiveSkill applies an item skill-mod percentage to a raw skill and clamps
// to the EQMac hard cap, matching Client::GetSkill for tradeskills.
func EffectiveSkill(rawSkill, skillModPct int) int {
	if rawSkill < 0 {
		rawSkill = 0
	}
	eff := rawSkill
	if skillModPct != 0 {
		eff = int(math.Floor(float64(rawSkill) * float64(100+skillModPct) / 100))
	}
	if eff > hardSkillCap {
		eff = hardSkillCap
	}
	if eff < 0 {
		eff = 0
	}
	return eff
}

// rawChance is the un-clamped success percentage for an effective skill against
// a trivial, before the 5/95 clamp, nofail, and AA adjustments.
func rawChance(effSkill, trivial int) float64 {
	if trivial >= 68 {
		return float64(effSkill) - 0.75*float64(trivial) + 51.5
	}
	return float64(effSkill) - float64(trivial) + 66
}

// floorEffSkill is the smallest effective skill whose un-clamped chance reaches
// 95 (the 5% failure floor).
func floorEffSkill(trivial int) int {
	if trivial >= 68 {
		// effSkill - 0.75*trivial + 51.5 >= 95  ->  effSkill >= 43.5 + 0.75*trivial
		return int(math.Ceil(43.5 + 0.75*float64(trivial)))
	}
	// effSkill - trivial + 66 >= 95  ->  effSkill >= trivial + 29
	return trivial + 29
}

// finalChance applies the 5/95 clamp, nofail, and AA fail-reduction to the raw
// curve for an effective skill, rounded to one decimal.
func finalChance(effSkill, trivial, aaFailReducePct int, nofail bool) float64 {
	chance := rawChance(effSkill, trivial)
	if chance < 5 {
		chance = 5
	} else if chance > 95 {
		chance = 95
	}
	if nofail {
		chance = 100
	}
	if aaFailReducePct > 0 && chance < 100 {
		chance += (100 - chance) * float64(aaFailReducePct) / 100
	}
	return math.Round(chance*10) / 10
}

// Chance computes the combine outcome for a recipe (trivial, nofail) at a raw
// skill with an item skill-mod percentage and an AA fail-reduction percentage.
// aaFailReducePct is 0 for characters without the relevant AA.
func Chance(rawSkill, trivial, skillModPct, aaFailReducePct int, nofail bool) Result {
	eff := EffectiveSkill(rawSkill, skillModPct)
	chance := finalChance(eff, trivial, aaFailReducePct, nofail)
	// Success once the raw skill first reaches trivial (mods still applied) —
	// the "trivial is where skill-ups stop, not where failures stop" number.
	atTrivial := finalChance(EffectiveSkill(trivial, skillModPct), trivial, aaFailReducePct, nofail)

	target := floorEffSkill(trivial)
	reachable := target <= hardSkillCap

	return Result{
		RawSkill:         rawSkill,
		SkillMod:         skillModPct,
		EffSkill:         eff,
		Success:          chance,
		Failure:          math.Round((100-chance)*10) / 10,
		SuccessAtTrivial: atTrivial,
		AtTrivial:        eff >= trivial,
		NoFail:           nofail,
		FloorSkill:       target,
		FloorReachable:   reachable,
		AtFloor:          nofail || (reachable && eff >= target),
	}
}

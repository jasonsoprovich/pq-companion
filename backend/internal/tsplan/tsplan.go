// Package tsplan builds a tradeskill LEVELING plan: an ordered list of "grind
// recipe X from skill A to skill B" stages that carry a character from a start
// skill to a target.
//
// It knows nothing about the database or HTTP — callers pass in an explicit,
// already-ordered list of candidate recipes (already resolved from quarm.db,
// with vendor costs and sub-combine edges attached) plus the character's
// stat/difficulty inputs, and get back a Plan. The recipe order is the
// caller's decision, not this package's: a hand-curated "Recommended" path
// (community-guide derived, see internal/db/tradeskill_paths.go) supplies
// recipes in guide order; a player's "Custom" path supplies whatever they
// picked, sorted by trivial. This package's only job is turning that ordered
// list into stages with real combine counts, costs, and success chances for
// this specific character.
//
// # Model
//
// The single hard rule of EQ tradeskilling: a combine only grants a skill-up
// while your raw skill is below the recipe's TRIVIAL. So a natural "stage" is
// grinding one recipe from the skill reached so far up to
// min(trivial, cap, target) — the point where that recipe stops teaching. A
// recipe that no longer teaches anything from the current skill (already
// surpassed, or a duplicate breakpoint) is skipped rather than erroring, so a
// path stays robust to a character starting partway through it.
//
// Per-stage combine counts come from tradeskill.SkillUpChanceAt (the ported
// CheckIncreaseTradeskill per-attempt skill-up probability), summed across the
// stage's skill range. Per-stage success chance comes from tradeskill.Chance
// at the stage's starting skill (the worst case within the stage).
package tsplan

import (
	"fmt"
	"math"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tradeskill"
)

// RecipeCandidate is one recipe a plan may use, with everything the builder
// needs pre-resolved. The DB layer builds these; tsplan never looks anything
// up.
type RecipeCandidate struct {
	RecipeID  int    `json:"recipe_id"`
	Name      string `json:"name"`
	Trivial   int    `json:"trivial"`
	NoFail    bool   `json:"no_fail"`
	Yield     int    `json:"yield"`               // successcount (per-combine output; display only)
	Container string `json:"container,omitempty"` // objecttype label, e.g. "Forge" (a stage note)

	// VendorCost is the plat to buy one combine's worth of ingredients from
	// vendors. VendorCostKnown is false when any ingredient is farmed/dropped
	// and therefore has no database price — such a stage's cost is reported as
	// unknown and the plan's total cost is marked incomplete.
	VendorCost      float64 `json:"vendor_cost"`
	VendorCostKnown bool    `json:"vendor_cost_known"`

	// SubCombineRecipeIDs are components that are themselves crafted (DAG
	// edges), surfaced on the stage for the caller to enrich with detail.
	SubCombineRecipeIDs []int `json:"sub_combine_recipe_ids,omitempty"`

	// Note is an optional curator annotation for this recipe (e.g. "bulk-buy
	// water flasks", "masks/wrists take 1 part — cheapest variant"), carried
	// from a Recommended path's authored data. Empty for Custom-mode picks.
	Note string `json:"-"`
}

// Params carries the plan-wide inputs (constant across all candidates): the
// skill window and the character's governing stat, skill difficulty, and
// worn skill-mod / AA / buff bonuses.
type Params struct {
	StartSkill  int // current raw skill
	TargetSkill int // desired skill
	ClassCap    int // class/level skill cap; 0 = unknown/none

	TradeStat    int     // governing stat value (from tradeskill.TradeStat)
	Difficulty   float64 // skill_difficulty for this tradeskill
	SkillMod     int     // worn item skill-mod % (max, not sum)
	AAReduce     int     // mastery AA fail-reduction %
	SkillupBonus int     // skill-up rate buff % (e.g. Maelin's, +75)
}

// Stage is one leg of the plan: grind Recipe from FromSkill up to ToSkill.
type Stage struct {
	FromSkill int    `json:"from_skill"`
	ToSkill   int    `json:"to_skill"`
	RecipeID  int    `json:"recipe_id"`
	Recipe    string `json:"recipe"`
	Trivial   int    `json:"trivial"`

	Combines  int     `json:"combines"`   // expected combines for this leg (rounded)
	Cost      float64 `json:"cost"`       // vendor plat for this leg; 0 when unknown
	CostKnown bool    `json:"cost_known"` // false if any ingredient is farmed/dropped

	// SuccessChancePct is the combine success chance (0-100) at FromSkill — the
	// worst case for this stage, since it only rises as skill climbs toward
	// Trivial. A distinct roll from the skill-up chance Combines is derived
	// from; see tradeskill.Chance.
	SuccessChancePct float64 `json:"success_chance_pct"`

	Container           string   `json:"container,omitempty"`
	NoFail              bool     `json:"no_fail,omitempty"`
	SubCombineRecipeIDs []int    `json:"sub_combine_recipe_ids,omitempty"`
	Notes               []string `json:"notes,omitempty"`
}

// Plan is the ordered leveling itinerary plus totals and caveats.
type Plan struct {
	StartSkill   int `json:"start_skill"`
	TargetSkill  int `json:"target_skill"`  // as requested
	ReachedSkill int `json:"reached_skill"` // where the plan actually ends

	Stages        []Stage `json:"stages"`
	TotalCombines int     `json:"total_combines"`
	TotalCost     float64 `json:"total_cost"`

	// CostComplete is false when at least one stage has unknown (farmed/dropped)
	// cost, so TotalCost is a lower bound. Always true for a fully vendor-sourced
	// plan.
	CostComplete bool     `json:"cost_complete"`
	Warnings     []string `json:"warnings,omitempty"`
}

// BuildFromRecipes builds a leveling plan by walking an explicit, caller-
// ordered list of recipes — a curated Recommended path or a saved Custom
// path — rather than solving for an optimum. Each recipe grinds from the
// skill reached so far up to min(trivial, cap, target); a recipe that grants
// no skill-up from there (its trivial is at or below the current point) is
// skipped so the plan stays robust to a character starting partway through
// it, or to two recipes sharing a breakpoint.
func BuildFromRecipes(recipes []RecipeCandidate, p Params) Plan {
	out := Plan{
		StartSkill:   p.StartSkill,
		TargetSkill:  p.TargetSkill,
		ReachedSkill: p.StartSkill,
		CostComplete: true,
		Stages:       []Stage{}, // never nil, so it marshals as [] not null
	}

	start := p.StartSkill
	if start < 0 {
		start = 0
		out.ReachedSkill = 0
	}

	// A class/level cap can't be exceeded no matter the recipe.
	target := p.TargetSkill
	if p.ClassCap > 0 && p.ClassCap < target {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"target skill %d exceeds the class/level cap %d; plan ends at %d",
			p.TargetSkill, p.ClassCap, p.ClassCap))
		target = p.ClassCap
	}

	if start >= target {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"current skill %d is already at or above the target %d", start, target))
		return out
	}
	if len(recipes) == 0 {
		out.Warnings = append(out.Warnings, "this path has no recipes yet")
		return out
	}

	s := start
	for _, r := range recipes {
		if s >= target {
			break
		}
		stop := r.Trivial
		if stop > target {
			stop = target
		}
		if stop <= s {
			continue // teaches nothing from here — already past it
		}
		emitStage(&out, r, s, stop, p)
		s = stop
	}

	out.ReachedSkill = s
	out.TotalCost = math.Round(out.TotalCost)
	if s < target {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"this path only reaches skill %d (target %d) — add more recipes to continue",
			s, target))
	}
	return out
}

// impossibleSkillUpCombines is the combine count attributed to a skill point
// whose skill-up chance computed as zero (a degenerate input, not a real
// server condition) so a stage still reports a finite, clearly-inflated
// number instead of infinity.
const impossibleSkillUpCombines = 9999

// emitStage computes one stage — recipe r ground from skill from to to — and
// appends it to out, updating totals.
func emitStage(out *Plan, r RecipeCandidate, from, to int, p Params) {
	combinesF := 0.0
	stalled := false
	for s := from; s < to; s++ {
		pUp := tradeskill.SkillUpChanceAt(
			s, r.Trivial, p.SkillMod, p.AAReduce, p.SkillupBonus, p.TradeStat, p.Difficulty, r.NoFail)
		if pUp <= 0 {
			combinesF += impossibleSkillUpCombines
			stalled = true
			continue
		}
		combinesF += 1.0 / pUp
	}
	combines := int(math.Round(combinesF))
	cost := 0.0
	if r.VendorCostKnown {
		cost = combinesF * r.VendorCost
	}
	successPct := tradeskill.Chance(from, r.Trivial, p.SkillMod, p.AAReduce, p.ClassCap, r.NoFail).Success

	stage := Stage{
		FromSkill:           from,
		ToSkill:             to,
		RecipeID:            r.RecipeID,
		Recipe:              r.Name,
		Trivial:             r.Trivial,
		Combines:            combines,
		Cost:                math.Round(cost), // whole copper; fractional copper is meaningless
		CostKnown:           r.VendorCostKnown,
		SuccessChancePct:    successPct,
		Container:           r.Container,
		NoFail:              r.NoFail,
		SubCombineRecipeIDs: r.SubCombineRecipeIDs,
	}
	if r.Container != "" {
		stage.Notes = append(stage.Notes, "requires "+r.Container)
	}
	if !r.VendorCostKnown {
		stage.Notes = append(stage.Notes, "some components are farmed or dropped — plat cost unknown")
		out.CostComplete = false
	}
	if stalled {
		stage.Notes = append(stage.Notes, "skill-up chance is effectively zero for part of this range — combine estimate is unreliable")
	}
	if r.Note != "" {
		stage.Notes = append(stage.Notes, r.Note)
	}
	// SubCombineRecipeIDs stay on the stage as data; callers render them with
	// their own detail (name / tradeskill / trivial), not a generic note.

	out.Stages = append(out.Stages, stage)
	out.TotalCombines += combines
	if r.VendorCostKnown {
		out.TotalCost += cost
	}
}

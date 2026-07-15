// Package tsplan is a pure solver that builds an optimized tradeskill LEVELING
// plan: an ordered list of "grind recipe X from skill A to skill B" stages that
// carry a character from a start skill to a target with the fewest combines
// (Objective Fastest) or the least vendor plat (Objective Cheapest).
//
// It knows nothing about the database or HTTP — callers pass in the candidate
// recipes (already resolved from quarm.db, with vendor costs and sub-combine
// edges attached) plus the character's stat/difficulty inputs, and get back a
// Plan. This mirrors internal/shoproute (a pure set-cover solver behind the
// spell shopping-route API) so the tradeskill planner can sit behind an API and
// page the same way.
//
// # Model
//
// The single hard rule of EQ tradeskilling: a combine only grants a skill-up
// while your raw skill is below the recipe's TRIVIAL. So a natural "stage" is
// grinding one recipe from the current skill up to min(trivial, cap, target) —
// the point where that recipe stops teaching. The planner chains stages.
//
// Choosing the chain is a shortest-path problem over skill levels. For each
// skill s, dp[s] = min over usable recipes X of
//
//	transition(s -> stopX) + dp[stopX]
//
// where the transition weight is the expected combines (Fastest) or plat
// (Cheapest = combines * per-combine vendor cost) to grind X from s to stopX,
// computed from tradeskill.SkillUpChanceAt (the ported CheckIncreaseTradeskill
// per-attempt skill-up probability). A configurable SwitchPenalty is added per
// stage so plans don't fragment into dozens of one-recipe hops when a wider
// grind is nearly as efficient.
//
// Because staying at high success rate means more skill-ups per combine, the
// optimum naturally switches recipes at trivial breakpoints — exactly the
// range->recipe->trivial cadence the community guides use. Paths are derived
// live from the candidate list, so they are always era-correct (a Luclin
// quarm.db yields Luclin-only recipes; no guide transcription, no PoP leakage).
package tsplan

import (
	"fmt"
	"math"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tradeskill"
)

// Objective selects what the planner minimizes.
type Objective string

const (
	// Fastest minimizes the total number of combines. Fully DB-derivable; needs
	// no cost data.
	Fastest Objective = "fastest"
	// Cheapest minimizes total vendor plat. It is PARTIAL by nature: farmed and
	// dropped components have no database price, so a plan may report an
	// incomplete cost (see Plan.CostComplete).
	Cheapest Objective = "cheapest"
)

// RecipeCandidate is one recipe the planner may use, with everything the solver
// needs pre-resolved. The DB layer builds these; tsplan never looks anything up.
type RecipeCandidate struct {
	RecipeID  int    `json:"recipe_id"`
	Name      string `json:"name"`
	Trivial   int    `json:"trivial"`
	NoFail    bool   `json:"no_fail"`
	Yield     int    `json:"yield"`               // successcount (per-combine output; display only)
	Container string `json:"container,omitempty"` // objecttype label, e.g. "Forge" (a stage note)

	// VendorCost is the plat to buy one combine's worth of ingredients from
	// vendors. VendorCostKnown is false when any ingredient is farmed/dropped and
	// therefore has no database price — such recipes can only be cost-optimized
	// under AllowFarming (their plat is treated as 0), and any stage using one
	// marks the plan cost incomplete.
	VendorCost      float64 `json:"vendor_cost"`
	VendorCostKnown bool    `json:"vendor_cost_known"`

	// SubCombineRecipeIDs are components that are themselves crafted (DAG edges).
	// Phase 1 only FLAGS these on the stage; Phase 2 will cost them recursively.
	SubCombineRecipeIDs []int `json:"sub_combine_recipe_ids,omitempty"`
}

// Params carries the plan-wide inputs (constant across all candidates): the
// skill window, the character's governing stat and skill difficulty, worn
// skill-mod / AA / buff bonuses, and the optimization knobs.
type Params struct {
	StartSkill  int // current raw skill
	TargetSkill int // desired skill
	ClassCap    int // class/level skill cap; 0 = unknown/none

	TradeStat    int     // governing stat value (from tradeskill.TradeStat)
	Difficulty   float64 // skill_difficulty for this tradeskill
	SkillMod     int     // worn item skill-mod % (max, not sum)
	AAReduce     int     // mastery AA fail-reduction %
	SkillupBonus int     // skill-up rate buff % (e.g. Maelin's, +75)

	Objective     Objective
	AllowFarming  bool    // false = drop recipes whose cost is unknown (farmed/dropped)
	SwitchPenalty float64 // per-stage penalty in objective units (combines / plat) to curb fragmentation

	// TrivialCeiling bounds how far below a recipe's trivial you may start
	// grinding it: recipe X is usable at skill s only when X.Trivial - s <=
	// TrivialCeiling. This keeps a stage from grinding a high-trivial recipe up
	// from far below (miserable early success, absurd combine counts) — the
	// guides' "stay within ~25 of your skill" rule. It matters most for the
	// Cheapest objective, which minimizes plat and would otherwise ignore how
	// many combines a cheap high-trivial recipe takes. 0 disables the bound.
	TrivialCeiling int
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
	Objective    Objective `json:"objective"`
	StartSkill   int       `json:"start_skill"`
	TargetSkill  int       `json:"target_skill"`  // as requested
	ReachedSkill int       `json:"reached_skill"` // where the plan actually ends

	Stages        []Stage `json:"stages"`
	TotalCombines int     `json:"total_combines"`
	TotalCost     float64 `json:"total_cost"`

	// CostComplete is false when at least one stage has unknown (farmed/dropped)
	// cost, so TotalCost is a lower bound. Always true for a fully vendor-sourced
	// plan.
	CostComplete bool     `json:"cost_complete"`
	Warnings     []string `json:"warnings,omitempty"`
}

// candidate is a RecipeCandidate with its stop skill and a precomputed suffix
// array of expected combines, so DP transitions are O(1).
type candidate struct {
	src     RecipeCandidate
	stop    int       // min(trivial, target): where this recipe stops teaching
	costPer float64   // plat per combine (0 when unknown)
	known   bool      // vendor cost fully known
	cum     []float64 // cum[s] = expected combines to grind from s to stop (+Inf if impractical)
}

// Solve builds the optimized leveling plan. It is deterministic: ties are broken
// by candidate order (which the caller controls).
func Solve(recipes []RecipeCandidate, p Params) Plan {
	out := Plan{
		Objective:    p.Objective,
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

	cands := buildCandidates(recipes, p, start, target)
	if len(cands) == 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"no usable recipe has a trivial above the current skill %d", start))
		return out
	}

	// How high can we actually climb from start? (The DB may lack recipes to
	// bridge a gap.) Cap the plan at the highest reachable breakpoint.
	maxReach := reachableCeiling(cands, start, target, p.TrivialCeiling)
	if maxReach < target {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"available recipes only reach skill %d (target %d) — no recipe grinds past that",
			maxReach, target))
		target = maxReach
	}
	if target <= start {
		return out
	}

	choice := runDP(cands, p, start, target)
	if choice[start] < 0 {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"could not assemble a viable plan from skill %d", start))
		return out
	}

	emit(&out, cands, choice, start, target, p)
	return out
}

// buildCandidates filters recipes to those usable from start and precomputes
// each one's stop skill and suffix combine-cost array.
func buildCandidates(recipes []RecipeCandidate, p Params, start, target int) []candidate {
	cands := make([]candidate, 0, len(recipes))
	for _, r := range recipes {
		if r.Trivial <= start {
			continue // never grants a skill-up at or above the current skill
		}
		if !r.VendorCostKnown && !p.AllowFarming {
			continue // no-farming mode: skip recipes with farmed/dropped components
		}
		stop := r.Trivial
		if stop > target {
			stop = target
		}
		if stop <= start {
			continue
		}

		c := candidate{src: r, stop: stop, known: r.VendorCostKnown}
		if r.VendorCostKnown {
			c.costPer = r.VendorCost
		}

		// Suffix sum of expected combines: cum[s] = cum[s+1] + 1/pUp(s).
		c.cum = make([]float64, stop+1)
		for s := stop - 1; s >= start; s-- {
			pUp := tradeskill.SkillUpChanceAt(
				s, r.Trivial, p.SkillMod, p.AAReduce, p.SkillupBonus, p.TradeStat, p.Difficulty, r.NoFail)
			if pUp <= 0 || math.IsInf(c.cum[s+1], 1) {
				c.cum[s] = math.Inf(1)
			} else {
				c.cum[s] = c.cum[s+1] + 1.0/pUp
			}
		}
		cands = append(cands, c)
	}
	return cands
}

// combines returns the precomputed expected combines to grind candidate c from
// skill s to its stop, or +Inf if s is out of the recipe's usable range.
func (c candidate) combines(s int) float64 {
	if s < 0 || s >= c.stop {
		return math.Inf(1)
	}
	return c.cum[s]
}

// startableAt reports whether grinding c may BEGIN at skill s under the trivial
// ceiling (s must be within `ceiling` of the recipe's trivial). ceiling <= 0
// disables the bound.
func (c candidate) startableAt(s, ceiling int) bool {
	return ceiling <= 0 || s >= c.src.Trivial-ceiling
}

// reachableCeiling returns the highest skill reachable from start by chaining
// candidates (each landing on its stop).
func reachableCeiling(cands []candidate, start, target, ceiling int) int {
	reach := make([]bool, target+1)
	reach[start] = true
	maxReach := start
	for s := start; s < target; s++ {
		if !reach[s] {
			continue
		}
		for i := range cands {
			c := &cands[i]
			if c.stop > s && c.stop <= target && c.startableAt(s, ceiling) && !math.IsInf(c.combines(s), 1) {
				reach[c.stop] = true
				if c.stop > maxReach {
					maxReach = c.stop
				}
			}
		}
	}
	return maxReach
}

// runDP runs the shortest-path DP over skill levels and returns, for each skill,
// the index of the chosen candidate (-1 if none / unreachable).
func runDP(cands []candidate, p Params, start, target int) []int {
	dp := make([]float64, target+1)
	choice := make([]int, target+1)
	for s := start; s < target; s++ {
		dp[s] = math.Inf(1)
		choice[s] = -1
	}
	dp[target] = 0

	for s := target - 1; s >= start; s-- {
		for i := range cands {
			c := &cands[i]
			if c.stop <= s || c.stop > target || !c.startableAt(s, p.TrivialCeiling) {
				continue
			}
			base := c.combines(s)
			if math.IsInf(base, 1) || math.IsInf(dp[c.stop], 1) {
				continue
			}
			trans := base + dp[c.stop] + p.SwitchPenalty
			if p.Objective == Cheapest {
				trans = base*c.costPer + dp[c.stop] + p.SwitchPenalty
			}
			if trans < dp[s] {
				dp[s] = trans
				choice[s] = i
			}
		}
	}
	return choice
}

// emit walks the chosen chain from start and fills the plan's stages and totals.
func emit(out *Plan, cands []candidate, choice []int, start, target int, p Params) {
	s := start
	for s < target {
		i := choice[s]
		if i < 0 {
			break
		}
		c := cands[i]
		combines := int(math.Round(c.combines(s)))
		cost := c.combines(s) * c.costPer
		successPct := tradeskill.Chance(s, c.src.Trivial, p.SkillMod, p.AAReduce, p.ClassCap, c.src.NoFail).Success

		stage := Stage{
			FromSkill:           s,
			ToSkill:             c.stop,
			RecipeID:            c.src.RecipeID,
			Recipe:              c.src.Name,
			Trivial:             c.src.Trivial,
			Combines:            combines,
			Cost:                math.Round(cost), // whole copper; fractional copper is meaningless
			CostKnown:           c.known,
			SuccessChancePct:    successPct,
			Container:           c.src.Container,
			NoFail:              c.src.NoFail,
			SubCombineRecipeIDs: c.src.SubCombineRecipeIDs,
		}
		if c.src.Container != "" {
			stage.Notes = append(stage.Notes, "requires "+c.src.Container)
		}
		if !c.known {
			stage.Notes = append(stage.Notes, "some components are farmed or dropped — plat cost unknown")
			out.CostComplete = false
		}
		// SubCombineRecipeIDs stay on the stage as data; callers render them with
		// their own detail (name / tradeskill / trivial), not a generic note.

		out.Stages = append(out.Stages, stage)
		out.TotalCombines += combines
		if c.known {
			out.TotalCost += cost
		}
		s = c.stop
	}
	out.ReachedSkill = s
	out.TotalCost = math.Round(out.TotalCost)
}

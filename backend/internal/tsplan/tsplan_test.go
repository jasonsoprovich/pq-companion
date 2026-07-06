package tsplan

import (
	"strings"
	"testing"
)

// baseParams returns realistic stat/difficulty inputs so SkillUpChanceAt yields
// finite, positive combine estimates. Individual tests override the window,
// objective, and knobs.
func baseParams(start, target int, obj Objective) Params {
	return Params{
		StartSkill:  start,
		TargetSkill: target,
		TradeStat:   100,
		Difficulty:  3.0,
		Objective:   obj,
	}
}

// vendorRecipe builds a fully vendor-priced candidate.
func vendorRecipe(id, trivial int, cost float64) RecipeCandidate {
	return RecipeCandidate{
		RecipeID: id, Name: "recipe" + itoa(id), Trivial: trivial,
		VendorCost: cost, VendorCostKnown: true,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func warningsJoined(p Plan) string { return strings.Join(p.Warnings, " | ") }

// assertMonotonic checks stages are contiguous and strictly climbing.
func assertMonotonic(t *testing.T, p Plan, wantStart int) {
	t.Helper()
	prev := wantStart
	for i, s := range p.Stages {
		if s.FromSkill != prev {
			t.Fatalf("stage %d: FromSkill = %d, want %d (contiguous)", i, s.FromSkill, prev)
		}
		if s.ToSkill <= s.FromSkill {
			t.Fatalf("stage %d: ToSkill %d not above FromSkill %d", i, s.ToSkill, s.FromSkill)
		}
		if s.Combines <= 0 {
			t.Fatalf("stage %d: Combines = %d, want > 0", i, s.Combines)
		}
		prev = s.ToSkill
	}
}

func TestPlan_AlreadyAtOrAboveTarget(t *testing.T) {
	p := Solve([]RecipeCandidate{vendorRecipe(1, 60, 1)}, baseParams(100, 50, Fastest))
	if len(p.Stages) != 0 {
		t.Fatalf("expected no stages, got %d", len(p.Stages))
	}
	if !strings.Contains(warningsJoined(p), "already at or above") {
		t.Fatalf("expected already-at-target warning, got %q", warningsJoined(p))
	}
}

func TestPlan_NoUsableRecipe(t *testing.T) {
	// All recipes trivial <= start: nothing can skill us up.
	p := Solve([]RecipeCandidate{vendorRecipe(1, 30, 1)}, baseParams(50, 100, Fastest))
	if len(p.Stages) != 0 {
		t.Fatalf("expected no stages, got %d", len(p.Stages))
	}
	if !strings.Contains(warningsJoined(p), "no usable recipe") {
		t.Fatalf("expected no-usable-recipe warning, got %q", warningsJoined(p))
	}
	if p.ReachedSkill != 50 {
		t.Fatalf("ReachedSkill = %d, want 50", p.ReachedSkill)
	}
}

func TestPlan_SimpleChain(t *testing.T) {
	recipes := []RecipeCandidate{
		vendorRecipe(1, 30, 1),
		vendorRecipe(2, 60, 1),
	}
	p := Solve(recipes, baseParams(10, 60, Fastest))
	if len(p.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d: %+v", len(p.Stages), p.Stages)
	}
	assertMonotonic(t, p, 10)
	if p.Stages[0].RecipeID != 1 || p.Stages[1].RecipeID != 2 {
		t.Fatalf("stage recipe order = %d,%d want 1,2", p.Stages[0].RecipeID, p.Stages[1].RecipeID)
	}
	if p.ReachedSkill != 60 {
		t.Fatalf("ReachedSkill = %d, want 60", p.ReachedSkill)
	}
	if p.TotalCombines <= 0 {
		t.Fatalf("TotalCombines = %d, want > 0", p.TotalCombines)
	}
	if !p.CostComplete {
		t.Fatalf("CostComplete = false, want true for all-vendor plan")
	}
}

func TestPlan_CapClamped(t *testing.T) {
	recipes := []RecipeCandidate{vendorRecipe(1, 150, 1)}
	pp := baseParams(20, 200, Fastest)
	pp.ClassCap = 100
	p := Solve(recipes, pp)
	if !strings.Contains(warningsJoined(p), "exceeds the class/level cap") {
		t.Fatalf("expected cap warning, got %q", warningsJoined(p))
	}
	if p.ReachedSkill != 100 {
		t.Fatalf("ReachedSkill = %d, want 100 (clamped to cap)", p.ReachedSkill)
	}
}

func TestPlan_Unreachable(t *testing.T) {
	// Highest trivial is 50 but target is 100 — plan should stop at 50.
	recipes := []RecipeCandidate{
		vendorRecipe(1, 30, 1),
		vendorRecipe(2, 50, 1),
	}
	p := Solve(recipes, baseParams(10, 100, Fastest))
	if p.ReachedSkill != 50 {
		t.Fatalf("ReachedSkill = %d, want 50", p.ReachedSkill)
	}
	if !strings.Contains(warningsJoined(p), "only reach skill 50") {
		t.Fatalf("expected reach warning, got %q", warningsJoined(p))
	}
	assertMonotonic(t, p, 10)
}

func TestPlan_NoFarmingExcludesUnknownCost(t *testing.T) {
	farmed := RecipeCandidate{RecipeID: 2, Name: "farmed", Trivial: 40, VendorCostKnown: false}
	recipes := []RecipeCandidate{vendorRecipe(1, 40, 5), farmed}

	// AllowFarming=false: farmed recipe excluded; only the vendor recipe is used.
	pp := baseParams(10, 40, Cheapest)
	pp.AllowFarming = false
	p := Solve(recipes, pp)
	if len(p.Stages) != 1 || p.Stages[0].RecipeID != 1 {
		t.Fatalf("no-farming plan should use only vendor recipe 1, got %+v", p.Stages)
	}
	if !p.CostComplete {
		t.Fatalf("CostComplete = false, want true when no farmed stage used")
	}

	// AllowFarming=true + Cheapest: farmed is treated as 0 plat, so it wins.
	pp.AllowFarming = true
	p = Solve(recipes, pp)
	if len(p.Stages) != 1 || p.Stages[0].RecipeID != 2 {
		t.Fatalf("cheapest+farming should pick farmed recipe 2, got %+v", p.Stages)
	}
	if p.CostComplete {
		t.Fatalf("CostComplete = true, want false when a farmed stage is used")
	}
	if p.Stages[0].CostKnown {
		t.Fatalf("farmed stage CostKnown = true, want false")
	}
}

func TestPlan_CheapestVsFastestPickDifferently(t *testing.T) {
	// Two recipes span the same band; equal combines, very different plat.
	// Fastest ties and keeps the first (index 0 = expensive). Cheapest picks the
	// cheap one.
	expensive := vendorRecipe(1, 40, 100)
	cheap := vendorRecipe(2, 40, 1)
	recipes := []RecipeCandidate{expensive, cheap}

	fast := Solve(recipes, baseParams(10, 40, Fastest))
	if len(fast.Stages) != 1 || fast.Stages[0].RecipeID != 1 {
		t.Fatalf("fastest tie should keep first recipe (1), got %+v", fast.Stages)
	}

	cheapPlan := Solve(recipes, baseParams(10, 40, Cheapest))
	if len(cheapPlan.Stages) != 1 || cheapPlan.Stages[0].RecipeID != 2 {
		t.Fatalf("cheapest should pick recipe 2, got %+v", cheapPlan.Stages)
	}
	if cheapPlan.TotalCost >= fast.Stages[0].Cost {
		t.Fatalf("cheapest total cost %.2f should be below expensive stage cost %.2f",
			cheapPlan.TotalCost, fast.Stages[0].Cost)
	}
}

func TestPlan_SwitchPenaltyConsolidates(t *testing.T) {
	// A narrow recipe and a wide recipe both available. With no penalty the
	// planner is free to chain; with a huge penalty it must collapse to the
	// single wide-recipe stage.
	recipes := []RecipeCandidate{
		vendorRecipe(1, 30, 1), // narrow
		vendorRecipe(2, 60, 1), // wide, covers the whole span alone
	}
	pp := baseParams(10, 60, Fastest)
	pp.SwitchPenalty = 1e6
	p := Solve(recipes, pp)
	if len(p.Stages) != 1 {
		t.Fatalf("huge switch penalty should yield 1 stage, got %d: %+v", len(p.Stages), p.Stages)
	}
	if p.Stages[0].FromSkill != 10 || p.Stages[0].ToSkill != 60 {
		t.Fatalf("single stage should cover 10->60, got %d->%d", p.Stages[0].FromSkill, p.Stages[0].ToSkill)
	}
}

func TestPlan_StageNotes(t *testing.T) {
	r := vendorRecipe(1, 60, 1)
	r.Container = "Forge"
	r.SubCombineRecipeIDs = []int{99}
	p := Solve([]RecipeCandidate{r}, baseParams(10, 60, Fastest))
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	notes := strings.Join(p.Stages[0].Notes, " | ")
	if !strings.Contains(notes, "Forge") {
		t.Fatalf("expected container note, got %q", notes)
	}
	if !strings.Contains(notes, "sub-component") {
		t.Fatalf("expected sub-combine note, got %q", notes)
	}
}

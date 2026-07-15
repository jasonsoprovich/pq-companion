package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tsplan"
)

// mapCandidates mirrors the LevelingRecipe -> tsplan.RecipeCandidate mapping the
// tradeskillPlan handler performs, so the pipeline test exercises the same glue.
func mapCandidates(recipes []db.LevelingRecipe) []tsplan.RecipeCandidate {
	cands := make([]tsplan.RecipeCandidate, len(recipes))
	for i, rc := range recipes {
		cands[i] = tsplan.RecipeCandidate{
			RecipeID:            rc.RecipeID,
			Name:                rc.Name,
			Trivial:             rc.Trivial,
			NoFail:              rc.NoFail,
			Yield:               rc.Yield,
			Container:           rc.Container,
			VendorCost:          float64(rc.VendorCost),
			VendorCostKnown:     rc.VendorCostKnown,
			SubCombineRecipeIDs: rc.SubCombineRecipeIDs,
		}
	}
	return cands
}

func assertPlanMonotonic(t *testing.T, plan tsplan.Plan, start int) {
	t.Helper()
	prev := start
	for i, s := range plan.Stages {
		if s.FromSkill != prev {
			t.Fatalf("stage %d FromSkill = %d, want %d (contiguous)", i, s.FromSkill, prev)
		}
		if s.ToSkill <= s.FromSkill {
			t.Fatalf("stage %d ToSkill %d not above FromSkill %d", i, s.ToSkill, s.FromSkill)
		}
		if s.Trivial <= s.FromSkill {
			t.Fatalf("stage %d recipe trivial %d not above FromSkill %d (no skill-ups)", i, s.Trivial, s.FromSkill)
		}
		prev = s.ToSkill
	}
	if plan.ReachedSkill != prev {
		t.Fatalf("ReachedSkill = %d, want %d (last stage ToSkill)", plan.ReachedSkill, prev)
	}
}

// TestTradeskillPlanPipeline_Blacksmithing runs the full query->map->solve
// pipeline over real Blacksmithing recipes, the way the handler does. It proves
// the solver produces a sane leveling path on real recipe shapes (realistic
// trivials, vendor costs, sub-combine edges) — coverage the synthetic tsplan
// unit tests can't give.
func TestTradeskillPlanPipeline_Blacksmithing(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	recipes, err := d.RecipesForTradeskill(63) // Blacksmithing
	if err != nil {
		t.Fatalf("recipes for blacksmithing: %v", err)
	}
	if len(recipes) == 0 {
		t.Fatal("no blacksmithing recipes")
	}
	cands := mapCandidates(recipes)

	const start, target = 1, 188

	// Fastest, farming allowed: should climb well up toward the target.
	fast := tsplan.Solve(cands, tsplan.Params{
		StartSkill: start, TargetSkill: target,
		TradeStat: 100, Difficulty: 3.0,
		Objective: tsplan.Fastest, AllowFarming: true, SwitchPenalty: 5,
		TrivialCeiling: tradeskillPlanTrivialCeiling,
	})
	if len(fast.Stages) == 0 {
		t.Fatalf("fastest plan produced no stages; warnings=%v", fast.Warnings)
	}
	assertPlanMonotonic(t, fast, start)
	if fast.ReachedSkill <= start {
		t.Fatalf("fastest reached %d, want above start %d", fast.ReachedSkill, start)
	}
	if fast.TotalCombines <= 0 {
		t.Fatalf("fastest TotalCombines = %d, want > 0", fast.TotalCombines)
	}
	t.Logf("blacksmithing fastest %d->%d: %d stages, %d combines, reached %d",
		start, target, len(fast.Stages), fast.TotalCombines, fast.ReachedSkill)

	// Cheapest, vendor-only: a valid (possibly partial) vendor-sourced path must
	// still climb above start, and every stage must be fully costed.
	cheap := tsplan.Solve(cands, tsplan.Params{
		StartSkill: start, TargetSkill: target,
		TradeStat: 100, Difficulty: 3.0,
		Objective: tsplan.Cheapest, AllowFarming: false, SwitchPenalty: 0,
		TrivialCeiling: tradeskillPlanTrivialCeiling,
	})
	if len(cheap.Stages) == 0 {
		t.Fatalf("vendor-only cheapest plan produced no stages; warnings=%v", cheap.Warnings)
	}
	assertPlanMonotonic(t, cheap, start)
	if cheap.ReachedSkill <= start {
		t.Fatalf("cheapest reached %d, want above start %d", cheap.ReachedSkill, start)
	}
	if !cheap.CostComplete {
		t.Errorf("vendor-only plan should have complete cost, got CostComplete=false")
	}
	for i, s := range cheap.Stages {
		if !s.CostKnown {
			t.Errorf("vendor-only stage %d (%s) has unknown cost", i, s.Recipe)
		}
	}
	t.Logf("blacksmithing cheapest (vendor-only) %d->%d: %d stages, %d copper, reached %d",
		start, target, len(cheap.Stages), int(cheap.TotalCost), cheap.ReachedSkill)
}

// TestResolveSubCombines_Blacksmithing exercises the API's sub-combine
// enrichment on real recipes: each referenced sub-combine resolves to a named
// discipline, cross-tradeskill flags match, and a cross-tradeskill dependency
// yields a warning.
func TestResolveSubCombines_Blacksmithing(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	h := &charactersHandler{db: d}
	recipes, err := d.RecipesForTradeskill(63)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}

	// Gather sub-combine ids across the whole discipline into one synthetic stage.
	var subIDs []int
	for _, r := range recipes {
		subIDs = append(subIDs, r.SubCombineRecipeIDs...)
	}
	if len(subIDs) == 0 {
		t.Skip("no blacksmithing sub-combines in fixture")
	}
	stage := tsplan.Stage{SubCombineRecipeIDs: subIDs}

	info, warnings := h.resolveSubCombines([]tsplan.Stage{stage}, 63)
	if len(info) == 0 {
		t.Fatal("expected sub-combine detail, got none")
	}
	hasCross := false
	for _, sc := range info {
		if sc.TradeskillName == "" {
			t.Errorf("sub-combine %d (%s) has empty tradeskill name", sc.RecipeID, sc.Name)
		}
		wantCross := sc.Tradeskill != 63 && sc.Tradeskill != 0 && sc.Tradeskill != 75
		if sc.CrossTradeskill != wantCross {
			t.Errorf("sub-combine %d cross flag %v but tradeskill %d (plan 63)",
				sc.RecipeID, sc.CrossTradeskill, sc.Tradeskill)
		}
		if sc.CrossTradeskill {
			hasCross = true
		}
	}
	if hasCross && len(warnings) == 0 {
		t.Error("cross-tradeskill sub-combines present but no warning produced")
	}
	t.Logf("blacksmithing sub-combines: %d distinct, %d cross-tradeskill warnings: %v",
		len(info), len(warnings), warnings)
}

// TestTradeskillPlan_AvoidOtherTradeskills verifies the "stay in this discipline"
// mode: dropping recipes that require another skill-gated tradeskill yields a
// plan with no cross-tradeskill warning.
func TestTradeskillPlan_AvoidOtherTradeskills(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	h := &charactersHandler{db: d}
	recipes, err := d.RecipesForTradeskill(63)
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}

	// The flag must be meaningful — some recipes cross, some stay in-discipline.
	var anyCross, anyClean bool
	var cands []tsplan.RecipeCandidate
	for _, rc := range recipes {
		if rc.RequiresCrossTradeskill {
			anyCross = true
			continue // avoid mode drops these
		}
		anyClean = true
		cands = append(cands, mapCandidates([]db.LevelingRecipe{rc})...)
	}
	if !anyCross {
		t.Skip("no cross-tradeskill blacksmithing recipes in fixture")
	}
	if !anyClean {
		t.Fatal("expected some in-discipline blacksmithing recipes")
	}

	plan := tsplan.Solve(cands, tsplan.Params{
		StartSkill: 1, TargetSkill: 188,
		TradeStat: 100, Difficulty: 3.0,
		Objective: tsplan.Fastest, AllowFarming: true, SwitchPenalty: 5,
		TrivialCeiling: tradeskillPlanTrivialCeiling,
	})
	if len(plan.Stages) == 0 {
		t.Fatalf("avoid-others plan produced no stages; warnings=%v", plan.Warnings)
	}
	_, warnings := h.resolveSubCombines(plan.Stages, 63)
	if len(warnings) != 0 {
		t.Errorf("avoid-others plan still has cross-tradeskill warnings: %v", warnings)
	}
	t.Logf("avoid-others blacksmithing 1->188: %d stages, reached %d (%d cross recipes excluded)",
		len(plan.Stages), plan.ReachedSkill, len(recipes)-len(cands))
}

// TestTradeskillPlan_ExcludeRecipeIDs verifies "Custom" mode (issue #149):
// dropping a recipe the default fastest plan actually uses reroutes the plan
// around it entirely (that recipe never appears in any stage) rather than
// leaving a gap, and the plan is recomputed from the full candidate pool minus
// the exclusion — not filtered from an already-built plan.
func TestTradeskillPlan_ExcludeRecipeIDs(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	recipes, err := d.RecipesForTradeskill(63) // Blacksmithing
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	if len(recipes) == 0 {
		t.Fatal("no blacksmithing recipes")
	}
	cands := mapCandidates(recipes)

	const start, target = 1, 188
	params := tsplan.Params{
		StartSkill: start, TargetSkill: target,
		TradeStat: 100, Difficulty: 3.0,
		Objective: tsplan.Fastest, AllowFarming: true, SwitchPenalty: 5,
		TrivialCeiling: tradeskillPlanTrivialCeiling,
	}

	baseline := tsplan.Solve(cands, params)
	if len(baseline.Stages) == 0 {
		t.Fatalf("baseline plan produced no stages; warnings=%v", baseline.Warnings)
	}
	excludeID := baseline.Stages[0].RecipeID

	filtered := make([]tsplan.RecipeCandidate, 0, len(cands))
	for _, c := range cands {
		if c.RecipeID != excludeID {
			filtered = append(filtered, c)
		}
	}
	rerouted := tsplan.Solve(filtered, params)
	if len(rerouted.Stages) == 0 {
		t.Fatalf("rerouted plan produced no stages; warnings=%v", rerouted.Warnings)
	}
	assertPlanMonotonic(t, rerouted, start)
	for _, s := range rerouted.Stages {
		if s.RecipeID == excludeID {
			t.Fatalf("excluded recipe %d still appears in rerouted plan", excludeID)
		}
	}
	if rerouted.ReachedSkill <= start {
		t.Fatalf("rerouted plan reached %d, want above start %d", rerouted.ReachedSkill, start)
	}
	t.Logf("blacksmithing exclude-recipe %d: baseline %d stages -> rerouted %d stages, reached %d",
		excludeID, len(baseline.Stages), len(rerouted.Stages), rerouted.ReachedSkill)
}

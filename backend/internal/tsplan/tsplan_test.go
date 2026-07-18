package tsplan

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tradeskill"
)

// An empty plan must serialize stages as [] not null, so the frontend can read
// stages.length without guarding against null (a crash that black-screened the
// page when every recipe was filtered out).
func TestPlan_EmptyStagesSerializeAsArray(t *testing.T) {
	p := BuildFromRecipes(nil, baseParams(1, 100))
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"stages":[]`) {
		t.Fatalf("expected \"stages\":[] in JSON, got %s", b)
	}
}

// baseParams returns realistic stat/difficulty inputs so SkillUpChanceAt yields
// finite, positive combine estimates. Individual tests override the window.
func baseParams(start, target int) Params {
	return Params{
		StartSkill:  start,
		TargetSkill: target,
		TradeStat:   100,
		Difficulty:  3.0,
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
	p := BuildFromRecipes([]RecipeCandidate{vendorRecipe(1, 60, 1)}, baseParams(100, 50))
	if len(p.Stages) != 0 {
		t.Fatalf("expected no stages, got %d", len(p.Stages))
	}
	if !strings.Contains(warningsJoined(p), "already at or above") {
		t.Fatalf("expected already-at-target warning, got %q", warningsJoined(p))
	}
}

func TestPlan_NoUsableRecipe(t *testing.T) {
	// The only recipe's trivial is already at/below start: it teaches nothing,
	// so the plan can't leave the starting skill.
	p := BuildFromRecipes([]RecipeCandidate{vendorRecipe(1, 30, 1)}, baseParams(50, 100))
	if len(p.Stages) != 0 {
		t.Fatalf("expected no stages, got %d", len(p.Stages))
	}
	if !strings.Contains(warningsJoined(p), "only reaches skill 50") {
		t.Fatalf("expected reach warning, got %q", warningsJoined(p))
	}
	if p.ReachedSkill != 50 {
		t.Fatalf("ReachedSkill = %d, want 50", p.ReachedSkill)
	}
}

func TestPlan_NoRecipesYet(t *testing.T) {
	p := BuildFromRecipes([]RecipeCandidate{}, baseParams(10, 100))
	if len(p.Stages) != 0 {
		t.Fatalf("expected no stages, got %d", len(p.Stages))
	}
	if !strings.Contains(warningsJoined(p), "no recipes yet") {
		t.Fatalf("expected empty-path warning, got %q", warningsJoined(p))
	}
}

func TestPlan_SimpleChain(t *testing.T) {
	recipes := []RecipeCandidate{
		vendorRecipe(1, 30, 1),
		vendorRecipe(2, 60, 1),
	}
	p := BuildFromRecipes(recipes, baseParams(10, 60))
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
	pp := baseParams(20, 200)
	pp.ClassCap = 100
	p := BuildFromRecipes(recipes, pp)
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
	p := BuildFromRecipes(recipes, baseParams(10, 100))
	if p.ReachedSkill != 50 {
		t.Fatalf("ReachedSkill = %d, want 50", p.ReachedSkill)
	}
	if !strings.Contains(warningsJoined(p), "only reaches skill 50") {
		t.Fatalf("expected reach warning, got %q", warningsJoined(p))
	}
	assertMonotonic(t, p, 10)
}

// TestPlan_CallerOrderNotResorted is the key behavior distinguishing
// BuildFromRecipes from the old DP solver: it walks recipes in EXACTLY the
// order given, never re-picking a "better" one. A curated Recommended path
// depends on this — the guide's teaching order must survive untouched. Here,
// a higher-trivial recipe listed FIRST greedily claims the whole span, even
// though a lower-trivial recipe would have made a tighter two-stage plan.
func TestPlan_CallerOrderNotResorted(t *testing.T) {
	recipes := []RecipeCandidate{
		vendorRecipe(2, 60, 1), // listed first, despite the higher trivial
		vendorRecipe(1, 30, 1),
	}
	p := BuildFromRecipes(recipes, baseParams(10, 60))
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage (caller order claims the whole span), got %d: %+v", len(p.Stages), p.Stages)
	}
	if p.Stages[0].RecipeID != 2 || p.Stages[0].FromSkill != 10 || p.Stages[0].ToSkill != 60 {
		t.Fatalf("expected recipe 2 covering 10->60, got %+v", p.Stages[0])
	}
}

func TestPlan_FarmedCostMarksIncomplete(t *testing.T) {
	farmed := RecipeCandidate{RecipeID: 2, Name: "farmed", Trivial: 40, VendorCostKnown: false}
	p := BuildFromRecipes([]RecipeCandidate{farmed}, baseParams(10, 40))
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	if p.Stages[0].CostKnown {
		t.Fatalf("farmed stage CostKnown = true, want false")
	}
	if p.CostComplete {
		t.Fatalf("CostComplete = true, want false when a farmed stage is used")
	}
	if !strings.Contains(strings.Join(p.Stages[0].Notes, " | "), "farmed or dropped") {
		t.Fatalf("expected farmed-cost note, got %v", p.Stages[0].Notes)
	}
}

func TestPlan_StageNotes(t *testing.T) {
	r := vendorRecipe(1, 60, 1)
	r.Container = "Forge"
	r.SubCombineRecipeIDs = []int{99}
	r.Note = "bulk-buy water flasks"
	p := BuildFromRecipes([]RecipeCandidate{r}, baseParams(10, 60))
	if len(p.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(p.Stages))
	}
	notes := strings.Join(p.Stages[0].Notes, " | ")
	if !strings.Contains(notes, "Forge") {
		t.Fatalf("expected container note, got %q", notes)
	}
	if !strings.Contains(notes, "bulk-buy water flasks") {
		t.Fatalf("expected curator note to surface, got %q", notes)
	}
	// Sub-combine ids ride on the stage as data (callers render their detail),
	// not as a generic note.
	if len(p.Stages[0].SubCombineRecipeIDs) != 1 || p.Stages[0].SubCombineRecipeIDs[0] != 99 {
		t.Fatalf("expected sub-combine id 99 on stage, got %v", p.Stages[0].SubCombineRecipeIDs)
	}
}

// TestPlan_SuccessChancePct verifies each stage carries the combine success %
// at its FromSkill (a distinct roll from the skill-up chance Combines is
// derived from), matching tradeskill.Chance exactly for the same inputs.
func TestPlan_SuccessChancePct(t *testing.T) {
	recipes := []RecipeCandidate{
		vendorRecipe(1, 30, 1),
		vendorRecipe(2, 60, 1),
	}
	params := baseParams(10, 60)
	p := BuildFromRecipes(recipes, params)
	if len(p.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(p.Stages))
	}
	for i, s := range p.Stages {
		want := tradeskill.Chance(s.FromSkill, s.Trivial, params.SkillMod, params.AAReduce, params.ClassCap, s.NoFail).Success
		if s.SuccessChancePct != want {
			t.Fatalf("stage %d: SuccessChancePct = %v, want %v (tradeskill.Chance at skill %d, trivial %d)",
				i, s.SuccessChancePct, want, s.FromSkill, s.Trivial)
		}
	}
}

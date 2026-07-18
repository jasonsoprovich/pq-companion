package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tsplan"
)

// mapCandidates mirrors the LevelingRecipe -> tsplan.RecipeCandidate mapping the
// tradeskillPlan handler performs, so the pipeline test exercises the same glue.
func mapCandidates(recipes []db.LevelingRecipe) []tsplan.RecipeCandidate {
	cands := make([]tsplan.RecipeCandidate, len(recipes))
	for i, rc := range recipes {
		cands[i] = tradeskillCandidate(rc, "")
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

// openRealQuarmDB opens the shipped quarm.db, skipping the test when it isn't
// present (e.g. a checkout without the game data fixture).
func openRealQuarmDB(t *testing.T) *db.DB {
	t.Helper()
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
	t.Cleanup(func() { d.Close() })
	return d
}

// TestBuildFromRecipesPipeline_Blacksmithing runs the query->map->build
// pipeline over real Blacksmithing recipes the way Custom mode does (sorted
// by trivial), proving BuildFromRecipes produces a sane leveling path on real
// recipe shapes (realistic trivials, vendor costs, sub-combine edges) —
// coverage the synthetic tsplan unit tests can't give.
func TestBuildFromRecipesPipeline_Blacksmithing(t *testing.T) {
	d := openRealQuarmDB(t)

	recipes, err := d.RecipesForTradeskill(63) // Blacksmithing
	if err != nil {
		t.Fatalf("recipes for blacksmithing: %v", err)
	}
	if len(recipes) == 0 {
		t.Fatal("no blacksmithing recipes")
	}
	sort.Slice(recipes, func(i, j int) bool {
		if recipes[i].Trivial != recipes[j].Trivial {
			return recipes[i].Trivial < recipes[j].Trivial
		}
		return recipes[i].RecipeID < recipes[j].RecipeID
	})
	cands := mapCandidates(recipes)

	const start, target = 1, 188
	plan := tsplan.BuildFromRecipes(cands, tsplan.Params{
		StartSkill: start, TargetSkill: target,
		TradeStat: 100, Difficulty: 3.0,
	})
	if len(plan.Stages) == 0 {
		t.Fatalf("plan produced no stages; warnings=%v", plan.Warnings)
	}
	assertPlanMonotonic(t, plan, start)
	if plan.ReachedSkill <= start {
		t.Fatalf("reached %d, want above start %d", plan.ReachedSkill, start)
	}
	if plan.TotalCombines <= 0 {
		t.Fatalf("TotalCombines = %d, want > 0", plan.TotalCombines)
	}
	t.Logf("blacksmithing %d->%d: %d stages, %d combines, reached %d",
		start, target, len(plan.Stages), plan.TotalCombines, plan.ReachedSkill)
}

// TestResolveSubCombines_Blacksmithing exercises the API's sub-combine
// enrichment on real recipes: each referenced sub-combine resolves to a named
// discipline, cross-tradeskill flags match, and a cross-tradeskill dependency
// yields a warning.
func TestResolveSubCombines_Blacksmithing(t *testing.T) {
	d := openRealQuarmDB(t)

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

// newPlanTestRouter wires a minimal chi router around a charactersHandler with
// a real quarm.db and a fresh temp-file character/config store, so the
// tradeskill-plan route (mode dispatch, race filter, cost/sub-combine
// enrichment) is exercised end-to-end the way the real API serves it.
func newPlanTestRouter(t *testing.T) (*chi.Mux, *character.Store, *db.DB) {
	t.Helper()
	d := openRealQuarmDB(t)

	store, err := character.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	mgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	h := &charactersHandler{store: store, mgr: mgr, db: d}
	r := chi.NewRouter()
	r.Post("/api/characters/{id}/tradeskill-plan", h.tradeskillPlan)
	return r, store, d
}

func postPlan(t *testing.T, r *chi.Mux, charID int, body map[string]any) tradeskillPlanResponse {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/characters/"+strconv.Itoa(charID)+"/tradeskill-plan", bytes.NewReader(b))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp tradeskillPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return resp
}

// TestTradeskillPlanHandler_CustomMode verifies mode="custom" end-to-end: a
// saved custom path (character.Store, global per tradeskill) drives the plan,
// sorted by trivial, through the real HTTP handler.
func TestTradeskillPlanHandler_CustomMode(t *testing.T) {
	r, store, d := newPlanTestRouter(t)

	char, err := store.Create("Testchar", 0, 0, 60)
	if err != nil {
		t.Fatalf("create character: %v", err)
	}

	recipes, err := d.RecipesForTradeskill(63) // Blacksmithing
	if err != nil {
		t.Fatalf("recipes: %v", err)
	}
	// Only recipes that actually teach something from skill 0 (trivial > 0)
	// are useful here — some low-index rows sit at trivial 0/1.
	var teaching []db.LevelingRecipe
	for _, rc := range recipes {
		if rc.Trivial > 0 {
			teaching = append(teaching, rc)
		}
	}
	if len(teaching) < 3 {
		t.Skip("not enough teaching blacksmithing recipes in fixture")
	}
	sort.Slice(teaching, func(i, j int) bool { return teaching[i].Trivial < teaching[j].Trivial })
	for _, rc := range teaching[:3] {
		if err := store.AddCustomLevelingRecipe(63, rc.RecipeID); err != nil {
			t.Fatalf("add custom recipe %d: %v", rc.RecipeID, err)
		}
	}

	resp := postPlan(t, r, char.ID, map[string]any{
		"tradeskill":   63,
		"start_skill":  0,
		"target_skill": 250,
		"mode":         "custom",
	})
	if resp.Mode != tradeskillModeCustom {
		t.Fatalf("Mode = %q, want %q", resp.Mode, tradeskillModeCustom)
	}
	if len(resp.Stages) == 0 {
		t.Fatalf("expected stages from a non-empty custom path, got none; warnings=%v", resp.Warnings)
	}
	if resp.ReachedSkill <= 0 {
		t.Fatalf("ReachedSkill = %d, want above start 0", resp.ReachedSkill)
	}
}

// TestTradeskillPlanHandler_RecommendedEmptyState verifies mode="recommended"
// for a discipline with no curated data returns a clean empty plan rather than
// an error. Sense Traps (62) is not a craftable discipline at all (no
// tradeskill_recipe rows reference it) so it can never gain a curated path —
// a permanent choice for this test, unlike the earlier now-curated
// placeholders. Every real crafting discipline (Fishing 55, Make Poison 56,
// Tinkering 57, Research 58, Alchemy 59, Baking 60, Tailoring 61,
// Blacksmithing 63, Fletching 64, Brewing 65, Jewelry Making 68, Pottery 69)
// is curated as of this P3 pass — see internal/db/tradeskill_paths.json.
func TestTradeskillPlanHandler_RecommendedEmptyState(t *testing.T) {
	r, store, _ := newPlanTestRouter(t)
	char, err := store.Create("Testchar2", 0, 0, 60)
	if err != nil {
		t.Fatalf("create character: %v", err)
	}

	resp := postPlan(t, r, char.ID, map[string]any{
		"tradeskill":   62,
		"start_skill":  1,
		"target_skill": 250,
		"mode":         "recommended",
	})
	if resp.Mode != tradeskillModeRecommended {
		t.Fatalf("Mode = %q, want %q", resp.Mode, tradeskillModeRecommended)
	}
	if len(resp.Stages) != 0 {
		t.Fatalf("expected no stages (no curated path yet), got %d", len(resp.Stages))
	}
}

// TestTradeskillPlanHandler_RecommendedMode_Curated locks in every curated
// Recommended path — all 12 real crafting disciplines, see
// internal/db/tradeskill_paths.json — against real quarm.db data: every
// curated recipe id must still exist and the plan must climb from 0 without
// gaps. Uses race 0 ("unknown") so the cultural race-restrict filter is a
// no-op (see charactersHandler.tradeskillPlan's usable() check) — this test
// is about the curated data being structurally sound, not about race
// filtering, which is covered elsewhere. 150 is a floor comfortably below
// every trade's actual reach (lowest is Research at 182) — it's deliberately
// loose so a future data-availability change (a trade's ceiling moving) isn't
// a false failure here.
func TestTradeskillPlanHandler_RecommendedMode_Curated(t *testing.T) {
	r, store, _ := newPlanTestRouter(t)
	char, err := store.Create("Testchar3", 0, 0, 60)
	if err != nil {
		t.Fatalf("create character: %v", err)
	}

	for _, ts := range []int{55, 56, 57, 58, 59, 60, 61, 63, 64, 65, 68, 69} {
		resp := postPlan(t, r, char.ID, map[string]any{
			"tradeskill":   ts,
			"start_skill":  0,
			"target_skill": 252,
			"mode":         "recommended",
		})
		if resp.Mode != tradeskillModeRecommended {
			t.Fatalf("tradeskill %d: Mode = %q, want %q", ts, resp.Mode, tradeskillModeRecommended)
		}
		if len(resp.Stages) == 0 {
			t.Fatalf("tradeskill %d: expected curated stages, got none; warnings=%v", ts, resp.Warnings)
		}
		assertPlanMonotonic(t, resp.Plan, 0)
		if resp.ReachedSkill < 150 {
			t.Errorf("tradeskill %d: curated path only reached %d, want at least 150", ts, resp.ReachedSkill)
		}
	}
}

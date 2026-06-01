package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// Recipe 791 is "Spell: Mesmerization" — Research (tradeskill 58), trivial 21,
// combining the two halves of Tasarin's Grimoire Pg. 26 (items 16072 + 16073)
// inside a Research tome to yield the spell scroll (item 16217).
func TestGetRecipe_Mesmerization(t *testing.T) {
	d := openTestDB(t)
	r, err := d.GetRecipe(791)
	if err != nil {
		t.Fatalf("get recipe 791: %v", err)
	}
	if r.Tradeskill != 58 {
		t.Errorf("tradeskill = %d, want 58 (Research)", r.Tradeskill)
	}
	if r.Trivial != 21 {
		t.Errorf("trivial = %d, want 21", r.Trivial)
	}
	if r.ProductItemID != 16217 {
		t.Errorf("product item = %d, want 16217", r.ProductItemID)
	}

	hasProduct := false
	for _, p := range r.Products {
		if p.ItemID == 16217 && p.Role == "product" {
			hasProduct = true
		}
	}
	if !hasProduct {
		t.Errorf("expected product 16217 among %+v", r.Products)
	}

	wantComponents := map[int]bool{16072: false, 16073: false}
	for _, c := range r.Components {
		if c.Role != "component" {
			t.Errorf("component %d has role %q", c.ItemID, c.Role)
		}
		if _, ok := wantComponents[c.ItemID]; ok {
			wantComponents[c.ItemID] = true
		}
	}
	for id, found := range wantComponents {
		if !found {
			t.Errorf("expected component %d in recipe, components=%+v", id, r.Components)
		}
	}

	if len(r.Containers) == 0 {
		t.Errorf("expected at least one container, got none")
	}
	// Container code 27 has no items row — it must resolve to the bagtype
	// station name (Enchanters Lexicon) and be flagged as a station, not
	// rendered as "(combine container)".
	foundStation := false
	for _, c := range r.Containers {
		if c.ItemID == 27 {
			foundStation = true
			if !c.Station {
				t.Errorf("container 27 should be a station")
			}
			if c.ItemName != "Enchanters Lexicon" {
				t.Errorf("container 27 name = %q, want Enchanters Lexicon", c.ItemName)
			}
		}
	}
	if !foundStation {
		t.Errorf("expected combine-station container (code 27) in recipe")
	}
}

func TestGetRecipe_NotFound(t *testing.T) {
	d := openTestDB(t)
	if _, err := d.GetRecipe(99999999); err == nil {
		t.Errorf("expected error for nonexistent recipe, got nil")
	}
}

func TestSearchRecipes_TrivialAndTradeskillFilter(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchRecipes(db.RecipeFilter{
		Tradeskill: 58, // Research
		TrivialMin: 15,
		TrivialMax: 30,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("search recipes: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected Research recipes in trivial 15-30, got none")
	}
	foundMez := false
	for _, s := range res.Items {
		if s.Tradeskill != 58 {
			t.Errorf("recipe %d tradeskill = %d, want 58", s.ID, s.Tradeskill)
		}
		if s.Trivial < 15 || s.Trivial > 30 {
			t.Errorf("recipe %d trivial = %d, outside 15-30", s.ID, s.Trivial)
		}
		if s.ID == 791 {
			foundMez = true
		}
	}
	if !foundMez {
		t.Errorf("expected recipe 791 (Mesmerization, trivial 21) in results")
	}
}

func TestSearchRecipes_AnyTradeskill(t *testing.T) {
	d := openTestDB(t)
	res, err := d.SearchRecipes(db.RecipeFilter{Tradeskill: -1, Query: "Mesmerization", Limit: 20})
	if err != nil {
		t.Fatalf("search recipes: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected a Mesmerization recipe, got none")
	}
}

func TestGetRecipeTradeskills(t *testing.T) {
	d := openTestDB(t)
	skills, err := d.GetRecipeTradeskills()
	if err != nil {
		t.Fatalf("get recipe tradeskills: %v", err)
	}
	if len(skills) == 0 {
		t.Fatal("expected at least one tradeskill with recipes")
	}
	hasResearch := false
	for _, s := range skills {
		if s.Tradeskill == 58 {
			hasResearch = true
			if s.Count == 0 {
				t.Errorf("Research tradeskill reported 0 recipes")
			}
		}
	}
	if !hasResearch {
		t.Errorf("expected Research (58) among tradeskills with recipes")
	}
}

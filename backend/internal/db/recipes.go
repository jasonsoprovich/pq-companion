package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
)

// RecipeSummary is a slim tradeskill recipe record for list/search views. The
// product item's id and icon are resolved so the list can render an icon and
// deep-link to the produced item without a follow-up request. A recipe with no
// product row (rare) reports ProductItemID 0.
type RecipeSummary struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Tradeskill    int    `json:"tradeskill"`
	Trivial       int    `json:"trivial"`
	ProductItemID int    `json:"product_item_id"`
	ProductIcon   int    `json:"product_icon"`
}

// RecipeEntry is one line of a recipe: a container, a consumed component, or a
// produced product. Role is derived from the underlying count flags. VendorPrice
// is set (base copper cost) only when the item is sold by at least one merchant,
// so the UI can show best-effort component cost and leave it blank otherwise.
// Craftable is true when the item is itself produced by at least one enabled
// recipe, which lets the UI offer an ingredient drill-down affordance.
type RecipeEntry struct {
	ItemID      int    `json:"item_id"`
	ItemName    string `json:"item_name"`
	Icon        int    `json:"icon"`
	Role        string `json:"role"` // "container", "component", or "product"
	Count       int    `json:"count"`
	VendorPrice *int   `json:"vendor_price,omitempty"`
	Craftable   bool   `json:"craftable,omitempty"`
	// Station marks a container entry that is a combine-station type (a bagtype
	// code such as Forge or Enchanters Lexicon) rather than a specific
	// inventory item. ItemID holds the bagtype code, and there is no icon or
	// item-detail page to link to. See enums.ContainerTypeName.
	Station bool `json:"station,omitempty"`
}

// RecipeDetail is a full recipe: header metadata plus its entries grouped by
// role. Containers are the combine vessels (any one suffices — they're an OR
// set, not all required); Components are consumed; Products are yielded.
type RecipeDetail struct {
	RecipeSummary
	SkillNeeded      int           `json:"skill_needed"`
	NoFail           bool          `json:"no_fail"`
	Quest            bool          `json:"quest"`
	ReplaceContainer bool          `json:"replace_container"`
	Containers       []RecipeEntry `json:"containers"`
	Components       []RecipeEntry `json:"components"`
	Products         []RecipeEntry `json:"products"`

	// RaceRestrict is the EQ race id required to craft this recipe when its only
	// combine container is a race-locked cultural kit (0 = any race). Derived from
	// the curated table in cultural.go, since quarm.db carries no race data. A
	// recipe with any unrestricted container option stays 0 (that vessel works for
	// everyone). See containersRaceRestrict.
	RaceRestrict int `json:"race_restrict,omitempty"`
}

// RecipeTradeskillCount reports how many enabled recipes exist for a tradeskill
// discipline. Drives the recipe browser's discipline filter so it only lists
// tradeskills that actually have recipes (not every skill in the enum).
type RecipeTradeskillCount struct {
	Tradeskill int `json:"tradeskill"`
	Count      int `json:"count"`
}

// RecipeFilter holds filter parameters for SearchRecipes. Tradeskill -1 means
// "any" (0 is a real value — "Common Combine"). TrivialMin/Max of 0 mean no
// bound on that side.
type RecipeFilter struct {
	Query      string
	Tradeskill int
	TrivialMin int
	TrivialMax int
	Limit      int
	Offset     int
}

// productSubquery resolves the canonical product (first success row) for a
// recipe. Shared by the SELECT list so the icon and item id come from the same
// row. col is the column to project from that row.
func productSubquery(col string) string {
	return fmt.Sprintf(`(SELECT %s
		FROM tradeskill_recipe_entries tre
		LEFT JOIN items pi ON pi.id = tre.item_id
		WHERE tre.recipe_id = r.id AND tre.successcount > 0
		ORDER BY tre.id LIMIT 1)`, col)
}

// SearchRecipes returns enabled tradeskill recipes matching the filter,
// paginated and ordered by trivial then name. Mirrors SearchItems' shape.
func (db *DB) SearchRecipes(f RecipeFilter) (*SearchResult[RecipeSummary], error) {
	pattern := "%" + escapeLike(f.Query) + "%"
	where := "r.enabled = 1 AND r.name LIKE ? ESCAPE '\\'"
	args := []any{pattern}

	if f.Tradeskill >= 0 {
		where += " AND r.tradeskill = ?"
		args = append(args, f.Tradeskill)
	}
	if f.TrivialMin > 0 {
		where += " AND r.trivial >= ?"
		args = append(args, f.TrivialMin)
	}
	if f.TrivialMax > 0 {
		where += " AND r.trivial <= ?"
		args = append(args, f.TrivialMax)
	}

	var total int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM tradeskill_recipe r WHERE "+where,
		args...,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count recipes: %w", err)
	}

	q := fmt.Sprintf(`
		SELECT r.id, r.name, r.tradeskill, r.trivial,
		       COALESCE(%s, 0) AS product_item_id,
		       COALESCE(%s, 0) AS product_icon
		FROM tradeskill_recipe r
		WHERE %s
		ORDER BY r.trivial, r.name, r.id
		LIMIT ? OFFSET ?`,
		productSubquery("tre.item_id"), productSubquery("pi.icon"), where)
	rows, err := db.Query(q, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, fmt.Errorf("search recipes: %w", err)
	}
	defer rows.Close()

	out := []RecipeSummary{}
	for rows.Next() {
		var s RecipeSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Tradeskill, &s.Trivial, &s.ProductItemID, &s.ProductIcon); err != nil {
			return nil, fmt.Errorf("scan recipe summary: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &SearchResult[RecipeSummary]{Items: out, Total: total}, nil
}

// GetRecipe returns the full recipe (header + entries grouped by role) for the
// given recipe id. Entries are LEFT JOINed to items so world-container codes
// that have no items row (e.g. the static "combine container" id 27) still
// appear, labelled generically. Returns sql.ErrNoRows when the recipe id or its
// disabled.
func (db *DB) GetRecipe(id int) (*RecipeDetail, error) {
	var (
		d                               RecipeDetail
		noFail, quest, replaceContainer int
	)
	err := db.QueryRow(`
		SELECT id, name, tradeskill, trivial, skillneeded, nofail, quest, replace_container
		FROM tradeskill_recipe
		WHERE id = ? AND enabled = 1`, id).
		Scan(&d.ID, &d.Name, &d.Tradeskill, &d.Trivial, &d.SkillNeeded,
			&noFail, &quest, &replaceContainer)
	if err != nil {
		return nil, err
	}
	d.NoFail = noFail != 0
	d.Quest = quest != 0
	d.ReplaceContainer = replaceContainer != 0

	rows, err := db.Query(`
		SELECT tre.item_id,
		       COALESCE(i.name, ''),
		       COALESCE(i.icon, 0),
		       tre.successcount, tre.componentcount, tre.iscontainer,
		       CASE WHEN i.id IS NOT NULL
		                 AND EXISTS (SELECT 1 FROM merchantlist m WHERE m.item = i.id)
		            THEN i.price ELSE NULL END AS vendor_price,
		       EXISTS (SELECT 1
		               FROM tradeskill_recipe_entries pe
		               JOIN tradeskill_recipe pr ON pr.id = pe.recipe_id
		               WHERE pe.item_id = tre.item_id
		                 AND pe.successcount > 0 AND pr.enabled = 1) AS craftable
		FROM tradeskill_recipe_entries tre
		LEFT JOIN items i ON i.id = tre.item_id
		WHERE tre.recipe_id = ?
		ORDER BY tre.id`, id)
	if err != nil {
		return nil, fmt.Errorf("get recipe entries %d: %w", id, err)
	}
	defer rows.Close()

	d.Containers = []RecipeEntry{}
	d.Components = []RecipeEntry{}
	d.Products = []RecipeEntry{}
	for rows.Next() {
		var (
			e                                   RecipeEntry
			successCount, componentCount, isCon int
			vendorPrice                         sql.NullInt64
			craftable                           int
		)
		if err := rows.Scan(&e.ItemID, &e.ItemName, &e.Icon,
			&successCount, &componentCount, &isCon, &vendorPrice, &craftable); err != nil {
			return nil, fmt.Errorf("scan recipe entry: %w", err)
		}
		if vendorPrice.Valid {
			p := int(vendorPrice.Int64)
			e.VendorPrice = &p
		}
		e.Craftable = craftable != 0
		switch {
		case isCon != 0:
			e.Role = "container"
			// A container row with no items row is a bagtype / combine-station
			// code, not an item — resolve it to its station name.
			if e.ItemName == "" {
				e.ItemName = enums.ContainerTypeName(e.ItemID)
				e.Station = true
			}
			d.Containers = append(d.Containers, e)
		case successCount > 0:
			e.Role = "product"
			e.Count = successCount
			if e.ItemName == "" {
				e.ItemName = fmt.Sprintf("Item #%d", e.ItemID)
			}
			d.Products = append(d.Products, e)
			if d.ProductItemID == 0 {
				d.ProductItemID = e.ItemID
				d.ProductIcon = e.Icon
			}
		case componentCount > 0:
			e.Role = "component"
			e.Count = componentCount
			if e.ItemName == "" {
				e.ItemName = fmt.Sprintf("Item #%d", e.ItemID)
			}
			d.Components = append(d.Components, e)
		}
	}
	d.RaceRestrict = containersRaceRestrict(d.Containers)
	return &d, rows.Err()
}

// GetRecipeSummariesByIDs returns recipe summaries for the given ids. Disabled
// or missing recipes are silently skipped. Result order is not guaranteed —
// callers that need a specific order (e.g. favorites) should reorder by id.
func (db *DB) GetRecipeSummariesByIDs(ids []int) ([]RecipeSummary, error) {
	if len(ids) == 0 {
		return []RecipeSummary{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT r.id, r.name, r.tradeskill, r.trivial,
		       COALESCE(%s, 0) AS product_item_id,
		       COALESCE(%s, 0) AS product_icon
		FROM tradeskill_recipe r
		WHERE r.enabled = 1 AND r.id IN (%s)`,
		productSubquery("tre.item_id"), productSubquery("pi.icon"), placeholders)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get recipe summaries: %w", err)
	}
	defer rows.Close()
	out := []RecipeSummary{}
	for rows.Next() {
		var s RecipeSummary
		if err := rows.Scan(&s.ID, &s.Name, &s.Tradeskill, &s.Trivial, &s.ProductItemID, &s.ProductIcon); err != nil {
			return nil, fmt.Errorf("scan recipe summary: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// LevelingRecipe is a recipe reduced to what the leveling planner
// (internal/tsplan) needs: the skill gate (Trivial), a per-combine vendor cost,
// and the sub-combine edges. It is the DB-side shape; the API maps it to
// tsplan.RecipeCandidate, keeping the db package independent of the solver
// (mirrors how SpellVendorOption feeds shoproute).
//
// VendorCost is the copper to obtain one combine's worth of COMPONENTS the
// cheapest way — buying each, or sub-crafting it from vendor materials when it
// isn't sold (containers are durable vessels and excluded). VendorCostKnown is
// false when some component is neither vendor-sold nor craftable from vendor
// materials (i.e. must be farmed/dropped), which is what lets the planner treat
// the recipe as farm-only.
type LevelingRecipe struct {
	RecipeID  int    `json:"recipe_id"`
	Name      string `json:"name"`
	Trivial   int    `json:"trivial"`
	NoFail    bool   `json:"no_fail"`
	Yield     int    `json:"yield"`               // successcount of the primary product
	Container string `json:"container,omitempty"` // primary combine vessel label (a stage note)

	VendorCost      int  `json:"vendor_cost"` // base copper for one combine's components
	VendorCostKnown bool `json:"vendor_cost_known"`

	// SubCombineRecipeIDs are the recipes that produce this recipe's crafted
	// components (DAG edges).
	SubCombineRecipeIDs []int `json:"sub_combine_recipe_ids,omitempty"`

	// RequiresCrossTradeskill is true when at least one component can ONLY be
	// obtained by crafting in a different skill-gated discipline (recursively) —
	// i.e. leveling on this recipe forces another tradeskill. Lets the planner
	// offer a "stay in this discipline" mode.
	RequiresCrossTradeskill bool `json:"requires_cross_tradeskill,omitempty"`

	// RaceRestrict is the EQ race id required to use this recipe's combine
	// container (a cultural newbie-armor kit), or 0 if any race can make it.
	// quarm.db carries no race data on containers, so this comes from a small
	// curated table — see cultural.go. The planner drops recipes a character's
	// race can't make.
	RaceRestrict int `json:"race_restrict,omitempty"`
}

// RecipesForTradeskill returns every enabled, non-quest recipe for one
// tradeskill discipline, reduced to the leveling planner's inputs. Quest recipes
// are excluded because they are one-off combines (unique components, often
// script-handled), not the repeatable grind recipes a leveling path is built
// from. Recipes are ordered by trivial then name so a caller can see the
// discipline's natural progression.
func (db *DB) RecipesForTradeskill(tradeskill int) ([]LevelingRecipe, error) {
	rows, err := db.Query(`
		SELECT id, name, trivial, nofail
		FROM tradeskill_recipe
		WHERE tradeskill = ? AND enabled = 1 AND quest = 0
		ORDER BY trivial, name, id`, tradeskill)
	if err != nil {
		return nil, fmt.Errorf("leveling recipe headers %d: %w", tradeskill, err)
	}
	defer rows.Close()

	out := []LevelingRecipe{}
	for rows.Next() {
		var (
			lr     LevelingRecipe
			nofail int
		)
		if err := rows.Scan(&lr.RecipeID, &lr.Name, &lr.Trivial, &nofail); err != nil {
			return nil, fmt.Errorf("scan leveling recipe: %w", err)
		}
		lr.NoFail = nofail != 0
		lr.VendorCostKnown = true // holds until a component proves otherwise
		out = append(out, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}

	// Index recipe id -> slice position (out is now fixed-size, so &out[i] is
	// stable) and a per-recipe set to dedupe sub-combine edges.
	idx := make(map[int]int, len(out))
	for i := range out {
		idx[out[i].RecipeID] = i
	}
	subSeen := make(map[int]map[int]bool, len(out))
	// A recipe with no consumed components can't be "ground" for skill-ups (it's
	// a transform/recharge/degenerate row, e.g. a no-cost trivial-255 combine),
	// and would look free to the cost optimizer. Drop such recipes at the end.
	hasComponent := make(map[int]bool, len(out))

	// The cost resolver holds the whole recipe DAG in memory so we can price a
	// recipe by the CHEAPEST way to obtain each component (buy it, or sub-craft
	// it from vendor materials) and tell a genuine "must craft it" sub-combine
	// from a component you'd just buy.
	resolver, err := db.newCostResolver()
	if err != nil {
		return nil, err
	}

	erows, err := db.Query(`
		SELECT tre.recipe_id, tre.item_id, COALESCE(i.name, ''),
		       tre.successcount, tre.componentcount, tre.iscontainer
		FROM tradeskill_recipe_entries tre
		JOIN tradeskill_recipe r ON r.id = tre.recipe_id
		LEFT JOIN items i ON i.id = tre.item_id
		WHERE r.tradeskill = ? AND r.enabled = 1 AND r.quest = 0
		ORDER BY tre.recipe_id, tre.id`, tradeskill)
	if err != nil {
		return nil, fmt.Errorf("leveling recipe entries %d: %w", tradeskill, err)
	}
	defer erows.Close()

	for erows.Next() {
		var (
			recipeID, itemID                    int
			name                                string
			successCount, componentCount, isCon int
		)
		if err := erows.Scan(&recipeID, &itemID, &name,
			&successCount, &componentCount, &isCon); err != nil {
			return nil, fmt.Errorf("scan leveling entry: %w", err)
		}
		i, ok := idx[recipeID]
		if !ok {
			continue
		}
		lr := &out[i]

		switch {
		case isCon != 0:
			// First container is the label; a container with no items row is a
			// bagtype/combine-station code (e.g. Forge), resolved to its name.
			if lr.Container == "" {
				if name == "" {
					lr.Container = enums.ContainerTypeName(itemID)
				} else {
					lr.Container = name
				}
				// Cultural kits (Vale/Fier`Dal/Erudite) are race-locked; quarm.db
				// has no race data on them, so tag from the curated table.
				lr.RaceRestrict = ContainerRaceRestrict(lr.Container)
			}
		case successCount > 0:
			if lr.Yield == 0 {
				lr.Yield = successCount // primary product's per-combine output
			}
		case componentCount > 0:
			hasComponent[recipeID] = true
			// Cheapest acquisition: vendor price, or sub-craft from vendor mats.
			if ic := resolver.cost(itemID); ic.known {
				lr.VendorCost += componentCount * ic.copper
			} else {
				lr.VendorCostKnown = false
			}
			// A component obtainable only by crafting in another skill-gated
			// discipline makes this recipe a cross-tradeskill dependency.
			if !resolver.obtainableWithin(itemID, tradeskill) {
				lr.RequiresCrossTradeskill = true
			}
			// Only a component you must CRAFT (not vendor-sold but produced by
			// another recipe) is a real sub-combine dependency; one you can buy is
			// not, even if some recipe also makes it.
			if pid, ok := resolver.craftableSubcombine(itemID, recipeID, tradeskill); ok {
				seen := subSeen[recipeID]
				if seen == nil {
					seen = map[int]bool{}
					subSeen[recipeID] = seen
				}
				if !seen[pid] {
					seen[pid] = true
					lr.SubCombineRecipeIDs = append(lr.SubCombineRecipeIDs, pid)
				}
			}
		}
	}
	if err := erows.Err(); err != nil {
		return nil, err
	}

	result := make([]LevelingRecipe, 0, len(out))
	for i := range out {
		if hasComponent[out[i].RecipeID] {
			result = append(result, out[i])
		}
	}
	return result, nil
}

// GetRecipeTradeskills returns the distinct tradeskill disciplines that have at
// least one enabled recipe, with their recipe counts, ordered by id.
func (db *DB) GetRecipeTradeskills() ([]RecipeTradeskillCount, error) {
	rows, err := db.Query(`
		SELECT tradeskill, COUNT(*)
		FROM tradeskill_recipe
		WHERE enabled = 1
		GROUP BY tradeskill
		ORDER BY tradeskill`)
	if err != nil {
		return nil, fmt.Errorf("get recipe tradeskills: %w", err)
	}
	defer rows.Close()
	out := []RecipeTradeskillCount{}
	for rows.Next() {
		var c RecipeTradeskillCount
		if err := rows.Scan(&c.Tradeskill, &c.Count); err != nil {
			return nil, fmt.Errorf("scan tradeskill count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

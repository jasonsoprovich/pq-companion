package db

import (
	"database/sql"
	"fmt"
	"strings"
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
	pattern := "%" + strings.ReplaceAll(f.Query, "%", "\\%") + "%"
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
			if e.ItemName == "" {
				e.ItemName = "(combine container)"
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

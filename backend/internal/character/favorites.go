package character

import (
	"time"
)

// favorite_recipes is a global (not per-character) list of tradeskill recipes
// the user has starred. Recipes aren't bound to a single alt the way a gear
// wishlist is — "I want to craft this" is account-wide knowledge — so the table
// is deliberately unscoped. A future per-character variant can be added as an
// additive character_id column without disturbing existing rows.
func (s *Store) migrateFavoriteRecipes() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS favorite_recipes (
			recipe_id  INTEGER PRIMARY KEY,
			created_at INTEGER NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_favorite_recipes_sort
		 ON favorite_recipes(sort_order)`,
	); err != nil {
		return err
	}
	return nil
}

// ListFavoriteRecipes returns the starred recipe ids in display order
// (sort_order, then recipe_id for stability).
func (s *Store) ListFavoriteRecipes() ([]int, error) {
	rows, err := s.db.Query(
		`SELECT recipe_id FROM favorite_recipes ORDER BY sort_order, recipe_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddFavoriteRecipe stars a recipe, appending it at the bottom of the order.
// Idempotent: re-starring an already-favorited recipe is a no-op.
func (s *Store) AddFavoriteRecipe(recipeID int) error {
	var maxPos int
	if err := s.db.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) FROM favorite_recipes`,
	).Scan(&maxPos); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO favorite_recipes (recipe_id, created_at, sort_order)
		 VALUES (?, ?, ?)`,
		recipeID, time.Now().Unix(), maxPos+1,
	)
	return err
}

// DeleteFavoriteRecipe unstars a recipe. No-op if it wasn't favorited.
func (s *Store) DeleteFavoriteRecipe(recipeID int) error {
	_, err := s.db.Exec(`DELETE FROM favorite_recipes WHERE recipe_id = ?`, recipeID)
	return err
}

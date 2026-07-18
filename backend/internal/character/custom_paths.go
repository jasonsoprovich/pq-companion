package character

import (
	"time"
)

// custom_leveling_recipes is a global (not per-character) set of recipes the
// user has added to their build-your-own tradeskill leveling path, one row
// per (tradeskill, recipe) pair. Global for the same reason as
// favorite_recipes: "here's how I level Tailoring" reflects the user's own
// play style and farming spots, not any one alt. Display order isn't stored —
// the leveling planner sorts a custom path by trivial at build time — so
// there's no sort_order column here.
func (s *Store) migrateCustomLevelingRecipes() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS custom_leveling_recipes (
			tradeskill INTEGER NOT NULL,
			recipe_id  INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (tradeskill, recipe_id)
		)
	`)
	return err
}

// ListCustomLevelingRecipes returns the recipe ids a user added to their
// custom path for one tradeskill, in no particular order — callers sort by
// whatever's meaningful to them (the planner sorts by trivial).
func (s *Store) ListCustomLevelingRecipes(tradeskill int) ([]int, error) {
	rows, err := s.db.Query(
		`SELECT recipe_id FROM custom_leveling_recipes WHERE tradeskill = ?`, tradeskill,
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

// AddCustomLevelingRecipe adds a recipe to a tradeskill's custom path.
// Idempotent: adding an already-present recipe is a no-op.
func (s *Store) AddCustomLevelingRecipe(tradeskill, recipeID int) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO custom_leveling_recipes (tradeskill, recipe_id, created_at)
		 VALUES (?, ?, ?)`,
		tradeskill, recipeID, time.Now().Unix(),
	)
	return err
}

// DeleteCustomLevelingRecipe removes a recipe from a tradeskill's custom
// path. No-op if it wasn't present.
func (s *Store) DeleteCustomLevelingRecipe(tradeskill, recipeID int) error {
	_, err := s.db.Exec(
		`DELETE FROM custom_leveling_recipes WHERE tradeskill = ? AND recipe_id = ?`,
		tradeskill, recipeID,
	)
	return err
}

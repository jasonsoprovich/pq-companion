package trigger

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// reservedUncategorized is the sentinel pack_name bucket the UI uses for
// user-authored triggers with an empty pack_name. It can never be created,
// renamed, or deleted as a real category.
const reservedUncategorized = "__uncategorized__"

// Category sentinel errors, mapped to HTTP status codes by the API layer.
var (
	ErrCategoryNameEmpty = errors.New("category name is required")
	ErrCategoryReserved  = errors.New("category name is reserved")
	ErrCategoryExists    = errors.New("category already exists")
	ErrCategoryBuiltin   = errors.New("built-in packs are managed from the Packs tab")
	ErrCategoryNotFound  = errors.New("category not found")
)

// Category is a trigger grouping surfaced to the UI. The category key is the
// pack_name column on triggers. Custom categories are persisted in the
// trigger_categories table so an empty, freshly-created group survives a
// restart; built-in (class) and imported packs appear here too — derived from
// the pack_name values currently in use — but are flagged IsBuiltin and stay
// read-only (managed from the Packs tab).
type Category struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`      // triggers currently in this category
	IsBuiltin bool   `json:"is_builtin"` // true = managed via the Packs tab, not editable here
	Custom    bool   `json:"custom"`     // true = has a row in trigger_categories
}

// builtinPackNames returns the set of pack names shipped as built-in packs.
// Used to keep category management custom-only: built-in pack names are
// reserved (can't be created) and protected (can't be renamed/deleted here).
func builtinPackNames() map[string]bool {
	out := make(map[string]bool)
	for _, p := range AllPacks() {
		out[p.PackName] = true
	}
	return out
}

// normalizeCategoryName trims surrounding whitespace and rejects the empty and
// reserved-sentinel names.
func normalizeCategoryName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", ErrCategoryNameEmpty
	}
	if n == reservedUncategorized {
		return "", ErrCategoryReserved
	}
	return n, nil
}

// categoryCounts returns the number of triggers per non-empty pack_name.
func (s *Store) categoryCounts() (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT pack_name, COUNT(*) FROM triggers WHERE pack_name <> '' GROUP BY pack_name`)
	if err != nil {
		return nil, fmt.Errorf("category counts: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

// categoryRows returns the set of category names persisted in the
// trigger_categories table (including ones with no triggers yet).
func (s *Store) categoryRows() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name FROM trigger_categories`)
	if err != nil {
		return nil, fmt.Errorf("category rows: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// categoryExists reports whether the name names a real category — either a
// persisted custom row or a pack_name currently carried by ≥1 trigger.
func (s *Store) categoryExists(name string) (bool, error) {
	rows, err := s.categoryRows()
	if err != nil {
		return false, err
	}
	if rows[name] {
		return true, nil
	}
	counts, err := s.categoryCounts()
	if err != nil {
		return false, err
	}
	return counts[name] > 0, nil
}

// categoryNameTaken reports whether the name is unavailable for a new category:
// already a custom row, already in use by triggers, or reserved by a built-in
// pack (even one not currently installed).
func (s *Store) categoryNameTaken(name string) (bool, error) {
	if builtinPackNames()[name] {
		return true, nil
	}
	return s.categoryExists(name)
}

// ListCategories returns every category surfaced to the UI: persisted custom
// categories plus any pack_name values currently in use, each with its trigger
// count and built-in flag. The reserved Uncategorized bucket is not included —
// the frontend renders it separately. Sorted by name.
func (s *Store) ListCategories() ([]Category, error) {
	counts, err := s.categoryCounts()
	if err != nil {
		return nil, err
	}
	rows, err := s.categoryRows()
	if err != nil {
		return nil, err
	}
	builtin := builtinPackNames()

	names := make(map[string]bool)
	for n := range counts {
		names[n] = true
	}
	for n := range rows {
		names[n] = true
	}

	out := make([]Category, 0, len(names))
	for n := range names {
		out = append(out, Category{
			Name:      n,
			Count:     counts[n],
			IsBuiltin: builtin[n],
			Custom:    rows[n],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// CreateCategory persists a new, empty custom category. Rejects empty/reserved
// names and any name already taken by a category, an in-use pack, or a
// built-in pack.
func (s *Store) CreateCategory(name string) (Category, error) {
	norm, err := normalizeCategoryName(name)
	if err != nil {
		return Category{}, err
	}
	taken, err := s.categoryNameTaken(norm)
	if err != nil {
		return Category{}, err
	}
	if taken {
		return Category{}, ErrCategoryExists
	}
	if _, err := s.db.Exec(
		`INSERT INTO trigger_categories (name, created_at) VALUES (?, ?)`,
		norm, time.Now().UTC().Unix(),
	); err != nil {
		return Category{}, fmt.Errorf("create category %s: %w", norm, err)
	}
	return Category{Name: norm, Count: 0, IsBuiltin: false, Custom: true}, nil
}

// RenameCategory renames a custom category, cascading the new name to every
// trigger that carried the old one. Built-in packs cannot be renamed here.
func (s *Store) RenameCategory(oldName, newName string) error {
	oldNorm, err := normalizeCategoryName(oldName)
	if err != nil {
		return err
	}
	newNorm, err := normalizeCategoryName(newName)
	if err != nil {
		return err
	}
	if builtinPackNames()[oldNorm] {
		return ErrCategoryBuiltin
	}
	exists, err := s.categoryExists(oldNorm)
	if err != nil {
		return err
	}
	if !exists {
		return ErrCategoryNotFound
	}
	if newNorm == oldNorm {
		return nil
	}
	taken, err := s.categoryNameTaken(newNorm)
	if err != nil {
		return err
	}
	if taken {
		return ErrCategoryExists
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin rename category: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE triggers SET pack_name=? WHERE pack_name=?`, newNorm, oldNorm); err != nil {
		return fmt.Errorf("rename category triggers: %w", err)
	}
	// No-ops for imported packs that never had a persisted row; the renamed
	// triggers above keep the new name in use either way.
	if _, err := tx.Exec(`UPDATE trigger_categories SET name=? WHERE name=?`, newNorm, oldNorm); err != nil {
		return fmt.Errorf("rename category row: %w", err)
	}
	return tx.Commit()
}

// DeleteCategory removes a custom category, moving its triggers to
// Uncategorized (empty pack_name) rather than deleting them — distinct from
// uninstalling a pack. Built-in packs cannot be deleted here.
func (s *Store) DeleteCategory(name string) error {
	norm, err := normalizeCategoryName(name)
	if err != nil {
		return err
	}
	if builtinPackNames()[norm] {
		return ErrCategoryBuiltin
	}
	exists, err := s.categoryExists(norm)
	if err != nil {
		return err
	}
	if !exists {
		return ErrCategoryNotFound
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete category: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE triggers SET pack_name='' WHERE pack_name=?`, norm); err != nil {
		return fmt.Errorf("orphan category triggers: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM trigger_categories WHERE name=?`, norm); err != nil {
		return fmt.Errorf("delete category row: %w", err)
	}
	return tx.Commit()
}

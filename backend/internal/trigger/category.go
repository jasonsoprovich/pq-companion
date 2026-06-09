package trigger

import (
	"database/sql"
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
// pack_name column on triggers.
//
// Custom categories (Explicit) are user-created, persisted in the
// trigger_categories table, and stay visible even when empty so they can serve
// as drag-and-drop targets. Built-in (class) and imported packs appear here too
// — derived from the pack_name values currently in use — but are flagged
// IsBuiltin and stay read-only (managed from the Packs tab). Pack categories
// vanish from the list when they hold no triggers.
type Category struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`      // triggers currently in this category
	IsBuiltin bool   `json:"is_builtin"` // true = managed via the Packs tab, not editable here
	Custom    bool   `json:"custom"`     // true = user-created (explicit, non-builtin)
	Explicit  bool   `json:"explicit"`   // true = has a persisted row (always visible)
	SortOrder int    `json:"sort_order"` // display order; lower sorts first
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

// categoryRow mirrors a trigger_categories row.
type categoryRow struct {
	Explicit  bool
	SortOrder int
}

// categoryRows returns the persisted trigger_categories rows keyed by name.
func (s *Store) categoryRows() (map[string]categoryRow, error) {
	rows, err := s.db.Query(`SELECT name, explicit, sort_order FROM trigger_categories`)
	if err != nil {
		return nil, fmt.Errorf("category rows: %w", err)
	}
	defer rows.Close()
	out := make(map[string]categoryRow)
	for rows.Next() {
		var name string
		var explicitInt, sortOrder int
		if err := rows.Scan(&name, &explicitInt, &sortOrder); err != nil {
			return nil, err
		}
		out[name] = categoryRow{Explicit: explicitInt != 0, SortOrder: sortOrder}
	}
	return out, rows.Err()
}

// categoryExists reports whether the name names a real category — either a
// persisted row or a pack_name currently carried by ≥1 trigger.
func (s *Store) categoryExists(name string) (bool, error) {
	rows, err := s.categoryRows()
	if err != nil {
		return false, err
	}
	if _, ok := rows[name]; ok {
		return true, nil
	}
	counts, err := s.categoryCounts()
	if err != nil {
		return false, err
	}
	return counts[name] > 0, nil
}

// categoryNameTaken reports whether the name is unavailable for a new category:
// already a persisted row, already in use by triggers, or reserved by a
// built-in pack (even one not currently installed).
func (s *Store) categoryNameTaken(name string) (bool, error) {
	if builtinPackNames()[name] {
		return true, nil
	}
	return s.categoryExists(name)
}

// ListCategories returns every category surfaced to the UI: persisted custom
// categories (always, even when empty) plus any pack_name values currently in
// use, each with its trigger count, built-in flag, and display order. The
// reserved Uncategorized bucket is not included — the frontend renders it
// separately. Sorted by SortOrder then name; categories without an explicit
// order fall to the end alphabetically.
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

	// Categories without a persisted order sort after ordered ones.
	const unordered = 1 << 30

	out := make([]Category, 0, len(names))
	for n := range names {
		row, hasRow := rows[n]
		count := counts[n]
		explicit := hasRow && row.Explicit
		// A pack-derived placeholder row (explicit=0) with no triggers — e.g.
		// a pack that was reordered then uninstalled — is not shown.
		if !explicit && count == 0 {
			continue
		}
		order := unordered
		if hasRow {
			order = row.SortOrder
		}
		out = append(out, Category{
			Name:      n,
			Count:     count,
			IsBuiltin: builtin[n],
			Custom:    explicit && !builtin[n],
			Explicit:  explicit,
			SortOrder: order,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// nextCategorySortOrder returns one past the highest persisted category order.
func (s *Store) nextCategorySortOrder() (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(sort_order) FROM trigger_categories`).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("next category sort order: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
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
	order, err := s.nextCategorySortOrder()
	if err != nil {
		return Category{}, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO trigger_categories (name, created_at, explicit, sort_order) VALUES (?, ?, 1, ?)`,
		norm, time.Now().UTC().Unix(), order,
	); err != nil {
		return Category{}, fmt.Errorf("create category %s: %w", norm, err)
	}
	return Category{Name: norm, Count: 0, IsBuiltin: false, Custom: true, Explicit: true, SortOrder: order}, nil
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

// DeleteCategory removes a custom category. When deleteTriggers is true its
// triggers are deleted outright; otherwise they move to Uncategorized (empty
// pack_name). Built-in packs cannot be deleted here.
func (s *Store) DeleteCategory(name string, deleteTriggers bool) error {
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
	if deleteTriggers {
		if _, err := tx.Exec(`DELETE FROM triggers WHERE pack_name=?`, norm); err != nil {
			return fmt.Errorf("delete category triggers: %w", err)
		}
	} else {
		if _, err := tx.Exec(`UPDATE triggers SET pack_name='' WHERE pack_name=?`, norm); err != nil {
			return fmt.Errorf("orphan category triggers: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM trigger_categories WHERE name=?`, norm); err != nil {
		return fmt.Errorf("delete category row: %w", err)
	}
	return tx.Commit()
}

// ReorderCategories rewrites category display order to match the position of
// each name in order (0-based). Custom rows keep their explicit flag; pack
// categories included in the list get a lazily-materialized placeholder row
// (explicit=0) so their order persists. Reserved/blank names are skipped;
// built-in names are allowed (they're reorderable, just not renamable/
// deletable).
func (s *Store) ReorderCategories(order []string) error {
	now := time.Now().UTC().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder categories: %w", err)
	}
	defer tx.Rollback()
	for i, name := range order {
		norm := strings.TrimSpace(name)
		if norm == "" || norm == reservedUncategorized {
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO trigger_categories (name, created_at, explicit, sort_order)
			 VALUES (?, ?, 0, ?)
			 ON CONFLICT(name) DO UPDATE SET sort_order = excluded.sort_order`,
			norm, now, i,
		); err != nil {
			return fmt.Errorf("reorder category %s: %w", norm, err)
		}
	}
	return tx.Commit()
}

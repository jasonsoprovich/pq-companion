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

// maxCategoryDepth is how many levels deep a category may nest: 0 = top
// level, 1 = child of a top-level category. A child cannot itself have
// children. Raising this later is a one-line change — nothing in the schema
// or storage enforces a shallower tree than this constant does.
const maxCategoryDepth = 1

// Category sentinel errors, mapped to HTTP status codes by the API layer.
var (
	ErrCategoryNameEmpty = errors.New("category name is required")
	ErrCategoryReserved  = errors.New("category name is reserved")
	ErrCategoryExists    = errors.New("category already exists")
	ErrCategoryBuiltin   = errors.New("built-in packs are managed from the Packs tab")
	ErrCategoryNotFound  = errors.New("category not found")
	ErrCategoryDepth     = errors.New("categories can only be nested one level deep")
	ErrCategoryCycle     = errors.New("a category can't be moved under its own subcategory")
	ErrCategoryNoSplit   = errors.New(`category name has no "/" to split on`)
)

// Category is a trigger grouping surfaced to the UI, backed by an id-keyed
// trigger_categories row. ParentID is empty for a top-level category; a
// category with a non-empty ParentID is always a leaf (see maxCategoryDepth).
//
// Custom categories (Explicit) are user-created, persisted, and stay visible
// even when empty so they can serve as drag-and-drop targets. Built-in (class)
// and imported packs appear here too — every in-use pack_name is materialized
// into a row (see materializePackCategoryRows) — but are flagged IsBuiltin and
// stay read-only (managed from the Packs tab) or, for imported packs, editable
// like any custom category. Pack categories vanish from the list when they
// (and, for a parent, its children) hold no triggers.
type Category struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id"`
	Count     int    `json:"count"`      // triggers in this category, plus (for a top-level category) its children's
	IsBuiltin bool   `json:"is_builtin"` // true = managed via the Packs tab, not editable here
	Custom    bool   `json:"custom"`     // true = user-created (explicit, non-builtin)
	Explicit  bool   `json:"explicit"`   // true = has a persisted row (always visible)
	SortOrder int    `json:"sort_order"` // display order among siblings; lower sorts first
}

// builtinPackNames returns the set of pack names shipped as built-in packs.
// Used to keep category management custom-only: built-in pack names are
// reserved at the top level (can't be created there) and protected (can't be
// renamed/deleted here) — built-in packs are always top-level, so nothing
// stops a subcategory from reusing one of these names.
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

// categoryRecord mirrors one trigger_categories row.
type categoryRecord struct {
	ID        string
	Name      string
	ParentID  string
	Explicit  bool
	SortOrder int
}

// queryRower is the subset of *sql.DB / *sql.Tx that getCategoryRow needs, so
// the same lookup works both standalone and inside a transaction.
type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

// getCategoryRow fetches one trigger_categories row by id, or ErrCategoryNotFound.
func (s *Store) getCategoryRow(id string) (categoryRecord, error) {
	return getCategoryRowWith(s.db, id)
}

func getCategoryRowWith(q queryRower, id string) (categoryRecord, error) {
	var r categoryRecord
	var explicitInt int
	err := q.QueryRow(
		`SELECT id, name, parent_id, explicit, sort_order FROM trigger_categories WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.ParentID, &explicitInt, &r.SortOrder)
	if err == sql.ErrNoRows {
		return categoryRecord{}, ErrCategoryNotFound
	}
	if err != nil {
		return categoryRecord{}, fmt.Errorf("get category %s: %w", id, err)
	}
	r.Explicit = explicitInt != 0
	return r, nil
}

// categoryCounts returns the number of triggers directly filed under each
// category id (not rolled up to parents — see ListCategories for that).
func (s *Store) categoryCounts() (map[string]int, error) {
	rows, err := s.db.Query(
		`SELECT category_id, COUNT(*) FROM triggers WHERE category_id <> '' GROUP BY category_id`)
	if err != nil {
		return nil, fmt.Errorf("category counts: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// categoryExistsAt reports whether a category named name already exists
// directly under parentID (root when empty).
func (s *Store) categoryExistsAt(parentID, name string) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM trigger_categories WHERE parent_id = ? AND name = ?`, parentID, name,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check category exists: %w", err)
	}
	return n > 0, nil
}

// categoryNameTaken reports whether name is unavailable for a new (or
// renamed) category at parentID: already a sibling row there, or — for a
// top-level category only — reserved by a built-in pack (even one not
// currently installed).
func (s *Store) categoryNameTaken(parentID, name string) (bool, error) {
	if parentID == "" && builtinPackNames()[name] {
		return true, nil
	}
	return s.categoryExistsAt(parentID, name)
}

// ListCategories returns every category surfaced to the UI: persisted custom
// categories (always, even when empty) plus every materialized pack category,
// each with its trigger count (rolled up to include children, for a
// top-level category), built-in flag, and display order. The reserved
// Uncategorized bucket is not included — the frontend renders it separately.
// Sorted by parent, then SortOrder, then name; the frontend assembles the
// flat list into a tree via ParentID.
func (s *Store) ListCategories() ([]Category, error) {
	direct, err := s.categoryCounts()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, name, parent_id, explicit, sort_order FROM trigger_categories`)
	if err != nil {
		return nil, fmt.Errorf("list category rows: %w", err)
	}
	defer rows.Close()
	var all []categoryRecord
	for rows.Next() {
		var r categoryRecord
		var explicitInt int
		if err := rows.Scan(&r.ID, &r.Name, &r.ParentID, &explicitInt, &r.SortOrder); err != nil {
			return nil, err
		}
		r.Explicit = explicitInt != 0
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	total := make(map[string]int, len(all))
	for _, r := range all {
		total[r.ID] += direct[r.ID]
	}
	for _, r := range all {
		if r.ParentID != "" {
			total[r.ParentID] += direct[r.ID]
		}
	}

	builtin := builtinPackNames()
	out := make([]Category, 0, len(all))
	for _, r := range all {
		if !r.Explicit && total[r.ID] == 0 {
			continue
		}
		isBuiltin := r.ParentID == "" && builtin[r.Name]
		out = append(out, Category{
			ID:        r.ID,
			Name:      r.Name,
			ParentID:  r.ParentID,
			Count:     total[r.ID],
			IsBuiltin: isBuiltin,
			Custom:    r.Explicit && !isBuiltin,
			Explicit:  r.Explicit,
			SortOrder: r.SortOrder,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentID != out[j].ParentID {
			return out[i].ParentID < out[j].ParentID
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// nextCategorySortOrder returns one past the highest persisted sort order
// among siblings sharing parentID.
func (s *Store) nextCategorySortOrder(parentID string) (int, error) {
	var max sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(sort_order) FROM trigger_categories WHERE parent_id = ?`, parentID).Scan(&max)
	if err != nil {
		return 0, fmt.Errorf("next category sort order: %w", err)
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// CreateCategory persists a new, empty custom category, optionally as a child
// of parentID (empty for top-level). Rejects empty/reserved names, a name
// already taken by a sibling, a parent that doesn't exist, and a parent that
// is itself a child (depth cap).
func (s *Store) CreateCategory(name, parentID string) (Category, error) {
	norm, err := normalizeCategoryName(name)
	if err != nil {
		return Category{}, err
	}
	parentID = strings.TrimSpace(parentID)
	if parentID != "" {
		parent, err := s.getCategoryRow(parentID)
		if err != nil {
			return Category{}, err
		}
		if parent.ParentID != "" {
			return Category{}, ErrCategoryDepth
		}
	}
	taken, err := s.categoryNameTaken(parentID, norm)
	if err != nil {
		return Category{}, err
	}
	if taken {
		return Category{}, ErrCategoryExists
	}
	id, err := NewID()
	if err != nil {
		return Category{}, fmt.Errorf("generate category id: %w", err)
	}
	order, err := s.nextCategorySortOrder(parentID)
	if err != nil {
		return Category{}, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO trigger_categories (id, name, parent_id, created_at, explicit, sort_order) VALUES (?, ?, ?, ?, 1, ?)`,
		id, norm, parentID, time.Now().UTC().Unix(), order,
	); err != nil {
		return Category{}, fmt.Errorf("create category %s: %w", norm, err)
	}
	return Category{ID: id, Name: norm, ParentID: parentID, Count: 0, IsBuiltin: false, Custom: true, Explicit: true, SortOrder: order}, nil
}

// RenameCategory renames a category in place. Categories are referenced by
// id (triggers.category_id), so renaming never cascades to descendants or
// siblings — only this category's own row, plus a pack_name cache refresh on
// its own directly-filed triggers. Built-in packs cannot be renamed here.
func (s *Store) RenameCategory(id, newName string) error {
	newNorm, err := normalizeCategoryName(newName)
	if err != nil {
		return err
	}
	row, err := s.getCategoryRow(id)
	if err != nil {
		return err
	}
	if builtinPackNames()[row.Name] {
		return ErrCategoryBuiltin
	}
	if newNorm == row.Name {
		return nil
	}
	taken, err := s.categoryNameTaken(row.ParentID, newNorm)
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
	if _, err := tx.Exec(`UPDATE trigger_categories SET name=? WHERE id=?`, newNorm, id); err != nil {
		return fmt.Errorf("rename category row: %w", err)
	}
	// Refresh the pack_name display cache on this category's own triggers —
	// not a cascade, since category_id (not name) is what actually links
	// them; every other trigger in the app is untouched by this rename.
	if _, err := tx.Exec(`UPDATE triggers SET pack_name=? WHERE category_id=?`, newNorm, id); err != nil {
		return fmt.Errorf("refresh category pack_name cache: %w", err)
	}
	return tx.Commit()
}

// DeleteCategory removes a category. When deleteTriggers is true, every
// trigger in the category AND its children is deleted outright; otherwise
// this category's own triggers move to Uncategorized while children's
// triggers are left exactly where they are. Either way, any children are
// promoted to top-level rather than deleted — only the category being
// deleted goes away. Built-in packs cannot be deleted here.
func (s *Store) DeleteCategory(id string, deleteTriggers bool) error {
	row, err := s.getCategoryRow(id)
	if err != nil {
		return err
	}
	if builtinPackNames()[row.Name] {
		return ErrCategoryBuiltin
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin delete category: %w", err)
	}
	defer tx.Rollback()

	childRows, err := tx.Query(`SELECT id FROM trigger_categories WHERE parent_id = ?`, id)
	if err != nil {
		return fmt.Errorf("list child categories: %w", err)
	}
	var childIDs []string
	for childRows.Next() {
		var cid string
		if err := childRows.Scan(&cid); err != nil {
			childRows.Close()
			return err
		}
		childIDs = append(childIDs, cid)
	}
	if err := childRows.Err(); err != nil {
		childRows.Close()
		return err
	}
	childRows.Close()

	if deleteTriggers {
		for _, cid := range append([]string{id}, childIDs...) {
			if _, err := tx.Exec(`DELETE FROM triggers WHERE category_id = ?`, cid); err != nil {
				return fmt.Errorf("delete category triggers: %w", err)
			}
		}
	} else {
		if _, err := tx.Exec(`UPDATE triggers SET category_id='', pack_name='' WHERE category_id=?`, id); err != nil {
			return fmt.Errorf("orphan category triggers: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE trigger_categories SET parent_id='' WHERE parent_id=?`, id); err != nil {
		return fmt.Errorf("promote child categories: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM trigger_categories WHERE id=?`, id); err != nil {
		return fmt.Errorf("delete category row: %w", err)
	}
	return tx.Commit()
}

// CategoryPlacement is one category's position in a reorder/reparent request:
// its new parent (empty for top-level) and its 0-based sort order among the
// siblings sharing that parent.
type CategoryPlacement struct {
	ID        string `json:"id"`
	ParentID  string `json:"parent_id"`
	SortOrder int    `json:"sort_order"`
}

// ReorderCategories applies a full set of position/parent changes in one
// transaction — a plain reorder (ParentID unchanged) and a reparent (drag a
// category onto another) are both just entries in items. Validates the
// resulting shape as a whole before writing anything: no category may end up
// as its own ancestor, and no category may end up more than maxCategoryDepth
// deep. Unknown ids are skipped rather than failing the whole batch.
func (s *Store) ReorderCategories(items []CategoryPlacement) error {
	newParent := make(map[string]string, len(items))
	for _, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		newParent[id] = strings.TrimSpace(it.ParentID)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder categories: %w", err)
	}
	defer tx.Rollback()

	for id, parentID := range newParent {
		if parentID == "" {
			continue
		}
		if parentID == id {
			return ErrCategoryCycle
		}
		if grandParent, inBatch := newParent[parentID]; inBatch {
			if grandParent != "" {
				return ErrCategoryDepth
			}
		} else {
			parentRow, err := getCategoryRowWith(tx, parentID)
			if err != nil {
				return err
			}
			if parentRow.ParentID != "" {
				return ErrCategoryDepth
			}
		}
		// id is becoming a child, so it can't keep children of its own — any
		// existing child not ALSO being relocated out by this same batch would
		// end up two levels deep once id's own parent_id is set below.
		childRows, err := tx.Query(`SELECT id FROM trigger_categories WHERE parent_id = ?`, id)
		if err != nil {
			return fmt.Errorf("check existing children of %s: %w", id, err)
		}
		var stillAChild bool
		for childRows.Next() {
			var childID string
			if err := childRows.Scan(&childID); err != nil {
				childRows.Close()
				return err
			}
			if newParentForChild, movedInBatch := newParent[childID]; !movedInBatch || newParentForChild == id {
				stillAChild = true
			}
		}
		if err := childRows.Err(); err != nil {
			childRows.Close()
			return err
		}
		childRows.Close()
		if stillAChild {
			return ErrCategoryDepth
		}
	}

	for _, it := range items {
		id := strings.TrimSpace(it.ID)
		if id == "" {
			continue
		}
		if _, err := tx.Exec(
			`UPDATE trigger_categories SET parent_id=?, sort_order=? WHERE id=?`,
			strings.TrimSpace(it.ParentID), it.SortOrder, id,
		); err != nil {
			return fmt.Errorf("reorder category %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// CategoryByID returns the public Category for id, with SortOrder/Explicit
// but no rolled-up Count (callers that need counts should use ListCategories),
// or ErrCategoryNotFound.
func (s *Store) CategoryByID(id string) (Category, error) {
	row, err := s.getCategoryRow(id)
	if err != nil {
		return Category{}, err
	}
	builtin := row.ParentID == "" && builtinPackNames()[row.Name]
	return Category{
		ID: row.ID, Name: row.Name, ParentID: row.ParentID,
		IsBuiltin: builtin, Custom: row.Explicit && !builtin,
		Explicit: row.Explicit, SortOrder: row.SortOrder,
	}, nil
}

// ResolveOrCreateCategory returns the id of the category named name directly
// under parentID (root when empty), creating it as an explicit custom
// category if it doesn't exist yet. Unlike resolveCategoryID (root-only,
// the compatibility path for legacy pack_name-only writes), this is
// nesting-aware — it's the entry point for callers that know a specific
// parent, such as importCommit splitting "Parent/Child" into two levels.
func (s *Store) ResolveOrCreateCategory(parentID, name string) (Category, error) {
	norm, err := normalizeCategoryName(name)
	if err != nil {
		return Category{}, err
	}
	parentID = strings.TrimSpace(parentID)
	cat, err := s.CreateCategory(norm, parentID)
	if err == nil {
		return cat, nil
	}
	if !errors.Is(err, ErrCategoryExists) {
		return Category{}, err
	}
	cats, err := s.ListCategories()
	if err != nil {
		return Category{}, err
	}
	for _, c := range cats {
		if c.ParentID == parentID && c.Name == norm {
			return c, nil
		}
	}
	// categoryNameTaken said this name is in use, but ListCategories doesn't
	// surface it — an empty, non-explicit placeholder row. Look it up
	// directly rather than fail an import that should just reuse it.
	var id string
	if err := s.db.QueryRow(
		`SELECT id FROM trigger_categories WHERE parent_id = ? AND name = ?`, parentID, norm,
	).Scan(&id); err != nil {
		return Category{}, fmt.Errorf("resolve category %s: %w", norm, err)
	}
	return Category{ID: id, Name: norm, ParentID: parentID}, nil
}

// categoryHasChildren reports whether any category has id as its parent.
func (s *Store) categoryHasChildren(id string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM trigger_categories WHERE parent_id = ?`, id).Scan(&n); err != nil {
		return false, fmt.Errorf("check category children: %w", err)
	}
	return n > 0, nil
}

// SplitCategoryName splits a top-level category's slash-containing name —
// left untouched, verbatim, by the id-hierarchy migration and by any GINA
// import predating real nested-category support — into an actual parent/
// child pair: a top-level category named by the part before the first "/"
// (created if it doesn't already exist, reused otherwise) and this category,
// renamed to the part after the "/" and reparented under it.
//
// Only a top-level category with no children of its own can be split, same
// as any other reparent (see ReorderCategories); built-in packs can't be
// split here. A name with nothing after the "/" (a bare trailing slash) just
// has it trimmed off — there's no child part to create.
//
// This isn't fully atomic: resolving/creating the parent happens before the
// rename+reparent step. If the parent had to be created and that later step
// fails, the parent is left behind rather than rolled back — but that's just
// an ordinary empty custom category, harmless and deletable like any other,
// not a broken intermediate state.
func (s *Store) SplitCategoryName(id string) (Category, error) {
	row, err := s.getCategoryRow(id)
	if err != nil {
		return Category{}, err
	}
	if builtinPackNames()[row.Name] {
		return Category{}, ErrCategoryBuiltin
	}
	if row.ParentID != "" {
		return Category{}, ErrCategoryDepth
	}
	idx := strings.Index(row.Name, "/")
	if idx < 0 {
		return Category{}, ErrCategoryNoSplit
	}
	parentName := strings.TrimSpace(row.Name[:idx])
	leafName := strings.TrimSpace(row.Name[idx+1:])
	if parentName == "" {
		return Category{}, ErrCategoryNoSplit
	}
	if leafName == "" {
		// Trailing slash with nothing after it — just drop it, no real split.
		if err := s.RenameCategory(id, parentName); err != nil {
			return Category{}, err
		}
		return s.CategoryByID(id)
	}
	hasChildren, err := s.categoryHasChildren(id)
	if err != nil {
		return Category{}, err
	}
	if hasChildren {
		return Category{}, ErrCategoryDepth
	}

	parent, err := s.ResolveOrCreateCategory("", parentName)
	if err != nil {
		return Category{}, err
	}
	taken, err := s.categoryNameTaken(parent.ID, leafName)
	if err != nil {
		return Category{}, err
	}
	if taken {
		return Category{}, ErrCategoryExists
	}

	tx, err := s.db.Begin()
	if err != nil {
		return Category{}, fmt.Errorf("begin split category: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE trigger_categories SET name=?, parent_id=? WHERE id=?`, leafName, parent.ID, id,
	); err != nil {
		return Category{}, fmt.Errorf("split category %s: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE triggers SET pack_name=? WHERE category_id=?`, leafName, id); err != nil {
		return Category{}, fmt.Errorf("refresh split category pack_name cache: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Category{}, err
	}
	return s.CategoryByID(id)
}

// resolveCategoryIDWith returns the id of a top-level category named name,
// materializing an implicit (explicit=0) row if none exists yet. This is the
// compatibility boundary for the many callers — pack installs, GINA import,
// and any other code that only knows a category by name — that set
// Trigger.PackName without knowing about ids: resolveCategoryLink resolves it
// to a real category_id here rather than requiring every call site to be
// rewritten. Always resolves at the top level; only nesting-aware callers
// (the create/import paths that pick a parent explicitly) produce children.
func resolveCategoryIDWith(ex execer, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	var id string
	err := ex.QueryRow(
		`SELECT id FROM trigger_categories WHERE parent_id = '' AND name = ?`, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("resolve category %s: %w", name, err)
	}
	newID, err := NewID()
	if err != nil {
		return "", fmt.Errorf("generate category id: %w", err)
	}
	if _, err := ex.Exec(
		`INSERT INTO trigger_categories (id, name, parent_id, created_at, explicit, sort_order)
		 VALUES (?, ?, '', ?, 0, 0)
		 ON CONFLICT(parent_id, name) DO NOTHING`,
		newID, name, time.Now().UTC().Unix(),
	); err != nil {
		return "", fmt.Errorf("create category %s: %w", name, err)
	}
	// Re-select rather than assume newID won — a concurrent resolve for the
	// same name may have won the ON CONFLICT race.
	if err := ex.QueryRow(
		`SELECT id FROM trigger_categories WHERE parent_id = '' AND name = ?`, name,
	).Scan(&id); err != nil {
		return "", fmt.Errorf("resolve category %s after create: %w", name, err)
	}
	return id, nil
}

// resolveCategoryLink keeps a trigger's category_id/pack_name pair consistent
// immediately before a write. When category_id is already set and pack_name
// either matches that category's current name or was left blank, category_id
// is authoritative: pack_name is (re)stamped from it, so a rename is never
// visible as stale data on a Trigger struct fetched before the rename.
//
// But mutating PackName directly and calling Insert/Update on an
// already-linked Trigger is also a supported way to move it — the mechanism
// every call site predating category_id still uses, id-oblivious pack
// installs and imports included. So when pack_name is set to something other
// than the linked category's name, that's treated as an intentional move:
// category_id is cleared and re-resolved from the new name at the top level,
// same as any other name-only caller. If a previously-set category_id no
// longer exists at all (the category was deleted concurrently), the trigger
// falls back the same way rather than failing the write.
func (s *Store) resolveCategoryLink(t *Trigger) error {
	return resolveCategoryLinkWith(s.db, t)
}

func resolveCategoryLinkWith(ex execer, t *Trigger) error {
	if t.CategoryID != "" {
		row, err := getCategoryRowWith(ex, t.CategoryID)
		switch {
		case err == ErrCategoryNotFound:
			t.CategoryID = ""
		case err != nil:
			return err
		case t.PackName == "" || t.PackName == row.Name:
			t.PackName = row.Name
			return nil
		default:
			t.CategoryID = ""
		}
	}
	if t.PackName == "" {
		return nil
	}
	id, err := resolveCategoryIDWith(ex, t.PackName)
	if err != nil {
		return err
	}
	t.CategoryID = id
	return nil
}

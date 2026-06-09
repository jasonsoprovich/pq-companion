package trigger

import (
	"errors"
	"testing"
	"time"
)

// makeTrigger builds a minimal log-source trigger in the given category.
func makeTrigger(name, packName string) *Trigger {
	return &Trigger{
		ID:        name + ":" + packName,
		Name:      name,
		Enabled:   true,
		Pattern:   `^` + name + `$`,
		PackName:  packName,
		CreatedAt: time.Unix(0, 0).UTC(),
		Actions:   []Action{{Type: ActionOverlayText, Text: name}},
	}
}

// catByName indexes a category slice for assertions.
func catByName(cats []Category) map[string]Category {
	m := make(map[string]Category, len(cats))
	for _, c := range cats {
		m[c.Name] = c
	}
	return m
}

func TestCreateCategory_PersistsEmpty(t *testing.T) {
	s := openTestStore(t)

	cat, err := s.CreateCategory("My Raids")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if !cat.Custom || cat.IsBuiltin || cat.Count != 0 || cat.Name != "My Raids" {
		t.Fatalf("unexpected category: %+v", cat)
	}

	cats, err := s.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	got := catByName(cats)
	c, ok := got["My Raids"]
	if !ok {
		t.Fatal("empty custom category not listed")
	}
	if c.Count != 0 || !c.Custom || c.IsBuiltin {
		t.Fatalf("unexpected listed category: %+v", c)
	}
}

func TestCreateCategory_TrimsName(t *testing.T) {
	s := openTestStore(t)
	cat, err := s.CreateCategory("  Spaced  ")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if cat.Name != "Spaced" {
		t.Fatalf("name not trimmed: %q", cat.Name)
	}
}

func TestCreateCategory_Rejects(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.CreateCategory("   "); !errors.Is(err, ErrCategoryNameEmpty) {
		t.Fatalf("empty name: want ErrCategoryNameEmpty, got %v", err)
	}
	if _, err := s.CreateCategory(reservedUncategorized); !errors.Is(err, ErrCategoryReserved) {
		t.Fatalf("reserved name: want ErrCategoryReserved, got %v", err)
	}

	if _, err := s.CreateCategory("Dupe"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := s.CreateCategory("Dupe"); !errors.Is(err, ErrCategoryExists) {
		t.Fatalf("duplicate: want ErrCategoryExists, got %v", err)
	}

	// A name already in use by a trigger's pack collides too.
	if err := s.Insert(makeTrigger("t1", "Imported Pack")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := s.CreateCategory("Imported Pack"); !errors.Is(err, ErrCategoryExists) {
		t.Fatalf("in-use name: want ErrCategoryExists, got %v", err)
	}

	// A built-in pack name is reserved even when not installed.
	packs := AllPacks()
	if len(packs) == 0 {
		t.Skip("no built-in packs to test reservation")
	}
	if _, err := s.CreateCategory(packs[0].PackName); !errors.Is(err, ErrCategoryExists) {
		t.Fatalf("built-in name: want ErrCategoryExists, got %v", err)
	}
}

func TestListCategories_CountsAndFlags(t *testing.T) {
	s := openTestStore(t)

	if err := s.Insert(makeTrigger("a", "Custom A")); err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	if err := s.Insert(makeTrigger("b", "Custom A")); err != nil {
		t.Fatalf("Insert b: %v", err)
	}
	if err := s.Insert(makeTrigger("u", "")); err != nil { // Uncategorized
		t.Fatalf("Insert u: %v", err)
	}
	if _, err := s.CreateCategory("Empty One"); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	cats, err := s.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	got := catByName(cats)

	// Uncategorized (empty pack_name) is never listed as a category.
	if _, ok := got[""]; ok {
		t.Fatal("empty pack_name should not be a category")
	}
	if _, ok := got[reservedUncategorized]; ok {
		t.Fatal("reserved sentinel should not be a category")
	}
	if c := got["Custom A"]; c.Count != 2 || c.IsBuiltin || c.Custom {
		t.Fatalf("Custom A: %+v (want count=2, not builtin, not custom-row)", c)
	}
	if c := got["Empty One"]; c.Count != 0 || !c.Custom {
		t.Fatalf("Empty One: %+v (want count=0, custom-row)", c)
	}

	// Sorted by name.
	for i := 1; i < len(cats); i++ {
		if cats[i-1].Name > cats[i].Name {
			t.Fatalf("not sorted: %q before %q", cats[i-1].Name, cats[i].Name)
		}
	}
}

func TestListCategories_BuiltinFlag(t *testing.T) {
	s := openTestStore(t)
	packs := AllPacks()
	if len(packs) == 0 {
		t.Skip("no built-in packs")
	}
	builtin := packs[0].PackName
	if err := s.Insert(makeTrigger("x", builtin)); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	cats, err := s.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	c, ok := catByName(cats)[builtin]
	if !ok {
		t.Fatalf("built-in pack %q not listed", builtin)
	}
	if !c.IsBuiltin {
		t.Fatalf("built-in pack %q not flagged IsBuiltin", builtin)
	}
}

func TestRenameCategory_CascadesToTriggers(t *testing.T) {
	s := openTestStore(t)
	if err := s.Insert(makeTrigger("a", "Old Name")); err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	if err := s.Insert(makeTrigger("b", "Old Name")); err != nil {
		t.Fatalf("Insert b: %v", err)
	}

	if err := s.RenameCategory("Old Name", "New Name"); err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, tr := range list {
		if tr.PackName != "New Name" {
			t.Fatalf("trigger %q still in %q", tr.Name, tr.PackName)
		}
	}
}

func TestRenameCategory_EmptyCustomRow(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateCategory("Before"); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if err := s.RenameCategory("Before", "After"); err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}
	got := catByName(mustList(t, s))
	if _, ok := got["Before"]; ok {
		t.Fatal("old name still present after rename")
	}
	if c, ok := got["After"]; !ok || !c.Custom {
		t.Fatalf("renamed empty category not persisted: %+v", c)
	}
}

func TestRenameCategory_Rejects(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateCategory("Source"); err != nil {
		t.Fatalf("CreateCategory Source: %v", err)
	}
	if _, err := s.CreateCategory("Target"); err != nil {
		t.Fatalf("CreateCategory Target: %v", err)
	}

	if err := s.RenameCategory("Missing", "Whatever"); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("missing source: want ErrCategoryNotFound, got %v", err)
	}
	if err := s.RenameCategory("Source", "Target"); !errors.Is(err, ErrCategoryExists) {
		t.Fatalf("collision: want ErrCategoryExists, got %v", err)
	}
	if err := s.RenameCategory("Source", "  "); !errors.Is(err, ErrCategoryNameEmpty) {
		t.Fatalf("empty new name: want ErrCategoryNameEmpty, got %v", err)
	}

	// Built-in packs can't be renamed here.
	packs := AllPacks()
	if len(packs) > 0 {
		if err := s.Insert(makeTrigger("x", packs[0].PackName)); err != nil {
			t.Fatalf("Insert builtin trigger: %v", err)
		}
		if err := s.RenameCategory(packs[0].PackName, "Hijacked"); !errors.Is(err, ErrCategoryBuiltin) {
			t.Fatalf("rename builtin: want ErrCategoryBuiltin, got %v", err)
		}
	}
}

func TestDeleteCategory_OrphansTriggers(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateCategory("Doomed"); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if err := s.Insert(makeTrigger("a", "Doomed")); err != nil {
		t.Fatalf("Insert a: %v", err)
	}
	if err := s.Insert(makeTrigger("b", "Doomed")); err != nil {
		t.Fatalf("Insert b: %v", err)
	}

	if err := s.DeleteCategory("Doomed"); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	// Triggers survive, now Uncategorized.
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected triggers to survive, got %d", len(list))
	}
	for _, tr := range list {
		if tr.PackName != "" {
			t.Fatalf("trigger %q not orphaned: pack=%q", tr.Name, tr.PackName)
		}
	}
	// Category row gone.
	if _, ok := catByName(mustList(t, s))["Doomed"]; ok {
		t.Fatal("category row survived delete")
	}
}

func TestDeleteCategory_Rejects(t *testing.T) {
	s := openTestStore(t)
	if err := s.DeleteCategory("Nope"); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("missing: want ErrCategoryNotFound, got %v", err)
	}
	packs := AllPacks()
	if len(packs) > 0 {
		if err := s.Insert(makeTrigger("x", packs[0].PackName)); err != nil {
			t.Fatalf("Insert builtin trigger: %v", err)
		}
		if err := s.DeleteCategory(packs[0].PackName); !errors.Is(err, ErrCategoryBuiltin) {
			t.Fatalf("delete builtin: want ErrCategoryBuiltin, got %v", err)
		}
	}
}

func mustList(t *testing.T, s *Store) []Category {
	t.Helper()
	cats, err := s.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	return cats
}

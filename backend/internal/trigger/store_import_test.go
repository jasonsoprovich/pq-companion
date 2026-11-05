package trigger

import (
	"path/filepath"
	"testing"
	"time"
)

// Import commits use InsertMany with a category ID. Category resolution must
// use that transaction, not wait for the sole connection it already owns.
func TestInsertManyCategoryTransaction(t *testing.T) {
	for _, mode := range []string{"linked", "name-only", "missing-id", "renamed-pack"} {
		t.Run(mode, func(t *testing.T) {
			s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			cat, err := s.ResolveOrCreateCategory("", "Existing")
			if err != nil {
				t.Fatal(err)
			}
			tr := &Trigger{ID: "batch", Name: "Imported", Pattern: "hello", Enabled: true, CreatedAt: time.Now()}
			wantName := "Existing"
			switch mode {
			case "linked":
				child, err := s.ResolveOrCreateCategory(cat.ID, "Child")
				if err != nil {
					t.Fatal(err)
				}
				tr.CategoryID = child.ID
				wantName = "Child"
			case "name-only":
				tr.PackName = "New Category"
				wantName = tr.PackName
			case "missing-id":
				tr.CategoryID = "deleted"
				tr.PackName = "Recovered"
				wantName = tr.PackName
			case "renamed-pack":
				tr.CategoryID = cat.ID
				tr.PackName = "Moved"
				wantName = tr.PackName
			}
			if err := s.InsertMany([]*Trigger{tr}); err != nil {
				t.Fatal(err)
			}
			got, err := s.Get(tr.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.CategoryID == "" || got.PackName != wantName {
				t.Fatalf("category link = %q/%q, want nonempty/%q", got.CategoryID, got.PackName, wantName)
			}
			row, err := s.getCategoryRow(got.CategoryID)
			if err != nil || row.Name != wantName {
				t.Fatalf("category row = %+v, %v", row, err)
			}
			if mode == "linked" && row.ParentID != cat.ID {
				t.Fatalf("nested category lost: %+v", row)
			}
		})
	}
}

func TestInsertManyCategoryRollbackAndRetry(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seed := &Trigger{ID: "existing", Name: "Existing", Pattern: "seed", CreatedAt: time.Now()}
	if err := s.Insert(seed); err != nil {
		t.Fatal(err)
	}
	first := &Trigger{ID: "first", Name: "First", Pattern: "one", PackName: "New Batch Category", CreatedAt: time.Now()}
	second := &Trigger{ID: seed.ID, Name: "Second", Pattern: "two", PackName: first.PackName, CreatedAt: time.Now()}
	if err := s.InsertMany([]*Trigger{first, second}); err == nil {
		t.Fatal("expected duplicate-ID insert failure")
	}
	rows, err := s.List()
	if err != nil || len(rows) != 1 || rows[0].ID != seed.ID {
		t.Fatalf("partial trigger write: %+v, %v", rows, err)
	}
	var categories int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM trigger_categories WHERE name = ?`, first.PackName).Scan(&categories); err != nil {
		t.Fatal(err)
	}
	if categories != 0 {
		t.Fatalf("category creation escaped rolled-back transaction: %d", categories)
	}
	// Retry the same structs, including their now-stale category IDs. The
	// resolver must recreate the rolled-back category, not duplicate survivors.
	second.ID = "second"
	if err := s.InsertMany([]*Trigger{first, second}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.List()
	if err != nil || len(rows) != 3 {
		t.Fatalf("retry count = %d, %v", len(rows), err)
	}
	if first.CategoryID == "" || first.CategoryID != second.CategoryID {
		t.Fatalf("retry links differ: %q/%q", first.CategoryID, second.CategoryID)
	}
	for _, tr := range []*Trigger{first, second} {
		got, err := s.Get(tr.ID)
		if err != nil || got.CategoryID != first.CategoryID || got.PackName != first.PackName {
			t.Fatalf("retry readback = %+v, %v", got, err)
		}
	}
}

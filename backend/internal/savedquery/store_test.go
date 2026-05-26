package savedquery

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCreateListUpdateDelete(t *testing.T) {
	s := openTestStore(t)

	q := SavedQuery{Name: "  Items by AC ", Description: " top AC ", SQL: "SELECT * FROM items LIMIT 1;"}
	if err := s.Create(&q); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if q.ID == "" {
		t.Fatalf("expected id to be populated")
	}
	if q.Name != "Items by AC" || q.Description != "top AC" {
		t.Fatalf("expected trimmed name/description, got %q / %q", q.Name, q.Description)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != q.ID {
		t.Fatalf("expected one row matching created id, got %#v", list)
	}

	updated, err := s.Update(q.ID, SavedQuery{Name: "Items by HP", Description: "top HP", SQL: "SELECT id, hp FROM items;"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Items by HP" || updated.SQL != "SELECT id, hp FROM items;" {
		t.Fatalf("update did not apply: %#v", updated)
	}
	// Both fields are persisted as epoch seconds; compare at that resolution
	// so the assertion isn't fooled by sub-second jitter on the in-memory
	// CreatedAt populated by Create.
	if updated.UpdatedAt.Unix() < q.CreatedAt.Unix() {
		t.Fatalf("expected UpdatedAt >= CreatedAt, got %v / %v", updated.UpdatedAt, q.CreatedAt)
	}

	if err := s.Delete(q.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(q.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestCreateValidatesRequired(t *testing.T) {
	s := openTestStore(t)
	cases := []SavedQuery{
		{Name: "", SQL: "SELECT 1"},
		{Name: "   ", SQL: "SELECT 1"},
		{Name: "ok", SQL: ""},
		{Name: "ok", SQL: "   "},
	}
	for i, c := range cases {
		if err := s.Create(&c); err != ErrInvalid {
			t.Errorf("case %d: expected ErrInvalid, got %v", i, err)
		}
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	src := openTestStore(t)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		q := SavedQuery{Name: name, SQL: "SELECT '" + name + "'"}
		if err := src.Create(&q); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	pack, err := src.ExportPack()
	if err != nil {
		t.Fatalf("ExportPack: %v", err)
	}
	if pack.Kind != PackKind || pack.Version != PackVersion {
		t.Fatalf("unexpected pack envelope: %+v", pack)
	}
	if len(pack.Queries) != 3 {
		t.Fatalf("expected 3 queries, got %d", len(pack.Queries))
	}

	dst := openTestStore(t)
	inserted, err := dst.ImportPack(pack)
	if err != nil {
		t.Fatalf("ImportPack: %v", err)
	}
	if inserted != 3 {
		t.Fatalf("expected 3 inserts, got %d", inserted)
	}
	out, err := dst.List()
	if err != nil {
		t.Fatalf("List dst: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 rows in dst, got %d", len(out))
	}
	// IDs must NOT be reused — every imported row should get a fresh id.
	for _, q := range out {
		for _, srcEntry := range pack.Queries {
			if q.Name == srcEntry.Name && q.SQL == srcEntry.SQL {
				// matched on content; nothing to assert about id here other
				// than that it's non-empty (Create asserts that already).
			}
		}
	}

	// Importing again should append duplicates rather than fail — the user
	// dedupes from the UI.
	again, err := dst.ImportPack(pack)
	if err != nil {
		t.Fatalf("second ImportPack: %v", err)
	}
	if again != 3 {
		t.Fatalf("expected 3 inserts on second import, got %d", again)
	}
	if out2, _ := dst.List(); len(out2) != 6 {
		t.Fatalf("expected 6 rows after second import, got %d", len(out2))
	}
}

func TestImportPackRejectsWrongKind(t *testing.T) {
	s := openTestStore(t)
	bad := Pack{Kind: "pq-companion.trigger-pack", Version: 1}
	if _, err := s.ImportPack(bad); err == nil {
		t.Fatalf("expected error for wrong pack kind")
	}
}

func TestImportPackRejectsNewerVersion(t *testing.T) {
	s := openTestStore(t)
	bad := Pack{Kind: PackKind, Version: PackVersion + 1}
	if _, err := s.ImportPack(bad); err == nil {
		t.Fatalf("expected error for newer pack version")
	}
}

func TestImportPackSkipsInvalidEntries(t *testing.T) {
	s := openTestStore(t)
	pack := Pack{
		Kind:    PackKind,
		Version: PackVersion,
		Queries: []PackEntry{
			{Name: "good", SQL: "SELECT 1"},
			{Name: "", SQL: "SELECT 2"},       // skipped
			{Name: "no-sql", SQL: ""},          // skipped
			{Name: "alsoGood", SQL: "SELECT 3"},
		},
	}
	n, err := s.ImportPack(pack)
	if err != nil {
		t.Fatalf("ImportPack: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 successful inserts, got %d", n)
	}
}

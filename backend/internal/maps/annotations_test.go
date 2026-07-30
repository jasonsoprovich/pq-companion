package maps

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *AnnotationStore {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewAnnotationStore(db)
	if err != nil {
		t.Fatalf("NewAnnotationStore: %v", err)
	}
	return s
}

func TestCreateListDelete(t *testing.T) {
	s := testStore(t)
	a, err := s.Create(UserAnnotation{
		Zone: "akheva", X: -100, Y: 200, Z: 12,
		Category: "wall", Label: "Fake wall",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == 0 || a.CreatedAt == 0 || a.UpdatedAt == 0 {
		t.Errorf("Create returned %+v, want id and timestamps set", a)
	}

	got, err := s.List("akheva")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Label != "Fake wall" {
		t.Fatalf("List = %+v, want one Fake wall", got)
	}

	// A different zone must not see it.
	other, err := s.List("necropolis")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(other) != 0 {
		t.Errorf("List(necropolis) = %+v, want empty", other)
	}

	if err := s.Delete(a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(a.ID); err == nil {
		t.Error("second Delete succeeded, want not-found error")
	}
}

func TestUpdateKeepsPosition(t *testing.T) {
	s := testStore(t)
	a, err := s.Create(UserAnnotation{
		Zone: "akheva", X: -100, Y: 200, Z: 12, Category: "note", Label: "camp",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := s.Update(a.ID, "hazard", "deadly fall")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.Category != "hazard" || up.Label != "deadly fall" {
		t.Errorf("got %q/%q, want hazard/deadly fall", up.Category, up.Label)
	}
	// Renaming must never move the marker.
	if up.X != -100 || up.Y != 200 || up.Z != 12 {
		t.Errorf("position moved to (%d,%d,%d), want (-100,200,12)", up.X, up.Y, up.Z)
	}
	if up.Zone != "akheva" {
		t.Errorf("zone = %q, want akheva", up.Zone)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	s := testStore(t)
	cases := []struct {
		name string
		in   UserAnnotation
	}{
		{"no zone", UserAnnotation{X: 1, Y: 1, Category: "note", Label: "x"}},
		{"bad category", UserAnnotation{Zone: "a", X: 1, Y: 1, Category: "secret", Label: "x"}},
		{"no label", UserAnnotation{Zone: "a", X: 1, Y: 1, Category: "note"}},
		// 0,0 is the unset sentinel shared with the DB-derived generators.
		{"origin", UserAnnotation{Zone: "a", X: 0, Y: 0, Category: "note", Label: "x"}},
		// Beyond what maps.db's int16 packing can carry, so an export would not
		// round-trip through the offline pipeline.
		{"out of range", UserAnnotation{Zone: "a", X: 40000, Y: 1, Category: "note", Label: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Create(tc.in); err == nil {
				t.Error("Create succeeded, want an error")
			}
		})
	}
}

func TestBuildExportNegatesBackToGameCoords(t *testing.T) {
	// Map space stores f1 = -game_x, f2 = -game_y; the export must undo exactly
	// that, or a submitted annotation lands mirrored for everyone.
	out := BuildExport([]UserAnnotation{
		{Zone: "akheva", X: -100, Y: 200, Z: 12, Category: "wall", Label: "Fake wall"},
	})
	rows := out.Zones["akheva"]
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].X != 100 || rows[0].Y != -200 || rows[0].Z != 12 {
		t.Errorf("got (%.0f,%.0f,%.0f), want (100,-200,12)", rows[0].X, rows[0].Y, rows[0].Z)
	}
	// Blank evidence is the gate: mapgen rejects it, so a raw export cannot be
	// merged without someone writing down where the fact came from.
	if rows[0].Evidence != "" {
		t.Errorf("Evidence = %q, want empty", rows[0].Evidence)
	}
	if len(out.Readme) == 0 {
		t.Error("export has no readme — it has to be explicable once detached from the repo")
	}
}

func TestBuildExportGroupsAndSorts(t *testing.T) {
	out := BuildExport([]UserAnnotation{
		{Zone: "akheva", X: 1, Y: 1, Category: "note", Label: "zebra"},
		{Zone: "necropolis", X: 1, Y: 1, Category: "note", Label: "yak"},
		{Zone: "akheva", X: 2, Y: 2, Category: "note", Label: "aardvark"},
	})
	if len(out.Zones) != 2 {
		t.Fatalf("got %d zones, want 2", len(out.Zones))
	}
	ak := out.Zones["akheva"]
	if len(ak) != 2 || ak[0].Label != "aardvark" || ak[1].Label != "zebra" {
		t.Errorf("akheva order = %v, want aardvark then zebra",
			[]string{ak[0].Label, ak[1].Label})
	}
}

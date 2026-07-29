package mapgen_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
)

// quarmPath locates quarm.db relative to this file. Unlike the geometry tests,
// POI generation needs no EQ client — it reads only the game database, which is
// always present.
func quarmPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db")
}

func generatePOIs(t *testing.T) []mapgen.POI {
	t.Helper()
	d, err := db.Open(quarmPath(t))
	if err != nil {
		t.Skipf("quarm.db unavailable: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	pois, err := mapgen.GeneratePOIs(d.DB)
	if err != nil {
		t.Fatalf("generate POIs: %v", err)
	}
	return pois
}

// TestGeneratePOIs_TransformMatchesGeometry is the check that matters most.
// POIs are transformed from quarm.db game coordinates while geometry comes from
// client mesh coordinates by a different route. If the two disagree every pin is
// silently misplaced, and nothing about the output would look wrong.
//
// The Priest of Discord is the anchor used throughout this work: quarm.db puts
// him at game x=235, y=6 in qeynos2, and the same in-game /map marker check
// confirmed that convention against the live server.
func TestGeneratePOIs_TransformMatchesGeometry(t *testing.T) {
	var found *mapgen.POI
	for _, p := range generatePOIs(t) {
		if p.Zone == "qeynos2" && p.Category == mapgen.CategoryVendor &&
			p.Label == "Priest of Discord" {
			found = &p
			break
		}
	}
	if found == nil {
		t.Fatal("Priest of Discord not generated as a qeynos2 vendor POI")
	}
	if found.X != -235 || found.Y != -6 {
		t.Errorf("position: got (%d, %d), want (-235, -6) — POI transform has "+
			"diverged from the geometry transform", found.X, found.Y)
	}
	if found.Source != "db:spawn2" {
		t.Errorf("source: got %q, want db:spawn2", found.Source)
	}
}

// TestGeneratePOIs_Categories checks every category produces rows and that all
// of them are marked db:*, since a data release regenerates exactly those and
// leaves anything else alone.
func TestGeneratePOIs_Categories(t *testing.T) {
	pois := generatePOIs(t)

	counts := map[string]int{}
	for _, p := range pois {
		counts[p.Category]++
		if len(p.Source) < 3 || p.Source[:3] != "db:" {
			t.Fatalf("POI %q in %s has source %q, want a db: prefix",
				p.Label, p.Zone, p.Source)
		}
		if p.Zone == "" || p.Label == "" {
			t.Fatalf("POI in %s has empty zone or label: %+v", p.Zone, p)
		}
	}

	for _, c := range []string{
		mapgen.CategoryZoneLine, mapgen.CategoryGroundSpawn, mapgen.CategoryTrap,
		mapgen.CategoryTradeskill, mapgen.CategorySuccor, mapgen.CategoryVendor,
		mapgen.CategoryRaidTarget, mapgen.CategoryDoor,
	} {
		if counts[c] == 0 {
			t.Errorf("category %q produced no POIs", c)
		}
	}
}

// TestGeneratePOIs_RejectsSentinels guards the zone_points "leave unchanged"
// marker. 999999 is a control value, not a position; drawn literally it puts a
// pin a million units off the map.
func TestGeneratePOIs_RejectsSentinels(t *testing.T) {
	for _, p := range generatePOIs(t) {
		if p.X <= -900000 || p.X >= 900000 || p.Y <= -900000 || p.Y >= 900000 {
			t.Fatalf("sentinel coordinate survived generation: %+v", p)
		}
	}
}

// TestGeneratePOIs_LabelsAreReadable checks the DB-name cleanup: underscores
// become spaces and the '#' script-spawn marker is stripped, since these labels
// are drawn on the map.
func TestGeneratePOIs_LabelsAreReadable(t *testing.T) {
	for _, p := range generatePOIs(t) {
		if p.Category != mapgen.CategoryVendor && p.Category != mapgen.CategoryRaidTarget {
			continue
		}
		for _, bad := range []string{"_", "#"} {
			if contains(p.Label, bad) {
				t.Errorf("label %q in %s still contains %q", p.Label, p.Zone, bad)
			}
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

package maps_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/maps"
)

func repoMapsDB(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "maps.db")
}

// TestOpen_MissingFileIsNotAnError is the important one: maps.db is generated
// offline from an EQ client, so plenty of checkouts and CI runs won't have it.
// A missing file must degrade to "no maps", never break server startup.
func TestOpen_MissingFileIsNotAnError(t *testing.T) {
	s, err := maps.Open(filepath.Join(t.TempDir(), "absent.db"))
	if err != nil {
		t.Fatalf("open missing maps.db: got error %v, want nil", err)
	}
	if s.Available() {
		t.Error("Available() on a missing file: got true, want false")
	}
	// Every accessor must be safe on the nil store.
	if zones, err := s.Zones(); err != nil || zones != nil {
		t.Errorf("Zones() on missing store: got (%v, %v), want (nil, nil)", zones, err)
	}
	if _, ok, err := s.Zone("qeynos2"); err != nil || ok {
		t.Errorf("Zone() on missing store: got (ok=%v, %v), want (false, nil)", ok, err)
	}
	if segs, err := s.Segments("qeynos2", 0); err != nil || segs != nil {
		t.Errorf("Segments() on missing store: got (%v, %v), want (nil, nil)", segs, err)
	}
	if pois, err := s.POIs("qeynos2"); err != nil || pois != nil {
		t.Errorf("POIs() on missing store: got (%v, %v), want (nil, nil)", pois, err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close() on missing store: %v", err)
	}
}

func TestStore_ReadsGeneratedMaps(t *testing.T) {
	s, err := maps.Open(repoMapsDB(t))
	if err != nil {
		t.Fatalf("open maps.db: %v", err)
	}
	if !s.Available() {
		t.Skip("maps.db not generated; run cmd/mapgen")
	}
	defer s.Close()

	z, ok, err := s.Zone("qeynos2")
	if err != nil || !ok {
		t.Fatalf("Zone(qeynos2): ok=%v err=%v", ok, err)
	}
	if z.MaxX <= z.MinX || z.MaxY <= z.MinY {
		t.Errorf("degenerate bounds: %+v", z)
	}

	segs, err := s.Segments("qeynos2", 0)
	if err != nil {
		t.Fatalf("Segments: %v", err)
	}
	if len(segs) == 0 {
		t.Fatal("Segments: got none")
	}

	pois, err := s.POIs("qeynos2")
	if err != nil {
		t.Fatalf("POIs: %v", err)
	}
	// Every generated POI should sit inside the zone's own geometry bounds; a
	// transform mismatch between POIs and geometry shows up here.
	const pad = 100
	for _, p := range pois {
		if p.X < z.MinX-pad || p.X > z.MaxX+pad || p.Y < z.MinY-pad || p.Y > z.MaxY+pad {
			t.Errorf("POI %q at (%d,%d) outside zone bounds %+v", p.Label, p.X, p.Y, z)
		}
	}
}

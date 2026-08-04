package mapgen_test

import (
	"math"
	"os"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
)

// clientDir is the TAKP client used to develop the pipeline. It lives on an
// external drive, so every test here skips when it isn't mounted — these are
// verification tests for an offline build tool, not CI gates.
const clientDir = "/Volumes/T7/EQ/TAKPv22"

func requireClient(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(clientDir); err != nil {
		t.Skipf("EQ client not mounted at %s", clientDir)
	}
}

// TestLoadZone_MatchesReferencePipeline pins the Go port to the numbers the
// Python prototype produced (docs/maps-feasibility.md 5b). If a refactor moves
// these, the port has diverged from the validated behaviour.
func TestLoadZone_MatchesReferencePipeline(t *testing.T) {
	requireClient(t)

	cases := []struct {
		zone      string
		triangles int // after welding
		walkable  int
	}{
		{"unrest", 26291, 9944},
		{"qeynos2", 19470, 7945},
		{"gfaydark", 74981, 41406},
		{"akheva", 91633, 31176},
		{"necropolis", 22878, 9369},
	}

	for _, tc := range cases {
		t.Run(tc.zone, func(t *testing.T) {
			z, err := mapgen.LoadZone(clientDir, tc.zone)
			if err != nil {
				t.Fatalf("load %s: %v", tc.zone, err)
			}
			// Welding can collapse a few degenerate faces the reference kept,
			// so allow a small tolerance rather than demanding an exact match.
			if !within(len(z.Triangles), tc.triangles, 0.02) {
				t.Errorf("triangles: got %d, want ~%d", len(z.Triangles), tc.triangles)
			}
			walk := len(z.WalkableFaces())
			if !within(walk, tc.walkable, 0.02) {
				t.Errorf("walkable faces: got %d, want ~%d", walk, tc.walkable)
			}
		})
	}
}

// TestArchiveNameTableCRCVariants covers the trap that broke the first
// prototype: TAKPv22 ships two different filename-table CRCs across its own
// archives, so the table must be found structurally, not by matching either
// constant. unrest uses 0xFFFFFFFF; akheva uses 0x61580AC9.
func TestArchiveNameTableCRCVariants(t *testing.T) {
	requireClient(t)

	for _, zone := range []string{"unrest", "akheva"} {
		t.Run(zone, func(t *testing.T) {
			arc, err := mapgen.OpenArchive(clientDir + "/" + zone + ".s3d")
			if err != nil {
				t.Fatalf("open %s.s3d: %v", zone, err)
			}
			if _, ok := arc[zone+".wld"]; !ok {
				t.Errorf("%s.wld missing from archive (%d entries)", zone, len(arc))
			}
		})
	}
}

// TestToMapSpace locks in the swap-and-negate established in 5b.2. A sign flip
// here silently mirrors every map, and bounds checks cannot detect it.
func TestToMapSpace(t *testing.T) {
	got := mapgen.ToMapSpace(100, 200, 300)
	want := mapgen.Vec3{X: -200, Y: -100, Z: 300}
	if got != want {
		t.Errorf("ToMapSpace(100,200,300) = %+v, want %+v", got, want)
	}
}

// TestPlacementsAgreeWithTransform is the end-to-end orientation check: object
// placements from objects.wld must land inside the zone's own geometry bounds.
// A mirrored transform pushes them outside.
func TestPlacementsAgreeWithTransform(t *testing.T) {
	requireClient(t)

	z, err := mapgen.LoadZone(clientDir, "unrest")
	if err != nil {
		t.Fatalf("load unrest: %v", err)
	}
	places, err := mapgen.LoadPlacements(clientDir, "unrest")
	if err != nil {
		t.Fatalf("load unrest placements: %v", err)
	}
	if len(places) == 0 {
		t.Skip("no placements in unrest")
	}

	minX, minY, maxX, maxY := z.Bounds()
	const pad = 50.0
	outside := 0
	for _, p := range places {
		if p.Pos.X < minX-pad || p.Pos.X > maxX+pad ||
			p.Pos.Y < minY-pad || p.Pos.Y > maxY+pad {
			outside++
		}
	}
	// Placements are furniture inside the zone; essentially all should fall
	// within its geometry bounds.
	if outside > len(places)/10 {
		t.Errorf("%d/%d placements outside zone bounds — transform likely wrong",
			outside, len(places))
	}
}

func within(got, want int, tol float64) bool {
	if want == 0 {
		return got == 0
	}
	d := float64(got-want) / float64(want)
	return d <= tol && d >= -tol
}

// TestExtractors_MatchReferencePipeline pins each extractor to a known-good
// segment count, and — more importantly — pins the technique the classifier
// picks. A wrong technique is the failure that matters: boundary extraction on
// terrain yields an empty rectangle, and a silhouette of an open zone is just
// its footprint.
func TestExtractors_MatchReferencePipeline(t *testing.T) {
	requireClient(t)

	cases := []struct {
		zone      string
		technique mapgen.Technique
		segments  int
	}{
		// These no longer match the Python prototype's original counts (5013 /
		// 4283): both zones have freestanding walls — built on a floor that
		// never stops beneath them, so BoundaryEdges alone never drew them —
		// that FreestandingWallEdges now recovers (see wallrim.go). Confirmed
		// by rendering both before and after with RenderComparison: unrest
		// gains an inner defensive wall ring around its courtyard and a couple
		// of small pillar bases; qeynos2 gains part of its district gate wall.
		// Both are real geometry, not noise. The number that matters is still
		// the technique, not the exact count — see the 5% comment below.
		{"unrest", mapgen.TechniqueBoundary, 5449},
		{"qeynos2", mapgen.TechniqueBoundary, 4633},
		// Contour and silhouette zones are covered by their own tests: both
		// deliberately diverge from the flat prototype's counts, contours
		// because they are now simplified and silhouettes because they are now
		// sliced by height.
	}

	for _, tc := range cases {
		t.Run(tc.zone, func(t *testing.T) {
			z, err := mapgen.LoadZone(clientDir, tc.zone)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			c := mapgen.Classify(z)
			if c.Technique != tc.technique {
				t.Errorf("technique: got %q, want %q (occ=%.2f bnd=%.2f)",
					c.Technique, tc.technique, c.Occupancy, c.BoundaryDensit)
			}
			// Go's map iteration order differs from Python's insertion order,
			// so chain start points differ and counts land a few percent apart.
			// 5% catches a real regression without chasing that noise.
			if n := len(mapgen.Extract(z, c)); !within(n, tc.segments, 0.05) {
				t.Errorf("segments: got %d, want ~%d", n, tc.segments)
			}
		})
	}
}

// TestChainKeyIncludesZ guards a bug that silently halved boundary detail:
// dropping Z from the chain key merges vertices stacked on different floors,
// which inflates their degree past 2 and cuts the chain walk short. The symptom
// was quiet — maps still rendered, just with far less detail.
func TestChainKeyIncludesZ(t *testing.T) {
	// A vertical run and a horizontal run meeting at one XY but two different
	// heights. With Z in the key these are two separate 1-segment chains; with
	// Z dropped they look like a single degree-4 junction.
	segs := []mapgen.Segment{
		{A: mapgen.Vec3{X: 0, Y: 0, Z: 0}, B: mapgen.Vec3{X: 10, Y: 0, Z: 0}},
		{A: mapgen.Vec3{X: 10, Y: 0, Z: 0}, B: mapgen.Vec3{X: 20, Y: 0, Z: 0}},
		{A: mapgen.Vec3{X: 0, Y: 0, Z: 100}, B: mapgen.Vec3{X: 10, Y: 0, Z: 100}},
		{A: mapgen.Vec3{X: 10, Y: 0, Z: 100}, B: mapgen.Vec3{X: 20, Y: 0, Z: 100}},
	}
	chains := mapgen.Chain(segs)
	if len(chains) != 2 {
		t.Fatalf("got %d chains, want 2 (one per storey)", len(chains))
	}
	for i, ch := range chains {
		if len(ch) != 3 {
			t.Errorf("chain %d: got %d points, want 3 (walk should not break)", i, len(ch))
		}
	}
}

// TestContours_SimplifiedNotRaw checks both halves of the contour path: the raw
// slicing still matches the prototype, and simplification then meaningfully
// reduces it. The prototype emitted contours unsimplified, so the shipped count
// is deliberately lower.
func TestContours_SimplifiedNotRaw(t *testing.T) {
	requireClient(t)

	z, err := mapgen.LoadZone(clientDir, "gfaydark")
	if err != nil {
		t.Fatalf("load gfaydark: %v", err)
	}
	c := mapgen.Classify(z)
	if c.Technique != mapgen.TechniqueContours {
		t.Fatalf("technique: got %q, want contours", c.Technique)
	}

	// The prototype produced 14,140 raw contour segments at a 30-unit interval.
	raw := z.Contours(30)
	if !within(len(raw), 14140, 0.05) {
		t.Errorf("raw contours: got %d, want ~14140", len(raw))
	}

	got := len(mapgen.Extract(z, c))
	if got >= len(raw) {
		t.Errorf("simplified contours (%d) should be fewer than raw (%d)", got, len(raw))
	}
	if got == 0 {
		t.Error("simplification removed every segment")
	}
}

// TestRDPMonotonic guards the slice-aliasing bug that made Douglas-Peucker
// return MORE points for a larger epsilon. The two recursive halves are
// sub-slices of one backing array, so appending into the left half could
// overwrite points the right half still referenced.
func TestRDPMonotonic(t *testing.T) {
	pts := make([]mapgen.Vec3, 0, 200)
	for i := 0; i < 200; i++ {
		f := float64(i)
		// A wobbly diagonal: enough structure that epsilon actually bites.
		pts = append(pts, mapgen.Vec3{X: f, Y: f + math.Sin(f/3)*4})
	}
	prev := len(mapgen.RDP(pts, 0.1))
	for _, eps := range []float64{0.5, 1, 2, 5, 10} {
		n := len(mapgen.RDP(pts, eps))
		if n > prev {
			t.Errorf("eps=%v produced %d points, more than the %d at the previous smaller eps",
				eps, n, prev)
		}
		prev = n
	}
}

// TestSilhouette_IsBandedByHeight is the guard for the change that made dense
// dungeons usable. A flat silhouette rasterises every walkable face onto one
// grid, so tunnels crossing at different heights merge into a single blob and
// every segment comes out at one height — which is why those zones looked
// featureless AND why the depth control did nothing on them.
//
// Asserting distinct heights rather than a segment count: the count is a tuning
// artefact, but "the output carries real height information" is the property
// that has to hold.
func TestSilhouette_IsBandedByHeight(t *testing.T) {
	requireClient(t)

	for _, zone := range []string{"akheva", "necropolis"} {
		t.Run(zone, func(t *testing.T) {
			z, err := mapgen.LoadZone(clientDir, zone)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			c := mapgen.Classify(z)
			if c.Technique != mapgen.TechniqueSilhouette {
				t.Fatalf("technique: got %q, want silhouette", c.Technique)
			}
			segs := mapgen.Extract(z, c)
			if len(segs) == 0 {
				t.Fatal("no segments")
			}

			levels := map[int]int{}
			for _, s := range segs {
				levels[int(s.A.Z)]++
			}
			if len(levels) < 4 {
				t.Errorf("distinct heights: got %d, want >= 4 — output looks flat, "+
					"so the depth control will do nothing here", len(levels))
			}
			// No band should hold nearly everything; that would mean the split
			// happened but did not actually separate the geometry.
			for zv, n := range levels {
				if float64(n) > 0.9*float64(len(segs)) {
					t.Errorf("height %d holds %d/%d segments — bands are not separating",
						zv, n, len(segs))
				}
			}
		})
	}
}

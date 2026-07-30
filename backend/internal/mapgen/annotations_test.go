package mapgen

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/maps"
)

// withAnnotations swaps the embedded corpus for the duration of a test.
func withAnnotations(t *testing.T, body string) {
	t.Helper()
	orig := annotationsJSON
	annotationsJSON = []byte(body)
	t.Cleanup(func() { annotationsJSON = orig })
}

const oneWall = `{"zones":{"akheva":[{
	"x": 100, "y": -200, "z": 12,
	"category": "wall",
	"label": "Fake wall — walk through",
	"evidence": "verified in game"
}]}}`

func TestShippedAnnotationsAreValid(t *testing.T) {
	// The real file, against the real rules. Catches a bad hand edit at test
	// time rather than at maps.db build time.
	if _, err := LoadAnnotations(nil); err != nil {
		t.Fatalf("shipped annotations.json is invalid: %v", err)
	}
	// The readme lives alongside the data; make sure it did not get renamed out
	// of the schema, since it is the only place the evidence rule is explained.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(annotationsJSON, &probe); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"_readme", "zones"} {
		if _, ok := probe[key]; !ok {
			t.Errorf("annotations.json has no %q key", key)
		}
	}
}

func TestLoadAnnotationsConvertsToMapSpace(t *testing.T) {
	withAnnotations(t, oneWall)
	pois, err := LoadAnnotations(map[string]bool{"akheva": true})
	if err != nil {
		t.Fatalf("LoadAnnotations: %v", err)
	}
	if len(pois) != 1 {
		t.Fatalf("got %d POIs, want 1", len(pois))
	}
	p := pois[0]
	// Same negation as the geometry pipeline: map_f1 = -game_x, map_f2 = -game_y.
	if p.X != -100 || p.Y != 200 || p.Z != 12 {
		t.Errorf("got (%d,%d,%d), want (-100,200,12)", p.X, p.Y, p.Z)
	}
	if p.Source != AnnotationSourceResearch {
		t.Errorf("Source = %q, want %q", p.Source, AnnotationSourceResearch)
	}
	if p.Category != CategoryWall {
		t.Errorf("Category = %q, want %q", p.Category, CategoryWall)
	}
}

// Every rejection below exists so bad data stops a build instead of shipping.
func TestLoadAnnotationsRejects(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
		known               map[string]bool
	}{
		{
			// A typo'd zone would otherwise silently drop rows somebody
			// researched by hand — the worst possible failure for this data.
			name:    "unknown zone",
			body:    `{"zones":{"akheeva":[{"x":1,"y":1,"z":0,"category":"wall","label":"x","evidence":"y"}]}}`,
			known:   map[string]bool{"akheva": true},
			wantErr: "unknown zone",
		},
		{
			name:    "unknown category",
			body:    `{"zones":{"akheva":[{"x":1,"y":1,"z":0,"category":"secret","label":"x","evidence":"y"}]}}`,
			known:   map[string]bool{"akheva": true},
			wantErr: "unknown category",
		},
		{
			name:    "missing label",
			body:    `{"zones":{"akheva":[{"x":1,"y":1,"z":0,"category":"wall","evidence":"y"}]}}`,
			known:   map[string]bool{"akheva": true},
			wantErr: "label is required",
		},
		{
			// The discipline that keeps the corpus defensible; enforced, not
			// merely documented.
			name:    "missing evidence",
			body:    `{"zones":{"akheva":[{"x":1,"y":1,"z":0,"category":"wall","label":"x"}]}}`,
			known:   map[string]bool{"akheva": true},
			wantErr: "evidence is required",
		},
		{
			// 0,0 is the "unset row" sentinel shared with the DB generators.
			name:    "origin coordinates",
			body:    `{"zones":{"akheva":[{"x":0,"y":0,"z":0,"category":"wall","label":"x","evidence":"y"}]}}`,
			known:   map[string]bool{"akheva": true},
			wantErr: "not a real position",
		},
		{
			name:    "malformed json",
			body:    `{"zones":`,
			known:   nil,
			wantErr: "parse annotations.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withAnnotations(t, tc.body)
			_, err := LoadAnnotations(tc.known)
			if err == nil {
				t.Fatalf("LoadAnnotations succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadAnnotationsIsOrderStable(t *testing.T) {
	// Two zones, so map iteration order could differ between runs. A stable
	// order keeps a maps.db rebuild diffable.
	withAnnotations(t, `{"zones":{
		"velketor":[{"x":1,"y":1,"z":0,"category":"note","label":"b","evidence":"e"}],
		"akheva":  [{"x":1,"y":1,"z":0,"category":"note","label":"a","evidence":"e"}]
	}}`)
	known := map[string]bool{"akheva": true, "velketor": true}
	for i := 0; i < 8; i++ {
		pois, err := LoadAnnotations(known)
		if err != nil {
			t.Fatalf("LoadAnnotations: %v", err)
		}
		if len(pois) != 2 || pois[0].Zone != "akheva" || pois[1].Zone != "velketor" {
			t.Fatalf("run %d: order = %v, want akheva then velketor", i, []string{pois[0].Zone, pois[1].Zone})
		}
	}
}

// TestExportRoundTripsBackToTheSamePlace closes the loop between a user's
// exported markers (internal/maps) and this package's importer.
//
// The two halves negate coordinates in opposite directions, in different
// packages, with no shared code — exactly the arrangement where a sign error
// hides. And it would hide *quietly*: a mirrored submission still parses, still
// draws, and lands somewhere plausible on the wrong side of the zone. This is
// the only test that would catch it.
func TestExportRoundTripsBackToTheSamePlace(t *testing.T) {
	const wantX, wantY, wantZ = -100, 200, 12
	exported := maps.BuildExport([]maps.UserAnnotation{{
		Zone: "akheva", X: wantX, Y: wantY, Z: wantZ,
		Category: CategoryWall, Label: "Fake wall",
	}})

	// A reviewer fills in evidence — the one manual step, by design.
	for zone := range exported.Zones {
		for i := range exported.Zones[zone] {
			exported.Zones[zone][i].Evidence = "verified in game"
		}
	}
	body, err := json.Marshal(map[string]any{"zones": exported.Zones})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	withAnnotations(t, string(body))
	pois, err := LoadAnnotations(map[string]bool{"akheva": true})
	if err != nil {
		t.Fatalf("LoadAnnotations on a filled-in export: %v", err)
	}
	if len(pois) != 1 {
		t.Fatalf("got %d POIs, want 1", len(pois))
	}
	if pois[0].X != wantX || pois[0].Y != wantY || pois[0].Z != wantZ {
		t.Errorf("round trip moved the marker: (%d,%d,%d), want (%d,%d,%d)",
			pois[0].X, pois[0].Y, pois[0].Z, wantX, wantY, wantZ)
	}
}

// TestRawExportIsRejected proves the evidence gate is real: an export merged
// without a human writing down where the fact came from must fail the build, not
// ship unverified markers to every user.
func TestRawExportIsRejected(t *testing.T) {
	exported := maps.BuildExport([]maps.UserAnnotation{{
		Zone: "akheva", X: -100, Y: 200, Z: 12,
		Category: CategoryWall, Label: "Fake wall",
	}})
	body, err := json.Marshal(map[string]any{"zones": exported.Zones})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	withAnnotations(t, string(body))
	if _, err := LoadAnnotations(map[string]bool{"akheva": true}); err == nil {
		t.Fatal("a raw export loaded successfully — the evidence gate does nothing")
	} else if !strings.Contains(err.Error(), "evidence is required") {
		t.Errorf("error = %q, want it to mention evidence", err)
	}
}

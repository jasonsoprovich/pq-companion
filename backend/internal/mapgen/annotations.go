package mapgen

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

//go:embed annotations.json
var annotationsJSON []byte

// Annotation categories. Deliberately few: a category earns its own map layer,
// and a layer nobody toggles is worse than a clear label. Specifics belong in
// the label — "Fake wall — ...", "One-way wall — ...", "Deadly fall — ...".
const (
	// CategoryWall is a wall that does not behave like one: illusory, one-way,
	// or passable in a way the geometry cannot show. This is the single largest
	// thing hand-drawn maps have that we cannot derive — collision meshes record
	// where surfaces are, not which ones lie.
	CategoryWall = "wall"
	// CategoryHazard is somewhere that kills or traps you and is not a trap NPC:
	// a fall with no way back up, a lava channel, a one-way drop.
	CategoryHazard = "hazard"
	// CategoryNote is anything else worth marking — a camp, a safe spot, a
	// landmark that makes directions legible.
	CategoryNote = "note"
)

// AnnotationSourceResearch tags every row from this file.
//
// Distinct from db:* on purpose: a quarm.db data release regenerates db:* rows
// from scratch, and these must survive that untouched (§7.3). It is also
// distinct from "community", so a submission can be accepted into the shipped
// set without losing the fact that it came from outside.
const AnnotationSourceResearch = "research"

// Annotation is one hand-researched fact about a zone.
type Annotation struct {
	// X, Y, Z are game coordinates, matching spawn2. See annotations.json for
	// the /loc ordering caveat.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	// Category is one of the Category* constants above.
	Category string `json:"category"`
	Label    string `json:"label"`
	// Evidence records where the fact was confirmed. Required — see the rationale
	// in annotations.json. An annotation without provenance is a rumour, and a
	// rumour that draws confidently on a map is worse than a blank space.
	Evidence string `json:"evidence"`
}

type annotationFile struct {
	Zones map[string][]Annotation `json:"zones"`
}

var annotationCategories = map[string]bool{
	CategoryWall:   true,
	CategoryHazard: true,
	CategoryNote:   true,
}

// LoadAnnotations parses the embedded annotation corpus into POIs.
//
// knownZones gates zone names so a typo cannot silently drop rows: an
// annotation filed under a misspelled zone would otherwise vanish with no
// error, which is the worst outcome for data somebody researched by hand. Pass
// nil to skip that check.
//
// Errors are returned rather than logged and swallowed. Unlike the curated
// tradeskill paths, which degrade to "not curated yet", a malformed annotation
// file means the build is about to ship a map missing information a person
// deliberately added — that should stop the build, not produce a warning nobody
// reads.
func LoadAnnotations(knownZones map[string]bool) ([]POI, error) {
	var f annotationFile
	if err := json.Unmarshal(annotationsJSON, &f); err != nil {
		return nil, fmt.Errorf("parse annotations.json: %w", err)
	}

	// Sorted so output ordering is stable across runs and a maps.db rebuild
	// produces a comparable diff.
	zones := make([]string, 0, len(f.Zones))
	for z := range f.Zones {
		zones = append(zones, z)
	}
	sort.Strings(zones)

	var out []POI
	for _, zone := range zones {
		if knownZones != nil && !knownZones[zone] {
			return nil, fmt.Errorf("annotations.json: unknown zone %q", zone)
		}
		for i, a := range f.Zones[zone] {
			where := fmt.Sprintf("annotations.json: %s[%d]", zone, i)
			if !annotationCategories[a.Category] {
				return nil, fmt.Errorf("%s: unknown category %q", where, a.Category)
			}
			if a.Label == "" {
				return nil, fmt.Errorf("%s: label is required", where)
			}
			if a.Evidence == "" {
				return nil, fmt.Errorf("%s: evidence is required — see the readme in that file", where)
			}
			x, y, z, ok := toMapPoint(a.X, a.Y, a.Z)
			if !ok {
				return nil, fmt.Errorf("%s: coordinates (%.1f, %.1f) are not a real position",
					where, a.X, a.Y)
			}
			if math.Abs(float64(x)) > coordMax || math.Abs(float64(y)) > coordMax {
				return nil, fmt.Errorf("%s: coordinates (%.1f, %.1f) are out of range",
					where, a.X, a.Y)
			}
			out = append(out, POI{
				Zone: zone, X: x, Y: y, Z: z,
				Category: a.Category, Label: a.Label,
				Source: AnnotationSourceResearch,
			})
		}
	}
	return out, nil
}

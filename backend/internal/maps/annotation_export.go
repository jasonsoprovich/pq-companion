package maps

import "sort"

// ExportedAnnotation is one row in the shape internal/mapgen/annotations.json
// reads.
//
// Deliberately the same format the shipped corpus uses, rather than a bespoke
// export: that is what makes this a submission path rather than a backup. A
// user's export can be reviewed and merged into annotations.json with no
// conversion step, so the distance between "I marked this on my map" and "every
// player gets this marked" is a pull request.
type ExportedAnnotation struct {
	// Game coordinates, negated back out of map space, because that is the
	// convention annotations.json documents and what a researcher reads from
	// /loc. Storing map space and exporting game space keeps the renderer simple
	// and the file human-editable, which are the two things that matter.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`

	Category string `json:"category"`
	Label    string `json:"label"`
	// Evidence is exported EMPTY on purpose, and mapgen rejects an empty one.
	//
	// So a raw export cannot be merged without a human writing down where the
	// fact was confirmed. That is the whole discipline of the corpus, and making
	// it a hard failure rather than a convention is what stops "I think there's
	// a wall here" becoming a line on 178 zones' worth of shipped maps.
	Evidence string `json:"evidence"`
}

// AnnotationExport is the top-level export document.
type AnnotationExport struct {
	Readme []string                        `json:"_readme"`
	Zones  map[string][]ExportedAnnotation `json:"zones"`
}

// exportReadme travels with the file so it is still explicable after being
// pasted into a Discord thread, detached from this repo.
var exportReadme = []string{
	"PQ Companion map annotations, exported from a user's own markers.",
	"",
	"Coordinates are GAME coordinates (as stored in quarm.db). EQ's /loc prints",
	"them in the order y, x, z.",
	"",
	"Every 'evidence' field is intentionally blank. Before this can be merged into",
	"backend/internal/mapgen/annotations.json, each row needs a note saying where",
	"the fact was confirmed — verified in game, or corroborated by two independent",
	"public sources. The build rejects an empty evidence string, so a blind merge",
	"fails rather than shipping unverified markers to everyone.",
}

// BuildExport converts stored annotations into the submission format.
func BuildExport(rows []UserAnnotation) AnnotationExport {
	byZone := map[string][]ExportedAnnotation{}
	for _, a := range rows {
		byZone[a.Zone] = append(byZone[a.Zone], ExportedAnnotation{
			// Reverse of the pipeline's map_f1 = -game_x, map_f2 = -game_y.
			X:        float64(-a.X),
			Y:        float64(-a.Y),
			Z:        float64(a.Z),
			Category: a.Category,
			Label:    a.Label,
			Evidence: "",
		})
	}
	// Stable ordering within each zone so a re-export produces a comparable diff.
	for _, list := range byZone {
		sort.SliceStable(list, func(i, j int) bool { return list[i].Label < list[j].Label })
	}
	return AnnotationExport{Readme: exportReadme, Zones: byZone}
}

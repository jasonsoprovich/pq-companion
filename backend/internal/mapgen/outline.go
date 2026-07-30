package mapgen

import "math"

// The outline layer: one clean, single-weight line drawing per zone, in the
// same visual language for every zone in the game.
//
// Why a third layer rather than tuning the existing ones: the classifier picks
// whichever technique describes a zone's shape best, which is right for
// fidelity and wrong for consistency. Contour zones come out looking like a
// topographic survey, silhouette zones like a stack of tracings, boundary zones
// like a floor plan — three different visual languages, so moving between zones
// feels like moving between three different apps. An outline layer gives up
// some information in exchange for looking the same everywhere, which is what
// makes a map readable at a glance.
//
// Two ingredients do most of the cleanup work:
//
//   - Never contours. Elevation hatching is the single biggest source of visual
//     noise; it covers the whole footprint in lines that are not walls and
//     cannot be walked along.
//   - Coarse Z bands. The detailed layers slice at ~40 units (one storey) and
//     draw every band at once, which is what produces the stacked-tracings
//     look. Outline mode slices far coarser, so a zone reads as a plan with a
//     couple of levels rather than a dozen superimposed ones.
const (
	// outlineBandTarget slices at roughly four storeys rather than one. Fungus
	// Grove drops from 14 bands to 4, which is the difference between an
	// impression of overlapping caves and a plan you can trace a route on.
	outlineBandTarget = 160.0
	// outlineMaxBands is deliberately small. Past about four superimposed
	// outlines the drawing stops reading as a plan no matter how clean each
	// individual band is.
	outlineMaxBands = 4
	// outlineRDPEpsilon simplifies harder than the detail layers. At 4 units a
	// cave wall keeps its shape while losing the marching-squares ripple that
	// makes our lines look furry next to a hand-drawn map.
	outlineRDPEpsilon = 4.0
	// outlineMinExtent drops any chain whose bounding-box diagonal is smaller
	// than this. These are the confetti — single ledges, mesh slivers, one-cell
	// islands — that a cartographer would simply not draw. Set in world units,
	// so it scales with the zone rather than the zoom.
	outlineMinExtent = 24.0
)

// ExtractOutline returns the clean outline layer for a zone.
//
// The technique split follows occupancy, not the classifier's full verdict,
// because only one question matters here: does a flat footprint of the walkable
// area mean anything?
//
//   - Sparse zones (caves, dungeons, corridor networks) — yes. The walkable
//     silhouette IS the map, and coarse bands keep stacked passages legible.
//   - Dense zones (open terrain, built interiors) — no. Walkable area covers
//     the whole footprint, so its silhouette degenerates into a bounding blob.
//     Boundary extraction is what carries information there: walls, building
//     footprints, the edges of ledges and water.
func ExtractOutline(z *Zone, c Classification) []Segment {
	if c.Occupancy < outlineSilhouetteMaxOcc {
		return z.SilhouetteCoarse()
	}
	chains := dropSmallChains(Chain(z.BoundaryEdges()), outlineMinExtent)
	return SimplifyRDP(chains, outlineRDPEpsilon)
}

// outlineSilhouetteMaxOcc is the occupancy above which a flat walkable
// footprint stops being informative. Deliberately more generous than
// Classify's sparseOccThreshold (0.35): that threshold decides whether a
// silhouette is the *best* description of a zone, whereas here it only needs to
// beat boundary extraction at looking clean, and it does that comfortably up to
// about half coverage.
const outlineSilhouetteMaxOcc = 0.50

// SilhouetteCoarse is Silhouette with outline-mode banding and simplification.
func (z *Zone) SilhouetteCoarse() []Segment {
	return z.silhouette(outlineBandTarget, outlineMaxBands,
		outlineRDPEpsilon, outlineMinExtent)
}

// dropSmallChains removes polylines too small to be worth drawing, measured by
// bounding-box diagonal so a long thin corridor survives but a stray triangle
// does not.
func dropSmallChains(chains [][]Vec3, minExtent float64) [][]Vec3 {
	out := chains[:0:0]
	for _, ch := range chains {
		if len(ch) < 2 {
			continue
		}
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for _, p := range ch {
			minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
			maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
		}
		if math.Hypot(maxX-minX, maxY-minY) >= minExtent {
			out = append(out, ch)
		}
	}
	return out
}

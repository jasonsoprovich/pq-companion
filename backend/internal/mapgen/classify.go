package mapgen

// Technique is the extraction method that suits a zone's shape.
type Technique string

const (
	// TechniqueContours suits continuous outdoor terrain.
	TechniqueContours Technique = "contours"
	// TechniqueSilhouette suits corridor and cave dungeons.
	TechniqueSilhouette Technique = "silhouette"
	// TechniqueBoundary suits structured interiors built from discrete floors.
	TechniqueBoundary Technique = "boundary"
)

// Classification records a zone's shape metrics and the technique chosen from
// them. The metrics are kept so the decision stays auditable rather than hidden
// behind a bare verdict.
type Classification struct {
	Zone           string
	Triangles      int
	WalkableFaces  int
	Occupancy      float64
	BoundaryDensit float64
	ZSpan          float64
	Technique      Technique
	// Overridden reports that the technique came from the manual table rather
	// than the thresholds.
	Overridden bool
}

// Classification thresholds, calibrated over all 178 Quarm zones (see
// docs/maps-feasibility.md 5b.6).
//
// Boundary density — unshared floor edges per floor face — is the primary
// signal because it is strongly bimodal: 71 zones cluster below 0.30, 87 above
// 0.60, with a sparse valley of 18 between. It measures the actual failure
// mode, since a continuous terrain mesh shares every interior edge.
//
// Occupancy is only a secondary split within the dense cluster. It looked
// bimodal on a five-zone sample but is roughly uniform across the full corpus,
// so it cannot carry the primary decision.
const (
	bdThreshold  = 0.45
	occThreshold = 0.60
)

// techniqueOverrides pins zones the thresholds get wrong.
//
// These sit in the 0.30-0.60 boundary-density valley, where the metric is
// genuinely ambiguous. A short hand-checked list is more honest than tuning
// thresholds until they happen to agree with 18 outliers — and tuning that
// finely would misclassify zones currently handled correctly.
var techniqueOverrides = map[string]Technique{
	// Indoor dungeon: low occupancy says sparse corridors, but its boundary
	// density falls just under the terrain threshold.
	"warrens": TechniqueSilhouette,
	// A built structure, not terrain.
	"arena": TechniqueBoundary,
	// Large indoor temple with a tall Z span.
	"ssratemple": TechniqueBoundary,
}

// Classify measures a zone and picks an extraction technique.
func Classify(z *Zone) Classification {
	c := Classification{Zone: z.ShortName, Triangles: len(z.Triangles)}

	faces := z.WalkableFaces()
	c.WalkableFaces = len(faces)
	if len(faces) == 0 {
		c.Technique = TechniqueBoundary
		return c
	}

	c.Occupancy = z.Rasterize().Occupancy()
	c.BoundaryDensit = float64(len(z.BoundaryEdges())) / float64(len(faces))

	minZ, maxZ := z.Vertices[0].Z, z.Vertices[0].Z
	for _, v := range z.Vertices {
		if v.Z < minZ {
			minZ = v.Z
		}
		if v.Z > maxZ {
			maxZ = v.Z
		}
	}
	c.ZSpan = maxZ - minZ

	switch {
	case c.BoundaryDensit < bdThreshold:
		c.Technique = TechniqueContours
	case c.Occupancy < occThreshold:
		c.Technique = TechniqueSilhouette
	default:
		c.Technique = TechniqueBoundary
	}

	if t, ok := techniqueOverrides[z.ShortName]; ok {
		c.Technique, c.Overridden = t, true
	}
	return c
}

// contourInterval picks a slice spacing that keeps a zone's contour count
// readable regardless of how much vertical range it covers.
func contourInterval(zSpan float64) float64 {
	switch {
	case zSpan > 2000:
		return 100
	case zSpan > 1000:
		return 50
	case zSpan > 400:
		return 30
	default:
		return 20
	}
}

// contourRDPEpsilon simplifies contour lines. Contours come out of
// triangle-plane intersection as one segment per crossed triangle, which is far
// finer than anything visible: at 0.5 world units the error is well under a
// pixel at any usable zoom, and it roughly halves the segment count. Contours
// are the bulk of the corpus, so this is where size is won.
const contourRDPEpsilon = 0.5

// Extract runs the technique the classification selected and returns the map
// segments for the zone.
func Extract(z *Zone, c Classification) []Segment {
	switch c.Technique {
	case TechniqueContours:
		raw := z.Contours(contourInterval(c.ZSpan))
		// Chaining keys on Z, so separate elevation bands never join.
		return SimplifyRDP(Chain(raw), contourRDPEpsilon)
	case TechniqueSilhouette:
		return z.Silhouette()
	default:
		return SimplifyCollinear(Chain(z.BoundaryEdges()), 0.03)
	}
}

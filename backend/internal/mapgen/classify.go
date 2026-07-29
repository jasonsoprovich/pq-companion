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

	// sparseOccThreshold / sparseBDFloor catch dungeons and cave networks whose
	// boundary density lands in the terrain range.
	//
	// Established by rendering all 18 valley zones three ways: echo, powater,
	// netherbian, paludal, thedeep and warrens all took the terrain branch on
	// boundary density alone, and contours came out fragmentary or dashed on
	// every one of them while silhouette was clean. What they share is very low
	// occupancy — walkable area covers a quarter of the footprint or less, which
	// no open terrain zone does.
	//
	// The BD floor is what keeps fungusgrove on contours. It is the one
	// low-occupancy zone whose contours render continuous and informative, and
	// its density (0.24) sits well below the others (0.34-0.38) — a genuinely
	// more continuous mesh, not a corridor network.
	sparseOccThreshold = 0.35
	sparseBDFloor      = 0.30
)

// techniqueOverrides pins the zones no threshold gets right.
//
// Both are high-occupancy built structures whose boundary density happens to
// fall in the terrain range, so neither the sparse rule nor the density rule
// reaches them. Verified by rendering all three techniques: for each, contours
// are scattered fragments and the silhouette is a bare outline, while boundary
// extraction produces the real floor plan.
//
// warrens used to need an entry; the sparse rule now classifies it directly,
// which is the better outcome — a rule that generalises beats a pinned name.
var techniqueOverrides = map[string]Technique{
	// The arena ring, its alcoves and the entrance tunnel. occ=0.78, bnd=0.38.
	"arena": TechniqueBoundary,
	// Ssraeshza Temple's multi-level floor plan. occ=0.69, bnd=0.43.
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
	case c.Occupancy < sparseOccThreshold && c.BoundaryDensit >= sparseBDFloor:
		c.Technique = TechniqueSilhouette // sparse corridor / cave network
	case c.BoundaryDensit < bdThreshold:
		c.Technique = TechniqueContours // continuous terrain surface
	case c.Occupancy < occThreshold:
		c.Technique = TechniqueSilhouette
	default:
		c.Technique = TechniqueBoundary // discrete floor slabs
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

package mapgen

import "math"

// Silhouette settings. Tuned against the Python prototype; see
// docs/maps-feasibility.md 5b.5.
const (
	// silTargetCells caps the long edge of the occupancy grid.
	silTargetCells = 1400
	// silMinCell keeps small zones from being rasterised absurdly fine.
	silMinCell = 1.0
	// silCloseUnits bridges seams between adjacent floor slabs. Deliberately
	// small: sweeping this from 0 to 8 units moved occupancy by only 1-3%, so
	// it closes hairline gaps and nothing more. RDP does the real reduction.
	silCloseUnits = 2.0
	// silRDPEpsilon collapses marching-squares staircases into straight runs.
	silRDPEpsilon = 1.5
)

// OccupancyGrid is a rasterised footprint of a zone's walkable area.
type OccupancyGrid struct {
	W, H             int
	Cell             float64 // world units per cell
	OriginX, OriginY float64
	Pad              int
	Filled           []bool
}

// Occupancy returns the fraction of grid cells covered by walkable surface.
//
// This is the primary input to Classify: it separates zones where walkable area
// is sparse (corridors and caves, where a silhouette is meaningful) from zones
// where it covers everything (open ground, where the silhouette degenerates
// into the zone footprint and says nothing).
func (g *OccupancyGrid) Occupancy() float64 {
	if len(g.Filled) == 0 {
		return 0
	}
	n := 0
	for _, f := range g.Filled {
		if f {
			n++
		}
	}
	return float64(n) / float64(len(g.Filled))
}

// gridFrame is the shared raster coordinate frame for a zone.
//
// Every Z band must rasterise into the SAME frame, or the bands come out at
// different scales and origins and will not composite.
type gridFrame struct {
	minX, minY float64
	cell       float64
	W, H, pad  int
	reps       int
}

func (z *Zone) gridFrame(faces []int) gridFrame {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, i := range faces {
		t := z.Triangles[i]
		for _, v := range [3]Vec3{z.Vertices[t.A], z.Vertices[t.B], z.Vertices[t.C]} {
			minX, minY = math.Min(minX, v.X), math.Min(minY, v.Y)
			maxX, maxY = math.Max(maxX, v.X), math.Max(maxY, v.Y)
		}
	}
	span := math.Max(maxX-minX, maxY-minY)
	cell := math.Max(silMinCell, span/silTargetCells)

	// Pad by more than the dilation radius. Without this the grown region
	// reaches the image border, gets clipped, and marching squares emits open
	// contours that run off the edge instead of closing.
	reps := int(silCloseUnits / cell)
	if reps > 10 {
		reps = 10
	}
	pad := reps + 3
	return gridFrame{
		minX: minX, minY: minY, cell: cell,
		W:   int((maxX-minX)/cell) + 2*pad,
		H:   int((maxY-minY)/cell) + 2*pad,
		pad: pad, reps: reps,
	}
}

// rasterizeInto fills a grid in the given frame from a subset of faces.
func (z *Zone) rasterizeInto(f gridFrame, faces []int) *OccupancyGrid {
	g := &OccupancyGrid{
		W: f.W, H: f.H, Cell: f.cell,
		OriginX: f.minX, OriginY: f.minY, Pad: f.pad,
		Filled: make([]bool, f.W*f.H),
	}
	toGrid := func(v Vec3) (float64, float64) {
		return (v.X-f.minX)/f.cell + float64(f.pad), (v.Y-f.minY)/f.cell + float64(f.pad)
	}
	for _, i := range faces {
		t := z.Triangles[i]
		ax, ay := toGrid(z.Vertices[t.A])
		bx, by := toGrid(z.Vertices[t.B])
		cx, cy := toGrid(z.Vertices[t.C])
		g.fillTriangle(ax, ay, bx, by, cx, cy)
	}
	for i := 0; i < f.reps; i++ {
		g.dilate()
	}
	for i := 0; i < f.reps; i++ {
		g.erode()
	}
	return g
}

// Rasterize projects every walkable face onto one flat occupancy grid.
//
// Flat on purpose: Classify() uses the occupancy ratio, and its thresholds were
// calibrated against the flattened footprint over all 178 zones. Extraction
// uses the banded path instead.
func (z *Zone) Rasterize() *OccupancyGrid {
	faces := z.WalkableFaces()
	if len(faces) == 0 {
		return &OccupancyGrid{}
	}
	return z.rasterizeInto(z.gridFrame(faces), faces)
}

// fillTriangle rasterises one triangle by testing cell centres inside its
// bounding box against the triangle's half-planes.
func (g *OccupancyGrid) fillTriangle(ax, ay, bx, by, cx, cy float64) {
	lo := func(v ...float64) int {
		m := v[0]
		for _, x := range v[1:] {
			m = math.Min(m, x)
		}
		return int(math.Floor(m))
	}
	hi := func(v ...float64) int {
		m := v[0]
		for _, x := range v[1:] {
			m = math.Max(m, x)
		}
		return int(math.Ceil(m))
	}
	x0, x1 := lo(ax, bx, cx), hi(ax, bx, cx)
	y0, y1 := lo(ay, by, cy), hi(ay, by, cy)
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 >= g.W {
		x1 = g.W - 1
	}
	if y1 >= g.H {
		y1 = g.H - 1
	}

	area := (bx-ax)*(cy-ay) - (by-ay)*(cx-ax)
	if math.Abs(area) < 1e-12 {
		return
	}
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			w0 := ((bx-ax)*(py-ay) - (by-ay)*(px-ax)) / area
			w1 := ((cx-bx)*(py-by) - (cy-by)*(px-bx)) / area
			w2 := ((ax-cx)*(py-cy) - (ay-cy)*(px-cx)) / area
			if w0 >= 0 && w1 >= 0 && w2 >= 0 {
				g.Filled[y*g.W+x] = true
			}
		}
	}
}

func (g *OccupancyGrid) dilate() { g.morph(true) }
func (g *OccupancyGrid) erode()  { g.morph(false) }

// morph applies a 3x3 max (dilate) or min (erode) filter.
func (g *OccupancyGrid) morph(max bool) {
	out := make([]bool, len(g.Filled))
	for y := 0; y < g.H; y++ {
		for x := 0; x < g.W; x++ {
			v := !max // dilate seeds false and ORs; erode seeds true and ANDs
			for dy := -1; dy <= 1 && ((max && !v) || (!max && v)); dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					var f bool
					if nx >= 0 && nx < g.W && ny >= 0 && ny < g.H {
						f = g.Filled[ny*g.W+nx]
					}
					if max && f {
						v = true
						break
					}
					if !max && !f {
						v = false
						break
					}
				}
			}
			out[y*g.W+x] = v
		}
	}
	g.Filled = out
}

// marchingSquaresCases maps a corner bitmask to the edge midpoints to connect.
// Edge codes: 0=top 1=right 2=bottom 3=left. Cases 5 and 10 are saddles.
var marchingSquaresCases = [16][][2]int{
	0:  nil,
	1:  {{3, 2}},
	2:  {{2, 1}},
	3:  {{3, 1}},
	4:  {{0, 1}},
	5:  {{3, 0}, {2, 1}},
	6:  {{0, 2}},
	7:  {{3, 0}},
	8:  {{3, 0}},
	9:  {{0, 2}},
	10: {{3, 2}, {0, 1}},
	11: {{0, 1}},
	12: {{3, 1}},
	13: {{2, 1}},
	14: {{3, 2}},
	15: nil,
}

// March traces the occupied region's boundary with marching squares over edge
// midpoints, which yields 45-degree diagonals instead of a pure axis-aligned
// staircase.
func (g *OccupancyGrid) March() []Segment {
	if g.W < 2 || g.H < 2 {
		return nil
	}
	wx := func(gx float64) float64 { return g.OriginX + (gx-float64(g.Pad))*g.Cell }
	wy := func(gy float64) float64 { return g.OriginY + (gy-float64(g.Pad))*g.Cell }
	at := func(x, y int) int {
		if g.Filled[y*g.W+x] {
			return 1
		}
		return 0
	}

	var out []Segment
	for y := 0; y < g.H-1; y++ {
		for x := 0; x < g.W-1; x++ {
			idx := at(x, y)*8 + at(x+1, y)*4 + at(x+1, y+1)*2 + at(x, y+1)
			cases := marchingSquaresCases[idx]
			if cases == nil {
				continue
			}
			mid := [4]Vec3{
				{X: wx(float64(x) + 0.5), Y: wy(float64(y))},
				{X: wx(float64(x + 1)), Y: wy(float64(y) + 0.5)},
				{X: wx(float64(x) + 0.5), Y: wy(float64(y + 1))},
				{X: wx(float64(x)), Y: wy(float64(y) + 0.5)},
			}
			for _, c := range cases {
				out = append(out, Segment{A: mid[c[0]], B: mid[c[1]]})
			}
		}
	}
	return out
}

// Z-banding parameters.
const (
	// silBandTarget is the height a band aims to cover. Roughly one storey: EQ
	// floors sit ~20-40 units apart and a tunnel is ~15-30 units tall, so this
	// separates stacked passages without shredding a single floor into slices.
	silBandTarget = 40.0
	// silMaxBands bounds cost and output size on very tall zones.
	silMaxBands = 14
	// silMinBandedSpan is the Z range below which banding buys nothing.
	silMinBandedSpan = 80.0
)

type zBand struct{ lo, hi, mid float64 }

// zBands divides a zone's walkable heights into bands of roughly bandTarget
// height, capped at maxBands.
func (z *Zone) zBands(faces []int, bandTarget float64, maxBands int) []zBand {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, i := range faces {
		t := z.Triangles[i]
		for _, v := range [3]Vec3{z.Vertices[t.A], z.Vertices[t.B], z.Vertices[t.C]} {
			lo, hi = math.Min(lo, v.Z), math.Max(hi, v.Z)
		}
	}
	span := hi - lo
	if !(span > silMinBandedSpan) {
		return []zBand{{lo: lo, hi: hi, mid: (lo + hi) / 2}}
	}
	n := int(math.Ceil(span / bandTarget))
	if n > maxBands {
		n = maxBands
	}
	if n < 2 {
		n = 2
	}
	h := span / float64(n)
	out := make([]zBand, 0, n)
	for i := 0; i < n; i++ {
		b := zBand{lo: lo + float64(i)*h, hi: lo + float64(i+1)*h}
		b.mid = (b.lo + b.hi) / 2
		out = append(out, b)
	}
	return out
}

// Silhouette returns the outline of the walkable area, sliced by height.
//
// Slicing is what makes this usable in zones with stacked passages. Rasterising
// every face onto one flat grid merges tunnels that cross at different heights
// into a single undifferentiated blob — which is both why dense dungeons looked
// featureless and why the depth control did nothing on them: a flat pass emits
// every segment at one height, so there is nothing for a height filter to
// separate. Each band is rasterised independently and its segments carry that
// band's height, so the renderer can fade by depth like a hand-drawn map.
//
// Bands share one grid frame, so they line up exactly when composited.
func (z *Zone) Silhouette() []Segment {
	return z.silhouette(silBandTarget, silMaxBands, silRDPEpsilon, 0)
}

// silhouette is the parameterised implementation shared by the detailed layer
// and the coarse outline layer. minExtent of 0 disables small-chain culling.
func (z *Zone) silhouette(bandTarget float64, maxBands int, rdpEps, minExtent float64) []Segment {
	faces := z.WalkableFaces()
	if len(faces) == 0 {
		return nil
	}
	frame := z.gridFrame(faces)
	bands := z.zBands(faces, bandTarget, maxBands)

	var out []Segment
	for _, b := range bands {
		// Assign each face to the band holding its centroid. No overlap: a ramp
		// crossing a boundary breaks into two pieces, each at its true height,
		// which reads correctly once the renderer fades by depth. Overlapping
		// bands would instead duplicate that geometry at two heights.
		sub := make([]int, 0, len(faces)/len(bands)+8)
		for _, i := range faces {
			t := z.Triangles[i]
			mid := (z.Vertices[t.A].Z + z.Vertices[t.B].Z + z.Vertices[t.C].Z) / 3
			if mid >= b.lo && (mid < b.hi || b.hi == bands[len(bands)-1].hi) {
				sub = append(sub, i)
			}
		}
		if len(sub) == 0 {
			continue
		}
		g := z.rasterizeInto(frame, sub)
		chains := Chain(g.March())
		if minExtent > 0 {
			chains = dropSmallChains(chains, minExtent)
		}
		segs := SimplifyRDP(chains, rdpEps)
		for i := range segs {
			segs[i].A.Z = b.mid
			segs[i].B.Z = b.mid
		}
		out = append(out, segs...)
	}
	return out
}

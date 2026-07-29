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

// Rasterize projects the walkable faces onto a 2D occupancy grid and closes
// hairline gaps.
func (z *Zone) Rasterize() *OccupancyGrid {
	faces := z.WalkableFaces()
	if len(faces) == 0 {
		return &OccupancyGrid{}
	}
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

	g := &OccupancyGrid{
		W: int((maxX-minX)/cell) + 2*pad, H: int((maxY-minY)/cell) + 2*pad,
		Cell: cell, OriginX: minX, OriginY: minY, Pad: pad,
	}
	g.Filled = make([]bool, g.W*g.H)

	toGrid := func(v Vec3) (float64, float64) {
		return (v.X-minX)/cell + float64(pad), (v.Y-minY)/cell + float64(pad)
	}
	for _, i := range faces {
		t := z.Triangles[i]
		ax, ay := toGrid(z.Vertices[t.A])
		bx, by := toGrid(z.Vertices[t.B])
		cx, cy := toGrid(z.Vertices[t.C])
		g.fillTriangle(ax, ay, bx, by, cx, cy)
	}
	for i := 0; i < reps; i++ {
		g.dilate()
	}
	for i := 0; i < reps; i++ {
		g.erode()
	}
	return g
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

// Silhouette returns the simplified outline of the walkable area.
//
// Only meaningful where walkable area is sparse. In open zones the walkable
// region IS the zone footprint, so this collapses to the perimeter and carries
// no information — Classify() decides.
func (z *Zone) Silhouette() []Segment {
	g := z.Rasterize()
	if len(g.Filled) == 0 {
		return nil
	}
	return SimplifyRDP(Chain(g.March()), silRDPEpsilon)
}

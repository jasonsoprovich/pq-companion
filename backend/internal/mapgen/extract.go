package mapgen

import "math"

// Segment is one drawn line in map space.
type Segment struct{ A, B Vec3 }

// ── Boundary extraction ───────────────────────────────────────────────────────

// BoundaryEdges returns the outline of the walkable surface: edges belonging to
// exactly one floor triangle.
//
// This works on discrete floor slabs — interiors, platforms, cave ledges — where
// a floor genuinely ends somewhere. It fails by design on continuous outdoor
// terrain, where every interior edge is shared and the only unshared edges are
// the zone rim, yielding an empty rectangle. Classify() routes those zones to
// Contours instead.
func (z *Zone) BoundaryEdges() []Segment {
	type edge struct{ a, b int }
	counts := make(map[edge]int)
	for _, i := range z.WalkableFaces() {
		t := z.Triangles[i]
		for _, e := range [3]edge{{t.A, t.B}, {t.B, t.C}, {t.C, t.A}} {
			if e.a > e.b {
				e.a, e.b = e.b, e.a
			}
			counts[e]++
		}
	}
	out := make([]Segment, 0, len(counts)/2)
	for e, n := range counts {
		if n == 1 {
			out = append(out, Segment{A: z.Vertices[e.a], B: z.Vertices[e.b]})
		}
	}
	return out
}

// ── Elevation contours ────────────────────────────────────────────────────────

// contourUpThreshold is looser than upThreshold: contouring wants the whole
// ground surface including slopes a player would slide down, not just standable
// floor.
const contourUpThreshold = 0.35

// Contours slices the ground surface at fixed height intervals, producing a
// topographic map.
//
// This is the technique for open terrain, and it carries information no existing
// EQ map set has: Brewall's outdoor maps show outlines and landmarks but no
// elevation at all.
func (z *Zone) Contours(interval float64) []Segment {
	if interval <= 0 || len(z.Vertices) == 0 {
		return nil
	}
	minZ, maxZ := math.Inf(1), math.Inf(-1)
	for _, v := range z.Vertices {
		minZ = math.Min(minZ, v.Z)
		maxZ = math.Max(maxZ, v.Z)
	}
	if !(maxZ > minZ) {
		return nil
	}

	var out []Segment
	for _, t := range z.Triangles {
		if t.Flags&polyPermeable != 0 {
			continue
		}
		up, _ := z.upNormal(t)
		if up < contourUpThreshold {
			continue
		}
		a, b, c := z.Vertices[t.A], z.Vertices[t.B], z.Vertices[t.C]
		lo := math.Min(a.Z, math.Min(b.Z, c.Z))
		hi := math.Max(a.Z, math.Max(b.Z, c.Z))

		first := math.Ceil((lo - minZ) / interval)
		for n := first; ; n++ {
			k := minZ + n*interval
			if k > hi {
				break
			}
			var pts []Vec3
			for _, e := range [3][2]Vec3{{a, b}, {b, c}, {c, a}} {
				p, q := e[0], e[1]
				// Strict sign change: a vertex exactly on the plane would
				// otherwise emit a duplicate point from both its edges.
				if (p.Z-k)*(q.Z-k) >= 0 {
					continue
				}
				f := (k - p.Z) / (q.Z - p.Z)
				pts = append(pts, Vec3{
					X: p.X + f*(q.X-p.X),
					Y: p.Y + f*(q.Y-p.Y),
					Z: k,
				})
			}
			if len(pts) == 2 {
				out = append(out, Segment{A: pts[0], B: pts[1]})
			}
		}
	}
	return out
}

// ── Polyline chaining and simplification ──────────────────────────────────────

type ptKey struct{ x, y, z int64 }

// key identifies a chain junction. Z is part of the identity, not decoration:
// multi-level zones stack floors at identical XY, so dropping Z merges points
// from different storeys, inflates their apparent degree past 2, and breaks the
// chain walk. That fragmented unrest into 3,814 chains averaging 2 segments
// instead of 969 averaging 8.
func key(p Vec3) ptKey {
	return ptKey{
		x: int64(math.Round(p.X * 100)),
		y: int64(math.Round(p.Y * 100)),
		z: int64(math.Round(p.Z * 100)),
	}
}

// Chain joins loose segments into polylines by walking through degree-2
// junctions, so simplification has runs to work on. Segments that meet at a
// fork are left as separate chains.
func Chain(segs []Segment) [][]Vec3 {
	type link struct {
		to ptKey
		pt Vec3
	}
	adj := make(map[ptKey][]link, len(segs))
	for _, s := range segs {
		ka, kb := key(s.A), key(s.B)
		adj[ka] = append(adj[ka], link{to: kb, pt: s.B})
		adj[kb] = append(adj[kb], link{to: ka, pt: s.A})
	}

	type edgeKey struct{ a, b ptKey }
	used := make(map[edgeKey]bool, len(segs))
	mark := func(a, b ptKey) bool {
		if used[edgeKey{a, b}] || used[edgeKey{b, a}] {
			return false
		}
		used[edgeKey{a, b}] = true
		return true
	}

	var out [][]Vec3
	for _, s := range segs {
		ka, kb := key(s.A), key(s.B)
		if !mark(ka, kb) {
			continue
		}
		chain := []Vec3{s.A, s.B}
		prev, cur := ka, kb
		for {
			next := adj[cur]
			if len(next) != 2 {
				break // endpoint or fork
			}
			var pick link
			found := false
			for _, l := range next {
				if l.to != prev {
					pick, found = l, true
					break
				}
			}
			if !found || !mark(cur, pick.to) {
				break
			}
			chain = append(chain, pick.pt)
			prev, cur = cur, pick.to
		}
		out = append(out, chain)
	}
	return out
}

// SimplifyCollinear drops interior points that barely deviate from the line
// through their neighbours. Used for boundary output, where the input is already
// mesh edges rather than a rasterised staircase.
func SimplifyCollinear(chains [][]Vec3, tol float64) []Segment {
	var out []Segment
	for _, ch := range chains {
		if len(ch) < 2 {
			continue
		}
		keep := []Vec3{ch[0]}
		for i := 1; i < len(ch)-1; i++ {
			p, c, n := keep[len(keep)-1], ch[i], ch[i+1]
			v1x, v1y := c.X-p.X, c.Y-p.Y
			v2x, v2y := n.X-c.X, n.Y-c.Y
			l1 := math.Hypot(v1x, v1y)
			l2 := math.Hypot(v2x, v2y)
			if l1 < 1e-9 || l2 < 1e-9 {
				continue
			}
			cross := math.Abs(v1x*v2y-v1y*v2x) / (l1 * l2)
			if cross > tol {
				keep = append(keep, c)
			}
		}
		keep = append(keep, ch[len(ch)-1])
		for i := 0; i < len(keep)-1; i++ {
			out = append(out, Segment{A: keep[i], B: keep[i+1]})
		}
	}
	return out
}

// RDP is Douglas-Peucker simplification. Used for silhouette output, where it
// does the heavy lifting: collapsing marching-squares staircases into straight
// runs takes akheva from 9,848 segments to 190.
func RDP(pts []Vec3, eps float64) []Vec3 {
	if len(pts) < 3 {
		return pts
	}
	first, last := pts[0], pts[len(pts)-1]
	dx, dy := last.X-first.X, last.Y-first.Y
	norm := math.Hypot(dx, dy)

	best, bestIdx := -1.0, 0
	for i := 1; i < len(pts)-1; i++ {
		var d float64
		if norm < 1e-9 {
			d = math.Hypot(pts[i].X-first.X, pts[i].Y-first.Y)
		} else {
			d = math.Abs(dy*pts[i].X-dx*pts[i].Y+last.X*first.Y-last.Y*first.X) / norm
		}
		if d > best {
			best, bestIdx = d, i
		}
	}
	if best <= eps {
		return []Vec3{first, last}
	}
	left := RDP(pts[:bestIdx+1], eps)
	right := RDP(pts[bestIdx:], eps)
	return append(left[:len(left)-1], right...)
}

// SimplifyRDP chains and Douglas-Peucker-simplifies in one step.
func SimplifyRDP(chains [][]Vec3, eps float64) []Segment {
	var out []Segment
	for _, ch := range chains {
		s := RDP(ch, eps)
		for i := 0; i < len(s)-1; i++ {
			out = append(out, Segment{A: s[i], B: s[i+1]})
		}
	}
	return out
}

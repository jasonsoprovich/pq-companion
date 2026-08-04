package mapgen

import "math"

// ── Freestanding wall extraction ──────────────────────────────────────────────
//
// BoundaryEdges finds walls where a floor genuinely stops — the common case,
// where a room's wall sits at the edge of its own floor slab. It misses walls
// standing on a floor that never stops underneath them: Sanctus Seru's maze to
// Lord Inquisitor Seru is one continuous slab (occupancy 0.89) with freestanding
// corridor walls built on top, so no floor edge ever forms there and the maze
// draws as nothing at all, regardless of Detailed vs Outline simplification.
//
// FreestandingWallEdges recovers those walls directly from the wall geometry
// itself: a near-vertical (non-walkable) triangle built as a flat rectangular
// panel has a near-horizontal top and/or bottom rim, and that rim IS the wall's
// footprint. Reading it straight off the wall's own geometry, rather than
// slicing the whole zone at one manually-picked height, is what lets this work
// across a multi-level zone without per-zone tuning — each wall's own base/cap
// height comes along with it, so a second-floor maze and a ground-floor one both
// resolve correctly from the same pass.

// freestandingWallUpMax and wallRimTolerance are both deliberately strict:
// nearly-perfectly-vertical panels with a nearly-perfectly-flat rim, so a
// sloped decorative element, ramp, or roof is never mistaken for a wall.
const (
	freestandingWallUpMax = 0.02
	wallRimTolerance      = 0.1
)

// wallRimEdges returns every near-horizontal top/bottom rim edge of the zone's
// near-vertical triangles — unfiltered, so it duplicates most ordinary walls
// BoundaryEdges already draws via their floor's edge. novelWallEdges is what
// narrows this down to the freestanding ones actually worth adding.
func (z *Zone) wallRimEdges() []Segment {
	type edge struct{ a, b int }
	seen := make(map[edge]bool, len(z.Triangles))
	var out []Segment
	for _, t := range z.Triangles {
		if t.Flags&polyPermeable != 0 {
			continue
		}
		up, _ := z.upNormal(t)
		if up > freestandingWallUpMax {
			continue
		}
		for _, e := range [3]edge{{t.A, t.B}, {t.B, t.C}, {t.C, t.A}} {
			va, vb := z.Vertices[e.a], z.Vertices[e.b]
			if math.Abs(va.Z-vb.Z) > wallRimTolerance {
				continue // a diagonal or vertical side edge, not a rim
			}
			if e.a > e.b {
				e.a, e.b = e.b, e.a
			}
			if seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, Segment{A: va, B: vb})
		}
	}
	return out
}

// freestandingWallXYTol / freestandingWallZTol bound the neighbourhood
// novelWallEdges searches for a matching floor-boundary segment. Wide enough to
// absorb a wall that sits a few units off its floor's exact edge (walls are
// rarely modeled with zero setback), tight enough that a different storey's
// boundary — stacked at the same XY, a real floor height apart — never
// suppresses a genuine freestanding wall above or below it.
const (
	freestandingWallXYTol = 16.0
	freestandingWallZTol  = 8.0
)

// novelWallEdges keeps only the wallEdges with no nearby match among
// floorEdges. An ordinary room wall already has a floor-boundary edge running
// right along it — that's the common case BoundaryEdges already draws — so
// only a wall standing on a floor with no discontinuity beneath it (a maze on
// a continuous slab) survives this filter. Matching is spatial (segment
// midpoint proximity), not exact, since a wall panel's rim and its floor's
// edge are built as separate meshes and rarely share vertices even where they
// run along the same line.
func novelWallEdges(wallEdges, floorEdges []Segment) []Segment {
	if len(wallEdges) == 0 || len(floorEdges) == 0 {
		return wallEdges
	}
	type cell struct{ x, y, z int64 }
	cellOf := func(p Vec3) cell {
		return cell{
			int64(math.Floor(p.X / freestandingWallXYTol)),
			int64(math.Floor(p.Y / freestandingWallXYTol)),
			int64(math.Floor(p.Z / freestandingWallZTol)),
		}
	}
	mid := func(s Segment) Vec3 {
		return Vec3{(s.A.X + s.B.X) / 2, (s.A.Y + s.B.Y) / 2, (s.A.Z + s.B.Z) / 2}
	}

	grid := make(map[cell][]Vec3, len(floorEdges))
	for _, s := range floorEdges {
		m := mid(s)
		c := cellOf(m)
		grid[c] = append(grid[c], m)
	}

	near := func(m Vec3) bool {
		base := cellOf(m)
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				for dz := int64(-1); dz <= 1; dz++ {
					c := cell{base.x + dx, base.y + dy, base.z + dz}
					for _, fm := range grid[c] {
						if math.Hypot(fm.X-m.X, fm.Y-m.Y) <= freestandingWallXYTol &&
							math.Abs(fm.Z-m.Z) <= freestandingWallZTol {
							return true
						}
					}
				}
			}
		}
		return false
	}

	out := make([]Segment, 0, len(wallEdges)/4)
	for _, s := range wallEdges {
		if !near(mid(s)) {
			out = append(out, s)
		}
	}
	return out
}

// FreestandingWallEdges returns the walls BoundaryEdges cannot see: those
// standing on a floor that never stops underneath them. floorEdges is the
// caller's own z.BoundaryEdges() result, passed in rather than recomputed so
// callers that already have it (Extract, ExtractOutline) don't pay for it
// twice.
func (z *Zone) FreestandingWallEdges(floorEdges []Segment) []Segment {
	return novelWallEdges(z.wallRimEdges(), floorEdges)
}

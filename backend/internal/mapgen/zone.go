package mapgen

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// weldTolerance is the grid size, in world units, used to merge coincident
// vertices.
//
// This is not cosmetic. A zone arrives as thousands of separate 0x36 mesh
// fragments, each with its own vertex index space, so two triangles meeting
// along a shared edge never share vertex indices. Without welding, every
// triangle looks like a boundary and edge extraction returns noise — unrest
// yields 18,005 boundary segments unwelded versus 7,796 welded.
const weldTolerance = 0.25

// upThreshold is the minimum |normal.Z| for a face to count as walkable floor.
// 0.55 is about a 57-degree slope: steep enough to exclude walls, shallow
// enough to keep ramps and rough cave floors.
const upThreshold = 0.55

// Zone is a loaded zone's geometry, with vertices welded across fragments.
type Zone struct {
	ShortName string
	Vertices  []Vec3
	Triangles []Triangle
}

// LoadZone reads <clientDir>/<shortName>.s3d and returns its welded geometry.
func LoadZone(clientDir, shortName string) (*Zone, error) {
	path := filepath.Join(clientDir, shortName+".s3d")
	arc, err := OpenArchive(path)
	if err != nil {
		return nil, err
	}
	raw, ok := arc[shortName+".wld"]
	if !ok {
		// A few archives name the main WLD differently; fall back to the only
		// .wld that isn't one of the known auxiliary files.
		for name, b := range arc {
			l := strings.ToLower(name)
			if strings.HasSuffix(l, ".wld") && l != "objects.wld" && l != "lights.wld" {
				raw, ok = b, true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("%s: no zone .wld inside archive", shortName)
	}

	w, err := ParseWLD(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", shortName, err)
	}
	z := &Zone{ShortName: shortName}
	weld := newWelder(weldTolerance)
	for _, m := range w.Meshes() {
		base := make([]int, len(m.Vertices))
		for i, v := range m.Vertices {
			base[i] = weld.add(v)
		}
		for _, t := range m.Triangles {
			a, b, c := base[t.A], base[t.B], base[t.C]
			if a == b || b == c || a == c {
				continue // collapsed by welding
			}
			z.Triangles = append(z.Triangles, Triangle{Flags: t.Flags, A: a, B: b, C: c})
		}
	}
	z.Vertices = weld.verts
	return z, nil
}

// LoadPlacements reads the object instances from a zone archive's objects.wld.
func LoadPlacements(clientDir, shortName string) ([]Placement, error) {
	arc, err := OpenArchive(filepath.Join(clientDir, shortName+".s3d"))
	if err != nil {
		return nil, err
	}
	raw, ok := arc["objects.wld"]
	if !ok {
		return nil, nil
	}
	w, err := ParseWLD(raw)
	if err != nil {
		return nil, fmt.Errorf("%s objects.wld: %w", shortName, err)
	}
	return w.Placements(), nil
}

type gridKey struct{ x, y, z int64 }

type welder struct {
	tol   float64
	index map[gridKey]int
	verts []Vec3
}

func newWelder(tol float64) *welder {
	return &welder{tol: tol, index: make(map[gridKey]int)}
}

func (w *welder) add(v Vec3) int {
	k := gridKey{
		x: int64(math.Round(v.X / w.tol)),
		y: int64(math.Round(v.Y / w.tol)),
		z: int64(math.Round(v.Z / w.tol)),
	}
	if i, ok := w.index[k]; ok {
		return i
	}
	i := len(w.verts)
	w.index[k] = i
	w.verts = append(w.verts, v)
	return i
}

// WalkableFaces returns the indices of triangles a player could stand on:
// collidable, and facing far enough upward.
func (z *Zone) WalkableFaces() []int {
	out := make([]int, 0, len(z.Triangles)/2)
	for i, t := range z.Triangles {
		if t.Flags&polyPermeable != 0 {
			continue
		}
		if _, ok := z.upNormal(t); !ok {
			continue
		}
		out = append(out, i)
	}
	return out
}

// upNormal returns |normal.Z| normalised, and whether the face is walkable.
func (z *Zone) upNormal(t Triangle) (float64, bool) {
	a, b, c := z.Vertices[t.A], z.Vertices[t.B], z.Vertices[t.C]
	ux, uy, uz := b.X-a.X, b.Y-a.Y, b.Z-a.Z
	vx, vy, vz := c.X-a.X, c.Y-a.Y, c.Z-a.Z
	nx := uy*vz - uz*vy
	ny := uz*vx - ux*vz
	nz := ux*vy - uy*vx
	l := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if l < 1e-9 {
		return 0, false
	}
	up := math.Abs(nz / l)
	return up, up >= upThreshold
}

// Bounds returns the zone's XY extent in map space.
func (z *Zone) Bounds() (minX, minY, maxX, maxY float64) {
	if len(z.Vertices) == 0 {
		return 0, 0, 0, 0
	}
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, v := range z.Vertices {
		minX = math.Min(minX, v.X)
		minY = math.Min(minY, v.Y)
		maxX = math.Max(maxX, v.X)
		maxY = math.Max(maxY, v.Y)
	}
	return minX, minY, maxX, maxY
}

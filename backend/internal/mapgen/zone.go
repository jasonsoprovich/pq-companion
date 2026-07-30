package mapgen

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
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

// Shell-plane detection. See dropShellPlanes for what this is and why.
const (
	// shellCoverage is the share of the zone footprint a candidate plane must
	// cover. Real ground is never this flat across a whole zone.
	shellCoverage = 0.85
	// shellGap is how far the plane must sit from any other walkable surface.
	// This is the condition doing the real work: a genuine floor has things
	// resting on it at nearby heights — ramps, objects, the bottom edges of
	// walls — while a skybox has nothing near it at all.
	shellGap = 250.0
	// shellTolerance groups faces into one plane. These are flat by
	// construction, so this only absorbs float noise.
	shellTolerance = 5.0
)

// WalkableFaces returns the indices of triangles a player could stand on:
// collidable, facing far enough upward, and not part of a shell plane.
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
	return z.dropShellPlanes(out)
}

// dropShellPlanes removes the flat planes that bound a zone rather than floor
// it — a skybox ceiling, a death plane, an ocean floor.
//
// These are collidable and face upward, so nothing about the geometry marks
// them as different, and in Plane of Sky they wrecked the map three ways at
// once: 578 faces at z=-2848 and 578 at z=+1847 each covered 100% of the
// footprint, which stretched the zone bounds so the actual islands were squeezed
// into the middle of the canvas, drove occupancy to 0.99 so the classifier chose
// boundary extraction over the banded silhouette that separates the islands, and
// drew a distracting rectangle around everything.
//
// Deliberately narrow. Loosening either threshold to 0.5/100 catches 39 planes
// across the corpus, including ocean surfaces and sky planes in outdoor zones
// where they may well be legitimate walkable geometry; at 0.85/250 it catches
// three, in Plane of Sky and Erud's Crossing, all unambiguous. Removing real
// floor would be a far worse failure than leaving a stray rectangle, so this
// errs at the strict end and the numbers were measured before they were picked.
func (z *Zone) dropShellPlanes(faces []int) []int {
	if len(faces) < 2 {
		return faces
	}
	minX, minY, maxX, maxY := z.Bounds()
	bbox := (maxX - minX) * (maxY - minY)
	if bbox <= 0 {
		return faces
	}

	type entry struct {
		idx  int
		z    float64
		area float64
	}
	fs := make([]entry, 0, len(faces))
	for _, i := range faces {
		t := z.Triangles[i]
		a, b, c := z.Vertices[t.A], z.Vertices[t.B], z.Vertices[t.C]
		fs = append(fs, entry{
			idx:  i,
			z:    (a.Z + b.Z + c.Z) / 3,
			area: math.Abs((b.X-a.X)*(c.Y-a.Y)-(b.Y-a.Y)*(c.X-a.X)) / 2,
		})
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].z < fs[j].z })

	drop := map[int]bool{}
	// shell walks inward from one end, accumulating the outermost plane, and
	// marks it for removal if it is both broad enough and isolated enough.
	shell := func(fromBottom bool) {
		var acc, gap float64
		var members []int
		if fromBottom {
			planeZ := fs[0].z
			for _, f := range fs {
				if f.z-planeZ > shellTolerance {
					gap = f.z - planeZ
					break
				}
				acc += f.area
				members = append(members, f.idx)
			}
		} else {
			planeZ := fs[len(fs)-1].z
			for i := len(fs) - 1; i >= 0; i-- {
				if planeZ-fs[i].z > shellTolerance {
					gap = planeZ - fs[i].z
					break
				}
				acc += fs[i].area
				members = append(members, fs[i].idx)
			}
		}
		// A zero gap means the plane is the entire zone; leave it alone rather
		// than delete every face.
		if gap < shellGap || acc/bbox < shellCoverage || len(members) == len(fs) {
			return
		}
		for _, i := range members {
			drop[i] = true
		}
	}
	shell(true)
	shell(false)
	if len(drop) == 0 {
		return faces
	}

	out := faces[:0:0]
	for _, i := range faces {
		if !drop[i] {
			out = append(out, i)
		}
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

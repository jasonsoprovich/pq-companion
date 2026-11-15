package mapgen

import (
	"encoding/binary"
	"math"
	"strings"
)

// Animated objects (flags, torches, braziers, signs) are skeletal: their
// actor points at a 0x11 reference to a 0x10 skeleton instead of a mesh.
// Each bone has a 0x13 track reference (to a 0x12 track of frames) and
// optionally a mesh of its own; a skeleton can also carry whole meshes whose
// vertex pieces follow bones. The 3D view is a map, so models are posed at
// the first frame of each track and drawn still.

const (
	fragSkeleton    = 0x10
	fragSkeletonRef = 0x11
	fragTrackDef    = 0x12
	fragTrack       = 0x13
)

// mat4 is a row-major affine transform in raw WLD space.
type mat4 [16]float64

var identity = mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}

func (a mat4) mul(b mat4) mat4 {
	var out mat4
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			var v float64
			for k := 0; k < 4; k++ {
				v += a[4*r+k] * b[4*k+c]
			}
			out[4*r+c] = v
		}
	}
	return out
}

func (a mat4) apply(x, y, z float64) (float64, float64, float64) {
	return a[0]*x + a[1]*y + a[2]*z + a[3],
		a[4]*x + a[5]*y + a[6]*z + a[7],
		a[8]*x + a[9]*y + a[10]*z + a[11]
}

type bone struct {
	track    int32 // 0x13 reference
	mesh     int32 // 0x2D reference, or <= 0 for none
	children []int
}

// skeletonMeshes returns a skeletal actor's meshes posed at the first frame,
// in map space like any other model. r is the actor's 0x11 reference.
func (w *WLD) skeletonMeshes(r int32) []Mesh {
	return w.skeletonMeshesPosed(r, nil)
}

// bonePose is how one bone is posed: the track and frame to read, and
// whether to keep the bone's rest offset (its translation) instead of the
// track's, for an animation borrowed from another model whose bones are
// a different length.
type bonePose struct {
	track    int32
	frame    int
	keepRest bool
	// src is the file the track is in, when it isn't the skeleton's own.
	src *WLD
}

// skeletonMeshesPosed is skeletonMeshes with each bone's 0x13 track
// reference passed through pose first, which can swap a bone's rest track
// for an animation's and pick the frame (see posedSkeleton); nil keeps the
// rest pose.
func (w *WLD) skeletonMeshesPosed(r int32, pose func(track int32) bonePose) []Mesh {
	meshes, _ := w.skeleton(r, pose, nil)
	return meshes
}

// skeleton is skeletonMeshesPosed that also returns each bone's world
// transform (raw space) by its rest track's name less "_TRACK"
// ("HUMR_POINT"), which places items at the attachment points. swap, when
// set, can replace a mesh by name ("HUMHE00_DMSPRITEDEF") with another in
// the file, as the client does for helms and robes; it returns "" to keep
// the mesh.
func (w *WLD) skeleton(r int32, pose func(track int32) bonePose, swap func(name string) string) ([]Mesh, map[string]mat4) {
	return w.skeletonBound(r, pose, swap, nil)
}

// skeletonBound is skeleton where bind, when set, can place a bone outright
// by its rest track's name less "_TRACK": a worn model's bones follow the
// wearer's like-named ones. Bones bind leaves alone hang from their parent
// as usual.
func (w *WLD) skeletonBound(r int32, pose func(track int32) bonePose, swap func(name string) string, bind func(name string) (mat4, bool)) ([]Mesh, map[string]mat4) {
	i, ok := w.ref(r, fragSkeletonRef)
	if !ok || len(w.Fragments[i].payload) < 4 {
		return nil, nil
	}
	j, ok := w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragSkeleton)
	if !ok {
		return nil, nil
	}
	bones, meshRefs := parseSkeleton(w.Fragments[j].payload)
	if len(bones) == 0 {
		return nil, nil
	}
	meshOf := func(r int32) (Mesh, bool) {
		if swap != nil {
			if m, ok := w.swappedMesh(r, swap); ok {
				return m, true
			}
		}
		return w.meshRef(r)
	}

	// World pose per bone: parent then the bone's own frame, walking down
	// from the root (bone 0).
	world := make([]mat4, len(bones))
	seen := make([]bool, len(bones))
	var walk func(b int, parent mat4)
	walk = func(b int, parent mat4) {
		if b < 0 || b >= len(bones) || seen[b] {
			return
		}
		seen[b] = true
		if bind != nil {
			if k, ok := w.ref(bones[b].track, fragTrack); ok {
				name := strings.TrimSuffix(strings.ToUpper(w.str(w.Fragments[k].nameRef)), "_TRACK")
				if m, ok := bind(name); ok {
					world[b] = m
					for _, c := range bones[b].children {
						walk(c, world[b])
					}
					return
				}
			}
		}
		local := w.trackPose(bones[b].track)
		if pose != nil {
			if bp := pose(bones[b].track); bp.track != bones[b].track || bp.frame != 0 || bp.src != nil {
				from := w
				if bp.src != nil {
					from = bp.src
				}
				anim := from.trackPoseFrame(bp.track, bp.frame)
				if bp.keepRest {
					anim[3], anim[7], anim[11] = local[3], local[7], local[11]
				}
				local = anim
			}
		}
		world[b] = parent.mul(local)
		for _, c := range bones[b].children {
			walk(c, world[b])
		}
	}
	walk(0, identity)
	for b := range bones {
		if !seen[b] {
			walk(b, identity) // unreachable bones posed on their own
		}
	}

	points := make(map[string]mat4, len(bones))
	for b, bn := range bones {
		if k, ok := w.ref(bn.track, fragTrack); ok {
			points[strings.TrimSuffix(strings.ToUpper(w.str(w.Fragments[k].nameRef)), "_TRACK")] = world[b]
		}
	}

	var out []Mesh
	for b, bn := range bones {
		if m, ok := meshOf(bn.mesh); ok {
			out = append(out, poseMesh(m, func(int) mat4 { return world[b] }))
		}
	}
	for _, r := range meshRefs {
		m, ok := meshOf(r)
		if !ok {
			continue
		}
		// Vertex pieces run in order; vertices past the last piece stay put.
		boneOf := make([]int, len(m.Vertices))
		for k := range boneOf {
			boneOf[k] = -1
		}
		v := 0
		for _, p := range m.Pieces {
			for k := 0; k < p.Count && v < len(boneOf); k++ {
				boneOf[v] = p.Bone
				v++
			}
		}
		out = append(out, poseMesh(m, func(v int) mat4 {
			if b := boneOf[v]; b >= 0 && b < len(world) {
				return world[b]
			}
			return identity
		}))
	}
	return out, points
}

// swappedMesh resolves a 0x2D reference to the mesh swap names in its
// place, if it names one the file has.
func (w *WLD) swappedMesh(r int32, swap func(string) string) (Mesh, bool) {
	i, ok := w.ref(r, fragMeshRef)
	if !ok || len(w.Fragments[i].payload) < 4 {
		return Mesh{}, false
	}
	j, ok := w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragMesh)
	if !ok {
		return Mesh{}, false
	}
	want := swap(strings.ToUpper(w.str(w.Fragments[j].nameRef)))
	if want == "" {
		return Mesh{}, false
	}
	for _, f := range w.Fragments {
		if f.kind == fragMesh && strings.ToUpper(w.str(f.nameRef)) == want {
			m, err := parseMesh(f.payload, w.Version)
			return m, err == nil
		}
	}
	return Mesh{}, false
}

// meshRef follows a 0x2D reference to its parsed 0x36 mesh.
func (w *WLD) meshRef(r int32) (Mesh, bool) {
	i, ok := w.ref(r, fragMeshRef)
	if !ok || len(w.Fragments[i].payload) < 4 {
		return Mesh{}, false
	}
	j, ok := w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragMesh)
	if !ok {
		return Mesh{}, false
	}
	m, err := parseMesh(w.Fragments[j].payload, w.Version)
	return m, err == nil
}

// poseMesh moves each map-space vertex by its bone's raw-space transform.
func poseMesh(m Mesh, boneFor func(v int) mat4) Mesh {
	out := m
	out.Vertices = make([]Vec3, len(m.Vertices))
	for i, v := range m.Vertices {
		// Back to raw space (see ToMapSpace), pose, and forward again.
		x, y, z := boneFor(i).apply(-v.Y, -v.X, v.Z)
		out.Vertices[i] = ToMapSpace(x, y, z)
	}
	return out
}

// parseSkeleton reads a 0x10 skeleton: flags, bone count, a 0x18
// reference, three dwords if flag bit 0, a bounding radius if bit 1, then
// each bone (name, flags, track reference, mesh reference, child count and
// child indices), then, if bit 9, a count, that many mesh references and as
// many unknown dwords.
func parseSkeleton(b []byte) ([]bone, []int32) {
	le := binary.LittleEndian
	if len(b) < 12 {
		return nil, nil
	}
	flags := le.Uint32(b)
	n := int(le.Uint32(b[4:]))
	p := 12
	if flags&1 != 0 {
		p += 12
	}
	if flags&2 != 0 {
		p += 4
	}
	if n < 0 || n > 1024 {
		return nil, nil
	}
	bones := make([]bone, 0, n)
	for i := 0; i < n; i++ {
		if p+20 > len(b) {
			return nil, nil
		}
		bn := bone{
			track: int32(le.Uint32(b[p+8:])),
			mesh:  int32(le.Uint32(b[p+12:])),
		}
		kids := int(le.Uint32(b[p+16:]))
		p += 20
		if kids < 0 || p+4*kids > len(b) {
			return nil, nil
		}
		for k := 0; k < kids; k++ {
			bn.children = append(bn.children, int(le.Uint32(b[p:])))
			p += 4
		}
		bones = append(bones, bn)
	}
	var meshes []int32
	if flags&(1<<9) != 0 && p+4 <= len(b) {
		m := int(le.Uint32(b[p:]))
		p += 4
		for k := 0; k < m && p+4 <= len(b); k++ {
			meshes = append(meshes, int32(le.Uint32(b[p:])))
			p += 4
		}
	}
	return bones, meshes
}

// trackPose returns a 0x13 track's first frame as translate * rotate *
// scale, or the identity when it can't be read. Object tracks (0x12 flag
// bit 3) store frames as int16s: rotation w, x, y, z (normalised), then
// translation x, y, z over 256 and scale over 256.
func (w *WLD) trackPose(r int32) mat4 {
	return w.trackPoseFrame(r, 0)
}

// trackPoseFrame is trackPose at frame frame (clamped to the track's last
// frame).
func (w *WLD) trackPoseFrame(r int32, frame int) mat4 {
	i, ok := w.ref(r, fragTrack)
	if !ok || len(w.Fragments[i].payload) < 4 {
		return identity
	}
	j, ok := w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragTrackDef)
	if !ok {
		return identity
	}
	b := w.Fragments[j].payload
	le := binary.LittleEndian
	if len(b) < 8 || le.Uint32(b[4:]) == 0 {
		return identity
	}
	if n := int(le.Uint32(b[4:])); frame >= n {
		frame = n - 1
	}
	if frame < 0 {
		frame = 0
	}
	var qw, qx, qy, qz, tx, ty, tz, s float64
	if le.Uint32(b)&8 != 0 {
		if len(b) < 24+16*frame {
			return identity
		}
		fo := 16 * frame
		i16 := func(off int) float64 { return float64(int16(le.Uint16(b[fo+off:]))) }
		qw, qx, qy, qz = i16(8), i16(10), i16(12), i16(14)
		if d := i16(22); d != 0 {
			tx, ty, tz, s = i16(16)/256, i16(18)/256, i16(20)/256, d/256
		}
	} else {
		if len(b) < 40+32*frame {
			return identity
		}
		fo := 32 * frame
		f := func(off int) float64 { return float64(math.Float32frombits(le.Uint32(b[fo+off:]))) }
		s, tx, ty, tz = f(8), f(12), f(16), f(20)
		qw, qx, qy, qz = f(24), f(28), f(32), f(36)
	}
	if s == 0 {
		s = 1
	}
	l := math.Sqrt(qw*qw + qx*qx + qy*qy + qz*qz)
	if l == 0 {
		qw, l = 1, 1
	}
	qw, qx, qy, qz = qw/l, qx/l, qy/l, qz/l
	return mat4{
		s * (1 - 2*(qy*qy+qz*qz)), s * 2 * (qx*qy - qz*qw), s * 2 * (qx*qz + qy*qw), tx,
		s * 2 * (qx*qy + qz*qw), s * (1 - 2*(qx*qx+qz*qz)), s * 2 * (qy*qz - qx*qw), ty,
		s * 2 * (qx*qz - qy*qw), s * 2 * (qy*qz + qx*qw), s * (1 - 2*(qx*qx+qy*qy)), tz,
		0, 0, 0, 1,
	}
}

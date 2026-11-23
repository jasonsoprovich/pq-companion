package mapgen

import (
	"math"
	"os"
	"testing"
)

const instClientDir = "/Volumes/T7/EQ/TAKPv22"

// drawn returns every drawn triangle's corner positions, instances placed
// by their matrices, plus their sum, for comparing meshes built different
// ways.
func drawn(m *Mesh3D) (tris int, sum [3]float64) {
	add := func(pos []float32, idx []uint32, x *[12]float32) {
		for _, i := range idx {
			v := [3]float64{float64(pos[3*i]), float64(pos[3*i+1]), float64(pos[3*i+2])}
			if x != nil {
				v = [3]float64{
					float64(x[0])*v[0] + float64(x[1])*v[1] + float64(x[2])*v[2] + float64(x[3]),
					float64(x[4])*v[0] + float64(x[5])*v[1] + float64(x[6])*v[2] + float64(x[7]),
					float64(x[8])*v[0] + float64(x[9])*v[1] + float64(x[10])*v[2] + float64(x[11]),
				}
			}
			sum[0], sum[1], sum[2] = sum[0]+v[0], sum[1]+v[1], sum[2]+v[2]
		}
		tris += len(idx) / 3
	}
	add(m.Pos, m.Idx, nil)
	for _, md := range m.Models {
		for k := range md.Instances {
			add(md.Pos, md.Idx, &md.Instances[k])
		}
	}
	return tris, sum
}

// Instancing draws the same triangles in the same places as copying every
// placement into the mesh, and keeps tilted and rarely placed models
// copied.
func TestMesh3DInstancingMatchesBaked(t *testing.T) {
	if _, err := os.Stat(instClientDir); err != nil {
		t.Skipf("EQ client not mounted at %s", instClientDir)
	}
	for _, zone := range []string{"wakening", "kael", "poknowledge"} {
		inst, err := LoadMesh3D(instClientDir, zone, nil)
		if err != nil {
			t.Fatal(err)
		}
		old := minInstances
		minInstances = math.MaxInt
		baked, err := LoadMesh3D(instClientDir, zone, nil)
		minInstances = old
		if err != nil {
			t.Fatal(err)
		}
		if len(baked.Models) != 0 || len(inst.Models) == 0 {
			t.Fatalf("%s: %d baked-run models, %d instanced", zone, len(baked.Models), len(inst.Models))
		}
		it, is := drawn(inst)
		bt, bs := drawn(baked)
		if it != bt || it != inst.Triangles() {
			t.Errorf("%s: %d triangles instanced (Triangles %d), %d baked", zone, it, inst.Triangles(), bt)
		}
		for c := range 3 {
			// float32 positions: compare the mean corner, not the sum.
			n := float64(3 * bt)
			if d := math.Abs(is[c]-bs[c]) / n; d > 0.01 {
				t.Errorf("%s: mean corner axis %d differs by %.4f", zone, c, d)
			}
		}
		for _, md := range inst.Models {
			if len(md.Instances) < minInstances {
				t.Errorf("%s: a model with %d instances", zone, len(md.Instances))
			}
			for _, g := range md.Groups {
				if !inst.Materials[g.Material].Object {
					t.Errorf("%s: model group uses a non-object material", zone)
				}
			}
		}
		t.Logf("%s: %d tris, %d in the zone section, %d models, encoded %.1f MB (baked %.1f MB)",
			zone, it, len(inst.Idx)/3, len(inst.Models), float64(len(inst.Encode()))/1e6, float64(len(baked.Encode()))/1e6)
	}
}

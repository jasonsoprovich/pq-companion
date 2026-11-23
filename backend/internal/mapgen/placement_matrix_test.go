package mapgen

import (
	"math"
	"math/rand"
	"testing"
)

// The matrix places a vertex where placeTransform does, for any yaw,
// tilt, scale and position, doors included.
func TestPlacementMatrixMatchesTransform(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	places := []Placement{(Door{Model: "D", X: 10, Y: -20, Z: 3, Heading: 128, Incline: 64, Size: 150}).placement()}
	for range 200 {
		raw := Vec3{r.Float64()*8000 - 4000, r.Float64()*8000 - 4000, r.Float64()*600 - 300}
		tilt := 0.0
		if r.Intn(4) == 0 {
			tilt = r.Float64() * 512
		}
		places = append(places, Placement{
			Raw: raw, Pos: ToMapSpace(raw.X, raw.Y, raw.Z),
			Yaw: r.Float64() * 512, Tilt: tilt, Scale: 0.2 + r.Float64()*3,
		})
	}
	for _, p := range places {
		f, m := placeTransform(p), placementMatrix(p)
		for range 5 {
			v := Vec3{r.Float64()*200 - 100, r.Float64()*200 - 100, r.Float64()*200 - 100}
			want := f(v)
			got := Vec3{
				float64(m[0])*v.X + float64(m[1])*v.Y + float64(m[2])*v.Z + float64(m[3]),
				float64(m[4])*v.X + float64(m[5])*v.Y + float64(m[6])*v.Z + float64(m[7]),
				float64(m[8])*v.X + float64(m[9])*v.Y + float64(m[10])*v.Z + float64(m[11]),
			}
			// float32 storage: allow ~1e-6 relative to the coordinates.
			tol := 1e-6 * (math.Abs(p.Raw.X) + math.Abs(p.Raw.Y) + 400)
			if math.Abs(got.X-want.X) > tol || math.Abs(got.Y-want.Y) > tol || math.Abs(got.Z-want.Z) > tol {
				t.Fatalf("placement %+v vertex %v: matrix %v, transform %v", p, v, got, want)
			}
		}
	}
}

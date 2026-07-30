package mapgen

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sort"
)

// Layer numbers within a zone. Layer 0 is the geometry the classifier chose;
// higher layers are overlays added later (POIs, generated packs).
const (
	LayerGeometry = 0
)

// LayerOutline is the clean single-language outline drawn in Outline mode.
// See outline.go for why it exists alongside the classifier's own output.
const LayerOutline = 2

// segmentBytes is the packed width of one segment: six int16 coordinates plus
// three colour bytes.
const segmentBytes = 6*2 + 3

// coordMin/coordMax bound what int16 can hold. Real zone coordinates span
// roughly -11147..23621, so this is headroom, not a real constraint — but a
// corrupt mesh could produce garbage and silently wrap.
const (
	coordMin = -32768
	coordMax = 32767
)

// WriteMapsDB creates a maps.db at path containing one row per (zone, layer).
//
// Segments are stored as a packed, zlib-compressed blob rather than a row per
// segment. Measured over the full corpus, row-per-segment came to 34.7 MB —
// barely better than the 41.2 MB of raw text it replaces, because SQLite's
// per-row overhead swamps a payload of nine small integers. The packed form is
// 4.0 MB and fetches in ~0.05 ms per zone, and rendering always wants every
// segment in a layer anyway, so there is nothing to gain from row granularity.
//
// Coordinates are rounded to int16. Real spans reach ~23,600 units, so 1-unit
// resolution costs at most half a unit of error — about a quarter pixel at any
// sane zoom.
func WriteMapsDB(path string, zones []ZoneOutput, pois []POI) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove existing %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer db.Close()

	if _, err := db.Exec(mapsSchema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	insLayer, err := tx.Prepare(
		`INSERT INTO map_layer (zone, layer, nlines, lines) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare map_layer: %w", err)
	}
	defer insLayer.Close()

	insZone, err := tx.Prepare(`INSERT INTO map_zone
		(zone, min_x, min_y, max_x, max_y, technique, occupancy, bnd_density,
		 z_span, min_z, max_z)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare map_zone: %w", err)
	}
	defer insZone.Close()

	for _, z := range zones {
		blob, err := packSegments(z.Segments)
		if err != nil {
			return fmt.Errorf("%s: %w", z.Zone, err)
		}
		if _, err := insLayer.Exec(z.Zone, LayerGeometry, len(z.Segments), blob); err != nil {
			return fmt.Errorf("insert %s layer: %w", z.Zone, err)
		}
		if len(z.Detail) > 0 {
			db, err := packSegments(z.Detail)
			if err != nil {
				return fmt.Errorf("%s detail: %w", z.Zone, err)
			}
			if _, err := insLayer.Exec(z.Zone, LayerDetail, len(z.Detail), db); err != nil {
				return fmt.Errorf("insert %s detail layer: %w", z.Zone, err)
			}
		}
		if len(z.Outline) > 0 {
			ob, err := packSegments(z.Outline)
			if err != nil {
				return fmt.Errorf("%s outline: %w", z.Zone, err)
			}
			if _, err := insLayer.Exec(z.Zone, LayerOutline, len(z.Outline), ob); err != nil {
				return fmt.Errorf("insert %s outline layer: %w", z.Zone, err)
			}
		}
		// Bounds come from the drawn segments across every layer, not from the raw
		// mesh, for the same reason in both axes: these numbers drive what the
		// renderer fits to screen and what the depth slider can reach, so they
		// have to describe what is actually on screen.
		//
		// The XY case is not hypothetical. Plane of Sky's mesh includes a skybox
		// and a death plane spanning the whole zone; those are dropped from the
		// drawn output (see dropShellPlanes), but a mesh-derived bound would
		// still stretch to them and squeeze the actual islands into the middle of
		// the canvas.
		minZ, maxZ := segmentZRange(z.Segments, z.Detail, z.Outline)
		minX, minY, maxX, maxY := z.MinX, z.MinY, z.MaxX, z.MaxY
		if bx0, by0, bx1, by1, ok := segmentXYRange(z.Segments, z.Detail, z.Outline); ok {
			minX, minY, maxX, maxY = bx0, by0, bx1, by1
		}
		if _, err := insZone.Exec(z.Zone,
			clampCoord(minX), clampCoord(minY),
			clampCoord(maxX), clampCoord(maxY),
			string(z.Technique), z.Occupancy, z.BoundaryDensity, z.ZSpan,
			clampCoord(minZ), clampCoord(maxZ),
		); err != nil {
			return fmt.Errorf("insert %s zone: %w", z.Zone, err)
		}
	}
	insPOI, err := tx.Prepare(`INSERT OR IGNORE INTO map_poi
		(zone, x, y, z, category, label, source, ref_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare map_poi: %w", err)
	}
	defer insPOI.Close()

	// OR IGNORE rather than a plain INSERT: several sources legitimately land
	// two identical pins on one spot — a spawngroup with the same NPC listed
	// twice, or a zone line recorded from both sides. The UNIQUE constraint
	// collapses them instead of failing the build.
	for _, p := range pois {
		if _, err := insPOI.Exec(p.Zone, p.X, p.Y, p.Z,
			p.Category, p.Label, p.Source, p.RefID); err != nil {
			return fmt.Errorf("insert poi %s/%s: %w", p.Zone, p.Label, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	// VACUUM cannot run inside a transaction and materially shrinks the file.
	if _, err := db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	return nil
}

const mapsSchema = `
CREATE TABLE map_layer (
  zone   TEXT    NOT NULL,
  layer  INTEGER NOT NULL,
  nlines INTEGER NOT NULL,
  lines  BLOB    NOT NULL,
  PRIMARY KEY (zone, layer)
) WITHOUT ROWID;

CREATE TABLE map_zone (
  zone        TEXT PRIMARY KEY,
  min_x       INTEGER NOT NULL,
  min_y       INTEGER NOT NULL,
  max_x       INTEGER NOT NULL,
  max_y       INTEGER NOT NULL,
  technique   TEXT    NOT NULL,
  occupancy   REAL    NOT NULL,
  bnd_density REAL    NOT NULL,
  z_span      REAL    NOT NULL,
  min_z       INTEGER NOT NULL,
  max_z       INTEGER NOT NULL
) WITHOUT ROWID;

CREATE TABLE map_poi (
  id       INTEGER PRIMARY KEY,
  zone     TEXT    NOT NULL,
  x        INTEGER NOT NULL,
  y        INTEGER NOT NULL,
  z        INTEGER NOT NULL,
  category TEXT    NOT NULL,
  label    TEXT    NOT NULL,
  -- source is load-bearing, not documentation: a quarm.db data release
  -- regenerates every db:* row from scratch, and anything else (hand research,
  -- community submissions) must survive that untouched. Without it there is no
  -- safe way to refresh one and keep the other.
  source   TEXT    NOT NULL,
  ref_id   INTEGER,
  UNIQUE (zone, x, y, category, label)
);

CREATE INDEX map_poi_zone ON map_poi (zone, category);
`

// ZoneOutput is one zone's finished map data, ready to write.
type ZoneOutput struct {
	Zone     string
	Segments []Segment
	// Detail is the optional boundary layer drawn under/over the primary one.
	Detail []Segment
	// Outline is the clean layer shown in Outline mode. Present for every zone.
	Outline         []Segment
	MinX, MinY      float64
	MaxX, MaxY      float64
	Technique       Technique
	Occupancy       float64
	BoundaryDensity float64
	ZSpan           float64
}

func packSegments(segs []Segment) ([]byte, error) {
	raw := make([]byte, 0, len(segs)*segmentBytes)
	var buf [segmentBytes]byte
	for _, s := range segs {
		put := func(off int, v float64) {
			binary.LittleEndian.PutUint16(buf[off:], uint16(int16(clampCoord(v))))
		}
		put(0, s.A.X)
		put(2, s.A.Y)
		put(4, s.A.Z)
		put(6, s.B.X)
		put(8, s.B.Y)
		put(10, s.B.Z)
		// Colour is reserved: the renderer themes segments itself rather than
		// baking Brewall's palette in. Kept in the format so per-category
		// colouring (traps, zone lines) can land without a schema change.
		buf[12], buf[13], buf[14] = 0, 0, 0
		raw = append(raw, buf[:]...)
	}

	var out bytes.Buffer
	zw, err := zlib.NewWriterLevel(&out, zlib.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("open zlib writer: %w", err)
	}
	if _, err := zw.Write(raw); err != nil {
		return nil, fmt.Errorf("compress segments: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("flush zlib: %w", err)
	}
	return out.Bytes(), nil
}

// UnpackSegments reverses packSegments. Used by the validation harness and the
// API layer; kept beside the writer so the two formats cannot drift.
func UnpackSegments(blob []byte) ([]Segment, error) {
	zr, err := zlib.NewReader(bytes.NewReader(blob))
	if err != nil {
		return nil, fmt.Errorf("open zlib reader: %w", err)
	}
	defer zr.Close()
	var raw bytes.Buffer
	if _, err := raw.ReadFrom(zr); err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	b := raw.Bytes()
	if len(b)%segmentBytes != 0 {
		return nil, fmt.Errorf("blob is %d bytes, not a multiple of %d", len(b), segmentBytes)
	}
	segs := make([]Segment, 0, len(b)/segmentBytes)
	for p := 0; p+segmentBytes <= len(b); p += segmentBytes {
		i16 := func(off int) float64 {
			return float64(int16(binary.LittleEndian.Uint16(b[p+off:])))
		}
		segs = append(segs, Segment{
			A: Vec3{X: i16(0), Y: i16(2), Z: i16(4)},
			B: Vec3{X: i16(6), Y: i16(8), Z: i16(10)},
		})
	}
	return segs, nil
}

// View-fit trimming. See segmentXYRange.
const (
	// viewTrimPercentile is how much of the endpoint distribution to ignore at
	// each end. Density-based on purpose: a long corridor is drawn with many
	// endpoints along its length and never lands in the tail, while a single
	// enormous water quad contributes only a handful of corners and always does.
	viewTrimPercentile = 0.005
	// viewTrimPadding re-expands the trimmed box, so content just outside the
	// percentile is still on screen rather than sitting one pixel past the edge.
	viewTrimPadding = 0.10
	// viewTrimMinGain is how much canvas area trimming must reclaim before it is
	// worth doing. Below this the untouched bounds are kept, because moving the
	// default view is only justified when it buys something obvious.
	viewTrimMinGain = 1.5
)

// segmentXYRange returns the footprint to fit the view to, and whether there was
// anything to measure.
//
// Not simply min/max. Several zones include large areas nothing can reach —
// South Qeynos and East Freeport are ringed by open water that is part of the
// zone mesh — and fitting to the extremes shrinks the part anyone cares about
// into a corner. East Freeport spends 6.6x more canvas on ocean than on the city.
//
// This trims to a percentile of the endpoint distribution instead, which is
// safe for the usual reason a density measure is: geometry people walk through
// is drawn with many endpoints, and a big empty quad has four corners.
//
// Crucially this changes *only the default view*, never the data. Every segment
// is still written and still drawn; trimmed content sits outside the initial
// fit and is reached by panning, so a mistake here costs a scroll rather than
// information.
func segmentXYRange(layers ...[]Segment) (minX, minY, maxX, maxY float64, ok bool) {
	var xs, ys []float64
	for _, segs := range layers {
		for _, s := range segs {
			xs = append(xs, s.A.X, s.B.X)
			ys = append(ys, s.A.Y, s.B.Y)
		}
	}
	if len(xs) == 0 {
		return 0, 0, 0, 0, false
	}
	sort.Float64s(xs)
	sort.Float64s(ys)

	fullMinX, fullMaxX := xs[0], xs[len(xs)-1]
	fullMinY, fullMaxY := ys[0], ys[len(ys)-1]

	// Too few segments for a percentile to mean anything.
	if len(xs) < 200 {
		return fullMinX, fullMinY, fullMaxX, fullMaxY, true
	}

	at := func(v []float64, p float64) float64 {
		i := int(float64(len(v)) * p)
		if i >= len(v) {
			i = len(v) - 1
		}
		return v[i]
	}
	tMinX, tMaxX := at(xs, viewTrimPercentile), at(xs, 1-viewTrimPercentile)
	tMinY, tMaxY := at(ys, viewTrimPercentile), at(ys, 1-viewTrimPercentile)

	padX := (tMaxX - tMinX) * viewTrimPadding
	padY := (tMaxY - tMinY) * viewTrimPadding
	tMinX, tMaxX = tMinX-padX, tMaxX+padX
	tMinY, tMaxY = tMinY-padY, tMaxY+padY

	fullArea := (fullMaxX - fullMinX) * (fullMaxY - fullMinY)
	trimArea := (tMaxX - tMinX) * (tMaxY - tMinY)
	if trimArea <= 0 || fullArea/trimArea < viewTrimMinGain {
		return fullMinX, fullMinY, fullMaxX, fullMaxY, true
	}
	// Never expand past the real extent — padding could otherwise push the view
	// out into empty space it was meant to remove.
	return math.Max(tMinX, fullMinX), math.Max(tMinY, fullMinY),
		math.Min(tMaxX, fullMaxX), math.Min(tMaxY, fullMaxY), true
}

// segmentZRange returns the height range spanned by every given layer.
func segmentZRange(layers ...[]Segment) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, segs := range layers {
		for _, s := range segs {
			lo = math.Min(lo, math.Min(s.A.Z, s.B.Z))
			hi = math.Max(hi, math.Max(s.A.Z, s.B.Z))
		}
	}
	if math.IsInf(lo, 1) {
		return 0, 0
	}
	return lo, hi
}

func clampCoord(v float64) int {
	r := math.Round(v)
	if r < coordMin {
		return coordMin
	}
	if r > coordMax {
		return coordMax
	}
	return int(r)
}

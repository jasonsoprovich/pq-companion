// Package maps serves the zone maps built offline by cmd/mapgen.
//
// maps.db is a separate artifact from quarm.db on purpose: it is derived from
// EQ client geometry on its own cadence, so a quarm.db data release must not be
// able to clobber it. See docs/maps-feasibility.md.
package maps

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// segmentBytes mirrors the packed layout cmd/mapgen writes: six int16
// coordinates plus three colour bytes.
const segmentBytes = 6*2 + 3

// Store reads maps.db. A nil *Store is valid and reports no maps, so a build
// shipped without the file degrades to "no maps available" rather than failing
// to start.
type Store struct {
	db   *sql.DB
	once sync.Once
	// zones caches the zone metadata table. It is ~178 small rows and every
	// request needs bounds, so it is read once rather than joined repeatedly.
	zones map[string]Zone
	err   error
}

// Zone is a zone's render metadata.
type Zone struct {
	Zone      string  `json:"zone"`
	MinX      int     `json:"min_x"`
	MinY      int     `json:"min_y"`
	MaxX      int     `json:"max_x"`
	MaxY      int     `json:"max_y"`
	Technique string  `json:"technique"`
	ZSpan     float64 `json:"z_span"`
	// MinZ/MaxZ bound the heights actually present in this zone's drawn
	// segments. Needed because the depth control has to offer a range that
	// overlaps the data: z_span alone is only a width, and a slider guessing a
	// range symmetric about zero missed the geometry entirely in every zone whose
	// heights do not happen to straddle 0.
	MinZ int `json:"min_z"`
	MaxZ int `json:"max_z"`
}

// Segment is one drawn line, in map space.
type Segment struct {
	X1, Y1, Z1 int
	X2, Y2, Z2 int
}

// POI is one labelled point.
type POI struct {
	ID       int    `json:"id"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Z        int    `json:"z"`
	Category string `json:"category"`
	Label    string `json:"label"`
	Source   string `json:"source"`
	RefID    int    `json:"ref_id,omitempty"`
}

// Open opens maps.db read-only. A missing file is not an error: it returns a
// nil Store, and every method on a nil Store reports "no maps".
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	// immutable=1 for the same reason quarm.db uses it: the file may be
	// installed under a non-writable directory, where SQLite would otherwise
	// try to create -shm alongside it and fail.
	dsn := "file:" + path + "?mode=ro&immutable=1&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open maps.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping maps.db: %w", err)
	}
	return &Store{db: db}, nil
}

// Available reports whether maps are usable.
func (s *Store) Available() bool { return s != nil && s.db != nil }

// Close releases the connection.
func (s *Store) Close() error {
	if !s.Available() {
		return nil
	}
	return s.db.Close()
}

func (s *Store) loadZones() error {
	s.once.Do(func() {
		rows, err := s.db.Query(
			`SELECT zone, min_x, min_y, max_x, max_y, technique, z_span, min_z, max_z
			   FROM map_zone`)
		if err != nil {
			s.err = fmt.Errorf("list map zones: %w", err)
			return
		}
		defer rows.Close()
		s.zones = map[string]Zone{}
		for rows.Next() {
			var z Zone
			if err := rows.Scan(&z.Zone, &z.MinX, &z.MinY, &z.MaxX, &z.MaxY,
				&z.Technique, &z.ZSpan, &z.MinZ, &z.MaxZ); err != nil {
				s.err = fmt.Errorf("scan map zone: %w", err)
				return
			}
			s.zones[z.Zone] = z
		}
		s.err = rows.Err()
	})
	return s.err
}

// Zones returns metadata for every zone that has a map.
func (s *Store) Zones() ([]Zone, error) {
	if !s.Available() {
		return nil, nil
	}
	if err := s.loadZones(); err != nil {
		return nil, err
	}
	out := make([]Zone, 0, len(s.zones))
	for _, z := range s.zones {
		out = append(out, z)
	}
	return out, nil
}

// Zone returns one zone's metadata, or ok=false when it has no map.
func (s *Store) Zone(short string) (Zone, bool, error) {
	if !s.Available() {
		return Zone{}, false, nil
	}
	if err := s.loadZones(); err != nil {
		return Zone{}, false, err
	}
	z, ok := s.zones[short]
	return z, ok, nil
}

// Segments returns a zone layer's geometry, unpacked from its blob.
func (s *Store) Segments(short string, layer int) ([]Segment, error) {
	if !s.Available() {
		return nil, nil
	}
	var blob []byte
	err := s.db.QueryRow(
		`SELECT lines FROM map_layer WHERE zone = ? AND layer = ?`, short, layer).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s layer %d: %w", short, layer, err)
	}
	return unpack(blob)
}

// POIs returns a zone's points of interest.
func (s *Store) POIs(short string) ([]POI, error) {
	if !s.Available() {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT id, x, y, z, category, label, source, COALESCE(ref_id, 0)
		 FROM map_poi WHERE zone = ? ORDER BY category, label`, short)
	if err != nil {
		return nil, fmt.Errorf("read %s POIs: %w", short, err)
	}
	defer rows.Close()
	out := []POI{}
	for rows.Next() {
		var p POI
		if err := rows.Scan(&p.ID, &p.X, &p.Y, &p.Z, &p.Category, &p.Label,
			&p.Source, &p.RefID); err != nil {
			return nil, fmt.Errorf("scan %s POI: %w", short, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func unpack(blob []byte) ([]Segment, error) {
	raw, err := inflate(blob)
	if err != nil {
		return nil, err
	}
	if len(raw)%segmentBytes != 0 {
		return nil, fmt.Errorf("segment blob is %d bytes, not a multiple of %d",
			len(raw), segmentBytes)
	}
	segs := make([]Segment, 0, len(raw)/segmentBytes)
	for p := 0; p+segmentBytes <= len(raw); p += segmentBytes {
		i16 := func(off int) int { return int(int16(binary.LittleEndian.Uint16(raw[p+off:]))) }
		segs = append(segs, Segment{
			X1: i16(0), Y1: i16(2), Z1: i16(4),
			X2: i16(6), Y2: i16(8), Z2: i16(10),
		})
	}
	return segs, nil
}

// DefaultPath returns maps.db next to the executable, falling back to the
// repo-relative development path — the same shape as quarm.db's resolver.
func DefaultPath() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "data", "maps.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("data", "maps.db")
}

// POIsByCategory returns every POI in the given categories, grouped by zone.
//
// Used by the in-game map export, which is a whole-corpus operation rather than
// a per-zone one — 178 separate queries to build one export would be a lot of
// round trips for data that is already a single small table.
func (s *Store) POIsByCategory(categories []string) (map[string][]POI, error) {
	if !s.Available() || len(categories) == 0 {
		return map[string][]POI{}, nil
	}
	ph := strings.Repeat("?,", len(categories))
	ph = ph[:len(ph)-1]
	args := make([]any, len(categories))
	for i, c := range categories {
		args[i] = c
	}
	rows, err := s.db.Query(
		`SELECT zone, id, x, y, z, category, label, source, COALESCE(ref_id, 0)
		 FROM map_poi WHERE category IN (`+ph+`) ORDER BY zone, category, label`, args...)
	if err != nil {
		return nil, fmt.Errorf("read POIs by category: %w", err)
	}
	defer rows.Close()
	out := map[string][]POI{}
	for rows.Next() {
		var zone string
		var p POI
		if err := rows.Scan(&zone, &p.ID, &p.X, &p.Y, &p.Z, &p.Category,
			&p.Label, &p.Source, &p.RefID); err != nil {
			return nil, fmt.Errorf("scan POI: %w", err)
		}
		out[zone] = append(out[zone], p)
	}
	return out, rows.Err()
}

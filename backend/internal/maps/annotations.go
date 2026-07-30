package maps

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// UserAnnotation is a marker the user placed on a zone map themselves.
//
// Stored in user.db, never in maps.db. maps.db is a shipped, read-only artifact
// replaced wholesale by every app update, so anything written there would be
// destroyed on the next release — which is the same reason the provenance column
// exists in the first place (docs/maps-feasibility.md 7.3). Keeping user
// annotations in user.db means they survive updates, app backups and restores
// like every other thing the user authored.
type UserAnnotation struct {
	ID   int64  `json:"id"`
	Zone string `json:"zone"`
	// X, Y, Z are map-space coordinates, matching the geometry and POI layers,
	// so the renderer needs no special case for these.
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
	// Category is one of mapgen's annotation categories: wall, hazard, note.
	Category  string `json:"category"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AnnotationStore persists user-placed map markers in user.db.
type AnnotationStore struct {
	db *sql.DB
}

// userAnnotationCategories mirrors mapgen's set. Duplicated rather than imported
// because internal/maps is the read-side package and must not depend on the
// offline build tooling; the pair is small and a mismatch fails the API request
// rather than corrupting data.
var userAnnotationCategories = map[string]bool{
	"wall": true, "hazard": true, "note": true,
}

// OpenAnnotationStore opens its own connection to user.db, matching every other
// store in the app — WAL handles the concurrent access, and a shared handle
// would couple this store's lifetime to an unrelated one's.
func OpenAnnotationStore(path string) (*AnnotationStore, error) {
	// Same DSN as the other user.db stores: WAL, a generous busy timeout, and an
	// immediate write lock so a read-then-write tx waits on the busy handler
	// instead of failing SQLITE_BUSY outright.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)&_txlock=immediate", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open user.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping user.db: %w", err)
	}
	s, err := NewAnnotationStore(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *AnnotationStore) Close() error { return s.db.Close() }

// NewAnnotationStore prepares the table on an existing user.db handle.
func NewAnnotationStore(db *sql.DB) (*AnnotationStore, error) {
	s := &AnnotationStore{db: db}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS map_annotations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			zone       TEXT    NOT NULL,
			x          INTEGER NOT NULL,
			y          INTEGER NOT NULL,
			z          INTEGER NOT NULL,
			category   TEXT    NOT NULL,
			label      TEXT    NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)
	`); err != nil {
		return nil, fmt.Errorf("create map_annotations: %w", err)
	}
	if _, err := db.Exec(
		`CREATE INDEX IF NOT EXISTS map_annotations_zone ON map_annotations(zone)`); err != nil {
		return nil, fmt.Errorf("index map_annotations: %w", err)
	}
	return s, nil
}

// List returns the user's annotations for one zone, oldest first.
func (s *AnnotationStore) List(zone string) ([]UserAnnotation, error) {
	rows, err := s.db.Query(`
		SELECT id, zone, x, y, z, category, label, created_at, updated_at
		FROM map_annotations WHERE zone = ? ORDER BY created_at`, zone)
	if err != nil {
		return nil, fmt.Errorf("list annotations: %w", err)
	}
	defer rows.Close()
	out := []UserAnnotation{}
	for rows.Next() {
		var a UserAnnotation
		if err := rows.Scan(&a.ID, &a.Zone, &a.X, &a.Y, &a.Z,
			&a.Category, &a.Label, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// All returns every annotation, for export.
func (s *AnnotationStore) All() ([]UserAnnotation, error) {
	rows, err := s.db.Query(`
		SELECT id, zone, x, y, z, category, label, created_at, updated_at
		FROM map_annotations ORDER BY zone, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list all annotations: %w", err)
	}
	defer rows.Close()
	out := []UserAnnotation{}
	for rows.Next() {
		var a UserAnnotation
		if err := rows.Scan(&a.ID, &a.Zone, &a.X, &a.Y, &a.Z,
			&a.Category, &a.Label, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Create adds an annotation and returns it with its assigned id.
func (s *AnnotationStore) Create(a UserAnnotation) (*UserAnnotation, error) {
	if err := validate(&a); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(`
		INSERT INTO map_annotations (zone, x, y, z, category, label, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Zone, a.X, a.Y, a.Z, a.Category, a.Label, now, now)
	if err != nil {
		return nil, fmt.Errorf("insert annotation: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("annotation id: %w", err)
	}
	a.ID, a.CreatedAt, a.UpdatedAt = id, now, now
	return &a, nil
}

// Update changes an existing annotation's category and label. Position is not
// editable: dragging a marker to a new spot is a different gesture from
// correcting its text, and conflating them would let a stray drag silently move
// a note the user meant only to rename.
func (s *AnnotationStore) Update(id int64, category, label string) (*UserAnnotation, error) {
	a := UserAnnotation{ID: id, Category: category, Label: label, Zone: "x", X: 1, Y: 1}
	if err := validate(&a); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`UPDATE map_annotations SET category = ?, label = ?, updated_at = ? WHERE id = ?`,
		category, label, now, id)
	if err != nil {
		return nil, fmt.Errorf("update annotation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("annotation %d not found", id)
	}
	var out UserAnnotation
	if err := s.db.QueryRow(`
		SELECT id, zone, x, y, z, category, label, created_at, updated_at
		FROM map_annotations WHERE id = ?`, id).Scan(&out.ID, &out.Zone,
		&out.X, &out.Y, &out.Z, &out.Category, &out.Label,
		&out.CreatedAt, &out.UpdatedAt); err != nil {
		return nil, fmt.Errorf("reload annotation: %w", err)
	}
	return &out, nil
}

// Delete removes one annotation.
func (s *AnnotationStore) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM map_annotations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("annotation %d not found", id)
	}
	return nil
}

func validate(a *UserAnnotation) error {
	if a.Zone == "" {
		return fmt.Errorf("zone is required")
	}
	if !userAnnotationCategories[a.Category] {
		return fmt.Errorf("unknown category %q", a.Category)
	}
	if a.Label == "" {
		return fmt.Errorf("label is required")
	}
	// int16 is what maps.db packs coordinates into, and an export has to be
	// loadable by the offline pipeline, so reject anything it could not carry.
	for _, v := range []int{a.X, a.Y, a.Z} {
		if v < -32768 || v > 32767 {
			return fmt.Errorf("coordinate %d is out of range", v)
		}
	}
	if a.X == 0 && a.Y == 0 {
		return fmt.Errorf("0,0 is the unset-position sentinel, not a location")
	}
	return nil
}

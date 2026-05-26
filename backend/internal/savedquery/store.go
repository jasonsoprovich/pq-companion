// Package savedquery persists user-authored SQL queries that the Developer
// tab's SQL sandbox can recall by name. Queries live in user.db, which is
// preserved across app updates (unlike quarm.db), and can be exported /
// imported as JSON "query packs" so users can share libraries with each
// other.
//
// These are saved queries — text the renderer feeds back into the sandbox
// "Run" endpoint — not SQL VIEW objects. The sandbox runs against quarm.db
// in read-only mode and that file is replaced on every release, so creating
// real views inside it isn't viable for user-owned content.
package savedquery

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// PackKind is the discriminator string written into exported pack files so
// the importer can refuse JSON that wasn't produced by this feature.
const PackKind = "pq-companion.query-pack"

// PackVersion is the schema version of the export pack. Bump only on
// breaking changes; importers should accept any version <= their own.
const PackVersion = 1

// ErrNotFound is returned by Get / Update / Delete when the id doesn't
// match any row.
var ErrNotFound = errors.New("saved query: not found")

// ErrInvalid is returned when a request payload is missing required fields.
var ErrInvalid = errors.New("saved query: name and sql are required")

// SavedQuery is one named, persisted SQL statement. CreatedAt / UpdatedAt
// are epoch seconds in the database; the model surfaces them as time.Time
// so callers can format consistently.
type SavedQuery struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SQL         string    `json:"sql"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Pack is the JSON envelope used for import / export. Kind + Version
// guard against the user opening an unrelated JSON file by accident.
type Pack struct {
	Kind       string          `json:"kind"`
	Version    int             `json:"version"`
	ExportedAt int64           `json:"exported_at"`
	Queries    []PackEntry     `json:"queries"`
}

// PackEntry is the per-query payload inside a Pack. We deliberately drop
// IDs and timestamps on export — importing always creates fresh rows so
// two users sharing the same pack don't fight over the same primary key.
type PackEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

// Store persists saved queries in user.db. Opened independently of the
// other user.db consumers (trigger, character, players, keyring); WAL +
// busy_timeout from the URI mirrors trigger.OpenStore so concurrent
// writers from different stores don't immediately surface SQLITE_BUSY.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) user.db at path and runs schema migrations.
func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open user.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping user.db: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate saved_queries: %w", err)
	}
	return s, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS saved_queries (
			id          TEXT    NOT NULL PRIMARY KEY,
			name        TEXT    NOT NULL,
			description TEXT    NOT NULL DEFAULT '',
			sql         TEXT    NOT NULL,
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_saved_queries_name ON saved_queries(name COLLATE NOCASE)`); err != nil {
		return err
	}
	return nil
}

// List returns every saved query ordered by name (case-insensitive). The
// list is small (a handful per user) so we don't bother paginating.
func (s *Store) List() ([]SavedQuery, error) {
	rows, err := s.db.Query(`SELECT id, name, description, sql, created_at, updated_at FROM saved_queries ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, fmt.Errorf("list saved queries: %w", err)
	}
	defer rows.Close()

	out := make([]SavedQuery, 0, 8)
	for rows.Next() {
		q, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Get returns the saved query with the given id, or ErrNotFound.
func (s *Store) Get(id string) (*SavedQuery, error) {
	row := s.db.QueryRow(`SELECT id, name, description, sql, created_at, updated_at FROM saved_queries WHERE id = ?`, id)
	q, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// Create inserts a new saved query, generating an id and timestamps. The
// caller's q.ID / q.CreatedAt / q.UpdatedAt are ignored — Create assigns
// them — so import flows can pass empty IDs without colliding on insert.
func (s *Store) Create(q *SavedQuery) error {
	name := strings.TrimSpace(q.Name)
	body := strings.TrimSpace(q.SQL)
	if name == "" || body == "" {
		return ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	q.ID = id
	q.Name = name
	q.SQL = body
	q.Description = strings.TrimSpace(q.Description)
	q.CreatedAt = now
	q.UpdatedAt = now
	_, err = s.db.Exec(
		`INSERT INTO saved_queries (id, name, description, sql, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		q.ID, q.Name, q.Description, q.SQL, q.CreatedAt.Unix(), q.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert saved query: %w", err)
	}
	return nil
}

// Update overwrites name/description/sql for an existing id and bumps
// UpdatedAt. CreatedAt is preserved.
func (s *Store) Update(id string, patch SavedQuery) (*SavedQuery, error) {
	name := strings.TrimSpace(patch.Name)
	body := strings.TrimSpace(patch.SQL)
	if name == "" || body == "" {
		return nil, ErrInvalid
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE saved_queries SET name = ?, description = ?, sql = ?, updated_at = ? WHERE id = ?`,
		name, strings.TrimSpace(patch.Description), body, now.Unix(), id,
	)
	if err != nil {
		return nil, fmt.Errorf("update saved query: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return s.Get(id)
}

// Delete removes the saved query with the given id. Returns ErrNotFound
// when the row is already absent.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM saved_queries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete saved query: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ExportPack returns every saved query packaged for sharing. IDs and
// timestamps are deliberately stripped — the receiving user gets fresh
// rows when they import.
func (s *Store) ExportPack() (Pack, error) {
	queries, err := s.List()
	if err != nil {
		return Pack{}, err
	}
	entries := make([]PackEntry, len(queries))
	for i, q := range queries {
		entries[i] = PackEntry{Name: q.Name, Description: q.Description, SQL: q.SQL}
	}
	return Pack{
		Kind:       PackKind,
		Version:    PackVersion,
		ExportedAt: time.Now().UTC().Unix(),
		Queries:    entries,
	}, nil
}

// ImportPack inserts every entry from pack as a new saved query. Entries
// with the same name as an existing row are kept anyway — the user can
// dedupe by hand from the UI. Returns the number of rows inserted.
func (s *Store) ImportPack(pack Pack) (int, error) {
	if pack.Kind != PackKind {
		return 0, fmt.Errorf("saved query: unrecognized pack kind %q", pack.Kind)
	}
	if pack.Version > PackVersion {
		return 0, fmt.Errorf("saved query: pack version %d is newer than supported %d", pack.Version, PackVersion)
	}
	inserted := 0
	for i := range pack.Queries {
		entry := pack.Queries[i]
		q := SavedQuery{
			Name:        entry.Name,
			Description: entry.Description,
			SQL:         entry.SQL,
		}
		if err := s.Create(&q); err != nil {
			// Skip individual bad entries (empty name/sql) rather than abort
			// the whole import — a pack with one malformed row is still
			// useful for the rest.
			if errors.Is(err, ErrInvalid) {
				continue
			}
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

// scan reads one row into a SavedQuery, accepting either *sql.Row or
// *sql.Rows so List and Get can share the body.
func scan(r interface {
	Scan(...any) error
}) (SavedQuery, error) {
	var (
		q          SavedQuery
		createdAt  int64
		updatedAt  int64
	)
	if err := r.Scan(&q.ID, &q.Name, &q.Description, &q.SQL, &createdAt, &updatedAt); err != nil {
		return SavedQuery{}, err
	}
	q.CreatedAt = time.Unix(createdAt, 0).UTC()
	q.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return q, nil
}

// newID returns a 16-char random hex string, matching the style of
// trigger.NewID for consistency across user.db tables.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

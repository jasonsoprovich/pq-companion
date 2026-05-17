// Package players persists rows from /who output so the user can build a
// history of every player they've encountered. Class / race / level / guild
// are remembered across anonymous toggles: anonymous sightings update only
// last-seen-time and zone, never wiping previously-observed non-anonymous
// data.
package players

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Sighting is the public-facing model for one player row in the database.
type Sighting struct {
	Name           string `json:"name"`
	Class          string `json:"class"`
	Race           string `json:"race"`
	Guild          string `json:"guild"`
	LastSeenLevel  int    `json:"last_seen_level"`
	LastSeenZone   string `json:"last_seen_zone"`
	LastSeenAt     int64  `json:"last_seen_at"`
	FirstSeenAt    int64  `json:"first_seen_at"`
	LastAnonymous  bool   `json:"last_anonymous"`
	SightingsCount int    `json:"sightings_count"`
}

// LevelHistoryEntry is one row in player_level_history — recorded only when
// a non-anonymous sighting differs from the previously-known level.
type LevelHistoryEntry struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Level      int    `json:"level"`
	Class      string `json:"class"`
	Zone       string `json:"zone"`
	ObservedAt int64  `json:"observed_at"`
}

// Store wraps the user.db connection and exposes the operations the API and
// event consumer need.
type Store struct {
	db *sql.DB
}

// OpenStore opens a new connection to user.db at the given path and applies
// the player_sightings + player_level_history migrations idempotently.
// Other packages also open their own connections to the same file under WAL
// mode; this works because SQLite WAL allows multiple concurrent readers.
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
		return nil, fmt.Errorf("migrate user.db: %w", err)
	}
	return s, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS player_sightings (
			name              TEXT    NOT NULL PRIMARY KEY,
			class             TEXT    NOT NULL DEFAULT '',
			race              TEXT    NOT NULL DEFAULT '',
			guild             TEXT    NOT NULL DEFAULT '',
			last_seen_level   INTEGER NOT NULL DEFAULT 0,
			last_seen_zone    TEXT    NOT NULL DEFAULT '',
			last_seen_at      INTEGER NOT NULL,
			first_seen_at     INTEGER NOT NULL,
			last_anonymous    INTEGER NOT NULL DEFAULT 0,
			sightings_count   INTEGER NOT NULL DEFAULT 1
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS player_sightings_class ON player_sightings(class)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS player_sightings_zone ON player_sightings(last_seen_zone)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS player_sightings_seen ON player_sightings(last_seen_at)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS player_level_history (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT    NOT NULL,
			level        INTEGER NOT NULL,
			class        TEXT    NOT NULL,
			zone         TEXT    NOT NULL,
			observed_at  INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS player_level_history_name ON player_level_history(name)`); err != nil {
		return err
	}
	return nil
}

// Sighting input represents a single observed /who row plus the active zone
// and observation timestamp. Keeps the upsert API decoupled from logparser
// types so future producers (Zeal pipe later?) can call without dragging
// log-specific structs in.
type SightingInput struct {
	Name      string
	Level     int
	Class     string
	Race      string
	Guild     string
	Anonymous bool
	Zone      string
	ObservedAt time.Time
}

// Upsert applies a sighting to player_sightings using the anonymous-aware
// merge rule:
//
//   - Non-anonymous sighting: writes class / race / guild / level / zone /
//     last_seen_at, clears last_anonymous, bumps sightings_count. If the
//     level changed from the prior known level, also appends a row to
//     player_level_history.
//   - Anonymous sighting on an existing row: only last_seen_at / zone /
//     count change; class / race / guild / level are preserved from the
//     last non-anonymous sighting.
//   - Anonymous sighting on a brand-new player: inserted with empty class /
//     race / guild / level and last_anonymous=1.
func (s *Store) Upsert(in SightingInput) error {
	if in.Name == "" {
		return fmt.Errorf("name required")
	}
	now := in.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		existsRow      bool
		prevLevel      int
		prevClass      string
	)
	row := tx.QueryRow(`SELECT last_seen_level, class FROM player_sightings WHERE name = ?`, in.Name)
	if err := row.Scan(&prevLevel, &prevClass); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else {
		existsRow = true
	}

	if in.Anonymous {
		if existsRow {
			if _, err := tx.Exec(`
				UPDATE player_sightings
				SET last_seen_zone = COALESCE(NULLIF(?, ''), last_seen_zone),
				    last_seen_at = ?,
				    last_anonymous = 1,
				    sightings_count = sightings_count + 1
				WHERE name = ?
			`, in.Zone, nowUnix, in.Name); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(`
				INSERT INTO player_sightings
					(name, last_seen_zone, last_seen_at, first_seen_at, last_anonymous, sightings_count)
				VALUES (?, ?, ?, ?, 1, 1)
			`, in.Name, in.Zone, nowUnix, nowUnix); err != nil {
				return err
			}
		}
	} else {
		if existsRow {
			if _, err := tx.Exec(`
				UPDATE player_sightings
				SET class = ?,
				    race = ?,
				    guild = ?,
				    last_seen_level = ?,
				    last_seen_zone = COALESCE(NULLIF(?, ''), last_seen_zone),
				    last_seen_at = ?,
				    last_anonymous = 0,
				    sightings_count = sightings_count + 1
				WHERE name = ?
			`, in.Class, in.Race, in.Guild, in.Level, in.Zone, nowUnix, in.Name); err != nil {
				return err
			}
			// Record a level-history row when level moved.
			if in.Level > 0 && in.Level != prevLevel {
				if _, err := tx.Exec(`
					INSERT INTO player_level_history (name, level, class, zone, observed_at)
					VALUES (?, ?, ?, ?, ?)
				`, in.Name, in.Level, in.Class, in.Zone, nowUnix); err != nil {
					return err
				}
			}
		} else {
			if _, err := tx.Exec(`
				INSERT INTO player_sightings
					(name, class, race, guild, last_seen_level, last_seen_zone, last_seen_at, first_seen_at, last_anonymous, sightings_count)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 1)
			`, in.Name, in.Class, in.Race, in.Guild, in.Level, in.Zone, nowUnix, nowUnix); err != nil {
				return err
			}
			if in.Level > 0 {
				if _, err := tx.Exec(`
					INSERT INTO player_level_history (name, level, class, zone, observed_at)
					VALUES (?, ?, ?, ?, ?)
				`, in.Name, in.Level, in.Class, in.Zone, nowUnix); err != nil {
					return err
				}
			}
		}
	}

	return tx.Commit()
}

// UpdateGuild applies a guild-affiliation update for a known player without
// touching class/race/level. Used by the /guildstat handler — that command
// reports only the player's guild membership, not their other stats, so a
// full Upsert would clobber known data. Creates the row when the player is
// otherwise unseen.
func (s *Store) UpdateGuild(name, guild, zone string, observedAt time.Time) error {
	if name == "" || guild == "" {
		return fmt.Errorf("name and guild required")
	}
	now := observedAt
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var exists bool
	row := tx.QueryRow(`SELECT 1 FROM player_sightings WHERE name = ?`, name)
	var sentinel int
	if err := row.Scan(&sentinel); err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else {
		exists = true
	}

	if exists {
		if _, err := tx.Exec(`
			UPDATE player_sightings
			SET guild = ?,
			    last_seen_zone = COALESCE(NULLIF(?, ''), last_seen_zone),
			    last_seen_at = ?,
			    sightings_count = sightings_count + 1
			WHERE name = ?
		`, guild, zone, nowUnix, name); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`
			INSERT INTO player_sightings
				(name, guild, last_seen_zone, last_seen_at, first_seen_at, last_anonymous, sightings_count)
			VALUES (?, ?, ?, ?, ?, 0, 1)
		`, name, guild, zone, nowUnix, nowUnix); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SearchFilters narrows the list of returned sightings.
type SearchFilters struct {
	NameContains string
	Class        string
	Zone         string
	Guild        string
	Limit        int
	Offset       int
}

// Search returns sightings matching the filters, newest-first.
func (s *Store) Search(f SearchFilters) ([]Sighting, error) {
	q := `SELECT name, class, race, guild, last_seen_level, last_seen_zone,
	             last_seen_at, first_seen_at, last_anonymous, sightings_count
	      FROM player_sightings
	      WHERE 1=1`
	args := []any{}
	if f.NameContains != "" {
		q += ` AND name LIKE ? COLLATE NOCASE`
		args = append(args, "%"+f.NameContains+"%")
	}
	if f.Class != "" {
		// Expand the filter against the class-title alias table so picking
		// "Enchanter" in the dropdown also matches Illusionists, Beguilers
		// and Phantasmists. expandClassFilter falls back to a single-element
		// slice for unknown / specific-title queries so direct matches still
		// work.
		titles := expandClassFilter(f.Class)
		if len(titles) == 1 {
			q += ` AND class = ? COLLATE NOCASE`
			args = append(args, titles[0])
		} else {
			placeholders := strings.Repeat("?,", len(titles))
			placeholders = placeholders[:len(placeholders)-1]
			q += ` AND class IN (` + placeholders + `) COLLATE NOCASE`
			for _, t := range titles {
				args = append(args, t)
			}
		}
	}
	if f.Zone != "" {
		q += ` AND last_seen_zone = ? COLLATE NOCASE`
		args = append(args, f.Zone)
	}
	if f.Guild != "" {
		q += ` AND guild = ? COLLATE NOCASE`
		args = append(args, f.Guild)
	}
	q += ` ORDER BY last_seen_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, f.Offset)
		}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sighting
	for rows.Next() {
		var v Sighting
		var lastAnon int
		if err := rows.Scan(&v.Name, &v.Class, &v.Race, &v.Guild, &v.LastSeenLevel,
			&v.LastSeenZone, &v.LastSeenAt, &v.FirstSeenAt, &lastAnon, &v.SightingsCount); err != nil {
			return nil, err
		}
		v.LastAnonymous = lastAnon != 0
		out = append(out, v)
	}
	return out, rows.Err()
}

// Get returns a single sighting by name, or (nil, nil) if not found.
func (s *Store) Get(name string) (*Sighting, error) {
	row := s.db.QueryRow(`
		SELECT name, class, race, guild, last_seen_level, last_seen_zone,
		       last_seen_at, first_seen_at, last_anonymous, sightings_count
		FROM player_sightings WHERE name = ? COLLATE NOCASE
	`, name)
	var v Sighting
	var lastAnon int
	if err := row.Scan(&v.Name, &v.Class, &v.Race, &v.Guild, &v.LastSeenLevel,
		&v.LastSeenZone, &v.LastSeenAt, &v.FirstSeenAt, &lastAnon, &v.SightingsCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	v.LastAnonymous = lastAnon != 0
	return &v, nil
}

// LevelHistory returns the level-progression rows for a player, oldest-first.
func (s *Store) LevelHistory(name string) ([]LevelHistoryEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, name, level, class, zone, observed_at
		FROM player_level_history
		WHERE name = ? COLLATE NOCASE
		ORDER BY observed_at ASC
	`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LevelHistoryEntry
	for rows.Next() {
		var e LevelHistoryEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Level, &e.Class, &e.Zone, &e.ObservedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Delete removes a single player.
func (s *Store) Delete(name string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM player_sightings WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM player_level_history WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	return tx.Commit()
}

// Clear wipes both tables. Returns number of sightings deleted (level
// history rows are deleted in the same transaction).
func (s *Store) Clear() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	res, err := tx.Exec(`DELETE FROM player_sightings`)
	if err != nil {
		return 0, err
	}
	deleted, _ := res.RowsAffected()
	if _, err := tx.Exec(`DELETE FROM player_level_history`); err != nil {
		return 0, err
	}
	return int(deleted), tx.Commit()
}

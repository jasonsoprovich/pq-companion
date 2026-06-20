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
	Note           string `json:"note"`
	PVP            bool   `json:"pvp"`
	TellCount      int    `json:"tell_count"`
	LastTellAt     int64  `json:"last_tell_at"`
	GroupCount     int    `json:"group_count"`
	LastGroupedAt  int64  `json:"last_grouped_at"`

	// Manual* are the user-entered overrides for players who stay
	// anonymous (always hidden) so the app can still colour their DPS bars
	// and show their class/level. They're stored alongside the note + PVP
	// flag and survive a "Clear all".
	ManualClass string `json:"manual_class"`
	ManualLevel int    `json:"manual_level"`
	ManualRace  string `json:"manual_race"`

	// Effective* resolve display/colours: a real /who-discovered value always
	// wins, falling back to the manual override only when nothing was ever
	// seen un-anonymously.
	EffectiveClass string `json:"effective_class"`
	EffectiveLevel int    `json:"effective_level"`
	EffectiveRace  string `json:"effective_race"`
}

// computeEffective fills the Effective* fields from the discovered values,
// falling back to the manual overrides. Discovered always wins (a /who
// sighting is ground truth); manual only fills the gaps for players never
// seen un-anonymously.
func (v *Sighting) computeEffective() {
	v.EffectiveClass = v.Class
	if v.EffectiveClass == "" {
		v.EffectiveClass = v.ManualClass
	}
	v.EffectiveLevel = v.LastSeenLevel
	if v.EffectiveLevel == 0 {
		v.EffectiveLevel = v.ManualLevel
	}
	v.EffectiveRace = v.Race
	if v.EffectiveRace == "" {
		v.EffectiveRace = v.ManualRace
	}
}

// Interaction kinds recorded by TouchInteraction.
const (
	InteractionTell  = "tell"
	InteractionGroup = "group"
)

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
	// User-authored notes + PVP flags live apart from sightings so the user's
	// curated intel survives a tracker "Clear all" — the note re-attaches the
	// next time the player shows up in a /who.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS player_notes (
			name        TEXT    NOT NULL PRIMARY KEY,
			note        TEXT    NOT NULL DEFAULT '',
			pvp         INTEGER NOT NULL DEFAULT 0,
			updated_at  INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	// Manual class/level/race overrides live on the notes row (same user-intel
	// table, same survives-Clear semantics). Added via ALTER for upgrading
	// users whose player_notes predates these columns.
	cols, err := s.tableColumns("player_notes")
	if err != nil {
		return err
	}
	for _, c := range []struct{ name, decl string }{
		{"manual_class", "manual_class TEXT NOT NULL DEFAULT ''"},
		{"manual_level", "manual_level INTEGER NOT NULL DEFAULT 0"},
		{"manual_race", "manual_race TEXT NOT NULL DEFAULT ''"},
	} {
		if cols[c.name] {
			continue
		}
		if _, err := s.db.Exec(`ALTER TABLE player_notes ADD COLUMN ` + c.decl); err != nil {
			return err
		}
	}
	// Direct interactions (tells exchanged, group joins) — auto-tracked so
	// the tracker shows who the user has actually played with, not just who
	// walked past a /who.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS player_interactions (
			name      TEXT    NOT NULL COLLATE NOCASE,
			kind      TEXT    NOT NULL,
			first_at  INTEGER NOT NULL,
			last_at   INTEGER NOT NULL,
			count     INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (name, kind)
		)
	`); err != nil {
		return err
	}
	return nil
}

// tableColumns returns the set of column names on a table, used to make
// ALTER TABLE migrations idempotent for upgrading users.
func (s *Store) tableColumns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var (
			cid         int
			name, ctype string
			notnull, pk int
			dfltValue   sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// TouchInteraction records one direct interaction (a tell exchanged or a
// group join) for a player, creating a minimal tracker row when the player
// has never appeared in a /who — that's the "auto track who I play with"
// behaviour. The minimal row carries sightings_count 0 so /who statistics
// stay truthful.
func (s *Store) TouchInteraction(name, kind string, at time.Time) error {
	if name == "" || kind == "" {
		return fmt.Errorf("name and kind required")
	}
	if at.IsZero() {
		at = time.Now()
	}
	ts := at.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	var sentinel int
	err = tx.QueryRow(`SELECT 1 FROM player_sightings WHERE name = ? COLLATE NOCASE`, name).Scan(&sentinel)
	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`
			INSERT INTO player_sightings (name, last_seen_at, first_seen_at, last_anonymous, sightings_count)
			VALUES (?, ?, ?, 1, 0)
		`, name, ts, ts); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO player_interactions (name, kind, first_at, last_at, count)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(name, kind) DO UPDATE SET
			first_at = MIN(excluded.first_at, first_at),
			last_at  = MAX(excluded.last_at, last_at),
			count    = count + 1
	`, name, kind, ts, ts); err != nil {
		return err
	}
	return tx.Commit()
}

// Sighting input represents a single observed /who row plus the active zone
// and observation timestamp. Keeps the upsert API decoupled from logparser
// types so future producers (Zeal pipe later?) can call without dragging
// log-specific structs in.
type SightingInput struct {
	Name       string
	Level      int
	Class      string
	Race       string
	Guild      string
	Anonymous  bool
	Zone       string
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
		existsRow bool
		prevLevel int
		prevClass string
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

// BackfillUpsert applies a historical /who sighting from a log replay in a
// timestamp-aware, idempotent way — the rules differ from the live Upsert so
// re-running a backfill (or backfilling after live tracking) never inflates
// counts, duplicates level history, or clobbers newer live data:
//
//   - New player: inserted with count 1 and first/last seen = the log
//     timestamp (a level-history row is added for non-anonymous sightings).
//   - Existing player: first_seen_at is pulled EARLIER when the log predates
//     it; last-seen fields are updated only when the log sighting is NEWER than
//     the stored last_seen_at (so live data always wins). sightings_count is
//     never bumped, and level history is added only when that (name, level)
//     pair isn't already recorded.
//
// Returns whether the row was created or changed (false ⇒ already current,
// which is what a second backfill pass produces).
func (s *Store) BackfillUpsert(in SightingInput) (bool, error) {
	if in.Name == "" {
		return false, fmt.Errorf("name required")
	}
	now := in.ObservedAt
	if now.IsZero() {
		now = time.Now()
	}
	nowUnix := now.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		exists      bool
		prevLevel   int
		prevSeenAt  int64
		prevFirstAt int64
	)
	row := tx.QueryRow(`SELECT last_seen_level, last_seen_at, first_seen_at FROM player_sightings WHERE name = ?`, in.Name)
	if err := row.Scan(&prevLevel, &prevSeenAt, &prevFirstAt); err != nil {
		if err != sql.ErrNoRows {
			return false, err
		}
	} else {
		exists = true
	}

	addLevelHistory := func() error {
		if in.Anonymous || in.Level <= 0 {
			return nil
		}
		var seen int
		err := tx.QueryRow(`SELECT 1 FROM player_level_history WHERE name = ? AND level = ?`, in.Name, in.Level).Scan(&seen)
		if err == nil {
			return nil // already recorded this (name, level)
		}
		if err != sql.ErrNoRows {
			return err
		}
		_, err = tx.Exec(`INSERT INTO player_level_history (name, level, class, zone, observed_at) VALUES (?, ?, ?, ?, ?)`,
			in.Name, in.Level, in.Class, in.Zone, nowUnix)
		return err
	}

	changed := false
	if !exists {
		if in.Anonymous {
			if _, err := tx.Exec(`
				INSERT INTO player_sightings (name, last_seen_zone, last_seen_at, first_seen_at, last_anonymous, sightings_count)
				VALUES (?, ?, ?, ?, 1, 1)
			`, in.Name, in.Zone, nowUnix, nowUnix); err != nil {
				return false, err
			}
		} else {
			if _, err := tx.Exec(`
				INSERT INTO player_sightings (name, class, race, guild, last_seen_level, last_seen_zone, last_seen_at, first_seen_at, last_anonymous, sightings_count)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 1)
			`, in.Name, in.Class, in.Race, in.Guild, in.Level, in.Zone, nowUnix, nowUnix); err != nil {
				return false, err
			}
			if err := addLevelHistory(); err != nil {
				return false, err
			}
		}
		changed = true
	} else {
		// Pull first_seen earlier when the log proves an earlier encounter.
		if nowUnix < prevFirstAt {
			if _, err := tx.Exec(`UPDATE player_sightings SET first_seen_at = ? WHERE name = ?`, nowUnix, in.Name); err != nil {
				return false, err
			}
			changed = true
		}
		// Only refresh last-seen data when this sighting is newer than stored.
		if nowUnix > prevSeenAt {
			if in.Anonymous {
				if _, err := tx.Exec(`
					UPDATE player_sightings
					SET last_seen_zone = COALESCE(NULLIF(?, ''), last_seen_zone), last_seen_at = ?, last_anonymous = 1
					WHERE name = ?
				`, in.Zone, nowUnix, in.Name); err != nil {
					return false, err
				}
			} else {
				if _, err := tx.Exec(`
					UPDATE player_sightings
					SET class = ?, race = ?, guild = ?, last_seen_level = ?,
					    last_seen_zone = COALESCE(NULLIF(?, ''), last_seen_zone), last_seen_at = ?, last_anonymous = 0
					WHERE name = ?
				`, in.Class, in.Race, in.Guild, in.Level, in.Zone, nowUnix, in.Name); err != nil {
					return false, err
				}
				if in.Level > 0 && in.Level != prevLevel {
					if err := addLevelHistory(); err != nil {
						return false, err
					}
				}
			}
			changed = true
		}
	}

	if !changed {
		return false, nil // deferred Rollback discards the no-op transaction
	}
	return true, tx.Commit()
}

// SearchFilters narrows the list of returned sightings.
type SearchFilters struct {
	NameContains string
	Class        string
	Zone         string
	Guild        string
	PVPOnly      bool
	Limit        int
	Offset       int
}

// sightingJoins is the FROM clause shared by Search and Count — sightings
// with the user's note + per-kind interaction rollups attached.
const sightingJoins = `
	      FROM player_sightings s
	      LEFT JOIN player_notes n ON n.name = s.name COLLATE NOCASE
	      LEFT JOIN player_interactions ti ON ti.name = s.name AND ti.kind = 'tell'
	      LEFT JOIN player_interactions gi ON gi.name = s.name AND gi.kind = 'group'
	      WHERE 1=1`

// buildFilterClause renders the WHERE conditions for f against the
// sightingJoins aliases. Shared by Search and Count so the total always
// matches the rows a paged fetch walks through.
func buildFilterClause(f SearchFilters) (string, []any) {
	q := ``
	args := []any{}
	if f.NameContains != "" {
		q += ` AND s.name LIKE ? COLLATE NOCASE`
		args = append(args, "%"+f.NameContains+"%")
	}
	if f.Class != "" {
		// Expand the filter against the class-title alias table so picking
		// "Enchanter" in the dropdown also matches Illusionists, Beguilers
		// and Phantasmists. expandClassFilter falls back to a single-element
		// slice for unknown / specific-title queries so direct matches still
		// work. Match the *effective* class so manually-classed always-anon
		// players appear under their set class too.
		const effClass = `COALESCE(NULLIF(s.class, ''), n.manual_class)`
		titles := expandClassFilter(f.Class)
		if len(titles) == 1 {
			q += ` AND ` + effClass + ` = ? COLLATE NOCASE`
			args = append(args, titles[0])
		} else {
			placeholders := strings.Repeat("?,", len(titles))
			placeholders = placeholders[:len(placeholders)-1]
			q += ` AND ` + effClass + ` IN (` + placeholders + `) COLLATE NOCASE`
			for _, t := range titles {
				args = append(args, t)
			}
		}
	}
	if f.Zone != "" {
		q += ` AND s.last_seen_zone = ? COLLATE NOCASE`
		args = append(args, f.Zone)
	}
	if f.Guild != "" {
		q += ` AND s.guild = ? COLLATE NOCASE`
		args = append(args, f.Guild)
	}
	if f.PVPOnly {
		q += ` AND COALESCE(n.pvp, 0) = 1`
	}
	return q, args
}

// Search returns sightings matching the filters, newest-first.
func (s *Store) Search(f SearchFilters) ([]Sighting, error) {
	q := `SELECT s.name, s.class, s.race, s.guild, s.last_seen_level, s.last_seen_zone,
	             s.last_seen_at, s.first_seen_at, s.last_anonymous, s.sightings_count,
	             COALESCE(n.note, ''), COALESCE(n.pvp, 0),
	             COALESCE(ti.count, 0), COALESCE(ti.last_at, 0),
	             COALESCE(gi.count, 0), COALESCE(gi.last_at, 0),
	             COALESCE(n.manual_class, ''), COALESCE(n.manual_level, 0), COALESCE(n.manual_race, '')` + sightingJoins
	clause, args := buildFilterClause(f)
	q += clause
	q += ` ORDER BY s.last_seen_at DESC`
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
		var lastAnon, pvp int
		if err := rows.Scan(&v.Name, &v.Class, &v.Race, &v.Guild, &v.LastSeenLevel,
			&v.LastSeenZone, &v.LastSeenAt, &v.FirstSeenAt, &lastAnon, &v.SightingsCount,
			&v.Note, &pvp, &v.TellCount, &v.LastTellAt, &v.GroupCount, &v.LastGroupedAt,
			&v.ManualClass, &v.ManualLevel, &v.ManualRace); err != nil {
			return nil, err
		}
		v.LastAnonymous = lastAnon != 0
		v.PVP = pvp != 0
		v.computeEffective()
		out = append(out, v)
	}
	return out, rows.Err()
}

// Count returns how many sightings match the filters, ignoring Limit/Offset
// — the "N tracked" total a paged Search walks through.
func (s *Store) Count(f SearchFilters) (int, error) {
	q := `SELECT COUNT(*)` + sightingJoins
	clause, args := buildFilterClause(f)
	q += clause
	var n int
	if err := s.db.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Get returns a single sighting by name, or (nil, nil) if not found.
func (s *Store) Get(name string) (*Sighting, error) {
	row := s.db.QueryRow(`
		SELECT s.name, s.class, s.race, s.guild, s.last_seen_level, s.last_seen_zone,
		       s.last_seen_at, s.first_seen_at, s.last_anonymous, s.sightings_count,
		       COALESCE(n.note, ''), COALESCE(n.pvp, 0),
		       COALESCE(ti.count, 0), COALESCE(ti.last_at, 0),
		       COALESCE(gi.count, 0), COALESCE(gi.last_at, 0),
		       COALESCE(n.manual_class, ''), COALESCE(n.manual_level, 0), COALESCE(n.manual_race, '')
		FROM player_sightings s
		LEFT JOIN player_notes n ON n.name = s.name COLLATE NOCASE
		LEFT JOIN player_interactions ti ON ti.name = s.name AND ti.kind = 'tell'
		LEFT JOIN player_interactions gi ON gi.name = s.name AND gi.kind = 'group'
		WHERE s.name = ? COLLATE NOCASE
	`, name)
	var v Sighting
	var lastAnon, pvp int
	if err := row.Scan(&v.Name, &v.Class, &v.Race, &v.Guild, &v.LastSeenLevel,
		&v.LastSeenZone, &v.LastSeenAt, &v.FirstSeenAt, &lastAnon, &v.SightingsCount,
		&v.Note, &pvp, &v.TellCount, &v.LastTellAt, &v.GroupCount, &v.LastGroupedAt,
		&v.ManualClass, &v.ManualLevel, &v.ManualRace); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	v.LastAnonymous = lastAnon != 0
	v.PVP = pvp != 0
	v.computeEffective()
	return &v, nil
}

// UpsertNote saves the user's note text + PVP flag for a player, leaving any
// manual class/level/race override on the row untouched. A row carrying no
// intel at all (no note, no PVP flag, no manual override) is pruned so the
// table only holds real intel.
func (s *Store) UpsertNote(name, note string, pvp bool) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	pvpInt := 0
	if pvp {
		pvpInt = 1
	}
	if _, err := s.db.Exec(`
		INSERT INTO player_notes (name, note, pvp, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET note = excluded.note, pvp = excluded.pvp, updated_at = excluded.updated_at
	`, name, note, pvpInt, time.Now().Unix()); err != nil {
		return err
	}
	return s.pruneEmptyNote(name)
}

// UpsertManual saves the user's manual class/level/race override for a player,
// leaving the note + PVP flag untouched. Used for guildmates/friends who stay
// permanently anonymous in /who but whose class the user knows. An override
// that clears every field (and leaves no note/PVP) prunes the row.
func (s *Store) UpsertManual(name, class string, level int, race string) error {
	if name == "" {
		return fmt.Errorf("name required")
	}
	if level < 0 {
		level = 0
	}
	if _, err := s.db.Exec(`
		INSERT INTO player_notes (name, manual_class, manual_level, manual_race, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			manual_class = excluded.manual_class,
			manual_level = excluded.manual_level,
			manual_race = excluded.manual_race,
			updated_at = excluded.updated_at
	`, name, class, level, race, time.Now().Unix()); err != nil {
		return err
	}
	return s.pruneEmptyNote(name)
}

// pruneEmptyNote deletes a player_notes row that holds nothing the user
// authored — no note, no PVP flag, and no manual override.
func (s *Store) pruneEmptyNote(name string) error {
	_, err := s.db.Exec(`
		DELETE FROM player_notes
		WHERE name = ? COLLATE NOCASE
		  AND note = '' AND pvp = 0
		  AND manual_class = '' AND manual_level = 0 AND manual_race = ''
	`, name)
	return err
}

// PVPNames returns every PVP-flagged player name, lowercased for
// case-insensitive matching against incoming /who entries.
func (s *Store) PVPNames() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT name FROM player_notes WHERE pvp = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[strings.ToLower(name)] = true
	}
	return out, rows.Err()
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

// Delete removes a single player, including any note — an explicit
// per-player removal means "forget this person entirely".
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
	if _, err := tx.Exec(`DELETE FROM player_notes WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM player_interactions WHERE name = ? COLLATE NOCASE`, name); err != nil {
		return err
	}
	return tx.Commit()
}

// Clear wipes the sighting tables but keeps player_notes — notes and PVP
// flags are user-curated and re-attach when a player is next seen. Returns
// number of sightings deleted (level history rows are deleted in the same
// transaction).
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
	if _, err := tx.Exec(`DELETE FROM player_interactions`); err != nil {
		return 0, err
	}
	return int(deleted), tx.Commit()
}

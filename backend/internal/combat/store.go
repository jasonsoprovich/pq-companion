package combat

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// HistoryStore persists archived combat fights to user.db so the in-memory
// recent-fights ring buffer is no longer the only record. Sized for personal
// use — a heavy week of raiding produces a few thousand fights at most, all
// in single-digit MB total.
//
// Combatant and healer slices are stored as JSON blobs rather than child
// tables. They're small (typically tens of entries, ~200 B each), and we
// never query into them — every list/filter happens on top-level columns.
// JSON keeps the schema flat and migrations cheap.
type HistoryStore struct {
	db *sql.DB
}

// StoredFight is a fight as it lives in user.db. Mirrors FightSummary plus
// the per-row identity and context fields the UI needs for filtering.
type StoredFight struct {
	ID            int64         `json:"id"`
	NPCName       string        `json:"npc_name"`
	Zone          string        `json:"zone"`
	CharacterName string        `json:"character_name"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	Duration      float64       `json:"duration_seconds"`
	TotalDamage   int64         `json:"total_damage"`
	YouDamage     int64         `json:"you_damage"`
	TotalHeal     int64         `json:"total_heal"`
	YouHeal       int64         `json:"you_heal"`
	Combatants    []EntityStats `json:"combatants"`
	Healers       []HealerStats `json:"healers"`
}

// FightFilter captures the query parameters the history list endpoint
// accepts. Zero values mean "no filter" for that field. Limit and Offset
// drive pagination; Limit==0 falls back to a sane default.
type FightFilter struct {
	StartTime     time.Time // inclusive lower bound on fight.start_time
	EndTime       time.Time // inclusive upper bound on fight.start_time
	NPCName       string    // case-insensitive substring match on npc_name
	CharacterName string    // exact match on character_name
	Zone          string    // exact match on zone
	Limit         int
	Offset        int
}

// OpenHistoryStore opens (or creates) the combat history store at path.
// Mirrors the conventions used by trigger/character/backup stores: WAL
// journal, generous busy_timeout for shared-DB contention, single open conn.
func OpenHistoryStore(path string) (*HistoryStore, error) {
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

	s := &HistoryStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate combat history: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *HistoryStore) Close() error { return s.db.Close() }

func (s *HistoryStore) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS combat_fights (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			npc_name        TEXT    NOT NULL,
			zone            TEXT    NOT NULL DEFAULT '',
			character_name  TEXT    NOT NULL DEFAULT '',
			start_time      INTEGER NOT NULL,
			end_time        INTEGER NOT NULL,
			duration_secs   REAL    NOT NULL,
			total_damage    INTEGER NOT NULL DEFAULT 0,
			you_damage      INTEGER NOT NULL DEFAULT 0,
			total_heal      INTEGER NOT NULL DEFAULT 0,
			you_heal        INTEGER NOT NULL DEFAULT 0,
			combatants_json TEXT    NOT NULL DEFAULT '[]',
			healers_json    TEXT    NOT NULL DEFAULT '[]'
		)
	`); err != nil {
		return err
	}
	// Indexes for the common queries: list by recency, filter by NPC name
	// substring, filter by character. zone gets a covering index too in case
	// the user filters by it from the UI.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_combat_fights_start    ON combat_fights(start_time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_combat_fights_npc      ON combat_fights(npc_name)`,
		`CREATE INDEX IF NOT EXISTS idx_combat_fights_char     ON combat_fights(character_name)`,
		`CREATE INDEX IF NOT EXISTS idx_combat_fights_zone     ON combat_fights(zone)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// SaveFight writes one archived fight to user.db. Returns the assigned row
// ID. Marshalling failures are surfaced — the in-memory FightSummary is
// authoritative until SaveFight succeeds, so callers that retry against a
// flapping disk should not double-count.
func (s *HistoryStore) SaveFight(fight FightSummary, zone, character string) (int64, error) {
	combatantsJSON, err := json.Marshal(fight.Combatants)
	if err != nil {
		return 0, fmt.Errorf("marshal combatants: %w", err)
	}
	healersJSON, err := json.Marshal(fight.Healers)
	if err != nil {
		return 0, fmt.Errorf("marshal healers: %w", err)
	}
	res, err := s.db.Exec(`
		INSERT INTO combat_fights (
			npc_name, zone, character_name,
			start_time, end_time, duration_secs,
			total_damage, you_damage,
			total_heal, you_heal,
			combatants_json, healers_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		fight.PrimaryTarget, zone, character,
		fight.StartTime.UnixNano(), fight.EndTime.UnixNano(), fight.Duration,
		fight.TotalDamage, fight.YouDamage,
		fight.TotalHeal, fight.YouHeal,
		string(combatantsJSON), string(healersJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("insert fight: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read insert id: %w", err)
	}
	return id, nil
}

// ListFights returns up to filter.Limit fights matching filter, ordered by
// start_time DESC (most recent first). Limit defaults to 100 and is capped
// at 1000 to keep response sizes predictable.
func (s *HistoryStore) ListFights(filter FightFilter) ([]StoredFight, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	clauses := []string{}
	args := []interface{}{}
	if !filter.StartTime.IsZero() {
		clauses = append(clauses, "start_time >= ?")
		args = append(args, filter.StartTime.UnixNano())
	}
	if !filter.EndTime.IsZero() {
		clauses = append(clauses, "start_time <= ?")
		args = append(args, filter.EndTime.UnixNano())
	}
	if filter.NPCName != "" {
		clauses = append(clauses, "lower(npc_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(filter.NPCName)+"%")
	}
	if filter.CharacterName != "" {
		clauses = append(clauses, "character_name = ?")
		args = append(args, filter.CharacterName)
	}
	if filter.Zone != "" {
		clauses = append(clauses, "zone = ?")
		args = append(args, filter.Zone)
	}

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit, offset)

	rows, err := s.db.Query(`
		SELECT id, npc_name, zone, character_name,
		       start_time, end_time, duration_secs,
		       total_damage, you_damage,
		       total_heal, you_heal,
		       combatants_json, healers_json
		FROM combat_fights `+where+`
		ORDER BY start_time DESC
		LIMIT ? OFFSET ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query fights: %w", err)
	}
	defer rows.Close()

	out := []StoredFight{}
	for rows.Next() {
		f, err := scanFight(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetFight returns the fight with the given ID, or (nil, nil) when no row
// matches. Keeping "not found" as a non-error simplifies the HTTP handler.
func (s *HistoryStore) GetFight(id int64) (*StoredFight, error) {
	row := s.db.QueryRow(`
		SELECT id, npc_name, zone, character_name,
		       start_time, end_time, duration_secs,
		       total_damage, you_damage,
		       total_heal, you_heal,
		       combatants_json, healers_json
		FROM combat_fights WHERE id = ?
	`, id)
	f, err := scanFight(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// DeleteFight removes a single row by ID. Returns the number of rows
// affected (0 if the ID was unknown — not treated as an error).
func (s *HistoryStore) DeleteFight(id int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM combat_fights WHERE id = ?`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteAll wipes the entire combat history. Returns the number of rows
// removed.
func (s *HistoryStore) DeleteAll() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM combat_fights`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PruneOlderThan deletes every fight whose start_time is strictly before
// cutoff. Returns the number of rows removed. Used by the retention
// goroutine and called once on startup so the DB doesn't grow unboundedly.
func (s *HistoryStore) PruneOlderThan(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM combat_fights WHERE start_time < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// HistoryFacets is the set of distinct values currently in the combat
// history table, used to populate filter dropdowns in the UI. Both lists
// are sorted alphabetically for stable rendering.
type HistoryFacets struct {
	Characters []string `json:"characters"`
	Zones      []string `json:"zones"`
}

// Facets returns the distinct character_name and zone values present in
// combat_fights. Empty strings are dropped so old rows that predate the
// per-row character/zone columns don't surface as blank dropdown entries.
func (s *HistoryStore) Facets() (HistoryFacets, error) {
	out := HistoryFacets{Characters: []string{}, Zones: []string{}}
	chars, err := s.distinctNonEmpty(`SELECT DISTINCT character_name FROM combat_fights WHERE character_name != '' ORDER BY character_name COLLATE NOCASE`)
	if err != nil {
		return out, fmt.Errorf("query characters: %w", err)
	}
	out.Characters = chars
	zones, err := s.distinctNonEmpty(`SELECT DISTINCT zone FROM combat_fights WHERE zone != '' ORDER BY zone COLLATE NOCASE`)
	if err != nil {
		return out, fmt.Errorf("query zones: %w", err)
	}
	out.Zones = zones
	return out, nil
}

// distinctNonEmpty runs a single-column query and returns the scanned strings.
// Always returns a non-nil slice (possibly empty) so callers can assign
// directly without a nil-check.
func (s *HistoryStore) distinctNonEmpty(query string) ([]string, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Count returns the total number of fights currently stored. Used by the
// list endpoint when the caller asks for a total (so the UI can render
// pagination controls).
func (s *HistoryStore) Count(filter FightFilter) (int64, error) {
	clauses := []string{}
	args := []interface{}{}
	if !filter.StartTime.IsZero() {
		clauses = append(clauses, "start_time >= ?")
		args = append(args, filter.StartTime.UnixNano())
	}
	if !filter.EndTime.IsZero() {
		clauses = append(clauses, "start_time <= ?")
		args = append(args, filter.EndTime.UnixNano())
	}
	if filter.NPCName != "" {
		clauses = append(clauses, "lower(npc_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(filter.NPCName)+"%")
	}
	if filter.CharacterName != "" {
		clauses = append(clauses, "character_name = ?")
		args = append(args, filter.CharacterName)
	}
	if filter.Zone != "" {
		clauses = append(clauses, "zone = ?")
		args = append(args, filter.Zone)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	var n int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM combat_fights `+where, args...).Scan(&n)
	return n, err
}

// rowScanner is the minimal interface satisfied by both *sql.Row and
// *sql.Rows so scanFight can serve both single-row and iterating queries.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanFight(r rowScanner) (StoredFight, error) {
	var (
		f                       StoredFight
		startNs, endNs          int64
		combatantsJSON, healers string
	)
	if err := r.Scan(
		&f.ID, &f.NPCName, &f.Zone, &f.CharacterName,
		&startNs, &endNs, &f.Duration,
		&f.TotalDamage, &f.YouDamage,
		&f.TotalHeal, &f.YouHeal,
		&combatantsJSON, &healers,
	); err != nil {
		return StoredFight{}, err
	}
	f.StartTime = time.Unix(0, startNs)
	f.EndTime = time.Unix(0, endNs)
	if err := json.Unmarshal([]byte(combatantsJSON), &f.Combatants); err != nil {
		return StoredFight{}, fmt.Errorf("unmarshal combatants for fight %d: %w", f.ID, err)
	}
	if f.Combatants == nil {
		f.Combatants = []EntityStats{}
	}
	if err := json.Unmarshal([]byte(healers), &f.Healers); err != nil {
		return StoredFight{}, fmt.Errorf("unmarshal healers for fight %d: %w", f.ID, err)
	}
	if f.Healers == nil {
		f.Healers = []HealerStats{}
	}
	return f, nil
}

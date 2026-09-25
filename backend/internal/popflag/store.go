package popflag

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Source precedence for the effective state of a (character, flag) row.
// manual > (seer/popflags) > auto: a manual toggle is a deliberate user
// correction that a later reading or live-event inference must never
// overwrite. Seer and popflags share a precedence tier — whichever reading
// was applied most recently wins for the qglobal-backed flags it covers (see
// qglobalBackedFlagIDs), since both are equally authoritative in-game
// snapshots of the same underlying qglobal state.
const (
	SourceManual   = "manual"
	SourceSeer     = "seer"
	SourcePopflags = "popflags"
	SourceAuto     = "auto"
)

// State is one persisted per-character flag row.
type State struct {
	FlagID    string `json:"flag_id"`
	Done      bool   `json:"done"`
	Source    string `json:"source"`
	UpdatedAt int64  `json:"updated_at"`
}

// Store persists per-character PoP flag progress in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db and runs the pop_flag_state migration. Coexists with
// the keyring / players / character / trigger stores under WAL mode.
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
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pop_flag_state (
			character  TEXT    NOT NULL COLLATE NOCASE,
			flag_id    TEXT    NOT NULL,
			done       INTEGER NOT NULL DEFAULT 0,
			source     TEXT    NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (character, flag_id)
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS pop_flag_state_character ON pop_flag_state(character)`); err != nil {
		return err
	}
	// Raw Seer snapshot per character — kept for audit and re-derivation when
	// the dataset's completion rules change.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pop_seer_snapshot (
			character TEXT PRIMARY KEY COLLATE NOCASE,
			qglobals  TEXT NOT NULL,
			raw_text  TEXT NOT NULL,
			taken_at  INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	return nil
}

// Get returns every stored flag row for the named character. Empty slice (not
// nil) when there are none.
func (s *Store) Get(character string) ([]State, error) {
	rows, err := s.db.Query(`
		SELECT flag_id, done, source, updated_at
		FROM pop_flag_state
		WHERE character = ?
	`, character)
	if err != nil {
		return nil, fmt.Errorf("query pop_flag_state for %q: %w", character, err)
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		var st State
		var done int
		if err := rows.Scan(&st.FlagID, &done, &st.Source, &st.UpdatedAt); err != nil {
			return nil, err
		}
		st.Done = done != 0
		out = append(out, st)
	}
	return out, rows.Err()
}

// SetManual records a deliberate user toggle (done=1 confirms, done=0 retracts
// a false auto/seer positive) with source='manual'.
//
// Ordering is enforced: a flag cannot be manually marked done while it is
// locked (a prerequisite isn't yet effectively done) UNLESS it is already
// effectively done — that exception lets the user confirm an optimistic auto/
// seer detection on a node whose prereqs aren't tracked. Retraction (done=0) is
// always allowed.
func (s *Store) SetManual(character, flagID string, done bool) error {
	if character == "" {
		return fmt.Errorf("character required")
	}
	if _, ok := ByID(flagID); !ok {
		return fmt.Errorf("unknown flag id %q", flagID)
	}
	states, err := s.Get(character)
	if err != nil {
		return err
	}
	resolved := Resolve(states)
	if done {
		// Can't complete a flag whose prerequisites aren't met — unless it's
		// already effectively done (confirming an auto/seer detection).
		for _, fs := range resolved.Flags {
			if fs.ID != flagID {
				continue
			}
			if fs.Locked && !fs.Done {
				return fmt.Errorf("complete prerequisites first: %s", missingLabels(fs.Missing))
			}
			break
		}
	} else {
		// Can't retract a flag that a completed later step depends on —
		// un-checking must proceed top-down.
		for _, fs := range resolved.Flags {
			if !fs.Done {
				continue
			}
			for _, p := range fs.Prereqs {
				if p == flagID {
					return fmt.Errorf("required by a completed step: %s", fs.Label)
				}
			}
		}
	}
	return s.upsert(character, flagID, done, SourceManual)
}

// missingLabels turns prereq flag IDs into a comma-separated label list for
// user-facing error messages.
func missingLabels(ids []string) string {
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		if f, ok := ByID(id); ok {
			labels = append(labels, f.Label)
		} else {
			labels = append(labels, id)
		}
	}
	return strings.Join(labels, ", ")
}

// SetAuto optimistically records a live-event detection as an 'auto'-sourced
// row. Auto is the lowest precedence, so it inserts only when no row exists for
// the (character, flag): any existing manual/seer/auto row wins (ON CONFLICT DO
// NOTHING). Returns true when a new row was actually inserted.
func (s *Store) SetAuto(character, flagID string) (bool, error) {
	if character == "" {
		return false, fmt.Errorf("character required")
	}
	if _, ok := ByID(flagID); !ok {
		return false, fmt.Errorf("unknown flag id %q", flagID)
	}
	res, err := s.db.Exec(`
		INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(character, flag_id) DO NOTHING
	`, character, flagID, SourceAuto, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("insert auto row char=%q flag=%q: %w", character, flagID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) upsert(character, flagID string, done bool, source string) error {
	d := 0
	if done {
		d = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(character, flag_id) DO UPDATE SET
			done = excluded.done,
			source = excluded.source,
			updated_at = excluded.updated_at
	`, character, flagID, d, source, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upsert pop_flag_state char=%q flag=%q: %w", character, flagID, err)
	}
	return nil
}

// Snapshot is the stored raw Seer reading for a character.
type Snapshot struct {
	Character string            `json:"character"`
	Qglobals  map[string]string `json:"qglobals"`
	RawText   string            `json:"raw_text"`
	TakenAt   int64             `json:"taken_at"`
}

// ApplySeer records a Seer guided-meditation reading: it replaces all
// non-manual rows for the character with seer-sourced rows for the flags the
// reading marks complete, and stores the raw snapshot for audit.
//
// Precedence (manual > seer > auto) is enforced here: existing manual rows are
// never touched (the seer insert hits ON CONFLICT DO NOTHING), while stale
// seer/auto rows are cleared first so a re-reading can retract a flag the
// character no longer shows.
func (s *Store) ApplySeer(character string, qglobals map[string]string, rawText string, observedAt time.Time) ([]string, error) {
	return s.ApplySeerOverriding(character, qglobals, rawText, observedAt, nil)
}

// ApplySeerOverriding is ApplySeer plus a caller-supplied list of flag IDs whose
// manual row should be dropped first, so the reading wins for exactly those
// flags. This is how the UI lets a user resolve a manual-vs-Seer conflict: a
// flag they once set by hand but now want to accept from the reading. Manual
// rows NOT in overrideManual stay protected as usual.
func (s *Store) ApplySeerOverriding(character string, qglobals map[string]string, rawText string, observedAt time.Time, overrideManual []string) ([]string, error) {
	if character == "" {
		return nil, fmt.Errorf("character required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	now := observedAt.Unix()
	done := DeriveCompletion(qglobals)

	qjson, err := json.Marshal(qglobals)
	if err != nil {
		return nil, fmt.Errorf("marshal qglobals: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Clear non-manual rows for qglobal-backed flags ONLY, so retractions take
	// effect and seer supersedes auto — but a reading must never retract an
	// 'auto' kill-detected row on a flag it has no way to observe (e.g.
	// poair_xegony has no backing qglobal at all).
	if err := clearNonManualQglobalBackedRows(tx, character); err != nil {
		return nil, err
	}
	// Drop the manual rows the user chose to override, so the seer insert below
	// applies the reading's value for them instead of being blocked.
	for _, id := range overrideManual {
		if _, err := tx.Exec(`DELETE FROM pop_flag_state WHERE character = ? AND flag_id = ? AND source = 'manual'`, character, id); err != nil {
			return nil, fmt.Errorf("clear manual override char=%q flag=%q: %w", character, id, err)
		}
	}
	// Insert seer rows; a surviving manual row wins (DO NOTHING).
	for _, id := range done {
		if _, err := tx.Exec(`
			INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
			VALUES (?, ?, 1, ?, ?)
			ON CONFLICT(character, flag_id) DO NOTHING
		`, character, id, SourceSeer, now); err != nil {
			return nil, fmt.Errorf("insert seer row char=%q flag=%q: %w", character, id, err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO pop_seer_snapshot (character, qglobals, raw_text, taken_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(character) DO UPDATE SET
			qglobals = excluded.qglobals,
			raw_text = excluded.raw_text,
			taken_at = excluded.taken_at
	`, character, string(qjson), rawText, now); err != nil {
		return nil, fmt.Errorf("upsert snapshot for %q: %w", character, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return done, nil
}

// qglobalBackedFlagIDs returns the IDs of every dataset flag whose completion
// is derived from a qglobal, directly (Qglobal) or via a replacement any-of
// (SatisfiedBy). A Seer or #popflags reading is authoritative ONLY for these
// — clearing non-manual rows on a reading must be scoped to this set, or it
// would retract an 'auto' kill-detected row on a flag with no qglobal at all
// (e.g. poair_xegony), which no reading has any way to observe.
func qglobalBackedFlagIDs() []string {
	all := Flags()
	out := make([]string, 0, len(all))
	for _, f := range all {
		if f.Qglobal != "" || len(f.SatisfiedBy) > 0 {
			out = append(out, f.ID)
		}
	}
	return out
}

// clearNonManualQglobalBackedRows deletes every non-manual pop_flag_state row
// for character that belongs to a qglobal-backed flag, within tx. Shared by
// ApplySeerOverriding and ApplyPopFlagsReport so both readings scope their
// retraction the same way.
func clearNonManualQglobalBackedRows(tx *sql.Tx, character string) error {
	ids := qglobalBackedFlagIDs()
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, character)
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf(
		`DELETE FROM pop_flag_state WHERE character = ? AND source != 'manual' AND flag_id IN (%s)`,
		placeholders,
	)
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("clear non-manual qglobal-backed rows for %q: %w", character, err)
	}
	return nil
}

// ApplyPopFlagsReport merges one '#popflags' report (see popflags_cmd.go)
// into the character's stored qglobal snapshot, re-derives completion, and
// persists it exactly like a Seer reading (source='popflags', same manual >
// (seer/popflags) > auto precedence enforced via clearNonManualQglobalBackedRows).
//
// Unlike ApplySeer (one self-contained reading that covers every qglobal),
// one '#popflags' report only covers the section the player ran — so this
// MERGES onto the last stored snapshot rather than replacing it: Exact values
// overwrite, AtLeast values raise the floor if higher than what's stored, and
// Pending flags are recorded present. Fields the report didn't mention are
// left exactly as they were. This lets a player progressively sync their full
// snapshot by running '#popflags 1' through '#popflags 5' (or just the
// overview) across several scans/pastes.
func (s *Store) ApplyPopFlagsReport(character string, report PopFlagsReport, rawText string, observedAt time.Time) ([]string, error) {
	if character == "" {
		return nil, fmt.Errorf("character required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	now := observedAt.Unix()

	prev, err := s.GetSnapshot(character)
	if err != nil {
		return nil, fmt.Errorf("load prior snapshot for %q: %w", character, err)
	}
	q := map[string]string{}
	if prev != nil {
		for k, v := range prev.Qglobals {
			q[k] = v
		}
	}
	for k, v := range report.Exact {
		q[k] = v
	}
	for k, floor := range report.AtLeast {
		if atoi(q[k]) < floor {
			q[k] = strconv.Itoa(floor)
		}
	}
	for k := range report.Pending {
		q[k] = "1"
	}

	done := DeriveCompletion(q)
	qjson, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("marshal merged qglobals: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := clearNonManualQglobalBackedRows(tx, character); err != nil {
		return nil, err
	}
	for _, id := range done {
		if _, err := tx.Exec(`
			INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
			VALUES (?, ?, 1, ?, ?)
			ON CONFLICT(character, flag_id) DO NOTHING
		`, character, id, SourcePopflags, now); err != nil {
			return nil, fmt.Errorf("insert popflags row char=%q flag=%q: %w", character, id, err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO pop_seer_snapshot (character, qglobals, raw_text, taken_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(character) DO UPDATE SET
			qglobals = excluded.qglobals,
			raw_text = excluded.raw_text,
			taken_at = excluded.taken_at
	`, character, string(qjson), rawText, now); err != nil {
		return nil, fmt.Errorf("upsert snapshot for %q: %w", character, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return done, nil
}

// GetSnapshot returns the stored raw Seer snapshot for a character, or nil when
// none has been recorded.
func (s *Store) GetSnapshot(character string) (*Snapshot, error) {
	row := s.db.QueryRow(`SELECT qglobals, raw_text, taken_at FROM pop_seer_snapshot WHERE character = ?`, character)
	var qjson, rawText string
	var takenAt int64
	if err := row.Scan(&qjson, &rawText, &takenAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	snap := &Snapshot{Character: character, RawText: rawText, TakenAt: takenAt}
	if err := json.Unmarshal([]byte(qjson), &snap.Qglobals); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot qglobals: %w", err)
	}
	return snap, nil
}

// Characters returns the distinct character names with at least one stored
// flag row, alphabetically. Used by the UI to render per-character tabs.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM pop_flag_state ORDER BY character COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query distinct characters: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

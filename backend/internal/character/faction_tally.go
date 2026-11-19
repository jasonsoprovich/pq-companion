package character

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FactionTallyRow is the persisted session-tally state for one character's
// tracked faction — the running "got better/worse" count plus the
// best-effort DB-estimated point delta and the last /con reading, surviving
// app restarts and character switches. See internal/factiontracker for how
// these numbers are derived; this table only stores the running result.
type FactionTallyRow struct {
	CharacterID  int    `json:"character_id"`
	FactionID    int    `json:"faction_id"`
	FactionName  string `json:"faction_name"`
	Better       int    `json:"better"`
	Worse        int    `json:"worse"`
	EstimatedNet int    `json:"estimated_net"`
	Unresolved   int    `json:"unresolved"`
	// LastBucket is the most recent /con disposition bucket resolved to this
	// faction's primary-faction NPCs (logparser.FactionBucket), or "" if
	// never considered.
	LastBucket string `json:"last_bucket"`
	// LastConsideredAt is the Unix timestamp of LastBucket, 0 if never set.
	LastConsideredAt int64 `json:"last_considered_at"`
	// LastConsiderSuspect flags that LastBucket may be wrong because the
	// player had an illusion active at the time of the reading.
	LastConsiderSuspect bool `json:"last_consider_suspect"`
	// Inferred, MaxStep and the Range* fields mirror factiontracker.Tally's
	// standing estimate. RangeAt is a Unix timestamp, 0 when unset.
	Inferred    int    `json:"inferred"`
	MaxStep     int    `json:"max_step"`
	RangeKnown  bool   `json:"range_known"`
	RangeLo     int    `json:"range_lo"`
	RangeHi     int    `json:"range_hi"`
	RangeSource string `json:"range_source"`
	RangeAt     int64  `json:"range_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

// factionTallyCols is the column list shared by every tally SELECT, in
// scanFactionTally's order.
const factionTallyCols = `character_id, faction_id, faction_name, better, worse, estimated_net,
	unresolved, last_bucket, last_considered_at, last_consider_suspect, inferred, max_step,
	range_known, range_lo, range_hi, range_source, range_at, updated_at`

func scanFactionTally(sc rowScanner) (FactionTallyRow, error) {
	var r FactionTallyRow
	var suspect, known int
	err := sc.Scan(
		&r.CharacterID, &r.FactionID, &r.FactionName, &r.Better, &r.Worse, &r.EstimatedNet,
		&r.Unresolved, &r.LastBucket, &r.LastConsideredAt, &suspect, &r.Inferred, &r.MaxStep,
		&known, &r.RangeLo, &r.RangeHi, &r.RangeSource, &r.RangeAt, &r.UpdatedAt,
	)
	r.LastConsiderSuspect = suspect != 0
	r.RangeKnown = known != 0
	return r, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) migrateFactionTally() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_faction_tally (
			character_id          INTEGER NOT NULL,
			faction_id            INTEGER NOT NULL,
			faction_name          TEXT    NOT NULL,
			better                INTEGER NOT NULL DEFAULT 0,
			worse                 INTEGER NOT NULL DEFAULT 0,
			estimated_net         INTEGER NOT NULL DEFAULT 0,
			unresolved            INTEGER NOT NULL DEFAULT 0,
			last_bucket           TEXT    NOT NULL DEFAULT '',
			last_considered_at    INTEGER NOT NULL DEFAULT 0,
			last_consider_suspect INTEGER NOT NULL DEFAULT 0,
			updated_at            INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (character_id, faction_id),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return err
	}
	// Standing-estimate columns, added after the table first shipped.
	for _, stmt := range []string{
		`ALTER TABLE character_faction_tally ADD COLUMN inferred INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_faction_tally ADD COLUMN max_step INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_faction_tally ADD COLUMN range_known INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_faction_tally ADD COLUMN range_lo INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_faction_tally ADD COLUMN range_hi INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE character_faction_tally ADD COLUMN range_source TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE character_faction_tally ADD COLUMN range_at INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add faction tally column: %w", err)
		}
	}
	return nil
}

// ListFactionTallies returns every persisted tally row for the character.
// Order is unspecified — callers merge these against ListFactionWishlist's
// (ordered) entries by faction_id.
func (s *Store) ListFactionTallies(characterID int) ([]FactionTallyRow, error) {
	rows, err := s.db.Query(
		`SELECT `+factionTallyCols+`
		 FROM character_faction_tally
		 WHERE character_id = ?`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FactionTallyRow{}
	for rows.Next() {
		r, err := scanFactionTally(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertFactionTally writes the full current state of one character+faction
// tally, overwriting whatever was there. Called after every tally mutation
// so the persisted state never lags the in-memory session engine.
func (s *Store) UpsertFactionTally(row FactionTallyRow) error {
	_, err := s.db.Exec(
		`INSERT INTO character_faction_tally
		   (character_id, faction_id, faction_name, better, worse, estimated_net,
		    unresolved, last_bucket, last_considered_at, last_consider_suspect,
		    inferred, max_step, range_known, range_lo, range_hi, range_source, range_at,
		    updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (character_id, faction_id) DO UPDATE SET
		   faction_name = excluded.faction_name,
		   better = excluded.better,
		   worse = excluded.worse,
		   estimated_net = excluded.estimated_net,
		   unresolved = excluded.unresolved,
		   last_bucket = excluded.last_bucket,
		   last_considered_at = excluded.last_considered_at,
		   last_consider_suspect = excluded.last_consider_suspect,
		   inferred = excluded.inferred,
		   max_step = excluded.max_step,
		   range_known = excluded.range_known,
		   range_lo = excluded.range_lo,
		   range_hi = excluded.range_hi,
		   range_source = excluded.range_source,
		   range_at = excluded.range_at,
		   updated_at = excluded.updated_at`,
		row.CharacterID, row.FactionID, row.FactionName, row.Better, row.Worse, row.EstimatedNet,
		row.Unresolved, row.LastBucket, row.LastConsideredAt, boolInt(row.LastConsiderSuspect),
		row.Inferred, row.MaxStep, boolInt(row.RangeKnown), row.RangeLo, row.RangeHi,
		row.RangeSource, row.RangeAt, time.Now().Unix(),
	)
	return err
}

// MergeBackfillConsiderReading reconciles one faction's backfill-recovered
// /con reading into storage — an approximate baseline only. It never
// touches better/worse/estimated_net/unresolved, which stay exclusively the
// live session tracker's concern (see internal/factiontracker.BackfillHandler
// for why replaying kills/quest turn-ins for those counts isn't attempted).
//
// A faction never seen before gets a new zero-count row seeded with this
// reading. Otherwise the reading only replaces what's stored if it's
// chronologically newer — re-running backfill against an unchanged log is a
// no-op, and it can never regress a more recent live /con reading.
//
// Returns changed=true if the stored row was created or its /con fields
// advanced, so the caller can report how many faction baselines backfill
// actually touched.
func (s *Store) MergeBackfillConsiderReading(characterID, factionID int, factionName, bucket string, consideredAt int64) (bool, error) {
	existing, ok, err := s.getFactionTally(characterID, factionID)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, s.UpsertFactionTally(FactionTallyRow{
			CharacterID: characterID, FactionID: factionID, FactionName: factionName,
			LastBucket: bucket, LastConsideredAt: consideredAt,
		})
	}
	if consideredAt <= existing.LastConsideredAt {
		return false, nil
	}
	existing.LastBucket = bucket
	existing.LastConsideredAt = consideredAt
	existing.LastConsiderSuspect = false
	return true, s.UpsertFactionTally(existing)
}

func (s *Store) getFactionTally(characterID, factionID int) (FactionTallyRow, bool, error) {
	r, err := scanFactionTally(s.db.QueryRow(
		`SELECT `+factionTallyCols+`
		 FROM character_faction_tally WHERE character_id = ? AND faction_id = ?`,
		characterID, factionID,
	))
	if err == sql.ErrNoRows {
		return FactionTallyRow{}, false, nil
	}
	if err != nil {
		return FactionTallyRow{}, false, err
	}
	return r, true, nil
}

// DeleteFactionTally removes one character's persisted tally for a single
// faction — called when the faction is removed from the wishlist, so
// re-adding it later starts fresh rather than resurrecting old history.
func (s *Store) DeleteFactionTally(characterID, factionID int) error {
	_, err := s.db.Exec(
		`DELETE FROM character_faction_tally WHERE character_id = ? AND faction_id = ?`,
		characterID, factionID,
	)
	return err
}

// ClearFactionTallies zeroes every persisted tally for the character without
// dropping the rows (so faction_name / last_bucket history for display
// context is discarded too — a full reset). Used by the explicit "Reset"
// action, not automatically on character switch.
func (s *Store) ClearFactionTallies(characterID int) error {
	_, err := s.db.Exec(`DELETE FROM character_faction_tally WHERE character_id = ?`, characterID)
	return err
}

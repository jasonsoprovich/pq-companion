package character

import "time"

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
	LastConsiderSuspect bool  `json:"last_consider_suspect"`
	UpdatedAt           int64 `json:"updated_at"`
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
	return err
}

// ListFactionTallies returns every persisted tally row for the character.
// Order is unspecified — callers merge these against ListFactionWishlist's
// (ordered) entries by faction_id.
func (s *Store) ListFactionTallies(characterID int) ([]FactionTallyRow, error) {
	rows, err := s.db.Query(
		`SELECT character_id, faction_id, faction_name, better, worse, estimated_net,
		        unresolved, last_bucket, last_considered_at, last_consider_suspect, updated_at
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
		var r FactionTallyRow
		var suspect int
		if err := rows.Scan(
			&r.CharacterID, &r.FactionID, &r.FactionName, &r.Better, &r.Worse, &r.EstimatedNet,
			&r.Unresolved, &r.LastBucket, &r.LastConsideredAt, &suspect, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.LastConsiderSuspect = suspect != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertFactionTally writes the full current state of one character+faction
// tally, overwriting whatever was there. Called after every tally mutation
// so the persisted state never lags the in-memory session engine.
func (s *Store) UpsertFactionTally(row FactionTallyRow) error {
	suspect := 0
	if row.LastConsiderSuspect {
		suspect = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO character_faction_tally
		   (character_id, faction_id, faction_name, better, worse, estimated_net,
		    unresolved, last_bucket, last_considered_at, last_consider_suspect, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (character_id, faction_id) DO UPDATE SET
		   faction_name = excluded.faction_name,
		   better = excluded.better,
		   worse = excluded.worse,
		   estimated_net = excluded.estimated_net,
		   unresolved = excluded.unresolved,
		   last_bucket = excluded.last_bucket,
		   last_considered_at = excluded.last_considered_at,
		   last_consider_suspect = excluded.last_consider_suspect,
		   updated_at = excluded.updated_at`,
		row.CharacterID, row.FactionID, row.FactionName, row.Better, row.Worse, row.EstimatedNet,
		row.Unresolved, row.LastBucket, row.LastConsideredAt, suspect, time.Now().Unix(),
	)
	return err
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

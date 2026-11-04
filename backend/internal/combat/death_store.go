package combat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// StoredDeath is a death as it lives in user.db.
type StoredDeath struct {
	ID int64 `json:"id"`
	DeathRecord
}

// DeathFilter narrows ListDeaths. Zero values mean "no filter".
type DeathFilter struct {
	StartTime     time.Time
	EndTime       time.Time
	CharacterName string
	Limit         int
}

func (s *HistoryStore) migrateDeaths() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS combat_deaths (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			character_name TEXT    NOT NULL DEFAULT '',
			zone           TEXT    NOT NULL DEFAULT '',
			slain_by       TEXT    NOT NULL DEFAULT '',
			died_at        INTEGER NOT NULL,
			recap_json     TEXT    NOT NULL DEFAULT ''
		)
	`); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_combat_deaths_at   ON combat_deaths(died_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_combat_deaths_char ON combat_deaths(character_name)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// SaveDeath writes one death, recap included. Returns the row ID.
func (s *HistoryStore) SaveDeath(d DeathRecord) (int64, error) {
	recap := ""
	if d.Recap != nil {
		b, err := json.Marshal(d.Recap)
		if err != nil {
			return 0, fmt.Errorf("marshal recap: %w", err)
		}
		recap = string(b)
	}
	res, err := s.db.Exec(`
		INSERT INTO combat_deaths (character_name, zone, slain_by, died_at, recap_json)
		VALUES (?, ?, ?, ?, ?)
	`, d.Character, d.Zone, d.SlainBy, d.Timestamp.UnixNano(), recap)
	if err != nil {
		return 0, fmt.Errorf("insert death: %w", err)
	}
	return res.LastInsertId()
}

// ListDeaths returns deaths newest first.
func (s *HistoryStore) ListDeaths(filter DeathFilter) ([]StoredDeath, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	clauses := []string{}
	args := []interface{}{}
	if !filter.StartTime.IsZero() {
		clauses = append(clauses, "died_at >= ?")
		args = append(args, filter.StartTime.UnixNano())
	}
	if !filter.EndTime.IsZero() {
		clauses = append(clauses, "died_at <= ?")
		args = append(args, filter.EndTime.UnixNano())
	}
	if filter.CharacterName != "" {
		clauses = append(clauses, "character_name = ?")
		args = append(args, filter.CharacterName)
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)
	rows, err := s.db.Query(`
		SELECT id, character_name, zone, slain_by, died_at, recap_json
		FROM combat_deaths `+where+`
		ORDER BY died_at DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("query deaths: %w", err)
	}
	defer rows.Close()
	out := []StoredDeath{}
	for rows.Next() {
		var (
			d     StoredDeath
			at    int64
			recap string
		)
		if err := rows.Scan(&d.ID, &d.Character, &d.Zone, &d.SlainBy, &at, &recap); err != nil {
			return nil, err
		}
		d.Timestamp = time.Unix(0, at)
		if recap != "" {
			d.Recap = &DeathRecap{}
			if err := json.Unmarshal([]byte(recap), d.Recap); err != nil {
				return nil, fmt.Errorf("unmarshal recap for death %d: %w", d.ID, err)
			}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteAllDeaths wipes the saved deaths. Returns the number removed.
func (s *HistoryStore) DeleteAllDeaths() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM combat_deaths`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

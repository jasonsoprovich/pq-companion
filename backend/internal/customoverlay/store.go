package customoverlay

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

// Store persists custom overlays in user.db: the definition and live state
// as JSON blobs, one row per overlay.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the custom overlay migration.
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
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS custom_overlays (
		id         TEXT    PRIMARY KEY,
		sort_order INTEGER NOT NULL DEFAULT 0,
		def        TEXT    NOT NULL,
		state      TEXT    NOT NULL DEFAULT '{}',
		updated_at INTEGER NOT NULL DEFAULT 0
	)`)
	return err
}

// Load returns every stored overlay in display order. A row whose JSON no
// longer decodes is skipped (and logged) rather than failing the rest.
func (s *Store) Load() ([]*Overlay, error) {
	rows, err := s.db.Query(`SELECT id, def, state, updated_at
		FROM custom_overlays ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("load custom overlays: %w", err)
	}
	defer rows.Close()
	var out []*Overlay
	for rows.Next() {
		var id, defJSON, stateJSON string
		var updated int64
		if err := rows.Scan(&id, &defJSON, &stateJSON, &updated); err != nil {
			return nil, fmt.Errorf("scan custom overlay: %w", err)
		}
		o := &Overlay{UpdatedAt: updated}
		if err := json.Unmarshal([]byte(defJSON), &o.Def); err != nil {
			slog.Warn("customoverlay: skip corrupt definition", "id", id, "err", err)
			continue
		}
		o.Def.ID = id
		if err := json.Unmarshal([]byte(stateJSON), &o.State); err != nil {
			slog.Warn("customoverlay: discard corrupt state", "id", id, "err", err)
			o.State = OverlayState{}
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Save upserts one overlay's definition, state and position.
func (s *Store) Save(o *Overlay, sortOrder int) error {
	def, err := json.Marshal(o.Def)
	if err != nil {
		return fmt.Errorf("marshal definition: %w", err)
	}
	state, err := json.Marshal(o.State)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO custom_overlays (id, sort_order, def, state, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET sort_order = excluded.sort_order,
			def = excluded.def, state = excluded.state, updated_at = excluded.updated_at`,
		o.Def.ID, sortOrder, string(def), string(state), o.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save custom overlay: %w", err)
	}
	return nil
}

// SaveState writes only an overlay's live state (the hot path while rules
// fire).
func (s *Store) SaveState(id string, st OverlayState, updatedAt int64) error {
	state, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	_, err = s.db.Exec(`UPDATE custom_overlays SET state = ?, updated_at = ? WHERE id = ?`,
		string(state), updatedAt, id)
	if err != nil {
		return fmt.Errorf("save custom overlay state: %w", err)
	}
	return nil
}

// Delete removes one overlay.
func (s *Store) Delete(id string) error {
	if _, err := s.db.Exec(`DELETE FROM custom_overlays WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete custom overlay: %w", err)
	}
	return nil
}

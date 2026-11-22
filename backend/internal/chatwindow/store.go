package chatwindow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	_ "modernc.org/sqlite"
)

// Store persists chat window definitions in user.db, one row per window.
// Contents aren't stored: chat lines are refilled from chat history on
// startup, and other log lines are live only.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the chat window migration.
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
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS chat_windows (
		id         TEXT    PRIMARY KEY,
		sort_order INTEGER NOT NULL DEFAULT 0,
		def        TEXT    NOT NULL
	)`)
	return err
}

// Load returns every stored definition in display order. A row whose JSON no
// longer decodes is skipped (and logged) rather than failing the rest.
func (s *Store) Load() ([]WindowDef, error) {
	rows, err := s.db.Query(`SELECT id, def FROM chat_windows ORDER BY sort_order, id`)
	if err != nil {
		return nil, fmt.Errorf("load chat windows: %w", err)
	}
	defer rows.Close()
	var out []WindowDef
	for rows.Next() {
		var id, defJSON string
		if err := rows.Scan(&id, &defJSON); err != nil {
			return nil, fmt.Errorf("scan chat window: %w", err)
		}
		var def WindowDef
		if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
			slog.Warn("chatwindow: skip corrupt definition", "id", id, "err", err)
			continue
		}
		def.ID = id
		out = append(out, def)
	}
	return out, rows.Err()
}

// Save upserts one definition and its position.
func (s *Store) Save(def WindowDef, sortOrder int) error {
	b, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal chat window: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO chat_windows (id, sort_order, def) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET sort_order = excluded.sort_order, def = excluded.def`,
		def.ID, sortOrder, string(b))
	if err != nil {
		return fmt.Errorf("save chat window: %w", err)
	}
	return nil
}

// Delete removes one definition.
func (s *Store) Delete(id string) error {
	if _, err := s.db.Exec(`DELETE FROM chat_windows WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete chat window: %w", err)
	}
	return nil
}

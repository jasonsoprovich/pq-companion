package skills

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Record is one stored skill row for a character.
type Record struct {
	SkillID   int    `json:"skill_id"`   // EQMac skill_id, or Unknown (-1)
	SkillName string `json:"skill_name"` // in-game display name from the log
	Value     int    `json:"value"`      // most recent observed rank
	UpdatedAt int64  `json:"updated_at"` // unix seconds of the last improvement
}

// Store wraps the user.db connection for skill tracking.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the skills migration.
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
	stmts := []string{
		// Keyed by (character, skill_name): skill_name is the ground truth from
		// the log and is unique per character, whereas skill_id can be Unknown
		// (-1) for several unmapped skills at once.
		`CREATE TABLE IF NOT EXISTS character_skills (
			character  TEXT    NOT NULL,
			skill_name TEXT    NOT NULL,
			skill_id   INTEGER NOT NULL DEFAULT -1,
			value      INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (character, skill_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_character_skills_char ON character_skills(character)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// Upsert records a skill value for a character. Skill values only ever rise in
// game, so an observed value is only written when it exceeds the stored one —
// this makes the operation idempotent and safe to replay during log backfill.
// Returns true when a row was inserted or updated.
func (s *Store) Upsert(character, skillName string, skillID, value int, ts time.Time) (bool, error) {
	if character == "" || skillName == "" {
		return false, nil
	}
	var prev int
	err := s.db.QueryRow(
		`SELECT value FROM character_skills WHERE character = ? COLLATE NOCASE AND skill_name = ? COLLATE NOCASE`,
		character, skillName,
	).Scan(&prev)
	switch {
	case err == sql.ErrNoRows:
		_, err := s.db.Exec(
			`INSERT INTO character_skills (character, skill_name, skill_id, value, updated_at)
			 VALUES (?, ?, ?, ?, ?)`,
			character, skillName, skillID, value, ts.Unix(),
		)
		if err != nil {
			return false, fmt.Errorf("insert skill: %w", err)
		}
		return true, nil
	case err != nil:
		return false, fmt.Errorf("query skill: %w", err)
	}
	if value <= prev {
		return false, nil
	}
	_, err = s.db.Exec(
		`UPDATE character_skills SET value = ?, skill_id = ?, updated_at = ?
		 WHERE character = ? COLLATE NOCASE AND skill_name = ? COLLATE NOCASE`,
		value, skillID, ts.Unix(), character, skillName,
	)
	if err != nil {
		return false, fmt.Errorf("update skill: %w", err)
	}
	return true, nil
}

// GetByCharacter returns every tracked skill for a character, ordered by name.
// The match is case-insensitive so it works regardless of how the character
// name was capitalized when the rows were written vs. requested.
func (s *Store) GetByCharacter(character string) ([]Record, error) {
	rows, err := s.db.Query(
		`SELECT skill_id, skill_name, value, updated_at
		 FROM character_skills WHERE character = ? COLLATE NOCASE
		 ORDER BY skill_name`,
		character,
	)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	out := []Record{}
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.SkillID, &r.SkillName, &r.Value, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

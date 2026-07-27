package emote

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists spell emote overrides and file-tracking metadata in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db and runs the emote_overrides/emote_meta migration.
// Coexists with the other feature stores under WAL mode.
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
		CREATE TABLE IF NOT EXISTS emote_overrides (
			spell_id      INTEGER PRIMARY KEY,
			you_cast      TEXT,
			other_casts   TEXT,
			cast_on_you   TEXT,
			cast_on_other TEXT,
			spell_fades   TEXT,
			updated_at    INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS emote_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`); err != nil {
		return err
	}
	return nil
}

// GetOverride returns the stored override row for spellID, or nil if the
// spell has no overrides at all.
func (s *Store) GetOverride(spellID int) (*OverrideRow, error) {
	row := s.db.QueryRow(`
		SELECT spell_id, you_cast, other_casts, cast_on_you, cast_on_other, spell_fades, updated_at
		FROM emote_overrides WHERE spell_id = ?
	`, spellID)
	o, err := scanOverride(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return o, err
}

// ListOverrides returns every customized spell's override row.
func (s *Store) ListOverrides() ([]OverrideRow, error) {
	rows, err := s.db.Query(`
		SELECT spell_id, you_cast, other_casts, cast_on_you, cast_on_other, spell_fades, updated_at
		FROM emote_overrides ORDER BY spell_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OverrideRow{}
	for rows.Next() {
		o, err := scanOverride(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOverride(r rowScanner) (*OverrideRow, error) {
	var o OverrideRow
	var youCast, otherCasts, castOnYou, castOnOther, spellFades sql.NullString
	if err := r.Scan(&o.SpellID, &youCast, &otherCasts, &castOnYou, &castOnOther, &spellFades, &o.UpdatedAt); err != nil {
		return nil, err
	}
	o.YouCast = nullToPtr(youCast)
	o.OtherCasts = nullToPtr(otherCasts)
	o.CastOnYou = nullToPtr(castOnYou)
	o.CastOnOther = nullToPtr(castOnOther)
	o.SpellFades = nullToPtr(spellFades)
	return &o, nil
}

func nullToPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

func ptrToNull(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

// SetColumns merges patch into spellID's stored override row (inserting one
// if it doesn't exist yet). Only patch's non-nil fields are changed; the
// rest of the row is left as-is.
func (s *Store) SetColumns(spellID int, patch ColumnsPatch) error {
	if patch.Empty() {
		return fmt.Errorf("no columns provided")
	}
	existing, err := s.GetOverride(spellID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = &OverrideRow{SpellID: spellID}
	}
	if patch.YouCast != nil {
		existing.YouCast = patch.YouCast
	}
	if patch.OtherCasts != nil {
		existing.OtherCasts = patch.OtherCasts
	}
	if patch.CastOnYou != nil {
		existing.CastOnYou = patch.CastOnYou
	}
	if patch.CastOnOther != nil {
		existing.CastOnOther = patch.CastOnOther
	}
	if patch.SpellFades != nil {
		existing.SpellFades = patch.SpellFades
	}
	_, err = s.db.Exec(`
		INSERT INTO emote_overrides (spell_id, you_cast, other_casts, cast_on_you, cast_on_other, spell_fades, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(spell_id) DO UPDATE SET
			you_cast = excluded.you_cast,
			other_casts = excluded.other_casts,
			cast_on_you = excluded.cast_on_you,
			cast_on_other = excluded.cast_on_other,
			spell_fades = excluded.spell_fades,
			updated_at = excluded.updated_at
	`, spellID, ptrToNull(existing.YouCast), ptrToNull(existing.OtherCasts), ptrToNull(existing.CastOnYou),
		ptrToNull(existing.CastOnOther), ptrToNull(existing.SpellFades), time.Now().UTC().Unix())
	return err
}

// DeleteOverride reverts spellID to defaults by removing its override row entirely.
func (s *Store) DeleteOverride(spellID int) error {
	_, err := s.db.Exec(`DELETE FROM emote_overrides WHERE spell_id = ?`, spellID)
	return err
}

// DeleteAllOverrides clears every stored override (used by Restore Defaults).
func (s *Store) DeleteAllOverrides() error {
	_, err := s.db.Exec(`DELETE FROM emote_overrides`)
	return err
}

// GetMeta returns a stored meta value, or "" with ok=false if absent.
func (s *Store) GetMeta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM emote_meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetMeta upserts a meta value.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO emote_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

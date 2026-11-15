package players

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Look is how a player was last seen rendered in game, from Zeal's raid and
// group pipe messages: race, gender, model size, face, Luclin hair and
// beard, and worn equipment with its tints. Kept so the 3D map and raid
// replay can draw people as they look, not guessed from /who.
type Look struct {
	Race       int     `json:"race"`
	Gender     int     `json:"gender"`
	Texture    int     `json:"texture"`
	Size       float64 `json:"size"`
	Face       int     `json:"face"`
	HairStyle  int     `json:"hair_style"`
	HairColor  int     `json:"hair_color"`
	BeardStyle int     `json:"beard_style"`
	BeardColor int     `json:"beard_color"`
	// Mats is the material of the head, chest, arms, wrist, hands, legs and
	// feet slots; Tints their colours (0xRRGGBB).
	Mats  [7]int    `json:"mats"`
	Tints [7]uint32 `json:"tints"`
	// Primary and Secondary are the held items' models ("IT38"), or "".
	Primary   string `json:"primary,omitempty"`
	Secondary string `json:"secondary,omitempty"`
}

// looksCache is the last look written per name, so a raid's worth of
// unchanged looks arriving ten times a second doesn't touch the database.
type looksCache struct {
	mu   sync.Mutex
	last map[string]Look
}

func (s *Store) migrateLooks() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS player_looks (
			name    TEXT    NOT NULL PRIMARY KEY COLLATE NOCASE,
			look    TEXT    NOT NULL,
			seen_at INTEGER NOT NULL
		)
	`)
	return err
}

// SetLook records how a player looks, writing only when it changed since
// the last write.
func (s *Store) SetLook(name string, l Look, at time.Time) error {
	if name == "" || l.Race <= 0 {
		return nil
	}
	key := strings.ToLower(name)
	c := &s.looks
	c.mu.Lock()
	prev, ok := c.last[key]
	same := ok && prev == l
	c.mu.Unlock()
	if same {
		return nil
	}
	raw, err := json.Marshal(l)
	if err != nil {
		return fmt.Errorf("encode look: %w", err)
	}
	if _, err := s.db.Exec(`
		INSERT INTO player_looks (name, look, seen_at) VALUES (?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET look = excluded.look, seen_at = excluded.seen_at`,
		name, string(raw), at.Unix()); err != nil {
		return fmt.Errorf("set look: %w", err)
	}
	c.mu.Lock()
	if c.last == nil {
		c.last = map[string]Look{}
	}
	c.last[key] = l
	c.mu.Unlock()
	return nil
}

// GetLook returns how a player was last seen; ok is false when never.
func (s *Store) GetLook(name string) (Look, bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT look FROM player_looks WHERE name = ?`, name).Scan(&raw)
	if err == sql.ErrNoRows {
		return Look{}, false, nil
	}
	if err != nil {
		return Look{}, false, fmt.Errorf("get look: %w", err)
	}
	var l Look
	if err := json.Unmarshal([]byte(raw), &l); err != nil {
		return Look{}, false, fmt.Errorf("decode look: %w", err)
	}
	return l, true, nil
}

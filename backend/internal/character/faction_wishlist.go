package character

import "time"

// FactionWishlistEntry is one faction a character is tracking for the
// session Faction Tracker. FactionID is the quarm.db faction_list.id;
// FactionName is denormalized at write time so the tracker and UI don't need
// a live quarm.db join to render the wishlist.
type FactionWishlistEntry struct {
	ID          int    `json:"id"`
	CharacterID int    `json:"character_id"`
	FactionID   int    `json:"faction_id"`
	FactionName string `json:"faction_name"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) migrateFactionWishlist() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_faction_wishlist (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			faction_id   INTEGER NOT NULL,
			faction_name TEXT    NOT NULL,
			sort_order   INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL,
			UNIQUE (character_id, faction_id),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_faction_wishlist_char_sort
		 ON character_faction_wishlist(character_id, sort_order)`,
	)
	return err
}

// ListFactionWishlist returns the character's tracked factions in display order.
func (s *Store) ListFactionWishlist(characterID int) ([]FactionWishlistEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, faction_id, faction_name, sort_order, created_at
		 FROM character_faction_wishlist
		 WHERE character_id = ?
		 ORDER BY sort_order, id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FactionWishlistEntry{}
	for rows.Next() {
		var e FactionWishlistEntry
		if err := rows.Scan(&e.ID, &e.CharacterID, &e.FactionID, &e.FactionName, &e.SortOrder, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddFactionWishlistEntry stars a faction for the character, appending it at
// the bottom of the order. Idempotent: re-starring an already-tracked faction
// returns the existing row unmodified.
func (s *Store) AddFactionWishlistEntry(characterID, factionID int, factionName string) (FactionWishlistEntry, error) {
	var existing FactionWishlistEntry
	err := s.db.QueryRow(
		`SELECT id, character_id, faction_id, faction_name, sort_order, created_at
		 FROM character_faction_wishlist
		 WHERE character_id = ? AND faction_id = ?`,
		characterID, factionID,
	).Scan(&existing.ID, &existing.CharacterID, &existing.FactionID, &existing.FactionName, &existing.SortOrder, &existing.CreatedAt)
	if err == nil {
		return existing, nil
	}

	var maxPos int
	if err := s.db.QueryRow(
		`SELECT COALESCE(MAX(sort_order), -1) FROM character_faction_wishlist WHERE character_id = ?`,
		characterID,
	).Scan(&maxPos); err != nil {
		return FactionWishlistEntry{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO character_faction_wishlist (character_id, faction_id, faction_name, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		characterID, factionID, factionName, maxPos+1, now,
	)
	if err != nil {
		return FactionWishlistEntry{}, err
	}
	id, _ := res.LastInsertId()
	return FactionWishlistEntry{
		ID:          int(id),
		CharacterID: characterID,
		FactionID:   factionID,
		FactionName: factionName,
		SortOrder:   maxPos + 1,
		CreatedAt:   now,
	}, nil
}

// DeleteFactionWishlistEntry unstars a faction. No-op if it wasn't tracked.
func (s *Store) DeleteFactionWishlistEntry(characterID, factionID int) error {
	_, err := s.db.Exec(
		`DELETE FROM character_faction_wishlist WHERE character_id = ? AND faction_id = ?`,
		characterID, factionID,
	)
	return err
}

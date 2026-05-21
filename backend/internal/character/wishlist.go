package character

import (
	"database/sql"
	"fmt"
	"time"
)

// WishlistEntry is one item the user has starred for a character, anchored to
// a specific slot bucket. A multi-slot item (e.g. Living Symbol of Cazic-Thule)
// can have multiple entries on the same character — one per slot the user
// cares about — each with its own sort order inside that slot.
type WishlistEntry struct {
	ID          int    `json:"id"`
	CharacterID int    `json:"character_id"`
	ItemID      int    `json:"item_id"`
	SlotBucket  string `json:"slot_bucket"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Store) migrateWishlist() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_wishlist (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id   INTEGER NOT NULL,
			item_id        INTEGER NOT NULL,
			slot_bucket    TEXT    NOT NULL,
			sort_order     INTEGER NOT NULL DEFAULT 0,
			created_at     INTEGER NOT NULL,
			UNIQUE (character_id, item_id, slot_bucket),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_wishlist_char_slot
		 ON character_wishlist(character_id, slot_bucket, sort_order)`,
	); err != nil {
		return err
	}
	return nil
}

// ListWishlist returns every wishlist entry for the character, sorted by
// slot bucket then sort_order.
func (s *Store) ListWishlist(characterID int) ([]WishlistEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, item_id, slot_bucket, sort_order, created_at
		 FROM character_wishlist
		 WHERE character_id = ?
		 ORDER BY slot_bucket, sort_order, id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WishlistEntry
	for rows.Next() {
		var e WishlistEntry
		if err := rows.Scan(&e.ID, &e.CharacterID, &e.ItemID, &e.SlotBucket, &e.SortOrder, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListWishlistForItem returns the entries (across all slot buckets) for one
// specific item on the character. Used by the star button on ItemDetailModal
// to render its current state.
func (s *Store) ListWishlistForItem(characterID, itemID int) ([]WishlistEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, item_id, slot_bucket, sort_order, created_at
		 FROM character_wishlist
		 WHERE character_id = ? AND item_id = ?
		 ORDER BY slot_bucket`,
		characterID, itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WishlistEntry
	for rows.Next() {
		var e WishlistEntry
		if err := rows.Scan(&e.ID, &e.CharacterID, &e.ItemID, &e.SlotBucket, &e.SortOrder, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddWishlistEntry inserts a new entry at the end of the slot bucket's order.
// On unique conflict (character/item/slot already starred) returns the existing
// row without modifying it.
func (s *Store) AddWishlistEntry(characterID, itemID int, slotBucket string) (WishlistEntry, error) {
	// Existing row check (idempotent add).
	var existing WishlistEntry
	err := s.db.QueryRow(
		`SELECT id, character_id, item_id, slot_bucket, sort_order, created_at
		 FROM character_wishlist
		 WHERE character_id = ? AND item_id = ? AND slot_bucket = ?`,
		characterID, itemID, slotBucket,
	).Scan(&existing.ID, &existing.CharacterID, &existing.ItemID, &existing.SlotBucket, &existing.SortOrder, &existing.CreatedAt)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return WishlistEntry{}, err
	}

	var maxPos sql.NullInt64
	if err := s.db.QueryRow(
		`SELECT MAX(sort_order) FROM character_wishlist WHERE character_id = ? AND slot_bucket = ?`,
		characterID, slotBucket,
	).Scan(&maxPos); err != nil {
		return WishlistEntry{}, err
	}
	pos := 0
	if maxPos.Valid {
		pos = int(maxPos.Int64) + 1
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO character_wishlist (character_id, item_id, slot_bucket, sort_order, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		characterID, itemID, slotBucket, pos, now,
	)
	if err != nil {
		return WishlistEntry{}, fmt.Errorf("create wishlist entry: %w", err)
	}
	id, _ := res.LastInsertId()
	return WishlistEntry{
		ID:          int(id),
		CharacterID: characterID,
		ItemID:      itemID,
		SlotBucket:  slotBucket,
		SortOrder:   pos,
		CreatedAt:   now,
	}, nil
}

// DeleteWishlistEntry removes a single entry by id. Scoped to characterID so
// a malformed request can't delete another character's entry.
func (s *Store) DeleteWishlistEntry(characterID, id int) error {
	_, err := s.db.Exec(
		`DELETE FROM character_wishlist WHERE id = ? AND character_id = ?`,
		id, characterID,
	)
	return err
}

// ReorderWishlistSlot rewrites sort_order for every entry in one slot bucket
// to match the supplied ID order. Entries omitted from the list keep their
// current sort_order but are pushed past the explicitly-ordered prefix.
func (s *Store) ReorderWishlistSlot(characterID int, slotBucket string, orderedIDs []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for i, id := range orderedIDs {
		if _, err := tx.Exec(
			`UPDATE character_wishlist SET sort_order = ?
			 WHERE id = ? AND character_id = ? AND slot_bucket = ?`,
			i, id, characterID, slotBucket,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

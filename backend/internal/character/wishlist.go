package character

import (
	"database/sql"
	"fmt"
	"time"
)

// WishlistEntry is one item the user has starred for a character, anchored to
// a specific slot bucket. A multi-slot item (e.g. Living Symbol of Cazic-Thule)
// can have multiple entries on the same character — one per slot the user
// cares about. SortOrder is a per-character global index — the same field
// drives ordering inside a slot card (filtered) and the "All items" flat view.
type WishlistEntry struct {
	ID          int    `json:"id"`
	CharacterID int    `json:"character_id"`
	ItemID      int    `json:"item_id"`
	SlotBucket  string `json:"slot_bucket"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   int64  `json:"created_at"`
}

// WishlistSlotLayout is the per-character layout for one slot bucket card:
// where the card sits relative to other cards and whether it's collapsed.
// Missing rows are treated as "canonical position, expanded."
type WishlistSlotLayout struct {
	SlotBucket string `json:"slot_bucket"`
	Position   int    `json:"position"`
	Collapsed  bool   `json:"collapsed"`
}

// CanonicalWishlistSlotOrder is the default top-to-bottom order of slot
// buckets, mirroring the EQ character-sheet layout. Mirrors
// frontend/src/lib/wishlistSlots.ts WISHLIST_SLOT_ORDER — keep in sync.
var CanonicalWishlistSlotOrder = []string{
	"Charm", "Ear", "Head", "Face", "Neck",
	"Shoulder", "Arms", "Back", "Wrist", "Range",
	"Hands", "Primary", "Secondary", "Finger",
	"Chest", "Legs", "Feet", "Waist", "Ammo",
	"General",
}

func (s *Store) migrateWishlist() error {
	// First-run detection: the slot_layout table is the marker. If it doesn't
	// exist yet, any existing wishlist rows still use the old per-bucket
	// sort_order semantics and need to be renumbered into a single global
	// order per character.
	hasLayoutTable, err := s.tableExists("character_wishlist_slot_layout")
	if err != nil {
		return err
	}

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
		`CREATE INDEX IF NOT EXISTS idx_character_wishlist_char_sort
		 ON character_wishlist(character_id, sort_order)`,
	); err != nil {
		return err
	}

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_wishlist_slot_layout (
			character_id INTEGER NOT NULL,
			slot_bucket  TEXT    NOT NULL,
			position     INTEGER NOT NULL DEFAULT 0,
			collapsed    INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (character_id, slot_bucket),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_wishlist_slot_layout_char
		 ON character_wishlist_slot_layout(character_id, position)`,
	); err != nil {
		return err
	}

	if !hasLayoutTable {
		if err := s.backfillWishlistGlobalOrder(); err != nil {
			return fmt.Errorf("backfill wishlist global sort_order: %w", err)
		}
	}
	return nil
}

func (s *Store) tableExists(name string) (bool, error) {
	var dummy string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
		name,
	).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// backfillWishlistGlobalOrder converts pre-existing per-bucket sort_order
// values into a single global ordering per character. Walks each character's
// entries in canonical bucket order (then existing sort_order, then id) and
// assigns sequential global positions.
func (s *Store) backfillWishlistGlobalOrder() error {
	bucketRank := make(map[string]int, len(CanonicalWishlistSlotOrder))
	for i, b := range CanonicalWishlistSlotOrder {
		bucketRank[b] = i
	}
	rows, err := s.db.Query(
		`SELECT id, character_id, slot_bucket, sort_order
		 FROM character_wishlist`,
	)
	if err != nil {
		return err
	}
	type ent struct {
		id, charID, sort int
		bucket           string
	}
	byChar := map[int][]ent{}
	for rows.Next() {
		var e ent
		if err := rows.Scan(&e.id, &e.charID, &e.bucket, &e.sort); err != nil {
			rows.Close()
			return err
		}
		byChar[e.charID] = append(byChar[e.charID], e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(byChar) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, list := range byChar {
		// Sort by canonical bucket order, then prior sort_order, then id.
		sortEntries := func(a, b ent) int {
			ai, ok := bucketRank[a.bucket]
			if !ok {
				ai = len(bucketRank)
			}
			bi, ok := bucketRank[b.bucket]
			if !ok {
				bi = len(bucketRank)
			}
			if ai != bi {
				return ai - bi
			}
			if a.sort != b.sort {
				return a.sort - b.sort
			}
			return a.id - b.id
		}
		// Stable insertion sort — len is small.
		for i := 1; i < len(list); i++ {
			for j := i; j > 0 && sortEntries(list[j-1], list[j]) > 0; j-- {
				list[j-1], list[j] = list[j], list[j-1]
			}
		}
		for i, e := range list {
			if _, err := tx.Exec(
				`UPDATE character_wishlist SET sort_order = ? WHERE id = ?`,
				i, e.id,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ListWishlist returns every wishlist entry for the character in global
// sort_order. Callers group by slot_bucket on the client side; within-bucket
// order is preserved because items keep their relative global order.
func (s *Store) ListWishlist(characterID int) ([]WishlistEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, item_id, slot_bucket, sort_order, created_at
		 FROM character_wishlist
		 WHERE character_id = ?
		 ORDER BY sort_order, id`,
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

// AddWishlistEntry appends a new entry at the bottom of the character's
// global ordering. Filtered into its slot card, it lands at the bottom of
// that card too. On unique conflict returns the existing row unmodified.
func (s *Store) AddWishlistEntry(characterID, itemID int, slotBucket string) (WishlistEntry, error) {
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
		`SELECT MAX(sort_order) FROM character_wishlist WHERE character_id = ?`,
		characterID,
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

// ReorderWishlist rewrites sort_order for the character's entries to match
// the supplied ID order. The slice must contain exactly the character's
// current entry IDs — extra/missing IDs are rejected to keep the global
// ordering consistent.
func (s *Store) ReorderWishlist(characterID int, orderedIDs []int) error {
	current, err := s.ListWishlist(characterID)
	if err != nil {
		return err
	}
	if len(current) != len(orderedIDs) {
		return fmt.Errorf("ordered_ids length %d does not match wishlist length %d",
			len(orderedIDs), len(current))
	}
	owned := make(map[int]bool, len(current))
	for _, e := range current {
		owned[e.ID] = true
	}
	seen := make(map[int]bool, len(orderedIDs))
	for _, id := range orderedIDs {
		if !owned[id] {
			return fmt.Errorf("entry %d does not belong to character %d", id, characterID)
		}
		if seen[id] {
			return fmt.Errorf("entry %d listed twice in ordered_ids", id)
		}
		seen[id] = true
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for i, id := range orderedIDs {
		if _, err := tx.Exec(
			`UPDATE character_wishlist SET sort_order = ? WHERE id = ?`,
			i, id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWishlistSlotLayout returns the saved slot-card layout for the
// character. Missing rows are not synthesised — the frontend fills in
// canonical-order/expanded defaults for any bucket not present.
func (s *Store) ListWishlistSlotLayout(characterID int) ([]WishlistSlotLayout, error) {
	rows, err := s.db.Query(
		`SELECT slot_bucket, position, collapsed
		 FROM character_wishlist_slot_layout
		 WHERE character_id = ?
		 ORDER BY position, slot_bucket`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WishlistSlotLayout{}
	for rows.Next() {
		var l WishlistSlotLayout
		var collapsed int
		if err := rows.Scan(&l.SlotBucket, &l.Position, &collapsed); err != nil {
			return nil, err
		}
		l.Collapsed = collapsed != 0
		out = append(out, l)
	}
	return out, rows.Err()
}

// ReplaceWishlistSlotLayout atomically replaces the saved layout for the
// character with the supplied rows. Buckets not in the slice are deleted so
// they fall back to canonical-order/expanded defaults on the next read.
func (s *Store) ReplaceWishlistSlotLayout(characterID int, layout []WishlistSlotLayout) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(
		`DELETE FROM character_wishlist_slot_layout WHERE character_id = ?`,
		characterID,
	); err != nil {
		return err
	}
	for _, l := range layout {
		collapsed := 0
		if l.Collapsed {
			collapsed = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO character_wishlist_slot_layout
			   (character_id, slot_bucket, position, collapsed)
			 VALUES (?, ?, ?, ?)`,
			characterID, l.SlotBucket, l.Position, collapsed,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

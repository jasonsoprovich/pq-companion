package db

import (
	"fmt"
	"strings"
)

// BandolierSlotBits maps a bandolier slot index (0..3 = Primary, Secondary,
// Range, Ammo — see zeal.BandolierSlotCount) to the items.slots bitmask bit for
// that equipment slot. Bit values are the EQMacEmu/Quarm worn-slot bits (see
// internal/db/enums/item_bitmasks.go), NOT modern EQEmu numbering.
var BandolierSlotBits = [4]int{
	0x002000, // 0 Primary   (bit 13)
	0x004000, // 1 Secondary (bit 14)
	0x000800, // 2 Range     (bit 11)
	0x200000, // 3 Ammo      (bit 21)
}

// BandolierItem is a lean item row for the bandolier slot picker: just what the
// UI needs to render a selectable entry (icon, name, slot bitmask for a sanity
// badge, and item type).
type BandolierItem struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Icon     int    `json:"icon"`
	Slots    int    `json:"slots"`
	ItemType int    `json:"item_type"`
}

// BandolierSlotItems returns the items the character owns (ownedIDs) that fit
// the given bandolier slot, optionally filtered by a case-insensitive name
// search. ownedIDs is the authoritative ownership guard: the picker can never
// surface an item the character does not have, so a saved set can't reference an
// item that would fail (or be skipped) when Zeal applies it in-game.
//
// Returns an empty slice (never an error) when ownedIDs is empty or the slot
// index is out of range.
func (db *DB) BandolierSlotItems(slotIndex int, ownedIDs []int, search string) ([]BandolierItem, error) {
	if slotIndex < 0 || slotIndex >= len(BandolierSlotBits) {
		return []BandolierItem{}, nil
	}
	if len(ownedIDs) == 0 {
		return []BandolierItem{}, nil
	}
	slotBit := BandolierSlotBits[slotIndex]

	// Deduplicate owned IDs — a character can own several copies of the same
	// item ID, but the picker only needs one row per distinct item.
	seen := make(map[int]bool, len(ownedIDs))
	ids := make([]int, 0, len(ownedIDs))
	for _, id := range ownedIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []BandolierItem{}, nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]

	where := fmt.Sprintf("(i.slots & ?) != 0 AND i.id IN (%s)", placeholders)
	args := []any{slotBit}
	for _, id := range ids {
		args = append(args, id)
	}

	if s := strings.TrimSpace(search); s != "" {
		where += " AND i.Name LIKE ? ESCAPE '\\'"
		args = append(args, "%"+escapeLike(s)+"%")
	}

	if clause, hargs := hiddenItemClause(); clause != "" {
		where += " AND " + strings.ReplaceAll(clause, "id ", "i.id ")
		args = append(args, hargs...)
	}

	q := `SELECT i.id, i.Name, i.icon, i.slots, i.itemtype
	  FROM items i WHERE ` + where + `
	  ORDER BY i.Name`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("bandolier slot items: %w", err)
	}
	defer rows.Close()

	out := []BandolierItem{}
	for rows.Next() {
		var it BandolierItem
		if err := rows.Scan(&it.ID, &it.Name, &it.Icon, &it.Slots, &it.ItemType); err != nil {
			return nil, fmt.Errorf("bandolier slot items scan: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// escapeLike escapes LIKE wildcards so a user-typed search term is matched
// literally (paired with ESCAPE '\\' in the query).
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

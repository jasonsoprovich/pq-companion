package db

import (
	"fmt"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/likequery"
)

// ItemAppearance is what an item looks like when worn, for the dressing
// room: Material picks the armor texture, Color tints it (0xRRGGBB, 0 for
// none), and IDFile is the model drawn for held items ("IT63").
type ItemAppearance struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Icon     int    `json:"icon"`
	Slots    int    `json:"slots"`
	ItemType int    `json:"item_type"`
	Material int    `json:"material"`
	Color    int    `json:"color"`
	IDFile   string `json:"id_file"`
}

// heldSlots are the items.slots bits whose item is drawn as a model in
// the hand: primary and secondary.
const heldSlots = 8192 | 16384

const itemAppearanceColumns = `id, Name, icon, slots, itemtype, material, color, idfile`

func scanItemAppearance(sc interface{ Scan(...any) error }) (ItemAppearance, error) {
	var a ItemAppearance
	var color int64
	if err := sc.Scan(&a.ID, &a.Name, &a.Icon, &a.Slots, &a.ItemType, &a.Material, &color, &a.IDFile); err != nil {
		return a, err
	}
	// The top byte is an alpha the client ignores; 0xFF000000 is no tint.
	a.Color = int(color & 0xFFFFFF)
	return a, nil
}

// ItemAppearances returns the appearance of each of ids that exists.
func (db *DB) ItemAppearances(ids []int) ([]ItemAppearance, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	q := `SELECT ` + itemAppearanceColumns + ` FROM items WHERE id IN (?` +
		strings.Repeat(",?", len(ids)-1) + `)`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("item appearances: %w", err)
	}
	defer rows.Close()
	var out []ItemAppearance
	for rows.Next() {
		a, err := scanItemAppearance(rows)
		if err != nil {
			return nil, fmt.Errorf("item appearances: %w", err)
		}
		if !isHiddenItem(a.ID) {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

// SearchWearables finds items that go in a slot (slotMask, the items.slots
// bits) and change how a character looks there: armor with a material, or
// a held item with a model. Name matches query (search box wildcards);
// one row per name.
func (db *DB) SearchWearables(slotMask int, query string, limit int) ([]ItemAppearance, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	// Armor shows by material; only the hands draw an item's model.
	looks := `material > 0`
	if slotMask&heldSlots != 0 {
		looks = `(material > 0 OR (idfile LIKE 'IT%' AND idfile != 'IT0'))`
	}
	q := `SELECT ` + itemAppearanceColumns + ` FROM items
		WHERE (slots & ?) != 0 AND Name LIKE ?` + likequery.Escape + `
		  AND ` + looks + `
		GROUP BY Name ORDER BY Name LIMIT ?`
	rows, err := db.Query(q, slotMask, likequery.Contains(query), limit)
	if err != nil {
		return nil, fmt.Errorf("search wearables: %w", err)
	}
	defer rows.Close()
	var out []ItemAppearance
	for rows.Next() {
		a, err := scanItemAppearance(rows)
		if err != nil {
			return nil, fmt.Errorf("search wearables: %w", err)
		}
		if !isHiddenItem(a.ID) {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

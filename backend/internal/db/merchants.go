package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// NPCMerchant is a vendor NPC's sale inventory: what it sells, at what price,
// and the gates a buyer has to clear.
type NPCMerchant struct {
	MerchantID int `json:"merchant_id"`
	// Greed is npc_types.greed, a percentage markup over the item's base price.
	// Zero for the overwhelming majority of vendors.
	Greed int               `json:"greed"`
	Items []NPCMerchantItem `json:"items"`
}

type NPCMerchantItem struct {
	Slot   int    `json:"slot"`
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Icon   int    `json:"icon"`
	// BasePrice is items.price in copper. Deliberately NOT adjusted for greed:
	// the exact greed formula has not been verified against EQMacEmu source, so
	// we surface the raw list price plus the greed flag rather than ship a
	// derived number that may be wrong. See LIMITATIONS.md.
	BasePrice int `json:"base_price"`
	// Quantity 0 means an unlimited stock slot.
	Quantity        int `json:"quantity"`
	FactionRequired int `json:"faction_required"`
	LevelRequired   int `json:"level_required"`
	ClassesRequired int `json:"classes_required"`
}

// GetNPCMerchant returns the sale inventory for an NPC, or nil when the NPC is
// not a merchant. Prices here are the base list: the running server further
// scales them per buyer by faction and charisma, so treat these as the floor
// rather than the exact number a player will be quoted.
func (db *DB) GetNPCMerchant(npcID int) (*NPCMerchant, error) {
	var merchantID, greed int
	err := db.QueryRow(
		`SELECT merchant_id, greed FROM npc_types WHERE id = ?`, npcID,
	).Scan(&merchantID, &greed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get npc %d merchant id: %w", npcID, err)
	}
	if merchantID == 0 {
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT ml.slot, ml.item, i.Name, i.icon, i.price, ml.quantity,
		       ml.faction_required, ml.level_required, ml.classes_required
		FROM merchantlist ml
		JOIN items i ON i.id = ml.item
		WHERE ml.merchantid = ?
		ORDER BY ml.slot`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list merchant %d inventory: %w", merchantID, err)
	}
	defer rows.Close()

	items := []NPCMerchantItem{}
	for rows.Next() {
		var it NPCMerchantItem
		if err := rows.Scan(&it.Slot, &it.ItemID, &it.Name, &it.Icon, &it.BasePrice,
			&it.Quantity, &it.FactionRequired, &it.LevelRequired, &it.ClassesRequired); err != nil {
			return nil, fmt.Errorf("scan merchant %d row: %w", merchantID, err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate merchant %d rows: %w", merchantID, err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	return &NPCMerchant{MerchantID: merchantID, Greed: greed, Items: items}, nil
}

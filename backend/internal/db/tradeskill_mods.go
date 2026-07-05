package db

import "fmt"

// TradeskillModifier is an item that boosts a tradeskill skill when worn. Value
// is a percentage bonus to that skill: EQMac's Client::GetSkill applies it as
// rawSkill*(100+value)/100, and per bonuses.cpp only the single highest worn
// item's value applies (mods don't stack). See internal/tradeskill.
type TradeskillModifier struct {
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Icon   int    `json:"icon"`
	Value  int    `json:"value"`
}

// TradeskillModifiers returns items whose skillmodtype boosts the given
// tradeskill skill id, best bonus first. Powers the recipe success calculator's
// "add a modifier" picker, which lists the full catalog regardless of whether
// the character owns the item.
func (db *DB) TradeskillModifiers(skillID int) ([]TradeskillModifier, error) {
	rows, err := db.Query(`
		SELECT id, name, COALESCE(icon, 0), skillmodvalue
		FROM items
		WHERE skillmodtype = ? AND skillmodvalue > 0
		ORDER BY skillmodvalue DESC, name`, skillID)
	if err != nil {
		return nil, fmt.Errorf("tradeskill modifiers %d: %w", skillID, err)
	}
	defer rows.Close()
	out := []TradeskillModifier{}
	for rows.Next() {
		var m TradeskillModifier
		if err := rows.Scan(&m.ItemID, &m.Name, &m.Icon, &m.Value); err != nil {
			return nil, fmt.Errorf("scan tradeskill modifier: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

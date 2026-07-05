package db

import (
	"database/sql"
	"fmt"
)

// SkillDifficulty returns the EQMacEmu skill-up difficulty for a tradeskill,
// from quarm.db skill_difficulty. classIdx is 1-indexed (1=Warrior … 15=
// Beastlord); tradeskill difficulties are class-invariant but the table is
// keyed per class, so query the character's class. Returns 0 (and no error)
// when the tradeskill has no row — the caller treats that as "unknown".
func (db *DB) SkillDifficulty(skillID, classIdx int) (float64, error) {
	var d float64
	err := db.QueryRow(
		`SELECT difficulty FROM skill_difficulty WHERE skillid = ? AND class = ? LIMIT 1`,
		skillID, classIdx,
	).Scan(&d)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("skill difficulty %d/%d: %w", skillID, classIdx, err)
	}
	return d, nil
}

// TradeskillModifier is an item that boosts a tradeskill skill when worn. Value
// is a percentage bonus to that skill: EQMac's Client::GetSkill applies it as
// rawSkill*(100+value)/100, and per bonuses.cpp only the single highest worn
// item's value applies (mods don't stack). See internal/tradeskill. Slots is
// the wearable-slot bitmask so the UI can show which slot it occupies (you
// swap it in, giving up that slot's usual gear).
type TradeskillModifier struct {
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Icon   int    `json:"icon"`
	Value  int    `json:"value"`
	Slots  int    `json:"slots"`
}

// TradeskillModifiers returns items whose skillmodtype boosts the given
// tradeskill skill id, best bonus first. Powers the recipe success calculator's
// "add a modifier" picker, which lists the full catalog regardless of whether
// the character owns the item.
func (db *DB) TradeskillModifiers(skillID int) ([]TradeskillModifier, error) {
	rows, err := db.Query(`
		SELECT id, name, COALESCE(icon, 0), skillmodvalue, slots
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
		if err := rows.Scan(&m.ItemID, &m.Name, &m.Icon, &m.Value, &m.Slots); err != nil {
			return nil, fmt.Errorf("scan tradeskill modifier: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

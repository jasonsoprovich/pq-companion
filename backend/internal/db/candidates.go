package db

import "fmt"

// UpgradeCandidate is a lean item row for the gear upgrade finder: just the
// flat scorable stats plus the metadata the UI needs (name/icon/usability/
// focus). It deliberately avoids the four correlated name-subqueries that the
// full itemColumns set carries, since the finder scans every item that fits a
// slot — only the single focus-name lookup is kept.
type UpgradeCandidate struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Icon     int    `json:"icon"`
	Slots    int    `json:"slots"`
	Classes  int    `json:"classes"`
	Races    int    `json:"races"`
	ReqLevel int    `json:"req_level"`
	RecLevel int    `json:"rec_level"`
	NoDrop   int    `json:"nodrop"`
	NoRent   int    `json:"norent"`
	ItemType int    `json:"item_type"`

	HP   int `json:"hp"`
	Mana int `json:"mana"`
	AC   int `json:"ac"`
	STR  int `json:"str"`
	STA  int `json:"sta"`
	AGI  int `json:"agi"`
	DEX  int `json:"dex"`
	WIS  int `json:"wis"`
	INT  int `json:"int"`
	CHA  int `json:"cha"`
	MR   int `json:"mr"`
	FR   int `json:"fr"`
	CR   int `json:"cr"`
	DR   int `json:"dr"`
	PR   int `json:"pr"`

	FocusEffect int    `json:"focus_effect"`
	FocusName   string `json:"focus_name"`
}

// CandidateFilter selects items usable in a slot by a character. A zero
// ClassBit/RaceBit/MaxLevel means "don't filter on that axis".
type CandidateFilter struct {
	SlotMask int // required: items whose slots bitmask intersects this
	ClassBit int // items.classes bit for the character's class
	RaceBit  int // items.races bit for the character's race
	MaxLevel int // character level; items requiring a higher level are excluded
}

// UpgradeCandidates returns every equippable item that fits the slot and is
// usable by the given class/race/level, as lean rows for in-memory scoring.
// Containers/books are excluded (itemclass 0 = common). Hidden items and
// non-canonical name variants are filtered the same way the catalog search is.
func (db *DB) UpgradeCandidates(f CandidateFilter) ([]UpgradeCandidate, error) {
	if f.SlotMask == 0 {
		return nil, fmt.Errorf("upgrade candidates: slot mask required")
	}
	where := "(i.slots & ?) != 0 AND i.itemclass = 0"
	args := []any{f.SlotMask}

	if f.ClassBit > 0 {
		where += " AND (i.classes = 0 OR i.classes >= 32767 OR (i.classes & ?) != 0)"
		args = append(args, f.ClassBit)
	}
	if f.RaceBit > 0 {
		where += " AND (i.races = 0 OR i.races >= 65535 OR (i.races & ?) != 0)"
		args = append(args, f.RaceBit)
	}
	if f.MaxLevel > 0 {
		where += " AND (i.reqlevel = 0 OR i.reqlevel <= ?)"
		args = append(args, f.MaxLevel)
	}

	if clause, hargs := hiddenItemClause(); clause != "" {
		where += " AND " + clause
		args = append(args, hargs...)
	}
	db.ensureVariants()
	if clause := db.itemVariants.excludeNonCanonical("i.id"); clause != "" {
		where += " AND " + clause
	}

	q := `SELECT i.id, i.Name, i.icon, i.slots, i.classes, i.races,
	  i.reqlevel, i.reclevel, i.nodrop, i.norent, i.itemtype,
	  i.hp, i.mana, i.ac, i.astr, i.asta, i.aagi, i.adex, i.awis, i.aint, i.acha,
	  i.mr, i.fr, i.cr, i.dr, i.pr,
	  i.focuseffect,
	  COALESCE(NULLIF(i.focusname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.focuseffect), '') AS focusname
	  FROM items i WHERE ` + where

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("upgrade candidates: %w", err)
	}
	defer rows.Close()

	var out []UpgradeCandidate
	for rows.Next() {
		var c UpgradeCandidate
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Icon, &c.Slots, &c.Classes, &c.Races,
			&c.ReqLevel, &c.RecLevel, &c.NoDrop, &c.NoRent, &c.ItemType,
			&c.HP, &c.Mana, &c.AC, &c.STR, &c.STA, &c.AGI, &c.DEX, &c.WIS, &c.INT, &c.CHA,
			&c.MR, &c.FR, &c.CR, &c.DR, &c.PR,
			&c.FocusEffect, &c.FocusName,
		); err != nil {
			return nil, fmt.Errorf("upgrade candidates scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

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

	Damage int `json:"damage"`
	Delay  int `json:"delay"`

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
	// WornEffect/WornLevel identify the item's worn-effect spell, from which the
	// finder derives worn ATK and melee haste.
	WornEffect int `json:"worn_effect"`
	WornLevel  int `json:"worn_level"`
}

// FocusOption is a distinct focus effect found on a class's usable gear, for
// the upgrade finder's "priority focus" picker. Count is how many usable items
// carry it (a rough popularity signal for ordering).
type FocusOption struct {
	SpellID int    `json:"spell_id"`
	Name    string `json:"name"`
	Count   int    `json:"count"`
}

// FocusOptions returns the distinct, named focus effects carried by items
// usable by the given class at or below the level. classBit/maxLevel of 0 mean
// "don't filter on that axis".
func (db *DB) FocusOptions(classBit, maxLevel int) ([]FocusOption, error) {
	where := "i.focuseffect > 0 AND i.itemclass = 0"
	args := []any{}
	if classBit > 0 {
		where += " AND (i.classes = 0 OR i.classes >= 32767 OR (i.classes & ?) != 0)"
		args = append(args, classBit)
	}
	if maxLevel > 0 {
		where += " AND (i.reqlevel = 0 OR i.reqlevel <= ?)"
		args = append(args, maxLevel)
	}
	q := `SELECT i.focuseffect,
	  COALESCE(NULLIF(i.focusname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.focuseffect), '') AS focusname,
	  COUNT(*) AS cnt
	  FROM items i WHERE ` + where + `
	  GROUP BY i.focuseffect HAVING focusname != ''
	  ORDER BY cnt DESC, focusname`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("focus options: %w", err)
	}
	defer rows.Close()
	out := []FocusOption{}
	for rows.Next() {
		var o FocusOption
		if err := rows.Scan(&o.SpellID, &o.Name, &o.Count); err != nil {
			return nil, fmt.Errorf("focus options scan: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// excludedGearItems are items the upgrade finder must never suggest even though
// quarm.db marks them as equippable with normal class/race masks: GM/dev-only
// items and other non-obtainable specials. There's no data flag distinguishing
// these, so they're listed explicitly — extend as more are found.
var excludedGearItems = map[int]bool{
	2660:  true, // Ban Hammer — GM-only, not obtainable by players
	11000: true, // Big Soul Devourer — GM-event weapon, no longer obtainable
	11001: true, // Soul Devourer — GM-event weapon, no longer obtainable
	2883:  true, // The Prime Healers Bulwark — GM-event item, no longer obtainable
	2446:  true, // Scepter of Al`Kabor — GM-event item, no longer obtainable
}

// CandidateFilter selects items usable in a slot by a character. A zero
// ClassBit/RaceBit/MaxLevel means "don't filter on that axis".
type CandidateFilter struct {
	SlotMask       int  // required: items whose slots bitmask intersects this
	ClassBit       int  // items.classes bit for the character's class
	RaceBit        int  // items.races bit for the character's race
	MaxLevel       int  // character level; items requiring a higher level are excluded
	ExcludePoP     bool // drop Planes-of-Power-gated items (not yet obtainable)
	ExcludeCrafted bool // drop tradeskill-made items (results of a recipe combine)
	ExcludeNoRent  bool // drop NO RENT items (expire on camp/zone) — noise by default
	ExcludeNoDrop  bool // drop NO DROP items (can't be traded for)
}

// UpgradeCandidates returns every equippable item that fits the slot and is
// usable by the given class/race/level, as lean rows for in-memory scoring.
// Containers/books are excluded (itemclass 0 = common). Hidden items and
// non-canonical name variants are filtered the same way the catalog search is.
func (db *DB) UpgradeCandidates(f CandidateFilter) ([]UpgradeCandidate, error) {
	if f.SlotMask == 0 {
		return nil, fmt.Errorf("upgrade candidates: slot mask required")
	}
	// classes <> 0: real gear always carries an explicit class mask (or the
	// all-class sentinel >= 32767). In this dataset every classes=0 equippable
	// row is a non-wearable special (quest/GM/book — e.g. Sword of Truth), so
	// classes=0 means "no class can equip", NOT "all".
	where := "(i.slots & ?) != 0 AND i.itemclass = 0 AND i.classes <> 0"
	args := []any{f.SlotMask}

	if f.ClassBit > 0 {
		where += " AND (i.classes >= 32767 OR (i.classes & ?) != 0)"
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
	if f.ExcludeCrafted {
		// successcount > 0 marks an item produced by a recipe combine (vs a
		// component or container). The item_id index keeps this subquery cheap.
		where += " AND NOT EXISTS (SELECT 1 FROM tradeskill_recipe_entries tre" +
			" WHERE tre.item_id = i.id AND tre.successcount > 0)"
	}
	if f.ExcludeNoRent {
		where += " AND i.norent = 0"
	}
	if f.ExcludeNoDrop {
		where += " AND i.nodrop = 0"
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
	  i.damage, i.delay,
	  i.hp, i.mana, i.ac, i.astr, i.asta, i.aagi, i.adex, i.awis, i.aint, i.acha,
	  i.mr, i.fr, i.cr, i.dr, i.pr,
	  i.focuseffect,
	  COALESCE(NULLIF(i.focusname, ''), (SELECT s.name FROM spells_new s WHERE s.id = i.focuseffect), '') AS focusname,
	  i.worneffect, i.wornlevel
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
			&c.Damage, &c.Delay,
			&c.HP, &c.Mana, &c.AC, &c.STR, &c.STA, &c.AGI, &c.DEX, &c.WIS, &c.INT, &c.CHA,
			&c.MR, &c.FR, &c.CR, &c.DR, &c.PR,
			&c.FocusEffect, &c.FocusName,
			&c.WornEffect, &c.WornLevel,
		); err != nil {
			return nil, fmt.Errorf("upgrade candidates scan: %w", err)
		}
		if excludedGearItems[c.ID] {
			continue // GM/non-obtainable special
		}
		if f.ExcludePoP && db.IsPoPGated(c.ID) {
			continue // not yet obtainable on Project Quarm
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

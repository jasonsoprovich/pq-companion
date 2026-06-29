package db

import (
	"fmt"
	"strings"
)

// Bard song instrument skills (spells_new.skill), EQMacEmu/Mac-era numbering.
// A bard song scales off whichever of these its skill column names.
const (
	skillPercussionInst = 12
	skillBrassInst      = 41
	skillSinging        = 49
	skillStringedInst   = 54
	skillWindInst       = 70
)

// Item instrument types (items.bardtype), EQMacEmu common/item_data.h. bardvalue
// holds the modifier magnitude (10 = 1.0x). Type 51 ("all") feeds every skill —
// it's how the bard epic (Singing Short Sword) boosts non-singing songs.
const (
	bardTypeWind       = 23
	bardTypeStringed   = 24
	bardTypeBrass      = 25
	bardTypePercussion = 26
	bardTypeSinging    = 50
	bardTypeAll        = 51
)

const (
	// instrumentModBase is the unmodified instrument mod (100% effectiveness).
	// A song's effect value is scaled by effectmod/10.
	instrumentModBase = 10
	// instrumentModSoftCap mirrors RuleI(Character, BaseInstrumentSoftCap) on
	// Project Quarm (36 = "3.6").
	instrumentModSoftCap = 36
	// seAddSingingMod (SPA 260) is how an AA grants an instrument-mod bonus:
	// base1 = the amount, base2 = the instrument type (a bardtype code).
	seAddSingingMod = 260
)

// Instrument-mastery AA client ids (altadv_vars.eqmacid).
const (
	aaInstrumentMastery = 90  // adds to wind/stringed/brass/percussion
	aaSingingMastery    = 118 // adds to singing
)

// InstrumentMods is a bard's effective instrument modifier per song skill, each
// an effectmod value (10 = 1.0x, capped at 36 = 3.6x). A bard song's effect
// magnitudes (resist debuffs included) are scaled by mod/10.
type InstrumentMods struct {
	Wind       int `json:"wind"`
	Stringed   int `json:"stringed"`
	Brass      int `json:"brass"`
	Percussion int `json:"percussion"`
	Singing    int `json:"singing"`
}

// NoInstrumentMods is the 1.0x-everything result for non-bards and bards with
// nothing equipped/trained.
func NoInstrumentMods() InstrumentMods {
	return InstrumentMods{
		Wind: instrumentModBase, Stringed: instrumentModBase, Brass: instrumentModBase,
		Percussion: instrumentModBase, Singing: instrumentModBase,
	}
}

// ModForSkill returns the effectmod for a song's instrument skill
// (spells_new.skill), or the base 10 for any non-instrument skill.
func (im InstrumentMods) ModForSkill(skill int) int {
	switch skill {
	case skillWindInst:
		return im.Wind
	case skillStringedInst:
		return im.Stringed
	case skillBrassInst:
		return im.Brass
	case skillPercussionInst:
		return im.Percussion
	case skillSinging:
		return im.Singing
	default:
		return instrumentModBase
	}
}

// BardInstrumentMods computes a bard's per-skill instrument modifier from the
// items they have equipped plus their trained AAs, following EQMacEmu's
// Mob::GetInstrumentMod: effectmod = max(10, best matching worn instrument) +
// the Instrument/Singing Mastery AA bonus, clamped to [10, 36].
//
// Pass only for bards — every other class always plays at 1.0x in-game. Items
// that aren't instruments (bardvalue 0) and untrained masteries contribute
// nothing.
func (db *DB) BardInstrumentMods(equippedItemIDs []int, trained []TrainedAA) (InstrumentMods, error) {
	item, err := db.bardItemMods(equippedItemIDs)
	if err != nil {
		return InstrumentMods{}, err
	}
	aa, err := db.bardAAMods(trained)
	if err != nil {
		return InstrumentMods{}, err
	}
	combine := func(itemMod, aaMod int) int {
		m := instrumentModBase
		if itemMod > m {
			m = itemMod
		}
		m += aaMod
		if m < instrumentModBase {
			m = instrumentModBase
		}
		if m > instrumentModSoftCap {
			m = instrumentModSoftCap
		}
		return m
	}
	return InstrumentMods{
		Wind:       combine(item.Wind, aa.Wind),
		Stringed:   combine(item.Stringed, aa.Stringed),
		Brass:      combine(item.Brass, aa.Brass),
		Percussion: combine(item.Percussion, aa.Percussion),
		Singing:    combine(item.Singing, aa.Singing),
	}, nil
}

// bardItemMods returns the best (max) worn-instrument bardvalue per skill type
// among the given item ids, matching EQMacEmu's AddItemBonuses (per type the
// strongest equipped instrument wins; type 51 feeds every type).
func (db *DB) bardItemMods(itemIDs []int) (InstrumentMods, error) {
	var m InstrumentMods
	ids := make([]any, 0, len(itemIDs))
	for _, id := range itemIDs {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return m, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := db.Query(
		`SELECT bardtype, bardvalue FROM items WHERE id IN (`+ph+`) AND bardvalue > 0`, ids...)
	if err != nil {
		return m, fmt.Errorf("query bard items: %w", err)
	}
	defer rows.Close()
	atLeast := func(dst *int, v int) {
		if v > *dst {
			*dst = v
		}
	}
	for rows.Next() {
		var bt, bv int
		if err := rows.Scan(&bt, &bv); err != nil {
			return m, fmt.Errorf("scan bard item: %w", err)
		}
		switch bt {
		case bardTypeWind:
			atLeast(&m.Wind, bv)
		case bardTypeStringed:
			atLeast(&m.Stringed, bv)
		case bardTypeBrass:
			atLeast(&m.Brass, bv)
		case bardTypePercussion:
			atLeast(&m.Percussion, bv)
		case bardTypeSinging:
			atLeast(&m.Singing, bv)
		case bardTypeAll:
			atLeast(&m.Wind, bv)
			atLeast(&m.Stringed, bv)
			atLeast(&m.Brass, bv)
			atLeast(&m.Percussion, bv)
			atLeast(&m.Singing, bv)
		}
	}
	return m, rows.Err()
}

// bardAAMods sums the instrument-mod grants from a bard's Instrument Mastery and
// Singing Mastery AAs (SPA 260, base2 = bardtype). Rank resolution mirrors
// AAStatBonuses: each rank is a consecutive skill_id row, the reported eqmacid
// points at rank 1, and the trained rank's base1 is the cumulative bonus.
func (db *DB) bardAAMods(trained []TrainedAA) (InstrumentMods, error) {
	var m InstrumentMods
	wantRank := map[int]int{} // eqmacid -> trained rank
	for _, t := range trained {
		if (t.AAID == aaInstrumentMastery || t.AAID == aaSingingMastery) && t.Rank > 0 {
			wantRank[t.AAID] = t.Rank
		}
	}
	if len(wantRank) == 0 {
		return m, nil
	}
	ids := make([]any, 0, len(wantRank))
	for id := range wantRank {
		ids = append(ids, id)
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := db.Query(
		`SELECT eqmacid, skill_id, max_level FROM altadv_vars WHERE eqmacid IN (`+ph+`)`, ids...)
	if err != nil {
		return m, fmt.Errorf("load bard aa rows: %w", err)
	}
	effAAID := make([]any, 0, len(wantRank))
	for rows.Next() {
		var eqmacid, skillID, maxLevel int
		if err := rows.Scan(&eqmacid, &skillID, &maxLevel); err != nil {
			rows.Close()
			return m, fmt.Errorf("scan bard aa row: %w", err)
		}
		rank := wantRank[eqmacid]
		if maxLevel > 0 && rank > maxLevel {
			rank = maxLevel
		}
		effAAID = append(effAAID, skillID+(rank-1))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return m, err
	}
	rows.Close()
	if len(effAAID) == 0 {
		return m, nil
	}
	ph2 := strings.TrimSuffix(strings.Repeat("?,", len(effAAID)), ",")
	args := append([]any{seAddSingingMod}, effAAID...)
	erows, err := db.Query(
		`SELECT base1, base2 FROM aa_effects WHERE effectid = ? AND aaid IN (`+ph2+`)`, args...)
	if err != nil {
		return m, fmt.Errorf("load bard aa effects: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var base1, base2 int
		if err := erows.Scan(&base1, &base2); err != nil {
			return m, fmt.Errorf("scan bard aa effect: %w", err)
		}
		switch base2 {
		case bardTypeWind:
			m.Wind += base1
		case bardTypeStringed:
			m.Stringed += base1
		case bardTypeBrass:
			m.Brass += base1
		case bardTypePercussion:
			m.Percussion += base1
		case bardTypeSinging:
			m.Singing += base1
		}
	}
	return m, erows.Err()
}

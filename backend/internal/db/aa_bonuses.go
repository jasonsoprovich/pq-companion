package db

import (
	"fmt"
	"strings"
)

// TrainedAA is one entry of a character's trained Alternate Advancement list:
// the client AA index (altadv_vars.eqmacid) and how many ranks are trained.
type TrainedAA struct {
	AAID int // altadv_vars.eqmacid (the Zeal "AAIndex" value)
	Rank int
}

// AABonuses aggregates the passive stat contributions a character's trained
// AAs add on top of their base attributes. Resolved from the aa_effects table
// (which keys on altadv_vars.skill_id, not eqmacid) plus a few HP-percent AAs
// the server hardcodes rather than storing as effect rows.
type AABonuses struct {
	STR, STA, AGI, DEX, WIS, INT, CHA int     // raw attribute grants (Innate X)
	MR, CR, FR, DR, PR                int     // resist grants (Innate/racial Protection AAs)
	HPRegen                           int     // SE_CurrentHP (effectid 0), per tick
	ManaRegen                         int     // SE_CurrentMana (effectid 15), per tick
	Attack                            int     // SE_ATK (effectid 2)
	HPPct                             float64 // Natural Durability + Physical Enhancement, percent
}

// aa_effects effectid (SE_*) codes we translate into stat bonuses. These match
// the EQEmu spell-effect numbering used in spells_new as well.
const (
	seCurrentHP     = 0 // HP regen / tick
	seATK           = 2 // attack
	seSTR           = 4
	seDEX           = 5
	seAGI           = 6
	seSTA           = 7
	seINT           = 8
	seWIS           = 9
	seCHA           = 10
	seCurrentMana   = 15 // mana regen / tick
	seResistFire    = 46
	seResistCold    = 47
	seResistPoison  = 48
	seResistDisease = 49
	seResistMagic   = 50
)

// AAStatBonuses resolves a character's trained AAs into their aggregate passive
// stat bonuses.
//
// Resolution model (Project Quarm / EQMacEmu): each AA rank is a separate
// altadv_vars row with a consecutive skill_id; the eqmacid the client/Zeal
// reports points at the rank-1 (lowest skill_id) row. So rank N's effects live
// in aa_effects at aaid = skill_id + (N-1), and each effect's base1 is the
// *cumulative* value at that rank. We look up the trained rank's effect rows
// and fold them in by effectid.
//
// HP-percent AAs (Natural Durability, Physical Enhancement) are not stored as
// percent effect rows — the server hardcodes them — so they're handled by name.
func (db *DB) AAStatBonuses(trained []TrainedAA) (AABonuses, error) {
	var b AABonuses
	if len(trained) == 0 {
		return b, nil
	}

	// Map each trained eqmacid to its rank-1 skill_id, max_level, and name.
	ids := make([]any, 0, len(trained))
	rankByID := make(map[int]int, len(trained))
	for _, t := range trained {
		if t.AAID <= 0 || t.Rank <= 0 {
			continue
		}
		ids = append(ids, t.AAID)
		rankByID[t.AAID] = t.Rank
	}
	if len(ids) == 0 {
		return b, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

	rows, err := db.Query(
		`SELECT eqmacid, skill_id, max_level, name FROM altadv_vars
		 WHERE eqmacid IN (`+placeholders+`)`, ids...)
	if err != nil {
		return b, fmt.Errorf("load aa rows: %w", err)
	}

	type resolved struct {
		effectAAID int // skill_id + (rank-1) — the rank's effect row id
		name       string
		rank       int
	}
	var list []resolved
	for rows.Next() {
		var eqmacid, skillID, maxLevel int
		var name string
		if err := rows.Scan(&eqmacid, &skillID, &maxLevel, &name); err != nil {
			rows.Close()
			return b, fmt.Errorf("scan aa row: %w", err)
		}
		rank := rankByID[eqmacid]
		if maxLevel > 0 && rank > maxLevel {
			rank = maxLevel
		}
		list = append(list, resolved{
			effectAAID: skillID + (rank - 1),
			name:       name,
			rank:       rank,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return b, err
	}
	rows.Close()

	// Hardcoded HP-percent AAs (server applies these as a percent of base+item
	// HP; the DB carries no percent for them).
	hasNaturalDurability := false
	for _, r := range list {
		switch r.name {
		case "Natural Durability":
			hasNaturalDurability = true
			switch r.rank {
			case 1:
				b.HPPct += 2
			case 2:
				b.HPPct += 5
			default: // rank 3+
				b.HPPct += 10
			}
		}
		// Planar Durability scales per level over 60; on Velious-locked Quarm
		// (level cap 60) it contributes nothing, so it is intentionally omitted.
	}
	for _, r := range list {
		if r.name == "Physical Enhancement" && hasNaturalDurability {
			b.HPPct += 2
		}
	}

	// Sum effect rows for the resolved per-rank aaids.
	effIDs := make([]any, 0, len(list))
	for _, r := range list {
		effIDs = append(effIDs, r.effectAAID)
	}
	effPlaceholders := strings.TrimSuffix(strings.Repeat("?,", len(effIDs)), ",")
	erows, err := db.Query(
		`SELECT effectid, base1 FROM aa_effects WHERE aaid IN (`+effPlaceholders+`)`,
		effIDs...)
	if err != nil {
		return b, fmt.Errorf("load aa effects: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var effectid, base1 int
		if err := erows.Scan(&effectid, &base1); err != nil {
			return b, fmt.Errorf("scan aa effect: %w", err)
		}
		switch effectid {
		case seSTR:
			b.STR += base1
		case seSTA:
			b.STA += base1
		case seAGI:
			b.AGI += base1
		case seDEX:
			b.DEX += base1
		case seINT:
			b.INT += base1
		case seWIS:
			b.WIS += base1
		case seCHA:
			b.CHA += base1
		case seResistFire:
			b.FR += base1
		case seResistCold:
			b.CR += base1
		case seResistPoison:
			b.PR += base1
		case seResistDisease:
			b.DR += base1
		case seResistMagic:
			b.MR += base1
		case seCurrentHP:
			b.HPRegen += base1
		case seCurrentMana:
			b.ManaRegen += base1
		case seATK:
			b.Attack += base1
		}
	}
	return b, erows.Err()
}

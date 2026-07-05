package tradeskill

// MasteryAA is a tradeskill combine-failure-reduction AA honored by the
// EQMacEmu combine code (zone/tradeskills.cpp Client::TradeskillExecute). Only
// three tradeskills have one. Note that "Fletching Mastery" exists in the AA
// data but the server never applies it to combines, and this era has no
// Salvage / component-return AA — so neither is modeled here.
type MasteryAA struct {
	Tradeskill int    `json:"tradeskill"` // spec->tradeskill skill id it reduces
	EqmacID    int    `json:"eqmac_id"`   // altadv_vars.eqmacid — how the quarmy export keys trained AAs
	Name       string `json:"name"`
}

// masteryByTradeskill mirrors the hardcoded per-tradeskill switch in
// Client::TradeskillExecute. Keyed by tradeskill skill id. eqmacid values are
// from quarm.db altadv_vars.
var masteryByTradeskill = map[int]MasteryAA{
	59: {Tradeskill: 59, EqmacID: 49, Name: "Alchemy Mastery"},    // SkillAlchemy
	68: {Tradeskill: 68, EqmacID: 56, Name: "Jewelcraft Mastery"}, // SkillJewelryMaking
	56: {Tradeskill: 56, EqmacID: 103, Name: "Poison Mastery"},    // SkillMakePoison
}

// MasteryFor returns the fail-reduction AA for a tradeskill, if the server
// honors one for that discipline.
func MasteryFor(tradeskill int) (MasteryAA, bool) {
	m, ok := masteryByTradeskill[tradeskill]
	return m, ok
}

// FailReducePct maps a Mastery AA rank to its failure-reduction percentage, per
// the EQMacEmu switch (rank 1/2/3 -> 10/25/50). Any other rank contributes 0.
// The percentage is applied to the combine chance as
// chance += (100 - chance) * pct / 100.
func FailReducePct(rank int) int {
	switch rank {
	case 1:
		return 10
	case 2:
		return 25
	case 3:
		return 50
	default:
		return 0
	}
}

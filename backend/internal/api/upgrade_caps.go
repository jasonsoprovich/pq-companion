package api

import (
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/upgrade"
)

// capEntry is one capped stat as the scorer sees it: the cap, the character's
// raw (uncapped) total, and how much of the cap comes from AAs.
type capEntry struct {
	Stat    string `json:"stat"`
	Cap     int    `json:"cap"`
	Current int    `json:"current"`
	AA      int    `json:"aa,omitempty"`
}

// upgradeCaps lists the caps the upgrade score is applying, so the finder can
// show them (e.g. that a level-65 character's attribute cap is 280 plus AAs).
// Attrs follow statDefs order; the soft caps are listed separately.
type upgradeCaps struct {
	Attrs []capEntry `json:"attrs"`
	FT    capEntry   `json:"ft"`
	ATK   capEntry   `json:"atk"`
	Haste capEntry   `json:"haste"`
}

// buildUpgradeCaps reads the caps off the same Context the scorer uses, so the
// display can't drift from the scoring. hasteByLoc is the best worn haste per
// location (worn haste is best-of, not summed).
func buildUpgradeCaps(ctx upgrade.Context, hasteByLoc map[string]int) upgradeCaps {
	attr := func(stat string, cur, aa int) capEntry {
		return capEntry{Stat: stat, Cap: eqstat.MaxStat(ctx.Level, ctx.CapMod+aa), Current: cur, AA: ctx.CapMod + aa}
	}
	c, m := ctx.Current, ctx.StatCapMod
	bestHaste := 0
	for _, h := range hasteByLoc {
		if h > bestHaste {
			bestHaste = h
		}
	}
	return upgradeCaps{
		Attrs: []capEntry{
			attr("str", c.STR, m.STR), attr("sta", c.STA, m.STA), attr("agi", c.AGI, m.AGI),
			attr("dex", c.DEX, m.DEX), attr("wis", c.WIS, m.WIS), attr("int", c.INT, m.INT),
			attr("cha", c.CHA, m.CHA),
		},
		FT:    capEntry{Stat: "mana_regen", Cap: upgrade.ItemFTCap, Current: c.ManaRegen},
		ATK:   capEntry{Stat: "atk", Cap: upgrade.ATKSoftCap, Current: c.Attack},
		Haste: capEntry{Stat: "haste", Cap: eqstat.MeleeHasteCap(ctx.Level), Current: bestHaste},
	}
}

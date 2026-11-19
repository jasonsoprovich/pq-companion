package api

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/upgrade"
)

func TestBuildUpgradeCaps_Level65WithAA(t *testing.T) {
	ctx := upgrade.Context{
		Level:      65,
		Current:    upgrade.StatLine{WIS: 296, STA: 271, ManaRegen: 12, Attack: 90},
		StatCapMod: upgrade.StatLine{WIS: 10},
	}
	caps := buildUpgradeCaps(ctx, map[string]int{"Hands": 21, "Waist": 36})
	byStat := map[string]capEntry{}
	for _, c := range caps.Attrs {
		byStat[c.Stat] = c
	}
	if w := byStat["wis"]; w.Cap != 290 || w.Current != 296 || w.AA != 10 {
		t.Errorf("wis = %+v, want cap 290 (280 + 10 AA), current 296", w)
	}
	if s := byStat["sta"]; s.Cap != 280 || s.AA != 0 {
		t.Errorf("sta = %+v, want cap 280 with no AA", s)
	}
	if caps.FT.Cap != 15 || caps.FT.Current != 12 {
		t.Errorf("ft = %+v", caps.FT)
	}
	if caps.ATK.Cap != 250 || caps.Haste.Cap != 100 || caps.Haste.Current != 36 {
		t.Errorf("atk = %+v haste = %+v", caps.ATK, caps.Haste)
	}
}

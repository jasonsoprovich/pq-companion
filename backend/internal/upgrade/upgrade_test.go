package upgrade

import (
	"sort"
	"testing"
)

// findDelta returns the StatDelta for a stat key, or a zero value.
func findDelta(res Result, stat string) (StatDelta, bool) {
	for _, d := range res.Deltas {
		if d.Stat == stat {
			return d, true
		}
	}
	return StatDelta{}, false
}

func TestScore_UncappedHPAndAC(t *testing.T) {
	ctx := Context{Level: 60}
	w := Weights{HP: 1.0, AC: 5.0}

	// Candidate: +50 HP, +30 AC. Current slot item: +20 HP, +10 AC.
	cur := StatLine{HP: 20, AC: 10}
	cand := StatLine{HP: 50, AC: 30}

	res := Score(ctx, w, cur, cand)

	// HP delta 30 × 1.0 = 30; AC delta 20 × 5.0 = 100. Total 130.
	if res.Score != 130 {
		t.Fatalf("score = %v, want 130", res.Score)
	}
	hp, _ := findDelta(res, "hp")
	if hp.Effective != 30 || hp.Weighted != 30 {
		t.Errorf("hp delta = %+v", hp)
	}
	ac, _ := findDelta(res, "ac")
	if ac.Effective != 20 || ac.Weighted != 100 {
		t.Errorf("ac delta = %+v", ac)
	}
}

func TestScore_AttributeOverCapScoresZero(t *testing.T) {
	// Character is already at the 255 STR cap (current total includes the
	// current slot item's +30 STR). A candidate with +40 STR should grant no
	// credit, because the loadout is capped with or without it.
	ctx := Context{Level: 60, Current: StatLine{STR: 255}}
	w := Weights{STR: 1.0}

	cur := StatLine{STR: 30}  // item currently in the slot contributes 30 STR
	cand := StatLine{STR: 40} // "better" on paper

	res := Score(ctx, w, cur, cand)

	// base = 255 - 30 = 225. effCur = min(225+30,255)-min(225,255)=255-225=30.
	// effCand = min(225+40,255)-225 = 255-225 = 30. delta = 0.
	str, ok := findDelta(res, "str")
	if !ok {
		t.Fatal("expected a str delta entry")
	}
	if str.Effective != 0 {
		t.Errorf("over-cap str effective = %d, want 0", str.Effective)
	}
	if !str.Capped {
		t.Errorf("expected str to be flagged capped")
	}
	if res.Score != 0 {
		t.Errorf("score = %v, want 0 (no useful STR gained)", res.Score)
	}
}

func TestScore_AttributePartialHeadroom(t *testing.T) {
	// Character at 240 STR total (current item gives 20 of it → base 220).
	// Candidate +50 STR can only use 35 before hitting 255.
	ctx := Context{Level: 60, Current: StatLine{STR: 240}}
	w := Weights{STR: 1.0}

	cur := StatLine{STR: 20}
	cand := StatLine{STR: 50}

	res := Score(ctx, w, cur, cand)

	// base = 220. effCur = 240-220 = 20. effCand = min(270,255)-220 = 35.
	// delta = 35 - 20 = 15.
	str, _ := findDelta(res, "str")
	if str.Effective != 15 {
		t.Fatalf("partial-headroom str effective = %d, want 15", str.Effective)
	}
	if !str.Capped {
		t.Errorf("expected capped flag (candidate clipped at 255)")
	}
	if res.Score != 15 {
		t.Errorf("score = %v, want 15", res.Score)
	}
}

func TestScore_AboveLevel60RaisesCap(t *testing.T) {
	// At level 65 the cap is 255 + 5*5 = 280, so a near-255 character still has
	// headroom that a level-60 character wouldn't.
	ctx := Context{Level: 65, Current: StatLine{DEX: 255}}
	w := Weights{DEX: 1.0}
	res := Score(ctx, w, StatLine{DEX: 10}, StatLine{DEX: 30})
	// base = 245. effCur = 255-245 = 10. effCand = min(275,280)-245 = 30.
	dex, _ := findDelta(res, "dex")
	if dex.Effective != 20 {
		t.Fatalf("dex effective at level 65 = %d, want 20", dex.Effective)
	}
}

func TestScore_DowngradeIsNegative(t *testing.T) {
	ctx := Context{Level: 60}
	w := Weights{HP: 1.0, Mana: 1.0}
	// Candidate trades 100 HP for 60 mana → net negative under equal weights.
	res := Score(ctx, w, StatLine{HP: 100}, StatLine{Mana: 60})
	if res.Score >= 0 {
		t.Fatalf("expected negative score for a downgrade, got %v", res.Score)
	}
}

func TestScore_RankingOrdersByScore(t *testing.T) {
	ctx := Context{Level: 60}
	w := DefaultWeights(classWarrior) // tank: AC heavy
	cur := StatLine{HP: 50, AC: 10}

	cands := map[string]StatLine{
		"all_ac":   {AC: 40},          // 30 effective AC × 5 = 150
		"all_hp":   {HP: 250},         // 200 HP × 1 = 200
		"balanced": {HP: 120, AC: 25}, // 70×1 + 15×5 = 145
	}
	type scored struct {
		name  string
		score float64
	}
	var out []scored
	for name, c := range cands {
		out = append(out, scored{name, Score(ctx, w, cur, c).Score})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })

	if out[0].name != "all_hp" {
		t.Fatalf("expected all_hp to rank first for a tank, got %q (%v)", out[0].name, out)
	}
}

func TestDefaultWeights_Archetypes(t *testing.T) {
	// Tanks value AC heavily and mana not at all.
	tank := DefaultWeights(classWarrior)
	if tank.AC < tank.HP || tank.Mana != 0 {
		t.Errorf("tank weights look wrong: %+v", tank)
	}
	// INT casters value mana and INT, and AC near zero.
	wiz := DefaultWeights(classWizard)
	if wiz.Mana == 0 || wiz.INT == 0 || wiz.AC >= 1.0 {
		t.Errorf("int-caster weights look wrong: %+v", wiz)
	}
	// Priests value WIS + mana.
	clr := DefaultWeights(classCleric)
	if clr.WIS == 0 || clr.Mana == 0 {
		t.Errorf("wis-caster weights look wrong: %+v", clr)
	}
	if ArchetypeFor(classBeastlord) != ArchHybrid {
		t.Errorf("beastlord should be hybrid")
	}
}

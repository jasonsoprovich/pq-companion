package upgrade

import "testing"

// PoP item stats score up to their per-character cap (db.Mod2Caps) and earn
// nothing past it.
func TestScore_Mod2StatsCapped(t *testing.T) {
	w := Weights{Strikethrough: 4, Accuracy: 1, SpellShield: 2}
	// Loadout already has 30% strikethrough (cap 35) and 100 accuracy (cap
	// 150), including the worn item's 0.
	ctx := Context{Level: 65, Current: StatLine{Strikethrough: 30, Accuracy: 100}}
	cur := StatLine{}
	cand := StatLine{Strikethrough: 10, Accuracy: 15, SpellShield: 5}

	res := Score(ctx, w, cur, cand)
	st, _ := findDelta(res, "strikethrough")
	if st.Effective != 5 || !st.Capped {
		t.Errorf("strikethrough: effective %d capped %v, want 5/true", st.Effective, st.Capped)
	}
	acc, _ := findDelta(res, "accuracy")
	if acc.Effective != 15 || acc.Capped {
		t.Errorf("accuracy: effective %d capped %v, want 15/false", acc.Effective, acc.Capped)
	}
	// 5×4 + 15×1 + 5×2 = 45.
	if res.Score != 45 {
		t.Errorf("score = %v, want 45", res.Score)
	}
}

func TestBackstabDamage(t *testing.T) {
	dagger := StatLine{ItemType: ItemType1HPiercing, Damage: 12}
	cases := []struct {
		name    string
		level   int
		skill   int
		mod     int
		primary StatLine
		want    int
	}{
		// (225×0.02 + 2) × 12 = 6.5 × 12 = 78.
		{"maxed skill, no mod", 65, 225, 0, dagger, 78},
		// 225×110/100 = 247 → 6.94 × 12 = 83.
		{"+10% mod", 65, 225, 10, dagger, 83},
		// 225×112/100 = 252 (hard cap) → 7.04 × 12 = 84; +15% adds nothing.
		{"+12% mod hits the cap", 65, 225, 12, dagger, 84},
		{"+15% mod wasted past the cap", 65, 225, 15, dagger, 84},
		// Not a 1H piercer: backstab fails outright.
		{"1H slashing", 65, 225, 0, StatLine{ItemType: 0, Damage: 20}, 0},
		{"2H piercing", 65, 225, 0, StatLine{ItemType: 35, Damage: 30}, 0},
		{"empty hand", 65, 225, 0, StatLine{}, 0},
		// Below rec level 60 at 50: 12 × 50/60 = 10 → (200×0.02+2) × 10 = 60.
		{"below recommended level", 50, 200, 0, StatLine{ItemType: ItemType1HPiercing, Damage: 12, RecLevel: 60}, 60},
	}
	for _, c := range cases {
		if got := BackstabDamage(c.level, c.skill, c.mod, c.primary); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// A rogue's Primary comparison: a sword (no backstab) is a big loss, a
// better dagger a gain; the best skill mod comes from either the slot or the
// rest of the loadout.
func TestScore_BackstabTerm(t *testing.T) {
	w := Weights{Backstab: 3}
	dagger := StatLine{ItemType: ItemType1HPiercing, Damage: 12}
	ctx := Context{Level: 65, BackstabSkill: 225, Primary: dagger, SlotIsPrimary: true}

	sword := StatLine{ItemType: 0, Damage: 20}
	res := Score(ctx, w, dagger, sword)
	if bs, _ := findDelta(res, "backstab"); bs.Current != 78 || bs.Cand != 0 || res.Score != -234 {
		t.Errorf("sword: delta %+v score %v, want 78→0, -234", bs, res.Score)
	}

	better := StatLine{ItemType: ItemType1HPiercing, Damage: 13}
	if res := Score(ctx, w, dagger, better); res.Score != 3*(84-78) {
		t.Errorf("13-dmg dagger: score %v, want %v", res.Score, 3*(84-78))
	}

	// An offhand carrying a +12% mod (OtherBackstabMod) applies to both.
	ctx.OtherBackstabMod = 12
	if res := Score(ctx, w, dagger, better); res.Score != 3*(91-84) {
		t.Errorf("with offhand mod: score %v, want %v", res.Score, 3*(91-84))
	}

	// Non-Primary slot: the worn Primary stays; only the mod changes.
	ctx = Context{Level: 65, BackstabSkill: 225, Primary: dagger}
	modOffhand := StatLine{ItemType: ItemType1HPiercing, Damage: 8, BackstabMod: 12}
	if res := Score(ctx, w, StatLine{}, modOffhand); res.Score != 3*(84-78) {
		t.Errorf("offhand mod: score %v, want %v", res.Score, 3*(84-78))
	}

	// No backstab skill (not a rogue): no term at all.
	ctx.BackstabSkill = 0
	if _, ok := findDelta(Score(ctx, w, dagger, sword), "backstab"); ok {
		t.Error("backstab delta without backstab skill")
	}
}

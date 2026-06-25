package resist

import (
	"math"
	"testing"
)

func mezSpell() Spell {
	var s Spell
	s.ResistType = resistMagic
	s.EffectIDs[0] = seMez
	return s
}

func nukeSpell(resistType, resistDiff int) Spell {
	var s Spell
	s.ResistType = resistType
	s.ResistDiff = resistDiff
	s.EffectIDs[0] = seCurrentHPOnce
	s.EffectBase[0] = -500
	return s
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestUnresistable(t *testing.T) {
	s := nukeSpell(resistNone, 0)
	r := ComputeChances(Input{Spell: s, CasterLevel: 60, TargetLevel: 60, TargetResist: 200})
	if !r.Unresistable || r.LandChance != 1 || r.FullDamage != 1 {
		t.Fatalf("unresistable spell should always land: %+v", r)
	}
}

func TestBinaryNegativeResistChanceAlwaysLands(t *testing.T) {
	// L60 caster vs L50 NPC, 0 resist → resist_chance is negative, mez always
	// lands.
	r := ComputeChances(Input{
		Spell:        mezSpell(),
		CasterLevel:  60,
		CasterClass:  11, // Wizard (not Enchanter)
		TargetLevel:  50,
		TargetResist: 0,
		Era:          Era{LuclinEnabled: true},
	})
	if r.ResistChance != -40 {
		t.Fatalf("resist_chance = %d, want -40", r.ResistChance)
	}
	if !r.Binary {
		t.Fatalf("mez should be binary")
	}
	approx(t, "LandChance", r.LandChance, 1)
	approx(t, "AvgCastsToLand", r.AvgCastsToLand, 1)
}

func TestBinaryPartialResistChance(t *testing.T) {
	// L60 caster vs L50 NPC, MR 100, ResistDiff 0:
	// levelMod = -(9*9/2) = -40, resist_chance = 100 - 40 = 60.
	// rolls 61..200 land (140), 0..60 resist (61).
	r := ComputeChances(Input{
		Spell:        mezSpell(),
		CasterLevel:  60,
		CasterClass:  11,
		TargetLevel:  50,
		TargetResist: 100,
		Era:          Era{LuclinEnabled: true},
	})
	if r.ResistChance != 60 {
		t.Fatalf("resist_chance = %d, want 60", r.ResistChance)
	}
	approx(t, "FullDamage", r.FullDamage, 140.0/201.0)
	approx(t, "FullResist", r.FullResist, 61.0/201.0)
	approx(t, "Partial", r.Partial, 0) // binary: no partials
	approx(t, "LandChance", r.LandChance, 140.0/201.0)
	approx(t, "AvgCastsToLand", r.AvgCastsToLand, 201.0/140.0)
}

func TestSixLevelRulePreLuclin(t *testing.T) {
	// Luclin OFF → six-level rule fires: L40 caster vs L55 NPC
	// (55 >= max(47, 50)=50) pins level_mod to 1000, resist_chance huge,
	// nothing lands.
	r := ComputeChances(Input{
		Spell:        mezSpell(),
		CasterLevel:  40,
		CasterClass:  11,
		TargetLevel:  55,
		TargetResist: 50,
		Era:          Era{LuclinEnabled: false},
	})
	if r.ResistChance != 1050 {
		t.Fatalf("resist_chance = %d, want 1050", r.ResistChance)
	}
	approx(t, "LandChance", r.LandChance, 0)
	approx(t, "AvgCastsToLand", r.AvgCastsToLand, 0)
}

func TestSixLevelRuleOffUnderLuclin(t *testing.T) {
	// Same setup but Luclin ON → six-level rule does NOT fire. L40 caster <50
	// vs L55 NPC: bump_level = 40+4+6 = 50, 55>=50 → level_mod += 70+240 = 310.
	// levelMod base: tempLevelDiff = 15 → 15*15/2 = 112. resist_chance =
	// 50 + 112 + 310 = 472.
	r := ComputeChances(Input{
		Spell:        mezSpell(),
		CasterLevel:  40,
		CasterClass:  11,
		TargetLevel:  55,
		TargetResist: 50,
		Era:          Era{LuclinEnabled: true},
	})
	if r.ResistChance != 472 {
		t.Fatalf("resist_chance = %d, want 472", r.ResistChance)
	}
	approx(t, "LandChance", r.LandChance, 0) // 472 > 200, still never lands
}

func TestPartialNukeDistribution(t *testing.T) {
	// L60 caster vs L55 NPC, FR 150, ResistDiff -50, classic resists (PoP off):
	// levelMod = -(5*5/2) = -12, resist_chance = 150 - 12 - 50 = 88.
	// partial_modifier(roll) = 150*(88-roll)/88 + (60-25)=35  [targetLevel>=30]
	//   roll 0..49  -> >=100 -> full resist (50 rolls)
	//   roll 50..88 -> partial            (39 rolls)
	//   roll 89..200-> full damage        (112 rolls)
	r := ComputeChances(Input{
		Spell:        nukeSpell(resistFire, -50),
		CasterLevel:  60,
		CasterClass:  11,
		TargetLevel:  55,
		TargetResist: 150,
		Era:          Era{LuclinEnabled: true}, // PoP off
	})
	if r.ResistChance != 88 {
		t.Fatalf("resist_chance = %d, want 88", r.ResistChance)
	}
	if r.Binary {
		t.Fatalf("nuke should not be binary")
	}
	approx(t, "FullResist", r.FullResist, 50.0/201.0)
	approx(t, "Partial", r.Partial, 39.0/201.0)
	approx(t, "FullDamage", r.FullDamage, 112.0/201.0)
	approx(t, "LandChance", r.LandChance, 151.0/201.0)
	// roll 50 -> eff 1 (0.01); roll 88 -> eff 65 (0.65)
	approx(t, "PartialMin", r.PartialMin, 0.01)
	approx(t, "PartialMax", r.PartialMax, 0.65)
}

func TestEnchanterCharismaLowersMezResist(t *testing.T) {
	// Enchanter CHA 235 vs L50 NPC MR 100: resist_modifier -= (235-75)/8 = 20.
	// resist_chance = 100 - 40 - 20 = 40 (vs 60 for a non-Enchanter).
	r := ComputeChances(Input{
		Spell:        mezSpell(),
		CasterLevel:  60,
		CasterClass:  classEnchanter,
		CasterCHA:    235,
		TargetLevel:  50,
		TargetResist: 100,
		Era:          Era{LuclinEnabled: true},
	})
	if r.ResistChance != 40 {
		t.Fatalf("resist_chance = %d, want 40", r.ResistChance)
	}
}

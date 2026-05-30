package eqstat

import "testing"

func TestBaseHP(t *testing.T) {
	// Enchanter L60: factor 120 → lm 12, levelHP 720. STA 147:
	// 720*147/300 + 720 = 352 + 720 = 1072.
	if got := BaseHP(Enchanter, 60, 147); got != 1072 {
		t.Errorf("BaseHP(Enchanter,60,147) = %d, want 1072", got)
	}
	// STA over 255 counts half: STA 305 → (305-255)/2+255 = 280.
	// 720*280/300 + 720 = 672 + 720 = 1392.
	if got := BaseHP(Enchanter, 60, 305); got != 1392 {
		t.Errorf("BaseHP(Enchanter,60,305) = %d, want 1392", got)
	}
}

func TestMaxHP(t *testing.T) {
	// No items/buffs/AA: base 1072 + 5.
	if got := MaxHP(Enchanter, 60, 147, 0, 0, 0, 0); got != 1077 {
		t.Errorf("MaxHP base = %d, want 1077", got)
	}
	// 10% AA HP (Natural Durability r3) applies to base+item, then +5.
	// (1072+0)*1.10 = 1179 (int), +5 = 1184.
	if got := MaxHP(Enchanter, 60, 147, 0, 0, 0, 10); got != 1184 {
		t.Errorf("MaxHP +10%% = %d, want 1184", got)
	}
	// Item HP is inside the AA percent; buff HP is added after.
	// (1072+1280)*1.0 +5 +1775 = 4132.
	if got := MaxHP(Enchanter, 60, 147, 1280, 1775, 0, 0); got != 4132 {
		t.Errorf("MaxHP Osui-like = %d, want 4132", got)
	}
}

func TestBaseMana(t *testing.T) {
	// Enchanter L60, INT 120: lf 900, sf 120 → 120+3*20/2=150;
	// 150*900/200 + 900 = 675 + 900 = 1575.
	if got := BaseMana(Enchanter, 60, 120); got != 1575 {
		t.Errorf("BaseMana(Enchanter,60,120) = %d, want 1575", got)
	}
	// INT 255: sf (255-200)/2+200=227 → 227+3*127/2=417;
	// 417*900/200 + 900 = 1876 + 900 = 2776.
	if got := BaseMana(Enchanter, 60, 255); got != 2776 {
		t.Errorf("BaseMana(Enchanter,60,255) = %d, want 2776", got)
	}
	// Pure melee classes have no mana.
	if got := BaseMana(Warrior, 60, 200); got != 0 {
		t.Errorf("BaseMana(Warrior) = %d, want 0", got)
	}
}

func TestMaxManaHybridFloor(t *testing.T) {
	if got := MaxMana(Ranger, 8, 0, 0, 0); got != 0 {
		t.Errorf("MaxMana(Ranger,8) = %d, want 0 (no mana before 9)", got)
	}
	if got := MaxMana(Ranger, 9, 100, 0, 0); got == 0 {
		t.Errorf("MaxMana(Ranger,9) = 0, want >0")
	}
}

func TestCasterType(t *testing.T) {
	cases := map[int]byte{
		Wizard: casterINT, Enchanter: casterINT, Necromancer: casterINT,
		Magician: casterINT, ShadowKnight: casterINT,
		Cleric: casterWIS, Druid: casterWIS, Shaman: casterWIS,
		Paladin: casterWIS, Ranger: casterWIS, Beastlord: casterWIS,
		Warrior: casterNone, Monk: casterNone, Rogue: casterNone, Bard: casterNone,
	}
	for class, want := range cases {
		if got := CasterType(class); got != want {
			t.Errorf("CasterType(%d) = %c, want %c", class, got, want)
		}
	}
}

func TestBaseResists(t *testing.T) {
	if r := BaseResists(RaceDarkElf); r != (Resists{MR: 25, CR: 25, FR: 25, DR: 15, PR: 15}) {
		t.Errorf("DarkElf resists = %+v", r)
	}
	if r := BaseResists(RaceDwarf); r.MR != 30 || r.PR != 20 {
		t.Errorf("Dwarf MR/PR = %d/%d, want 30/20", r.MR, r.PR)
	}
	if r := BaseResists(RaceTroll); r.FR != 5 {
		t.Errorf("Troll FR = %d, want 5", r.FR)
	}
	if r := BaseResists(RaceIksar); r.FR != 30 || r.CR != 15 {
		t.Errorf("Iksar FR/CR = %d/%d, want 30/15", r.FR, r.CR)
	}
}

func TestResistanceClassLevel(t *testing.T) {
	// Level-60 Ranger fire: racial 25 + (4 + (60-49)) = 25 + 15 = 40.
	r := Resistance(Ranger, 60, RaceHuman, Resists{}, Resists{})
	if r.FR != 40 {
		t.Errorf("Ranger L60 FR = %d, want 40", r.FR)
	}
	// Warrior MR gets + level/2.
	w := Resistance(Warrior, 60, RaceHuman, Resists{}, Resists{})
	if w.MR != 25+30 {
		t.Errorf("Warrior L60 MR = %d, want 55", w.MR)
	}
	// Resist floor is 1, cap is 500 (+capMod).
	low := Resistance(Enchanter, 60, RaceTroll, Resists{FR: -100}, Resists{})
	if low.FR != 1 {
		t.Errorf("floored FR = %d, want 1", low.FR)
	}
	high := Resistance(Enchanter, 60, RaceDarkElf, Resists{MR: 1000}, Resists{MR: 20})
	if high.MR != 520 {
		t.Errorf("capped MR = %d, want 520", high.MR)
	}
}

func TestMaxStat(t *testing.T) {
	if got := MaxStat(60, 0); got != 255 {
		t.Errorf("MaxStat(60) = %d, want 255", got)
	}
	if got := MaxStat(65, 0); got != 280 {
		t.Errorf("MaxStat(65) = %d, want 280", got)
	}
	if got := MaxStat(60, 5); got != 260 {
		t.Errorf("MaxStat(60,+5) = %d, want 260", got)
	}
}

func TestAvoidance(t *testing.T) {
	// Osui: defense 100 → 100*400/225 = 177; agi 121, L60 → bonusAdj 80,
	// 2*(80 - (200-121)/5)/3 = 2*(80-15)/3 = 130/3 = 43. Sum 220.
	if got := avoidance(100, 121, 60); got != 220 {
		t.Errorf("avoidance(100,121,60) = %d, want 220", got)
	}
}

func TestDisplayedACOsui(t *testing.T) {
	// Dark Elf Enchanter, defense 100, AGI 121, item AC 470, spell AC 130.
	// avoidance 220; mitigation (caster, no ×4/3): 470 + defense/2(50) +
	// spellAC/3(43) + agi/20(6) = 569. (220+569)*1000/847 = 931.
	if got := DisplayedAC(Enchanter, 60, RaceDarkElf, 470, 130, 121, 100, 0); got != 931 {
		t.Errorf("DisplayedAC Osui = %d, want 931", got)
	}
}

func TestDisplayedACPlateTank(t *testing.T) {
	// Warrior gets item AC ×4/3 and defense/3. Smoke test for ordering, not a
	// pinned in-game value.
	got := DisplayedAC(Warrior, 60, RaceHuman, 600, 100, 100, 210, 0)
	// mitigation: 600*4/3=800 + 210/3=70 + 100/4=25 + 100/20=5 = 900.
	// avoidance: 210*400/225=373 + agi100 L60 → 2*(80-20)/3=40 → 413.
	// (413+900)*1000/847 = 1550.
	if got != 1550 {
		t.Errorf("DisplayedAC plate = %d, want 1550", got)
	}
}

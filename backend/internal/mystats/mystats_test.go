package mystats

import (
	"strings"
	"testing"
)

// sampleSK is a real /mystats capture (character Kess, a Shadowknight with a
// 2H weapon so there is no Secondary block), message text only — the log
// timestamp prefix stripped.
const sampleSK = `---- Misc stats ----
Movement speed: 0%
Movement modifier: +55%
---- Defensive stats ----
AC (display): 1783 = (Mit: 1012  + Avoidance: 499) * 1000/847
Mitigation: 497 (softcap: 451)
Avoidance: 558 (with AAs)
---- Melee Primary: Goldenrod ----
Offense: 395 (Skill 225 + Stat 90 + SpellAtk 10 + ItemAtk 70 + Class 0)
To Hit: 457
Display ATK: 1145 = (offense + to hit) * 1000 / 744
Dmg = BonusDmg + BaseDmg * MitFactor * DmgMultiplier
Dmg = 30 + 40 * (0.1 to 2.0x) * (1 to 2.64, ave = 1.62)
Dmg = 34.00 to 242.99, ave = 95.19
DPS = 10.62 to 75.93, ave = 29.75
DPS = 20.00 to 142.94, ave = 56.00 (81% haste)`

func TestParseSampleSK(t *testing.T) {
	s, ok := Parse(strings.Split(sampleSK, "\n"))
	if !ok {
		t.Fatal("Parse returned ok=false for a real capture")
	}

	if s.MovementSpeedPct != 0 {
		t.Errorf("MovementSpeedPct = %d, want 0", s.MovementSpeedPct)
	}
	if s.MovementModifierPct != 55 {
		t.Errorf("MovementModifierPct = %d, want 55", s.MovementModifierPct)
	}
	if s.ACDisplay != 1783 || s.ACRawMit != 1012 || s.ACRawAvoid != 499 {
		t.Errorf("AC line = %d/%d/%d, want 1783/1012/499", s.ACDisplay, s.ACRawMit, s.ACRawAvoid)
	}
	if s.Mitigation != 497 || s.MitigationCap != 451 || s.MitigationCapKind != "softcap" {
		t.Errorf("mitigation = %d cap %s:%d, want 497 softcap:451",
			s.Mitigation, s.MitigationCapKind, s.MitigationCap)
	}
	if s.Avoidance != 558 {
		t.Errorf("Avoidance = %d, want 558 (must be the standalone line, not the AC line's 499)", s.Avoidance)
	}

	if len(s.Melee) != 1 {
		t.Fatalf("len(Melee) = %d, want 1 (no Secondary in this capture)", len(s.Melee))
	}
	b := s.Melee[0]
	if b.Slot != "Primary" || b.Weapon != "Goldenrod" {
		t.Errorf("melee[0] slot/weapon = %q/%q", b.Slot, b.Weapon)
	}
	if b.Offense != 395 || b.OffenseSkill != 225 || b.OffenseStat != 90 ||
		b.OffenseSpellAtk != 10 || b.OffenseItemAtk != 70 || b.OffenseClass != 0 {
		t.Errorf("offense breakdown = %+v", b)
	}
	if b.ToHit != 457 {
		t.Errorf("ToHit = %d, want 457", b.ToHit)
	}
	if b.DisplayATK != 1145 {
		t.Errorf("DisplayATK = %d, want 1145", b.DisplayATK)
	}
	if b.BonusDmg != 30 || b.BaseDmg != 40 || b.DmgMultMax != 2.64 || b.DmgMultAvg != 1.62 {
		t.Errorf("dmg formula = bonus %d base %d mult %.2f/%.2f", b.BonusDmg, b.BaseDmg, b.DmgMultMax, b.DmgMultAvg)
	}
	if b.DmgMin != 34.00 || b.DmgMax != 242.99 || b.DmgAvg != 95.19 {
		t.Errorf("dmg result = %.2f/%.2f/%.2f", b.DmgMin, b.DmgMax, b.DmgAvg)
	}
	if b.DPSMin != 10.62 || b.DPSMax != 75.93 || b.DPSAvg != 29.75 {
		t.Errorf("dps = %.2f/%.2f/%.2f", b.DPSMin, b.DPSMax, b.DPSAvg)
	}
	if b.HastePct != 81 || b.DPSHasteMin != 20.00 || b.DPSHasteMax != 142.94 || b.DPSHasteAvg != 56.00 {
		t.Errorf("dps+haste = %d%% %.2f/%.2f/%.2f", b.HastePct, b.DPSHasteMin, b.DPSHasteMax, b.DPSHasteAvg)
	}
}

func TestParseDualWieldTwoBlocks(t *testing.T) {
	// Synthetic: a dual-wielder gets a Secondary block. Only the structural
	// shape is asserted (two blocks, no Display ATK on Secondary, hand-to-hand
	// weapon name) — the numbers are made up.
	in := `---- Misc stats ----
Movement speed: 0%
---- Defensive stats ----
AC (display): 1000 = (Mit: 600 + Avoidance: 300) * 1000/847
Mitigation: 400 (hardcap: 405)
Avoidance: 320
---- Melee Primary: Fleshtile Bane ----
Offense: 300
To Hit: 400
Display ATK: 900
Dmg = 20 + 30 * (0.1 to 2.0x) * (1 to 2.00, ave = 1.40)
Dmg = 10.00 to 120.00, ave = 60.00
DPS = 5.00 to 60.00, ave = 30.00
---- Melee Secondary: HandToHand ----
Offense: 280
To Hit: 380
Dmg = 0 + 9 * (0.1 to 2.0x) * (1 to 1.80, ave = 1.30)
Dmg = 0.90 to 32.40, ave = 11.70
DPS = 2.00 to 20.00, ave = 10.00`
	s, ok := Parse(strings.Split(in, "\n"))
	if !ok {
		t.Fatal("ok=false")
	}
	if s.MitigationCapKind != "hardcap" || s.MitigationCap != 405 {
		t.Errorf("cap = %s:%d, want hardcap:405", s.MitigationCapKind, s.MitigationCap)
	}
	if len(s.Melee) != 2 {
		t.Fatalf("len(Melee) = %d, want 2", len(s.Melee))
	}
	if s.Melee[0].Slot != "Primary" || s.Melee[1].Slot != "Secondary" {
		t.Errorf("slots = %q/%q", s.Melee[0].Slot, s.Melee[1].Slot)
	}
	if s.Melee[1].Weapon != "HandToHand" {
		t.Errorf("secondary weapon = %q", s.Melee[1].Weapon)
	}
	if s.Melee[1].DisplayATK != 0 {
		t.Errorf("secondary should have no Display ATK, got %d", s.Melee[1].DisplayATK)
	}
	if s.Melee[1].Offense != 280 || s.Melee[1].DPSAvg != 10.00 {
		t.Errorf("secondary block = %+v", s.Melee[1])
	}
}

func TestParseNotMyStats(t *testing.T) {
	// The `/mystats info` formula-reference dump and unrelated chatter must not
	// produce a snapshot.
	for _, in := range []string{
		"---- mystats Beta info ----\nMitigation: modifies incoming damage based on offense vs mitigation",
		"You begin casting Shadow Vortex.\nYou no longer have a target.",
		"",
	} {
		if _, ok := Parse(strings.Split(in, "\n")); ok {
			t.Errorf("Parse(%q) returned ok=true, want false", in)
		}
	}
}

func TestIsBlockLine(t *testing.T) {
	in := []string{
		"---- Defensive stats ----", "Movement speed: 0%", "AC (display): 1 = (Mit: 1 + Avoidance: 1)",
		"To Hit: 5", "DPS = 1.0 to 2.0, ave = 1.5", "Dmg = 1 + 2 * (0.1 to 2.0x)",
	}
	for _, l := range in {
		if !IsBlockLine(l) {
			t.Errorf("IsBlockLine(%q) = false, want true", l)
		}
	}
	for _, l := range []string{"You begin casting X.", "Loading bandolier set [DPS]", "a gnoll hits you for 10."} {
		if IsBlockLine(l) {
			t.Errorf("IsBlockLine(%q) = true, want false", l)
		}
	}
}

package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// TestDerivedStats_OsuiPipeline exercises the full server-side derivation
// against real fixtures: it resolves Osui's actual worn gear from
// testdata/Osui-Quarmy.txt through quarm.db, folds in her real AA passives,
// and derives the equipped-layer vitals. This catches integration breakage the
// pure-formula tests can't — e.g. items not resolving, or class/race indexing
// drift — and confirms the numbers land in a believable range for a geared
// level-60 Enchanter.
func TestDerivedStats_OsuiPipeline(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	testdata := filepath.Join(repoRoot, "testdata")
	if _, err := os.Stat(filepath.Join(testdata, "Osui-Quarmy.txt")); err != nil {
		t.Skip("Osui-Quarmy.txt fixture not present")
	}
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	h := &charactersHandler{db: d}

	item, itemHaste := h.sumEquipment(testdata, "Osui")
	if item.AC <= 0 {
		t.Fatalf("expected positive item AC from Osui's gear, got %d (items not resolving?)", item.AC)
	}

	// Osui's real trained AAs (eqmacid + rank), from user.db.
	aa, err := d.AAStatBonuses([]db.TrainedAA{
		{AAID: 5, Rank: 3}, {AAID: 13, Rank: 3}, {AAID: 20, Rank: 3},
		{AAID: 21, Rank: 3}, {AAID: 35, Rank: 1}, {AAID: 55, Rank: 1},
		{AAID: 113, Rank: 1}, {AAID: 211, Rank: 3}, {AAID: 224, Rank: 3},
		{AAID: 225, Rank: 3},
	})
	if err != nil {
		t.Fatalf("aa bonuses: %v", err)
	}
	if aa.INT != 6 {
		t.Fatalf("Osui AA INT = %d, want 6", aa.INT)
	}

	defense, _ := d.DefenseSkillCap(14, 60) // Enchanter, level 60
	// Defense is skill_caps.skill_id 15 in the Mac-era enum; the caster cap is
	// 145 (matches in-game AC and EQMacEmu's GetAvoidance comments). Querying
	// skill_id 9 here previously returned Bind Wound's 100 and made AC ~120 low.
	if defense != 145 {
		t.Fatalf("Enchanter L60 defense cap = %d, want 145", defense)
	}

	offense, _ := d.OffenseSkillCap(14, 60) // Offense is universal; Enchanter L60 cap is 140
	weapon, _ := d.BestWeaponSkillCap(14, 60)
	if offense != 140 {
		t.Fatalf("Enchanter L60 offense cap = %d, want 140", offense)
	}
	if weapon != 110 {
		t.Fatalf("Enchanter L60 best weapon cap = %d, want 110", weapon)
	}

	base := osui()
	skills := skillCaps{defense: defense, offense: offense, weapon: weapon}
	equipped := h.deriveBlock(base, aa, spellHasteSplit{}, skills, item, itemHaste, nil)

	// ATK rating is present and positive even for a pure caster (weapon skill +
	// STR term carry it). It must exceed the raw worn ATK bonus it derives from.
	if equipped.ATKRating <= equipped.Attack {
		t.Errorf("equipped ATKRating = %d, expected > worn ATK bonus %d", equipped.ATKRating, equipped.Attack)
	}
	t.Logf("Osui equipped: HP=%d Mana=%d AC=%d ATK=%d | wornATK=%d (item %d, aa %d, buff %d) | manaRegen=%d (item %d, aa %d, buff %d) | FT=%d",
		equipped.HP, equipped.Mana, equipped.AC, equipped.ATKRating,
		equipped.Attack, equipped.Breakdown.Attack.Item, equipped.Breakdown.Attack.AA, equipped.Breakdown.Attack.Buff,
		equipped.ManaRegen, equipped.Breakdown.ManaRegen.Item, equipped.Breakdown.ManaRegen.AA, equipped.Breakdown.ManaRegen.Buff,
		equipped.FT)

	// Sanity ranges for a geared level-60 Enchanter (no raid buffs):
	// HP a couple thousand, mana a few thousand, AC a few hundred.
	if equipped.HP < 1500 || equipped.HP > 6000 {
		t.Errorf("equipped HP = %d, out of believable range", equipped.HP)
	}
	if equipped.Mana < 2000 || equipped.Mana > 7000 {
		t.Errorf("equipped Mana = %d, out of believable range", equipped.Mana)
	}
	if equipped.AC < 300 || equipped.AC > 1200 {
		t.Errorf("equipped AC = %d, out of believable range", equipped.AC)
	}
	// Dark Elf disease/poison floors carry through to the equipped layer.
	if equipped.DR < 15 || equipped.PR < 15 {
		t.Errorf("equipped DR/PR = %d/%d, want >= DE floor 15", equipped.DR, equipped.PR)
	}
	t.Logf("Osui equipped (no buffs): HP=%d Mana=%d AC=%d MR=%d itemAC=%d",
		equipped.HP, equipped.Mana, equipped.AC, equipped.MR, item.AC)
}

// TestSumEquipment_AmmoSlotExcluded pins the in-game rule that the Ammo slot
// confers no stats: EQMacEmu's Client::CalcItemBonuses iterates
// slotEar1 <= i < slotAmmo ("should not include 21 (SLOT_AMMO)"). A statted
// item parked in Ammo (e.g. a tradeskill trophy) must not leak into the
// equipment column.
func TestSumEquipment_AmmoSlotExcluded(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	// The fixture item must itself carry stats, or the test proves nothing.
	amulet, err := d.GetItem(28901) // Shade Stone Amulet: HP 50, Mana 125, AC 20
	if err != nil || amulet == nil {
		t.Fatalf("load item 28901: %v", err)
	}
	if amulet.HP == 0 && amulet.Mana == 0 && amulet.AC == 0 {
		t.Fatalf("item 28901 has no stats; pick a different fixture item")
	}

	dir := t.TempDir()
	quarmy := "Character\tName\tLastName\tLevel\tClass\tRace\tGender\tDeity\tGuild\tGuildRank\tBaseSTR\tBaseSTA\tBaseCHA\tBaseDEX\tBaseINT\tBaseAGI\tBaseWIS\n" +
		"Character\tAmmotest\t\t60\t14\t6\t1\t396\t\t0\t60\t65\t95\t75\t114\t90\t83\n" +
		"Location\tName\tID\tCount\tSlots\n" +
		"Ammo\tShade Stone Amulet\t28901\t1\t0\n"
	if err := os.WriteFile(filepath.Join(dir, "Ammotest-Quarmy.txt"), []byte(quarmy), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Guard against a vacuous pass: the synthetic export must parse and the
	// Ammo entry must be visible to the inventory walk.
	q, err := zeal.ParseQuarmy(filepath.Join(dir, "Ammotest-Quarmy.txt"), "Ammotest")
	if err != nil || q == nil || len(q.Inventory) != 1 || q.Inventory[0].Location != "Ammo" || q.Inventory[0].ID != 28901 {
		t.Fatalf("synthetic Quarmy fixture did not parse as expected: %+v err=%v", q, err)
	}

	h := &charactersHandler{db: d}
	block, haste := h.sumEquipment(dir, "Ammotest")
	if block != (statBlock{}) || haste != 0 {
		t.Errorf("ammo-slot item contributed stats: %+v (haste %d), want all zero", block, haste)
	}
}

// TestSumEquipment_EdibleBonuses pins EQMacEmu Client::CalcEdibleBonuses: the
// first food and first drink found while scanning general slots (and their bag
// contents) in order contribute their stats; a second food/drink, and any
// food/drink outside the general inventory (bank), are ignored.
func TestSumEquipment_EdibleBonuses(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("quarm.db not present")
	}
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	// Fixtures (type 14 food / type 15 drink):
	//   7759  Talionn's Finest Stew  food  +3 WIS +3 INT   (first food → applies)
	//   22871 Paludal Beetle Crunch  food  +1 WIS          (second food → ignored)
	//   5805  Vile Sarnak Brew       drink +50 mana        (first drink → applies)
	dir := t.TempDir()
	quarmy := "Character\tName\tLastName\tLevel\tClass\tRace\tGender\tDeity\tGuild\tGuildRank\tBaseSTR\tBaseSTA\tBaseCHA\tBaseDEX\tBaseINT\tBaseAGI\tBaseWIS\n" +
		"Character\tFoodtest\t\t60\t14\t6\t1\t396\t\t0\t60\t65\t95\t75\t114\t90\t83\n" +
		"Location\tName\tID\tCount\tSlots\n" +
		"General1-Slot1\tTalionn's Finest Stew\t7759\t1\t0\n" +
		"General1-Slot2\tPaludal Beetle Crunchies\t22871\t1\t0\n" +
		"General2\tVile Sarnak Brew\t5805\t1\t0\n" +
		"Bank1\tTalionn's Finest Stew\t7759\t1\t0\n"
	if err := os.WriteFile(filepath.Join(dir, "Foodtest-Quarmy.txt"), []byte(quarmy), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	h := &charactersHandler{db: d}
	block, _ := h.sumEquipment(dir, "Foodtest")

	// Only the first food (stew: +3 WIS +3 INT) and first drink (brew: +50 mana)
	// apply. The second food's +1 WIS and the bank food are excluded.
	if block.WIS != 3 {
		t.Errorf("WIS = %d, want 3 (first food only; second food +1 and bank food must not count)", block.WIS)
	}
	if block.INT != 3 {
		t.Errorf("INT = %d, want 3 (first food)", block.INT)
	}
	if block.Mana != 50 {
		t.Errorf("Mana = %d, want 50 (first drink)", block.Mana)
	}
}

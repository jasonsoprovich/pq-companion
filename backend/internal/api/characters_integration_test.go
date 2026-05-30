package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
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

	base := osui()
	equipped := h.deriveBlock(base, aa, 0, defense, item, itemHaste, nil)

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

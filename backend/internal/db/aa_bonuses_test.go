package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestAAStatBonuses_Osui verifies the eqmacid → skill_id → aa_effects join and
// cumulative-rank resolution against Osui's real trained AAs in quarm.db.
//
// Osui (DE Enchanter) trains Innate Intelligence to rank 3 (eqmacid 5,
// skill_id 22, rank-3 row 24 → INT +6) plus Mental Clarity (mana regen) and
// Innate Regeneration (HP regen). No stat-cap or HP-percent AAs.
func TestAAStatBonuses_Osui(t *testing.T) {
	d := openTestDB(t)

	trained := []db.TrainedAA{
		{AAID: 5, Rank: 3},   // Innate Intelligence → INT +6
		{AAID: 13, Rank: 3},  // Innate Run Speed (no stat effect)
		{AAID: 20, Rank: 3},  // Spell Casting Mastery (no stat effect)
		{AAID: 21, Rank: 3},  // Spell Casting Reinforcement
		{AAID: 35, Rank: 1},  // Mass Group Buff
		{AAID: 55, Rank: 1},  // Permanent Illusion
		{AAID: 113, Rank: 1}, // Spell Casting Reinforcement Mastery
		{AAID: 211, Rank: 3}, // Fleet of Foot
		{AAID: 224, Rank: 3}, // Mental Clarity → mana regen
		{AAID: 225, Rank: 3}, // Innate Regeneration → HP regen
	}

	b, err := d.AAStatBonuses(trained)
	if err != nil {
		t.Fatalf("AAStatBonuses: %v", err)
	}
	if b.INT != 6 {
		t.Errorf("INT grant = %d, want 6 (Innate Intelligence rank 3)", b.INT)
	}
	if b.STR != 0 || b.STA != 0 || b.WIS != 0 {
		t.Errorf("unexpected non-INT stat grant: %+v", b)
	}
	if b.HPPct != 0 {
		t.Errorf("HPPct = %v, want 0 (no Natural Durability)", b.HPPct)
	}
	if b.ManaRegen <= 0 {
		t.Errorf("ManaRegen = %d, want >0 (Mental Clarity)", b.ManaRegen)
	}
	if b.HPRegen <= 0 {
		t.Errorf("HPRegen = %d, want >0 (Innate Regeneration)", b.HPRegen)
	}
}

// TestAAStatBonuses_Hatemod verifies the Spell Casting Subtlety AA (eqmacid 25)
// resolves to its cumulative hate-reduction percentage: -5/-10/-20 across ranks.
func TestAAStatBonuses_Hatemod(t *testing.T) {
	d := openTestDB(t)
	for _, tc := range []struct {
		rank int
		want int
	}{
		{1, -5},
		{2, -10},
		{3, -20},
	} {
		b, err := d.AAStatBonuses([]db.TrainedAA{{AAID: 25, Rank: tc.rank}})
		if err != nil {
			t.Fatalf("AAStatBonuses rank %d: %v", tc.rank, err)
		}
		if b.Hatemod != tc.want {
			t.Errorf("Spell Casting Subtlety rank %d Hatemod = %d, want %d", tc.rank, b.Hatemod, tc.want)
		}
	}
}

// TestAAStatBonuses_Empty guards the no-AA path.
func TestAAStatBonuses_Empty(t *testing.T) {
	d := openTestDB(t)
	b, err := d.AAStatBonuses(nil)
	if err != nil {
		t.Fatalf("AAStatBonuses(nil): %v", err)
	}
	if (b != db.AABonuses{}) {
		t.Errorf("empty bonuses = %+v, want zero", b)
	}
}

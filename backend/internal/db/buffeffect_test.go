package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestApplyLevelFormula pins the level-scaling formula port against the
// frontend applyLevelFormula (spellHelpers.ts). Level 60 is the server cap.
func TestApplyLevelFormula(t *testing.T) {
	const lvl = 60
	cases := []struct {
		name                    string
		formula, base, max, exp int
	}{
		{"static-100", 100, 250, 0, 250},
		{"static-0", 0, 230, 0, 230},
		{"104-uncapped", 104, 250, 0, 430},    // Khura's Focusing HP: 250 + 60*3
		{"104-capped", 104, 250, 400, 400},    // max clamps the scaled result
		{"102-linear", 102, 10, 0, 70},        // 10 + 60
		{"101-half", 101, 10, 0, 40},          // 10 + 60/2
		{"neg-102-clamp", 102, -10, -45, -45}, // Malo MR: -10-60 → clamp to -45
		{"unknown-fallback", 203, 69, 1300, 69},
	}
	for _, c := range cases {
		if got := db.ApplyLevelFormula(c.formula, c.base, c.max, lvl); got != c.exp {
			t.Errorf("%s: ApplyLevelFormula(%d,%d,%d,%d)=%d, want %d",
				c.name, c.formula, c.base, c.max, lvl, got, c.exp)
		}
	}
}

// TestComputeBuffStatDelta_Scaling verifies real Quarm buffs resolve to their
// at-cap values: Khura's Focusing (formula 104 HP) scales, Aego (formula 100)
// does not. Guards the internal consistency between the character buff list and
// the spell database page.
func TestComputeBuffStatDelta_Scaling(t *testing.T) {
	d := openTestDB(t)

	// Khura's Focusing (2530): HP SPA 69 base 250 formula 104 → 430 at level 60.
	// STR/DEX slots are formula 100 (static) → unchanged.
	khura, err := d.GetSpell(2530)
	if err != nil || khura == nil {
		t.Fatalf("GetSpell(Khura's Focusing): %v", err)
	}
	kd := db.ComputeBuffStatDelta(khura)
	if kd.HP != 430 {
		t.Errorf("Khura HP = %d, want 430 (250 + 60*3, formula 104)", kd.HP)
	}
	if kd.STR != 67 || kd.DEX != 60 {
		t.Errorf("Khura STR/DEX = %d/%d, want 67/60 (static)", kd.STR, kd.DEX)
	}

	// Ancient: Gift of Aegolism (2122): AC SPA 1 base 230 f100, HP SPA 69 1300
	// f100 — both static, so the raw values must survive unchanged.
	aego, err := d.GetSpell(2122)
	if err != nil || aego == nil {
		t.Fatalf("GetSpell(Aegolism): %v", err)
	}
	ad := db.ComputeBuffStatDelta(aego)
	if ad.AC != 230 {
		t.Errorf("Aego AC = %d, want 230 (raw SPA 1, formula 100)", ad.AC)
	}
	if ad.HP != 1300 {
		t.Errorf("Aego HP = %d, want 1300 (formula 100)", ad.HP)
	}
}

package db_test

import "testing"

// TestSpellSPA_Anchors verifies that a curated set of well-known spells in
// quarm.db carry the canonical EQEmu SPA codes (as defined in EQEmu's
// spdat.h SE_* enum) in the expected effect slots.
//
// The frontend renders spell effects using a static SPA → label table
// (frontend/src/lib/spellHelpers.ts) sourced from spdat.h. The DB converter
// is a pass-through for effectidN columns, so as long as the upstream Quarm
// MySQL dump uses standard EQEmu SPA codes, the frontend table is correct.
//
// If a future DB rebuild ever shifted the meaning of these effectid values,
// this test fails first — flagging the regression before users see broken
// labels in the Spells tab.
func TestSpellSPA_Anchors(t *testing.T) {
	d := openTestDB(t)

	type slotCheck struct {
		slot      int // 0-indexed (0 = effectid1)
		wantSPA   int
		wantBase  int  // expected effect_base_valueN
		checkBase bool // if false, only the SPA code is asserted
	}

	cases := []struct {
		spellID int
		name    string
		slots   []slotCheck
	}{
		{
			spellID: 200, name: "Minor Healing",
			slots: []slotCheck{{slot: 0, wantSPA: 0, wantBase: 10, checkBase: true}}, // SPA 0 = Hitpoints
		},
		{
			spellID: 159, name: "Strength",
			slots: []slotCheck{{slot: 0, wantSPA: 4, wantBase: 42, checkBase: true}}, // SPA 4 = STR
		},
		{
			spellID: 278, name: "Spirit of Wolf",
			slots: []slotCheck{{slot: 1, wantSPA: 3, wantBase: 30, checkBase: true}}, // SPA 3 = Movement Speed
		},
		{
			spellID: 174, name: "Clarity",
			slots: []slotCheck{{slot: 1, wantSPA: 15, checkBase: false}}, // SPA 15 = Mana
		},
		{
			spellID: 213, name: "Cure Disease",
			slots: []slotCheck{{slot: 0, wantSPA: 35, checkBase: false}}, // SPA 35 = Disease Counter
		},
		{
			spellID: 2570, name: "Koadic's Endless Intellect",
			slots: []slotCheck{
				{slot: 0, wantSPA: 97, wantBase: 250, checkBase: true}, // SPA 97 = Mana Pool
				{slot: 2, wantSPA: 8, wantBase: 25, checkBase: true},   // SPA 8 = INT
			},
		},
		{
			// Extended Enhancement III is the focus item that triggered this work
			// (Dragon Scaled Mask). All six slots are focus/limit SPAs.
			spellID: 2335, name: "Extended Enhancement III",
			slots: []slotCheck{
				{slot: 0, wantSPA: 128, wantBase: 15, checkBase: true},   // Spell Duration
				{slot: 1, wantSPA: 134, wantBase: 60, checkBase: true},   // Limit: Max Level
				{slot: 2, wantSPA: 138, wantBase: 1, checkBase: true},    // Limit: Spell Type (Beneficial)
				{slot: 3, wantSPA: 137, wantBase: -101, checkBase: true}, // Exclude: Effect(Complete Heal)
				{slot: 4, wantSPA: 137, wantBase: -40, checkBase: true},  // Exclude: Effect(Divine Aura)
				{slot: 5, wantSPA: 140, wantBase: 8, checkBase: true},    // Limit: Min Duration (8 ticks = 48s)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sp, err := d.GetSpell(tc.spellID)
			if err != nil {
				t.Fatalf("GetSpell(%d %q): %v", tc.spellID, tc.name, err)
			}
			if sp.Name != tc.name {
				t.Fatalf("spell ID %d: name = %q, want %q (DB layout shifted?)",
					tc.spellID, sp.Name, tc.name)
			}
			for _, s := range tc.slots {
				gotSPA := sp.EffectIDs[s.slot]
				if gotSPA != s.wantSPA {
					t.Errorf("%s slot %d: SPA = %d, want canonical EQEmu SPA %d",
						tc.name, s.slot, gotSPA, s.wantSPA)
				}
				if s.checkBase {
					gotBase := sp.EffectBaseValues[s.slot]
					if gotBase != s.wantBase {
						t.Errorf("%s slot %d: base = %d, want %d",
							tc.name, s.slot, gotBase, s.wantBase)
					}
				}
			}
		})
	}
}

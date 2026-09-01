package db_test

import "testing"

// IllusionSpellIDs must return the SPA-58 illusion family — enough of it that
// the spell-timer engine can recognise a shared-land-text collision as "some
// illusion" and collapse it to one generic timer.
func TestIllusionSpellIDs(t *testing.T) {
	d := openTestDB(t)

	ids, err := d.IllusionSpellIDs()
	if err != nil {
		t.Fatalf("IllusionSpellIDs: %v", err)
	}
	if len(ids) == 0 {
		t.Fatal("IllusionSpellIDs returned nothing")
	}

	// Every spell that shares the "'s image shimmers." cast_on_other text is an
	// illusion, so each must be in the set (this is exactly the candidate list
	// the engine has to collapse).
	want := map[int]string{
		243:  "Illusion: Iksar",
		582:  "Illusion: Human",
		588:  "Illusion: Wood Elf",
		590:  "Illusion: Dark Elf",
		287:  "Minor Illusion",
		3847: "Cloak of Khala Dun",
		3854: "Form of Protection",
	}
	for id, name := range want {
		if !ids[id] {
			t.Errorf("expected illusion set to contain %d (%s)", id, name)
		}
	}

	// A plain non-illusion spell must NOT be in the set.
	if ids[15] { // 15 = Greater Healing
		t.Errorf("illusion set unexpectedly contains spell 15 (Greater Healing)")
	}
}

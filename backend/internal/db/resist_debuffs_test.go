package db_test

import "testing"

func TestResistDebuffSpells(t *testing.T) {
	d := openTestDB(t)
	list, err := d.ResistDebuffSpells()
	if err != nil {
		t.Fatalf("ResistDebuffSpells: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected some resist debuffs")
	}

	by := make(map[string]struct{ MR, CR, FR, DR, PR int })
	for _, sp := range list {
		by[sp.Name] = struct{ MR, CR, FR, DR, PR int }{sp.MR, sp.CR, sp.FR, sp.DR, sp.PR}
		// Every entry must lower at least one resist.
		if sp.MR >= 0 && sp.CR >= 0 && sp.FR >= 0 && sp.DR >= 0 && sp.PR >= 0 {
			t.Errorf("%s has no resist reduction", sp.Name)
		}
	}

	// Tashanian lowers MR by 9 (SPA 50 -9).
	if tash, ok := by["Tashanian"]; !ok {
		t.Error("Tashanian missing from resist debuffs")
	} else if tash.MR != -9 {
		t.Errorf("Tashanian MR = %d, want -9", tash.MR)
	}

	// Malo lowers cold and magic by 10 each.
	if malo, ok := by["Malo"]; !ok {
		t.Error("Malo missing from resist debuffs")
	} else if malo.MR != -10 || malo.CR != -10 {
		t.Errorf("Malo MR/CR = %d/%d, want -10/-10", malo.MR, malo.CR)
	}
}

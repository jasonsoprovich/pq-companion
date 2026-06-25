package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestResistDebuffSpells(t *testing.T) {
	d := openTestDB(t)
	list, err := d.ResistDebuffSpells()
	if err != nil {
		t.Fatalf("ResistDebuffSpells: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected some resist debuffs")
	}

	by := map[string][]db.ResistMod{}
	for _, sp := range list {
		by[sp.Name] = sp.Mods
		if len(sp.Mods) == 0 {
			t.Errorf("%s has no resist reduction", sp.Name)
		}
	}

	find := func(name, resist string) (db.ResistMod, bool) {
		for _, m := range by[name] {
			if m.Resist == resist {
				return m, true
			}
		}
		return db.ResistMod{}, false
	}

	// Tashanian: SPA 50 base -9, max 43, formula 101 (level-scaled).
	if m, ok := find("Tashanian", "mr"); !ok {
		t.Error("Tashanian missing MR mod")
	} else if m.Base != -9 || m.Max != 43 || m.Formula != 101 {
		t.Errorf("Tashanian MR mod = %+v, want base -9 max 43 formula 101", m)
	}

	// Malo: SPA 47/50 base -10, max 45, formula 102.
	if m, ok := find("Malo", "mr"); !ok {
		t.Error("Malo missing MR mod")
	} else if m.Base != -10 || m.Max != 45 || m.Formula != 102 {
		t.Errorf("Malo MR mod = %+v, want base -10 max 45 formula 102", m)
	}
	if _, ok := find("Malo", "cr"); !ok {
		t.Error("Malo missing CR mod")
	}
}

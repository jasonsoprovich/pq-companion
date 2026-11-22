package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// The 2026-11-23 patch's PoP mod2 stats reach GetItem (replayed onto
// quarm.db by scripts/quarm-db-patch).
func TestGetItem_Mod2Stats(t *testing.T) {
	d := openTestDB(t)
	cases := []struct {
		id   int
		want db.Mod2Stats
	}{
		// Shroud of Eternity: Shielding 4, Damage Shield 5.
		{15805, db.Mod2Stats{Shielding: 4, DamageShield: 5}},
		// Wristband of Icy Vengeance: Avoidance 15, Accuracy 15.
		{32110, db.Mod2Stats{Avoidance: 15, Accuracy: 15}},
		// Padded Tigerskin Gloves: Strikethrough 10.
		{27000, db.Mod2Stats{Strikethrough: 10}},
	}
	for _, c := range cases {
		it, err := d.GetItem(c.id)
		if err != nil {
			t.Fatalf("GetItem(%d): %v", c.id, err)
		}
		if it.Mod2Stats != c.want {
			t.Errorf("item %d mod2 = %+v, want %+v", c.id, it.Mod2Stats, c.want)
		}
	}
}

func TestMod2Capped(t *testing.T) {
	var sum db.Mod2Stats
	sum.Add(db.Mod2Stats{Avoidance: 70, Strikethrough: 20, Accuracy: 10})
	sum.Add(db.Mod2Stats{Avoidance: 45, Strikethrough: 20, DamageShield: 5})
	got := sum.Capped()
	want := db.Mod2Stats{Avoidance: 100, Strikethrough: 35, Accuracy: 10, DamageShield: 5}
	if got != want {
		t.Errorf("Capped() = %+v, want %+v", got, want)
	}
}

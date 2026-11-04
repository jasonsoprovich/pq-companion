package db_test

import "testing"

func TestZoneDoors(t *testing.T) {
	d := openTestDB(t)
	doors, err := d.ZoneDoors("qeynos2")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, door := range doors {
		if door.Name == "DOOR1" && door.Size == 100 {
			found = true
		}
	}
	if !found {
		t.Fatalf("qeynos2: no DOOR1 among %d doors", len(doors))
	}

	// Plane of Mischief has 30 POMDOOR201 rows: 9 for every era and 21
	// gated to expansions 0-1, out of the current one.
	doors, err = d.ZoneDoors("mischiefplane")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, door := range doors {
		if door.Name == "POMDOOR201" {
			n++
		}
	}
	if n != 9 {
		t.Fatalf("POMDOOR201 doors = %d, want the 9 ungated", n)
	}
}

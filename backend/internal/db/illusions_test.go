package db_test

import "testing"

func TestIllusions(t *testing.T) {
	d := openTestDB(t)
	got, err := d.Illusions()
	if err != nil {
		t.Fatal(err)
	}
	byRace := map[int]string{}
	for _, il := range got {
		byRace[il.Race] = il.Name
		if len(il.Spells) == 0 {
			t.Errorf("race %d has no spells", il.Race)
		}
	}
	for race, want := range map[int]string{60: "Skeleton", 6: "Dark Elf", 143: "Tree"} {
		if byRace[race] != want {
			t.Errorf("race %d = %q, want %q", race, byRace[race], want)
		}
	}
}

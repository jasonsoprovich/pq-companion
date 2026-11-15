package db_test

import "testing"

func TestIllusions(t *testing.T) {
	d := openTestDB(t)
	got, err := d.Illusions()
	if err != nil {
		t.Fatal(err)
	}
	type look struct{ race, texture, gender int }
	byName := map[string]look{}
	for _, il := range got {
		byName[il.Name] = look{il.Race, il.Texture, il.Gender}
		if len(il.Spells) == 0 {
			t.Errorf("%s has no spells", il.Name)
		}
	}
	for name, want := range map[string]look{
		"Skeleton":          {60, 0, 2},
		"Dark Elf":          {6, 0, -1},
		"Tree":              {143, 0, 2},
		"Earth Elemental":   {75, 0, 2},
		"Fire Elemental":    {75, 1, 2},
		"Water Elemental":   {75, 2, 2},
		"Air Elemental":     {75, 3, 2},
		"Scaled Wolf":       {42, 0, 1},
		"Greater Wolf Form": {42, 3, 2},
		"Wolf Form":         {42, 0, 2},
	} {
		if g, ok := byName[name]; !ok || g != want {
			t.Errorf("%s = %+v (found %v), want %+v", name, g, ok, want)
		}
	}
}

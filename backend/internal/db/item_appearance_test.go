package db_test

import "testing"

func TestItemAppearances(t *testing.T) {
	d := openTestDB(t)
	// Cobalt Breastplate: plate, tinted, with the 0xFF alpha byte dropped.
	got, err := d.ItemAppearances([]int{4516, 999999999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if a := got[0]; a.Material != 3 || a.Color != 0x323264 || a.IDFile != "IT63" {
		t.Errorf("Cobalt Breastplate = %+v", a)
	}
	if none, err := d.ItemAppearances(nil); err != nil || none != nil {
		t.Errorf("no ids: %v, %v", none, err)
	}
}

func TestSearchWearables(t *testing.T) {
	d := openTestDB(t)
	cases := []struct {
		name  string
		slots int
		q     string
		want  string
	}{
		{"chest armor", 131072, "cobalt breast", "Cobalt Breastplate"},
		{"held weapon", 8192, "fine steel long", "Fine Steel Long Sword"},
		{"shield", 16384, "kite shield", "Kite Shield"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := d.SearchWearables(c.slots, c.q, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, a := range got {
				if a.Slots&c.slots == 0 {
					t.Errorf("%s doesn't go in slots %d", a.Name, c.slots)
				}
				found = found || a.Name == c.want
			}
			if !found {
				t.Errorf("%q not found in %d results", c.want, len(got))
			}
		})
	}
	// Outside the hands, an item's model doesn't count: most rings have
	// one (IT63), and only the few with a stray material are listed.
	got, err := d.SearchWearables(32768, "ring", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a.Material == 0 {
			t.Errorf("%s listed for its model alone", a.Name)
		}
	}
}

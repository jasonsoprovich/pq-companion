package character

import "testing"

func TestAppearanceRoundTrip(t *testing.T) {
	s, _ := openTestStore(t)
	cases := []struct {
		name string
		set  Appearance
		want Appearance
	}{
		{"female luclin", Appearance{Name: "Bela", Gender: 1, Luclin: true}, Appearance{Name: "Bela", Gender: 1, Luclin: true}},
		{"overwrite", Appearance{Name: "BELA", Gender: 0}, Appearance{Name: "bela", Gender: 0}},
		{"bad gender is male", Appearance{Name: "Odd", Gender: 7}, Appearance{Name: "Odd", Gender: 0}},
		{"bald", Appearance{Name: "Kojak", Luclin: true, Hair: -1}, Appearance{Name: "Kojak", Luclin: true, Hair: -1}},
		{"second style", Appearance{Name: "Kojak", Hair: 2}, Appearance{Name: "Kojak", Hair: 2}},
		{"bad style is the first", Appearance{Name: "Kojak", Hair: 40}, Appearance{Name: "Kojak", Hair: 0}},
		{
			"looks",
			Appearance{Name: "Bela", Face: 5, HairColor: 12, Beard: 3, BeardColor: 7, Eyes: 9},
			Appearance{Name: "Bela", Face: 5, HairColor: 12, Beard: 3, BeardColor: 7, Eyes: 9},
		},
		{
			"bad looks are the defaults",
			Appearance{Name: "Bela", Face: 30, HairColor: 24, Beard: -4, BeardColor: 99, Eyes: 11},
			Appearance{Name: "Bela", HairColor: -1, Beard: -1, BeardColor: -1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := s.SetAppearance(c.set); err != nil {
				t.Fatal(err)
			}
			got, ok, err := s.GetAppearance(c.want.Name)
			if err != nil || !ok {
				t.Fatalf("get: ok=%v err=%v", ok, err)
			}
			got.Name = c.want.Name
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
	if a, ok, err := s.GetAppearance("Nobody"); ok || err != nil || a != DefaultAppearance("Nobody") {
		t.Errorf("unsaved: %+v ok=%v err=%v", a, ok, err)
	}
	if err := s.SetAppearance(Appearance{}); err == nil {
		t.Error("saved an appearance with no name")
	}
}

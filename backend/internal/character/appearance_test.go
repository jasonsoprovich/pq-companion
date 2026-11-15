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
			if got.Gender != c.want.Gender || got.Luclin != c.want.Luclin {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
	if _, ok, err := s.GetAppearance("Nobody"); ok || err != nil {
		t.Errorf("unsaved: ok=%v err=%v", ok, err)
	}
	if err := s.SetAppearance(Appearance{}); err == nil {
		t.Error("saved an appearance with no name")
	}
}

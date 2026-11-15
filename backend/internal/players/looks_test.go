package players

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLooksRoundTrip(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, ok, err := s.GetLook("Bela"); ok || err != nil {
		t.Fatalf("unseen: ok=%v err=%v", ok, err)
	}
	l := Look{Race: 6, Gender: 1, Size: 5, Face: 3, HairStyle: 1, HairColor: 12, BeardStyle: 255,
		Mats: [7]int{0, 3, 3, 3, 3, 3, 3}, Tints: [7]uint32{0, 0x323264}, Primary: "IT38"}
	now := time.Now()
	if err := s.SetLook("Bela", l, now); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetLook("bela")
	if err != nil || !ok || got != l {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	// Unchanged: no write (seen_at stays); changed: written.
	if err := s.SetLook("Bela", l, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var seen int64
	s.db.QueryRow(`SELECT seen_at FROM player_looks WHERE name = 'Bela'`).Scan(&seen)
	if seen != now.Unix() {
		t.Errorf("unchanged look rewrote seen_at")
	}
	l.Mats[1] = 2
	if err := s.SetLook("Bela", l, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.GetLook("Bela"); got.Mats[1] != 2 {
		t.Errorf("changed look not written: %+v", got)
	}
	// No race: ignored.
	if err := s.SetLook("Nobody", Look{}, now); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetLook("Nobody"); ok {
		t.Error("stored a look with no race")
	}
}

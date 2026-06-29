package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// Known quarm.db item ids used below (percussion instruments). If a future data
// dump renumbers these, update them — the bardvalue is what matters.
const (
	itemDrumsOfTheBeast  = 24737 // bardtype 26 (percussion), bardvalue 26
	itemWarDrumsOfRathe  = 27998 // bardtype 26 (percussion), bardvalue 34
	occlusionOfSoundID   = 1451  // wind song, lowers MR/CR/FR
	tuyensChantOfFrostID = 744   // wind song, lowers CR
)

// TestBardInstrumentMods_AAOnly verifies the Instrument/Singing Mastery AA
// resolution (cumulative per rank: +2/+4/+6) with no instrument equipped.
func TestBardInstrumentMods_AAOnly(t *testing.T) {
	d := openTestDB(t)

	// Instrument Mastery 3 → +6 to the four instrument types; singing untouched.
	got, err := d.BardInstrumentMods(nil, []db.TrainedAA{{AAID: 90, Rank: 3}})
	if err != nil {
		t.Fatalf("BardInstrumentMods: %v", err)
	}
	for _, c := range []struct {
		name string
		got  int
	}{
		{"wind", got.Wind}, {"stringed", got.Stringed},
		{"brass", got.Brass}, {"percussion", got.Percussion},
	} {
		if c.got != 16 { // base 10 + 6
			t.Errorf("IM3 %s = %d, want 16", c.name, c.got)
		}
	}
	if got.Singing != 10 {
		t.Errorf("IM3 singing = %d, want 10 (untouched)", got.Singing)
	}

	// Add Singing Mastery rank 2 → +4 to singing only.
	got, err = d.BardInstrumentMods(nil, []db.TrainedAA{{AAID: 90, Rank: 3}, {AAID: 118, Rank: 2}})
	if err != nil {
		t.Fatalf("BardInstrumentMods: %v", err)
	}
	if got.Singing != 14 {
		t.Errorf("SM2 singing = %d, want 14", got.Singing)
	}
}

// TestBardInstrumentMods_ItemPlusAA reproduces the reported scenario: a
// percussion drum (value 26) + Instrument Mastery 3 → percussion effectmod 32
// (3.2x). Non-matching types only get the AA bonus.
func TestBardInstrumentMods_ItemPlusAA(t *testing.T) {
	d := openTestDB(t)
	got, err := d.BardInstrumentMods([]int{itemDrumsOfTheBeast}, []db.TrainedAA{{AAID: 90, Rank: 3}})
	if err != nil {
		t.Fatalf("BardInstrumentMods: %v", err)
	}
	if got.Percussion != 32 { // max(10, 26) + 6
		t.Errorf("drums + IM3 percussion = %d, want 32", got.Percussion)
	}
	if got.Wind != 16 { // AA only (drum is percussion)
		t.Errorf("drums + IM3 wind = %d, want 16 (AA only)", got.Wind)
	}
}

// TestBardInstrumentMods_SoftCap verifies the effectmod is clamped to 36.
func TestBardInstrumentMods_SoftCap(t *testing.T) {
	d := openTestDB(t)
	got, err := d.BardInstrumentMods([]int{itemWarDrumsOfRathe}, []db.TrainedAA{{AAID: 90, Rank: 3}})
	if err != nil {
		t.Fatalf("BardInstrumentMods: %v", err)
	}
	if got.Percussion != 36 { // max(10, 34) + 6 = 40, capped to 36
		t.Errorf("war drums + IM3 percussion = %d, want 36 (soft cap)", got.Percussion)
	}
}

// TestBardInstrumentMods_NonBardAAsIgnored ensures only the mastery AAs feed in.
func TestBardInstrumentMods_NonBardAAsIgnored(t *testing.T) {
	d := openTestDB(t)
	got, err := d.BardInstrumentMods(nil, []db.TrainedAA{{AAID: 1, Rank: 5}})
	if err != nil {
		t.Fatalf("BardInstrumentMods: %v", err)
	}
	if got.Wind != 10 || got.Singing != 10 {
		t.Errorf("unrelated AA leaked into mods: %+v", got)
	}
}

// TestResistDebuffsCarryBardSkill checks that bard songs are tagged with their
// instrument skill while non-bard debuffs are not.
func TestResistDebuffsCarryBardSkill(t *testing.T) {
	d := openTestDB(t)
	debuffs, err := d.ResistDebuffSpells()
	if err != nil {
		t.Fatalf("ResistDebuffSpells: %v", err)
	}
	byID := map[int]db.ResistDebuff{}
	for _, rd := range debuffs {
		byID[rd.ID] = rd
	}

	const skillWind = 70
	if rd := byID[occlusionOfSoundID]; rd.BardSkill != skillWind {
		t.Errorf("Occlusion of Sound bard_skill = %d, want %d", rd.BardSkill, skillWind)
	}
	if rd := byID[tuyensChantOfFrostID]; rd.BardSkill != skillWind {
		t.Errorf("Tuyen's Chant of Frost bard_skill = %d, want %d", rd.BardSkill, skillWind)
	}

	// A non-bard debuff line (Tashani/Malo, etc.) must not be tagged.
	for _, dd := range debuffs {
		if dd.BardSkill != 0 {
			continue
		}
		// at least one untagged debuff exists; that's enough
		return
	}
	t.Error("expected at least one non-bard resist debuff with bard_skill 0")
}

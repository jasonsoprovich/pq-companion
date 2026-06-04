package skills

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSkillID(t *testing.T) {
	cases := []struct {
		name   string
		wantID int
		wantOK bool
	}{
		{"Defense", 15, true},
		{"Offense", 33, true}, // not 22 — 22 is Dual Wield in the EQMac enum
		{"Dual Wield", 22, true},
		{"Hand to Hand", 28, true},
		{"1H Blunt", 0, true},
		{"swimming", 50, true}, // case-insensitive
		{"Piercing", 36, true}, // alias for 1H Piercing
		{"Frobnicate", Unknown, false},
	}
	for _, c := range cases {
		id, ok := SkillID(c.name)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("SkillID(%q) = (%d,%v), want (%d,%v)", c.name, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestStore_UpsertOnlyIncreases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	ts := time.Unix(1_700_000_000, 0)

	// First observation inserts.
	if changed, err := s.Upsert("Osui", "Swimming", 50, 100, ts); err != nil || !changed {
		t.Fatalf("insert: changed=%v err=%v, want true/nil", changed, err)
	}
	// Higher value updates.
	if changed, err := s.Upsert("Osui", "Swimming", 50, 150, ts); err != nil || !changed {
		t.Fatalf("increase: changed=%v err=%v, want true/nil", changed, err)
	}
	// Equal or lower is a no-op (idempotent for backfill replay).
	if changed, err := s.Upsert("Osui", "Swimming", 50, 150, ts); err != nil || changed {
		t.Fatalf("equal: changed=%v err=%v, want false/nil", changed, err)
	}
	if changed, err := s.Upsert("Osui", "Swimming", 50, 120, ts); err != nil || changed {
		t.Fatalf("lower: changed=%v err=%v, want false/nil", changed, err)
	}

	// Case-insensitive read returns the latest value.
	recs, err := s.GetByCharacter("osui")
	if err != nil {
		t.Fatalf("GetByCharacter: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].Value != 150 || recs[0].SkillID != 50 || recs[0].SkillName != "Swimming" {
		t.Errorf("record = %+v, want value=150 skill_id=50 name=Swimming", recs[0])
	}
}

func TestStore_UnmappedSkillStillTracked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	// A skill name we don't recognise is recorded with Unknown id (no cap),
	// not dropped.
	if changed, err := s.Upsert("Osui", "Mystery Skill", Unknown, 42, time.Unix(1, 0)); err != nil || !changed {
		t.Fatalf("insert unmapped: changed=%v err=%v", changed, err)
	}
	recs, _ := s.GetByCharacter("Osui")
	if len(recs) != 1 || recs[0].SkillID != Unknown || recs[0].Value != 42 {
		t.Errorf("unmapped record = %+v, want id=-1 value=42", recs)
	}
}

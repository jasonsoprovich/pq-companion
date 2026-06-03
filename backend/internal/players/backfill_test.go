package players

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBackfillUpsert(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	t0 := time.Unix(1_700_000_000, 0)
	in := func(name string, level int, when time.Time) SightingInput {
		return SightingInput{Name: name, Level: level, Class: "Warrior", Race: "Human", Zone: "Qeynos", ObservedAt: when}
	}

	// First backfill creates the row.
	if ch, err := s.BackfillUpsert(in("Soandso", 30, t0)); err != nil || !ch {
		t.Fatalf("first upsert changed=%v err=%v, want true/nil", ch, err)
	}
	// Re-running the exact same sighting changes nothing (idempotent).
	if ch, _ := s.BackfillUpsert(in("Soandso", 30, t0)); ch {
		t.Error("identical re-backfill reported a change; want idempotent no-op")
	}

	got, _ := s.Get("Soandso")
	if got == nil || got.SightingsCount != 1 {
		t.Fatalf("count=%v, want 1 (backfill must not inflate count)", got)
	}

	// An OLDER sighting must pull first_seen_at earlier without touching
	// last-seen data.
	older := t0.Add(-48 * time.Hour)
	if ch, _ := s.BackfillUpsert(in("Soandso", 28, older)); !ch {
		t.Error("older sighting should extend first_seen_at")
	}
	got, _ = s.Get("Soandso")
	if got.FirstSeenAt != older.Unix() {
		t.Errorf("first_seen_at=%d, want %d (pulled earlier)", got.FirstSeenAt, older.Unix())
	}
	if got.LastSeenLevel != 30 || got.LastSeenAt != t0.Unix() {
		t.Errorf("older sighting clobbered last-seen data: level=%d seen=%d", got.LastSeenLevel, got.LastSeenAt)
	}
	if got.SightingsCount != 1 {
		t.Errorf("count=%d after older sighting, want 1 (no inflation)", got.SightingsCount)
	}

	// A NEWER sighting refreshes last-seen fields.
	newer := t0.Add(72 * time.Hour)
	if ch, _ := s.BackfillUpsert(in("Soandso", 35, newer)); !ch {
		t.Error("newer sighting should refresh last-seen")
	}
	got, _ = s.Get("Soandso")
	if got.LastSeenLevel != 35 || got.LastSeenAt != newer.Unix() {
		t.Errorf("newer sighting not applied: level=%d seen=%d", got.LastSeenLevel, got.LastSeenAt)
	}
}

package progress

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_AppendEvent_DedupesOnReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	ts := time.Unix(1_700_000_000, 0)
	ev := Event{Character: "Osui", At: ts, Kind: KindLevel, Value: 55}

	inserted, err := s.AppendEvent(ev)
	if err != nil || !inserted {
		t.Fatalf("first insert: inserted=%v err=%v, want true/nil", inserted, err)
	}
	// Replaying the exact same event (e.g. a backfill re-run) must be a
	// no-op, not a duplicate row.
	inserted, err = s.AppendEvent(ev)
	if err != nil || inserted {
		t.Fatalf("replay: inserted=%v err=%v, want false/nil", inserted, err)
	}

	got, err := s.EventsSince("Osui", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("EventsSince returned %d rows, want 1", len(got))
	}

	// A second, genuinely distinct level-up at the same character must
	// still be recorded (different value).
	ev2 := Event{Character: "Osui", At: ts.Add(time.Hour), Kind: KindLevel, Value: 56}
	if inserted, err := s.AppendEvent(ev2); err != nil || !inserted {
		t.Fatalf("distinct event insert: inserted=%v err=%v, want true/nil", inserted, err)
	}
	got, err = s.EventsSince("Osui", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("EventsSince returned %d rows, want 2", len(got))
	}
}

func TestStore_EventsSince_FiltersByCharacterAndWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	base := time.Unix(1_700_000_000, 0)
	events := []Event{
		{Character: "Osui", At: base, Kind: KindLevel, Value: 50},
		{Character: "Osui", At: base.Add(48 * time.Hour), Kind: KindLevel, Value: 51},
		{Character: "Nariana", At: base.Add(time.Hour), Kind: KindAA, Value: 3},
	}
	for _, ev := range events {
		if _, err := s.AppendEvent(ev); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}

	// Window starting after the first Osui event should only surface the
	// second one.
	got, err := s.EventsSince("Osui", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 1 || got[0].Value != 51 {
		t.Fatalf("EventsSince = %+v, want single event with Value=51", got)
	}

	all, err := s.AllEventsSince(base)
	if err != nil {
		t.Fatalf("AllEventsSince: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("AllEventsSince returned %d rows, want 3", len(all))
	}
}

func TestStore_Snapshots_FingerprintSkipAndBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	t0 := time.Unix(1_700_000_000, 0)
	first := Snapshot{Character: "Osui", TakenAt: t0, Level: 55, AARanks: 100, TradeskillTotal: 900, SpellsKnown: 60, Copper: 50_000}
	if _, err := s.AppendSnapshot(first); err != nil {
		t.Fatalf("AppendSnapshot: %v", err)
	}

	// An identical snapshot's fingerprint matches the stored one — the
	// Recorder is expected to check this before calling AppendSnapshot
	// again, so verify the fingerprints actually agree.
	dup := first
	dup.TakenAt = t0.Add(time.Hour)
	latest, ok, err := s.LatestSnapshot("Osui")
	if err != nil || !ok {
		t.Fatalf("LatestSnapshot: ok=%v err=%v", ok, err)
	}
	if latest.Fingerprint() != dup.Fingerprint() {
		t.Fatalf("fingerprints differ for identical totals: %q vs %q", latest.Fingerprint(), dup.Fingerprint())
	}

	// A later, genuinely different snapshot should have a different
	// fingerprint and become the new latest / baseline lookup target.
	second := Snapshot{Character: "Osui", TakenAt: t0.Add(24 * time.Hour), Level: 56, AARanks: 103, TradeskillTotal: 900, SpellsKnown: 61, Copper: 40_000}
	if second.Fingerprint() == first.Fingerprint() {
		t.Fatalf("expected different fingerprints for different totals")
	}
	if _, err := s.AppendSnapshot(second); err != nil {
		t.Fatalf("AppendSnapshot: %v", err)
	}

	latest, ok, err = s.LatestSnapshot("Osui")
	if err != nil || !ok || latest.Level != 56 {
		t.Fatalf("LatestSnapshot after second = %+v, ok=%v err=%v, want Level=56", latest, ok, err)
	}

	// The baseline lookup for a window starting between the two snapshots
	// should return the first (older) one.
	baseline, ok, err := s.SnapshotAtOrBefore("Osui", t0.Add(12*time.Hour))
	if err != nil || !ok || baseline.Level != 55 {
		t.Fatalf("SnapshotAtOrBefore = %+v, ok=%v err=%v, want Level=55", baseline, ok, err)
	}
}

func TestStore_ActiveDays_DedupesAndFiltersByWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	// Anchored at local noon (not a raw Unix epoch) so the +2h/+48h offsets
	// below can't cross a calendar-day boundary differently depending on the
	// machine's timezone (e.g. CI runs UTC, dev machines don't).
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.Local)
	if err := s.MarkActiveDay("Osui", base); err != nil {
		t.Fatalf("MarkActiveDay: %v", err)
	}
	// A second mark the same calendar day (different hour) must not create a
	// second row.
	if err := s.MarkActiveDay("Osui", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("MarkActiveDay: %v", err)
	}
	if err := s.MarkActiveDay("Osui", base.Add(48*time.Hour)); err != nil {
		t.Fatalf("MarkActiveDay: %v", err)
	}
	// A different character must be tracked independently.
	if err := s.MarkActiveDay("Nariana", base); err != nil {
		t.Fatalf("MarkActiveDay: %v", err)
	}

	all, err := s.ActiveDaysSince("Osui", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ActiveDaysSince: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ActiveDaysSince returned %d days, want 2 (dedup within a day)", len(all))
	}

	windowed, err := s.ActiveDaysSince("Osui", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ActiveDaysSince: %v", err)
	}
	if len(windowed) != 1 {
		t.Fatalf("ActiveDaysSince (windowed) returned %d days, want 1", len(windowed))
	}
}

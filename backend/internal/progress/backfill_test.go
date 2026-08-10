package progress

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/backfill"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Compile-time check that BackfillHandler satisfies backfill.Handler.
var _ backfill.Handler = (*BackfillHandler)(nil)

func TestBackfillHandler_ReplayIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	events := []logparser.LogEvent{
		{Type: logparser.EventLevelChange, Timestamp: time.Unix(1_700_000_000, 0), Data: logparser.LevelChangeData{Level: 55, Delta: 1}},
		{Type: logparser.EventAAGain, Timestamp: time.Unix(1_700_000_100, 0), Data: logparser.AAGainData{Points: 12}},
		{Type: logparser.EventSpellScribed, Timestamp: time.Unix(1_700_000_200, 0), Data: logparser.SpellScribedData{SpellName: "Mesmerization"}},
		{Type: logparser.EventSkillUp, Timestamp: time.Unix(1_700_000_300, 0), Data: logparser.SkillUpData{SkillName: "Baking", Rank: 10}},
		// Not a progression event type — must be ignored, not error.
		{Type: logparser.EventZone, Timestamp: time.Unix(1_700_000_400, 0), Data: logparser.ZoneData{ZoneName: "The North Karana"}},
	}

	run := func() *BackfillHandler {
		h := NewBackfillHandler(s, "Osui")
		for _, ev := range events {
			h.HandleEvent(ev)
		}
		h.Finalize()
		return h
	}

	first := run()
	if first.Inserted() != 4 {
		t.Fatalf("first run Inserted() = %d, want 4", first.Inserted())
	}

	// Re-running the backfill (e.g. the user re-runs Settings → Log Backfill
	// after a new export) must not duplicate rows.
	second := run()
	if second.Inserted() != 0 {
		t.Fatalf("second run Inserted() = %d, want 0", second.Inserted())
	}

	got, err := s.AllEventsSince(time.Unix(0, 0))
	if err != nil {
		t.Fatalf("AllEventsSince: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("stored %d rows after two runs, want 4", len(got))
	}
}

func TestBackfillHandler_HandleLine_MarksActiveDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	h := NewBackfillHandler(s, "Osui")
	// Anchored at local noon so the +2h/+48h offsets below can't cross a
	// calendar-day boundary differently depending on the runner's timezone.
	base := time.Date(2023, 11, 14, 12, 0, 0, 0, time.Local)
	h.HandleLine(base, "You have entered The North Karana.")
	h.HandleLine(base.Add(2*time.Hour), "You hit a gnoll for 10 points of damage.")
	h.HandleLine(base.Add(48*time.Hour), "You have entered The North Karana.")

	days, err := s.ActiveDaysSince("Osui", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("ActiveDaysSince: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("ActiveDaysSince returned %d days, want 2", len(days))
	}
}

func TestBackfillHandler_EmptyCharacterIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	h := NewBackfillHandler(s, "")
	h.HandleEvent(logparser.LogEvent{Type: logparser.EventLevelChange, Data: logparser.LevelChangeData{Level: 55, Delta: 1}})
	if h.Inserted() != 0 {
		t.Fatalf("Inserted() = %d, want 0", h.Inserted())
	}
}

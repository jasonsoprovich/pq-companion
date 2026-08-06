package progress

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

func TestConsumer_Handle_RecordsTrackedKinds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	c := NewConsumer(s, func() string { return "Osui" })

	var updates []Event
	c.SetOnUpdate(func(ev Event) { updates = append(updates, ev) })

	ts := time.Unix(1_700_000_000, 0)
	c.Handle(logparser.LogEvent{Type: logparser.EventLevelChange, Timestamp: ts, Data: logparser.LevelChangeData{Level: 55, Delta: 1}})
	c.Handle(logparser.LogEvent{Type: logparser.EventAAGain, Timestamp: ts, Data: logparser.AAGainData{Points: 12}})
	c.Handle(logparser.LogEvent{Type: logparser.EventSpellScribed, Timestamp: ts, Data: logparser.SpellScribedData{SpellName: "Mesmerization"}})
	c.Handle(logparser.LogEvent{Type: logparser.EventSkillUp, Timestamp: ts, Data: logparser.SkillUpData{SkillName: "Baking", Rank: 10}})
	// Unrelated event types must be ignored.
	c.Handle(logparser.LogEvent{Type: logparser.EventZone, Timestamp: ts, Data: logparser.ZoneData{ZoneName: "The North Karana"}})

	got, err := s.EventsSince("Osui", time.Unix(0, 0))
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("EventsSince returned %d rows, want 4", len(got))
	}
	if len(updates) != 4 {
		t.Fatalf("onUpdate fired %d times, want 4", len(updates))
	}
}

func TestConsumer_Handle_NoActiveCharacterIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	c := NewConsumer(s, func() string { return "" })
	c.Handle(logparser.LogEvent{Type: logparser.EventLevelChange, Data: logparser.LevelChangeData{Level: 55, Delta: 1}})

	all, err := s.AllEventsSince(time.Unix(0, 0))
	if err != nil {
		t.Fatalf("AllEventsSince: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("AllEventsSince returned %d rows, want 0", len(all))
	}
}

package players

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// TestConsumer_WhoSummaryFlushesPendingWithZone verifies the core /who-block
// flow: entries arrive without zone context, the trailing summary line
// supplies the zone, and the pending entries flush into the store with that
// zone stamped on each.
func TestConsumer_WhoSummaryFlushesPendingWithZone(t *testing.T) {
	s := openTest(t)
	c := NewConsumer(s)
	ts := time.Unix(1_700_000_000, 0)

	c.Handle(logparser.LogEvent{
		Type:      logparser.EventWhoEntry,
		Timestamp: ts,
		Data:      logparser.WhoEntryData{Name: "Alpha", Level: 50, Class: "Warrior"},
	})
	c.Handle(logparser.LogEvent{
		Type:      logparser.EventWhoEntry,
		Timestamp: ts,
		Data:      logparser.WhoEntryData{Name: "Beta", Anonymous: true},
	})

	// Nothing persisted before the summary.
	if got, _ := s.Get("Alpha"); got != nil {
		t.Error("pending entry persisted before summary")
	}

	c.Handle(logparser.LogEvent{
		Type:      logparser.EventWhoSummary,
		Timestamp: ts,
		Data:      logparser.WhoSummaryData{Zone: "The Nexus"},
	})

	a, _ := s.Get("Alpha")
	b, _ := s.Get("Beta")
	if a == nil || a.LastSeenZone != "The Nexus" {
		t.Errorf("Alpha zone = %+v, want The Nexus", a)
	}
	if b == nil || b.LastSeenZone != "The Nexus" {
		t.Errorf("Beta zone = %+v, want The Nexus", b)
	}
}

// TestConsumer_ZoneChangeFlushesPending verifies the safety valve: if a
// player zones mid-buffer (no summary arrived), the buffered entries are
// committed under the OLD zone before the new one becomes current.
func TestConsumer_ZoneChangeFlushesPending(t *testing.T) {
	s := openTest(t)
	c := NewConsumer(s)

	c.Handle(logparser.LogEvent{Type: logparser.EventZone, Data: logparser.ZoneData{ZoneName: "Plane of Knowledge"}})
	c.Handle(logparser.LogEvent{
		Type:      logparser.EventWhoEntry,
		Timestamp: time.Unix(1_700_000_000, 0),
		Data:      logparser.WhoEntryData{Name: "Charlie", Level: 60, Class: "Druid"},
	})
	// Zone change before any summary.
	c.Handle(logparser.LogEvent{Type: logparser.EventZone, Data: logparser.ZoneData{ZoneName: "The Nexus"}})

	got, _ := s.Get("Charlie")
	if got == nil {
		t.Fatal("entry should have been flushed on zone change")
	}
	if got.LastSeenZone != "Plane of Knowledge" {
		t.Errorf("zone = %q, want Plane of Knowledge", got.LastSeenZone)
	}
}

// TestConsumer_GuildStat updates only the guild field, leaving class/level
// intact from a prior /who sighting.
func TestConsumer_GuildStatPreservesOtherFields(t *testing.T) {
	s := openTest(t)
	c := NewConsumer(s)

	// Seed with a full /who.
	c.Handle(logparser.LogEvent{
		Type:      logparser.EventWhoEntry,
		Timestamp: time.Unix(1_700_000_000, 0),
		Data:      logparser.WhoEntryData{Name: "Osui", Level: 60, Class: "Enchanter", Race: "Halfling"},
	})
	c.Handle(logparser.LogEvent{
		Type: logparser.EventWhoSummary,
		Data: logparser.WhoSummaryData{Zone: "Plane of Knowledge"},
	})

	// /guildstat reply.
	c.Handle(logparser.LogEvent{
		Type:      logparser.EventGuildStat,
		Timestamp: time.Unix(1_700_000_100, 0),
		Data:      logparser.GuildStatData{Player: "Osui", Guild: "Seekers of Souls"},
	})

	got, _ := s.Get("Osui")
	if got == nil {
		t.Fatal("expected Osui row")
	}
	if got.Guild != "Seekers of Souls" {
		t.Errorf("guild = %q, want Seekers of Souls", got.Guild)
	}
	if got.LastSeenLevel != 60 || got.Class != "Enchanter" {
		t.Errorf("/guildstat clobbered prior fields: level=%d class=%q", got.LastSeenLevel, got.Class)
	}
}

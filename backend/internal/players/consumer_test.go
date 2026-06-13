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

// TestConsumer_PVPSightingFiresAlertWithCooldown verifies a /who entry for a
// PVP-flagged player fires the alert callback once, case-insensitively, and
// that the cooldown suppresses an immediate repeat.
func TestConsumer_PVPSightingFiresAlertWithCooldown(t *testing.T) {
	s := openTest(t)
	if err := s.UpsertNote("Ganker", "kos", true); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	c := NewConsumer(s)
	type alert struct{ name, zone, source string }
	var alerts []alert
	c.SetOnPVPSighting(func(name, zone, source string) {
		alerts = append(alerts, alert{name, zone, source})
	})
	ts := time.Unix(1_700_000_000, 0)

	whoBlock := func() {
		c.Handle(logparser.LogEvent{
			Type:      logparser.EventWhoEntry,
			Timestamp: ts,
			Data:      logparser.WhoEntryData{Name: "GANKER", Level: 60, Class: "Rogue"},
		})
		c.Handle(logparser.LogEvent{
			Type:      logparser.EventWhoEntry,
			Timestamp: ts,
			Data:      logparser.WhoEntryData{Name: "Friendly", Level: 10, Class: "Cleric"},
		})
		c.Handle(logparser.LogEvent{
			Type:      logparser.EventWhoSummary,
			Timestamp: ts,
			Data:      logparser.WhoSummaryData{Zone: "East Commonlands"},
		})
	}

	whoBlock()
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1 (%+v)", len(alerts), alerts)
	}
	if alerts[0].name != "GANKER" || alerts[0].zone != "East Commonlands" || alerts[0].source != "who" {
		t.Errorf("alert = %+v", alerts[0])
	}

	// A second /who inside the cooldown stays quiet.
	whoBlock()
	if len(alerts) != 1 {
		t.Errorf("cooldown failed: alerts = %d, want 1", len(alerts))
	}

	// Expiring the cooldown re-arms the warning.
	c.mu.Lock()
	c.lastPVPAlert["ganker"] = time.Now().Add(-2 * pvpAlertCooldown)
	c.mu.Unlock()
	whoBlock()
	if len(alerts) != 2 {
		t.Errorf("re-arm failed: alerts = %d, want 2", len(alerts))
	}
}

// TestConsumer_GroupJoinRecordsInteractionAndPVPAlert verifies the raw-line
// path: "X has joined the group." records a group interaction, "You have
// joined the group." is ignored, and a flagged groupmate fires the PVP alert
// with source "group".
func TestConsumer_GroupJoinRecordsInteractionAndPVPAlert(t *testing.T) {
	s := openTest(t)
	if err := s.UpsertNote("Ganker", "", true); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	c := NewConsumer(s)
	var alerts []string
	c.SetOnPVPSighting(func(name, zone, source string) {
		alerts = append(alerts, name+"|"+source)
	})
	ts := time.Unix(1_700_000_000, 0)

	c.HandleLine(ts, "You have joined the group.")
	c.HandleLine(ts, "Tiliki has joined the group.")
	c.HandleLine(ts, "Ganker has joined the group.")
	c.HandleLine(ts, "Tiliki has left the group.")

	if got, _ := s.Get("You"); got != nil {
		t.Error("'You have joined' should not create a row")
	}
	tiliki, _ := s.Get("Tiliki")
	if tiliki == nil || tiliki.GroupCount != 1 {
		t.Errorf("Tiliki group interaction missing: %+v", tiliki)
	}
	if len(alerts) != 1 || alerts[0] != "Ganker|group" {
		t.Errorf("alerts = %v, want [Ganker|group]", alerts)
	}
}

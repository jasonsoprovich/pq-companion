package overlay

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestTracker returns an NPCTracker with a real (unstarted) hub and no DB.
// The nil-DB guard in lookupNPC means NPC data always comes back nil, which
// is fine for behavioural tests that only care about target/zone state.
func newTestTracker() *NPCTracker {
	hub := ws.NewHub()
	return NewNPCTracker(hub, nil)
}

func TestNPCTracker_ZoneEventClearsTarget(t *testing.T) {
	tr := newTestTracker()

	// Simulate a combat hit so the tracker has an active target.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "You", Skill: "slash", Target: "a gnoll", Damage: 50},
	})
	if !tr.GetState().HasTarget {
		t.Fatal("expected HasTarget=true after combat hit")
	}

	// A zone-change must clear the target and record the zone name.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The North Karana"},
	})

	st := tr.GetState()
	if st.HasTarget {
		t.Errorf("HasTarget = true after zone change, want false")
	}
	if st.CurrentZone != "The North Karana" {
		t.Errorf("CurrentZone = %q, want %q", st.CurrentZone, "The North Karana")
	}
}

func TestNPCTracker_ZoneNameNotSetAsTarget(t *testing.T) {
	tr := newTestTracker()

	// Enter a zone.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The North Karana"},
	})

	// Attempt to set the target to the zone name (simulates a false-positive
	// from the log parser where a zone-entry line is misidentified as a
	// consider event).  The tracker must silently reject this.
	tr.setTarget("The North Karana")

	st := tr.GetState()
	if st.HasTarget {
		t.Errorf("HasTarget = true after setting target to zone name, want false")
	}
}

func TestNPCTracker_ConsiderEventSetsTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "Crushbone"},
	})

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "an orc centurion"},
	})

	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false after consider event, want true")
	}
	if st.TargetName != "an orc centurion" {
		t.Errorf("TargetName = %q, want %q", st.TargetName, "an orc centurion")
	}
}

func TestNPCTracker_KillClearsMatchingTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "You", Target: "a gnoll"},
	})

	if tr.GetState().HasTarget {
		t.Error("HasTarget = true after killing target, want false")
	}
}

func TestNPCTracker_KillDoesNotClearUnrelatedTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	// A group member kills a different mob — our target should remain.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Guildmate", Target: "a kobold"},
	})

	st := tr.GetState()
	if !st.HasTarget {
		t.Error("HasTarget = false after unrelated kill, want true")
	}
	if st.TargetName != "a gnoll" {
		t.Errorf("TargetName = %q, want %q", st.TargetName, "a gnoll")
	}
}

func TestNPCTracker_DeathClearsTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventDeath,
		Data: logparser.DeathData{SlainBy: "a gnoll"},
	})

	if tr.GetState().HasTarget {
		t.Error("HasTarget = true after player death, want false")
	}
}

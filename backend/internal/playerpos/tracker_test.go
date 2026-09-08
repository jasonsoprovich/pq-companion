package playerpos

import (
	"testing"
	"time"
)

// newTestTracker returns a tracker with a controllable clock and a capture of
// every broadcast.
func newTestTracker() (*Tracker, *[]State, *time.Time) {
	var sent []State
	clock := time.Unix(1_700_000_000, 0)
	t := New(func(s State) { sent = append(sent, s) })
	t.now = func() time.Time { return clock }
	return t, &sent, &clock
}

func TestUpdateNegatesToMapSpace(t *testing.T) {
	tr, sent, _ := newTestTracker()
	tr.Update("akheva", 100, -250, 42, 128)

	if len(*sent) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(*sent))
	}
	got := (*sent)[0]
	// map_f1 = -game_x, map_f2 = -game_y, Z and heading pass through.
	if got.X != -100 || got.Y != 250 || got.Z != 42 || got.Heading != 128 {
		t.Errorf("got %+v, want X=-100 Y=250 Z=42 Heading=128", got)
	}
}

func TestUnknownZoneNeverBroadcasts(t *testing.T) {
	tr, sent, clock := newTestTracker()
	tr.Update("", 10, 10, 0, 0)
	*clock = clock.Add(time.Hour)
	tr.Update("", 500, 500, 0, 0)

	if len(*sent) != 0 {
		t.Fatalf("broadcasts = %d, want 0 — an unplaceable position must not draw", len(*sent))
	}
	if _, ok := tr.Snapshot(); ok {
		t.Error("Snapshot ok = true, want false for an unknown zone")
	}
}

func TestRateLimitAndMovementThreshold(t *testing.T) {
	tr, sent, clock := newTestTracker()
	tr.Update("akheva", 0, 0, 0, 0) // first is always sent
	if len(*sent) != 1 {
		t.Fatalf("initial broadcasts = %d, want 1", len(*sent))
	}

	// Inside the rate limit: dropped however far the player moved.
	*clock = clock.Add(50 * time.Millisecond)
	tr.Update("akheva", 900, 900, 0, 0)
	if len(*sent) != 1 {
		t.Errorf("broadcasts = %d after 50ms, want 1 (rate limited)", len(*sent))
	}

	// Past the rate limit but barely moved from the last *sent* position, which
	// is what the threshold compares against — deliberately, so that slow drift
	// still accumulates into a broadcast instead of never qualifying.
	//
	// Sized from the constant rather than written as a literal: a hard-coded
	// value silently stops testing what it claims the moment the threshold is
	// retuned, which is exactly what happened when it was lowered.
	*clock = clock.Add(minInterval)
	sub := moveEpsilon / 4
	tr.Update("akheva", sub, sub, 0, 0)
	if len(*sent) != 1 {
		t.Errorf("broadcasts = %d after a sub-epsilon move, want 1", len(*sent))
	}

	// Past the rate limit and genuinely moved: sent.
	*clock = clock.Add(minInterval)
	tr.Update("akheva", 50, 0, 0, 0)
	if len(*sent) != 2 {
		t.Errorf("broadcasts = %d after a real move, want 2", len(*sent))
	}
}

func TestHeartbeatKeepsStationaryPlayerAlive(t *testing.T) {
	tr, sent, clock := newTestTracker()
	tr.Update("akheva", 0, 0, 0, 0)

	// Standing perfectly still past the heartbeat must still broadcast:
	// the renderer hides a stale arrow, so silence would read as a dead pipe.
	*clock = clock.Add(heartbeat)
	tr.Update("akheva", 0, 0, 0, 0)
	if len(*sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2 — heartbeat must fire while stationary", len(*sent))
	}
}

func TestZoneChangeBroadcastsImmediately(t *testing.T) {
	tr, sent, clock := newTestTracker()
	tr.Update("akheva", 0, 0, 0, 0)

	// Same coordinates, different zone. Without the zone check this would be
	// dropped as "not moved" and the arrow would sit on the previous zone's map.
	*clock = clock.Add(minInterval)
	tr.Update("necropolis", 0, 0, 0, 0)
	if len(*sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2 — a zone change is always worth a frame", len(*sent))
	}
	if (*sent)[1].Zone != "necropolis" {
		t.Errorf("zone = %q, want necropolis", (*sent)[1].Zone)
	}
}

func TestHeadingChangeAloneBroadcasts(t *testing.T) {
	tr, sent, clock := newTestTracker()
	tr.Update("akheva", 0, 0, 0, 0)
	*clock = clock.Add(minInterval)
	tr.Update("akheva", 0, 0, 0, headingEpsilon)
	if len(*sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2 — turning in place still moves the arrow", len(*sent))
	}
}

func TestResetClearsSnapshot(t *testing.T) {
	tr, _, _ := newTestTracker()
	tr.Update("akheva", 10, 20, 30, 40)
	if _, ok := tr.Snapshot(); !ok {
		t.Fatal("Snapshot ok = false before Reset")
	}
	tr.Reset()
	if _, ok := tr.Snapshot(); ok {
		t.Error("Snapshot ok = true after Reset — a stale arrow looks authoritative")
	}
}

// ── Group positions (Zeal MsgGroup) ─────────────────────────────────────────

func newGroupTestTracker() (*Tracker, *[]GroupState, *time.Time) {
	var sent []GroupState
	clock := time.Unix(1_700_000_000, 0)
	tr := New(func(State) {})
	tr.now = func() time.Time { return clock }
	tr.SetGroupBroadcast(func(gs GroupState) { sent = append(sent, gs) })
	return tr, &sent, &clock
}

func TestUpdateGroupNegatesAndBroadcasts(t *testing.T) {
	tr, sent, _ := newGroupTestTracker()
	tr.UpdateGroup("akheva", []GroupMemberInput{
		{Name: "Tank", GameX: 100, GameY: -250, GameZ: 42, Heading: 128},
	})
	if len(*sent) != 1 {
		t.Fatalf("broadcasts = %d, want 1", len(*sent))
	}
	m := (*sent)[0].Members[0]
	if m.Name != "Tank" || m.X != -100 || m.Y != 250 || m.Z != 42 || m.Heading != 128 {
		t.Errorf("member = %+v", m)
	}
	if (*sent)[0].Zone != "akheva" {
		t.Errorf("zone = %q", (*sent)[0].Zone)
	}
}

func TestUpdateGroupRateLimitsAndDetectsChange(t *testing.T) {
	tr, sent, clock := newGroupTestTracker()
	mk := func(x float64) []GroupMemberInput {
		return []GroupMemberInput{{Name: "Tank", GameX: x, GameY: 0, GameZ: 0, Heading: 0}}
	}

	tr.UpdateGroup("z", mk(0)) // 1: first
	*clock = clock.Add(50 * time.Millisecond)
	tr.UpdateGroup("z", mk(50)) // under minInterval — dropped
	*clock = clock.Add(60 * time.Millisecond)
	tr.UpdateGroup("z", mk(0)) // back at origin, past floor — no real move vs last SENT (still origin)
	*clock = clock.Add(120 * time.Millisecond)
	tr.UpdateGroup("z", mk(50)) // 2: moved
	*clock = clock.Add(120 * time.Millisecond)
	tr.UpdateGroup("z", mk(50)) // no change — dropped
	*clock = clock.Add(120 * time.Millisecond)
	tr.UpdateGroup("z", []GroupMemberInput{
		{Name: "Tank", GameX: 50}, {Name: "Healer"},
	}) // 3: member added

	if len(*sent) != 3 {
		t.Fatalf("broadcasts = %d, want 3 (first, moved, member-added)", len(*sent))
	}
}

func TestUpdateGroupHeartbeat(t *testing.T) {
	tr, sent, clock := newGroupTestTracker()
	tr.UpdateGroup("z", []GroupMemberInput{{Name: "A"}})
	*clock = clock.Add(3 * time.Second) // past heartbeat
	tr.UpdateGroup("z", []GroupMemberInput{{Name: "A"}})
	if len(*sent) != 2 {
		t.Fatalf("broadcasts = %d, want 2 (heartbeat forces a resend)", len(*sent))
	}
}

func TestResetGroupEmitsEmptyThenQuiet(t *testing.T) {
	tr, sent, clock := newGroupTestTracker()
	tr.UpdateGroup("z", []GroupMemberInput{{Name: "A"}})
	*clock = clock.Add(time.Second)
	tr.ResetGroup()
	if len(*sent) != 2 || len((*sent)[1].Members) != 0 {
		t.Fatalf("after reset: sent=%d, last members=%d; want 2 and an empty frame", len(*sent), len((*sent)[len(*sent)-1].Members))
	}
	tr.ResetGroup() // nothing to clear — must stay quiet
	if len(*sent) != 2 {
		t.Errorf("a second reset broadcast again: sent=%d", len(*sent))
	}
}

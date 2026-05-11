package rolltracker

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

func newTrackerForTest() *Tracker {
	return New(nil)
}

func feedRoll(t *testing.T, tr *Tracker, roller string, max, value int, ts time.Time) {
	t.Helper()
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventRollAnnounce,
		Timestamp: ts,
		Data:      logparser.RollAnnounceData{Roller: roller},
	})
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventRollResult,
		Timestamp: ts,
		Data:      logparser.RollResultData{Min: 0, Max: max, Value: value},
	})
}

func TestTrackerGroupsByMax(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "Astrael", 333, 59, base)
	feedRoll(t, tr, "Sopphia", 333, 261, base.Add(time.Second))
	feedRoll(t, tr, "Sandrian", 444, 426, base.Add(2*time.Second))

	st := tr.State()
	if len(st.Sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(st.Sessions))
	}
	// Newest first → 444 then 333.
	if st.Sessions[0].Max != 444 || st.Sessions[1].Max != 333 {
		t.Fatalf("unexpected session order: %d, %d", st.Sessions[0].Max, st.Sessions[1].Max)
	}
	if len(st.Sessions[1].Rolls) != 2 {
		t.Fatalf("333 session: want 2 rolls, got %d", len(st.Sessions[1].Rolls))
	}
}

func TestDuplicateRollerFlagged(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "Sopphia", 333, 100, base)
	feedRoll(t, tr, "Sopphia", 333, 200, base.Add(time.Second))

	st := tr.State()
	rolls := st.Sessions[0].Rolls
	if len(rolls) != 2 {
		t.Fatalf("want 2 rolls, got %d", len(rolls))
	}
	if rolls[0].Duplicate {
		t.Fatalf("first roll should not be a duplicate")
	}
	if !rolls[1].Duplicate {
		t.Fatalf("second roll should be flagged as duplicate")
	}
}

func TestStopSession(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)

	if !tr.Stop(333) {
		t.Fatalf("Stop(333) should return true for active session")
	}
	st := tr.State()
	if st.Sessions[0].Active {
		t.Fatalf("session should be inactive after Stop")
	}
	if tr.Stop(333) {
		t.Fatalf("Stop(333) should return false when session already stopped")
	}
}

func TestStaleSessionStartsNew(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)
	// Later than staleAfter — a new 333 raid drop should open a fresh session.
	feedRoll(t, tr, "B", 333, 75, base.Add(staleAfter+time.Second))

	st := tr.State()
	if len(st.Sessions) != 2 {
		t.Fatalf("want 2 sessions for stale-split, got %d", len(st.Sessions))
	}
}

func TestOrphanResultDropped(t *testing.T) {
	tr := newTrackerForTest()
	ts := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	// Result with no preceding announce — must be ignored, not crash.
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventRollResult,
		Timestamp: ts,
		Data:      logparser.RollResultData{Min: 0, Max: 333, Value: 7},
	})
	if len(tr.State().Sessions) != 0 {
		t.Fatalf("orphan result should not open a session")
	}
}

func TestClear(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)
	feedRoll(t, tr, "B", 444, 60, base.Add(time.Second))
	tr.Clear()
	if len(tr.State().Sessions) != 0 {
		t.Fatalf("Clear should drop every session")
	}
}

func TestSetWinnerRule(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetWinnerRule(WinnerLowest)
	if tr.State().WinnerRule != WinnerLowest {
		t.Fatalf("winner rule not applied")
	}
	tr.SetWinnerRule("bogus")
	if tr.State().WinnerRule != WinnerLowest {
		t.Fatalf("invalid winner rule should be ignored")
	}
}

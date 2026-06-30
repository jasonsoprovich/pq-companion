package rolltracker

import (
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// stubMatcher returns an ItemMatcher that recognizes any of the given names
// as a case-insensitive substring of the line, longest match winning — a
// DB-free stand-in for db.MatchItemNameInText.
func stubMatcher(names ...string) ItemMatcher {
	return func(line string) (string, bool) {
		low := strings.ToLower(line)
		best := ""
		for _, n := range names {
			if strings.Contains(low, strings.ToLower(n)) && len(n) > len(best) {
				best = n
			}
		}
		if best == "" {
			return "", false
		}
		return best, true
	}
}

func newTrackerForTest() *Tracker {
	return New(nil)
}

func feedRoll(t *testing.T, tr *Tracker, roller string, max, value int, ts time.Time) {
	t.Helper()
	feedRollRange(t, tr, roller, 0, max, value, ts)
}

func feedRollRange(t *testing.T, tr *Tracker, roller string, min, max, value int, ts time.Time) {
	t.Helper()
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventRollAnnounce,
		Timestamp: ts,
		Data:      logparser.RollAnnounceData{Roller: roller},
	})
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventRollResult,
		Timestamp: ts,
		Data:      logparser.RollResultData{Min: min, Max: max, Value: value},
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

func TestNonZeroMinGetsOwnSession(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	// Two normal 0–611 rolls plus one player who fat-fingered "/random
	// 222 611". The 222–611 roll must NOT merge into the 0–611 session —
	// a non-zero floor skews the odds under highest-wins, so it belongs
	// in its own bucket.
	feedRoll(t, tr, "Astrael", 611, 400, base)
	feedRoll(t, tr, "Sopphia", 611, 510, base.Add(time.Second))
	feedRollRange(t, tr, "Newbie", 222, 611, 590, base.Add(2*time.Second))

	st := tr.State()
	if len(st.Sessions) != 2 {
		t.Fatalf("want 2 sessions (0–611 and 222–611), got %d", len(st.Sessions))
	}

	var zero, offset *Session
	for i := range st.Sessions {
		s := &st.Sessions[i]
		switch {
		case s.Min == 0 && s.Max == 611:
			zero = s
		case s.Min == 222 && s.Max == 611:
			offset = s
		}
	}
	if zero == nil {
		t.Fatalf("missing 0–611 session: %+v", st.Sessions)
	}
	if offset == nil {
		t.Fatalf("missing 222–611 session: %+v", st.Sessions)
	}
	if len(zero.Rolls) != 2 {
		t.Fatalf("0–611 session: want 2 rolls, got %d", len(zero.Rolls))
	}
	if len(offset.Rolls) != 1 || offset.Rolls[0].Roller != "Newbie" {
		t.Fatalf("222–611 session: want only Newbie's roll, got %+v", offset.Rolls)
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

func TestSetItemName(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)
	id := tr.State().Sessions[0].ID

	if !tr.SetItemName(id, "Robe of the Lost Circle") {
		t.Fatalf("SetItemName should return true for a known session")
	}
	if got := tr.State().Sessions[0].ItemName; got != "Robe of the Lost Circle" {
		t.Fatalf("item name not applied, got %q", got)
	}
	// Empty name clears the label.
	if !tr.SetItemName(id, "") {
		t.Fatalf("SetItemName('') should still match the session")
	}
	if got := tr.State().Sessions[0].ItemName; got != "" {
		t.Fatalf("empty name should clear the label, got %q", got)
	}
	if tr.SetItemName(9999, "x") {
		t.Fatalf("SetItemName should return false for an unknown session")
	}
}

func TestAutoSuggestCallBeforeRoll(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetItemMatcher(stubMatcher("Robe of the Lost Circle"))
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	// Leader announces the item, then the rolls follow.
	tr.HandleLine(base, "Belnoctourne tells the raid, 'Robe of the Lost Circle 333'")
	feedRoll(t, tr, "Astrael", 333, 200, base.Add(2*time.Second))

	if got := tr.State().Sessions[0].ItemName; got != "Robe of the Lost Circle" {
		t.Fatalf("session should be auto-labeled, got %q", got)
	}
}

func TestAutoSuggestCallAfterFirstRoll(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetItemMatcher(stubMatcher("Sword of Foo"))
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "Astrael", 444, 200, base)
	if tr.State().Sessions[0].ItemName != "" {
		t.Fatalf("should be unlabeled before any call")
	}
	// The call arrives a moment later; the active session still gets labeled.
	tr.HandleLine(base.Add(time.Second), "Leader tells the raid, 'Sword of Foo 444'")
	if got := tr.State().Sessions[0].ItemName; got != "Sword of Foo" {
		t.Fatalf("session should be labeled after the call, got %q", got)
	}
}

func TestAutoSuggestNumberMustMatch(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetItemMatcher(stubMatcher("Robe of the Lost Circle"))
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	// Call names 555 but the roll is a 333 — different drop, no label.
	tr.HandleLine(base, "Leader tells the raid, 'Robe of the Lost Circle 555'")
	feedRoll(t, tr, "Astrael", 333, 100, base.Add(time.Second))
	if got := tr.State().Sessions[0].ItemName; got != "" {
		t.Fatalf("a 555 call must not label a 333 roll, got %q", got)
	}
}

func TestAutoSuggestIgnoresNonChat(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetItemMatcher(stubMatcher("Robe of the Lost Circle"))
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	// A combat line carries a 333 but isn't player chat — must be ignored,
	// otherwise damage numbers would masquerade as roll calls.
	tr.HandleLine(base, "You crush a goblin for 333 points of damage.")
	feedRoll(t, tr, "Astrael", 333, 100, base.Add(time.Second))
	if got := tr.State().Sessions[0].ItemName; got != "" {
		t.Fatalf("non-chat line must not trigger auto-suggest, got %q", got)
	}
}

func TestAutoSuggestKeepsManualLabel(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetItemMatcher(stubMatcher("Robe of the Lost Circle"))
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "Astrael", 333, 100, base)
	id := tr.State().Sessions[0].ID
	tr.SetItemName(id, "Officer's Pick")
	// A later matching call must not clobber the user's label.
	tr.HandleLine(base.Add(time.Second), "Leader tells the raid, 'Robe of the Lost Circle 333'")
	if got := tr.State().Sessions[0].ItemName; got != "Officer's Pick" {
		t.Fatalf("manual label must be preserved, got %q", got)
	}
}

func TestProfileValidate(t *testing.T) {
	// Zero value and explicit simple both normalize to simple, no error.
	for _, in := range []RollProfile{{}, {Mode: ProfileSimple}} {
		got, err := in.Validate()
		if err != nil || got.Mode != ProfileSimple {
			t.Fatalf("simple normalize: got %+v err %v", got, err)
		}
	}
	// Suffix scheme with no divisor defaults to 100.
	got, err := RollProfile{
		Mode:   ProfileTiered,
		Scheme: SchemeSuffix,
		Tiers:  []ProfileTier{{Match: 11, Label: "Pick"}},
	}.Validate()
	if err != nil || got.Divisor != 100 {
		t.Fatalf("suffix divisor default: got %+v err %v", got, err)
	}
	// Tiered with no tiers, a bad scheme, and duplicate matches all error.
	bad := []RollProfile{
		{Mode: ProfileTiered, Scheme: SchemeSuffix},
		{Mode: ProfileTiered, Scheme: "bogus", Tiers: []ProfileTier{{Match: 1, Label: "x"}}},
		{Mode: ProfileTiered, Scheme: SchemeExact, Tiers: []ProfileTier{{Match: 111, Label: "a"}, {Match: 111, Label: "b"}}},
		{Mode: ProfileTiered, Scheme: SchemeExact, Tiers: []ProfileTier{{Match: 111, Label: ""}}},
		{Mode: "weird"},
	}
	for i, p := range bad {
		if _, err := p.Validate(); err == nil {
			t.Fatalf("bad profile %d should have errored: %+v", i, p)
		}
	}
}

func TestSetProfileRoundTrips(t *testing.T) {
	tr := newTrackerForTest()
	if tr.State().Profile.Mode != ProfileSimple {
		t.Fatalf("default profile should be simple, got %q", tr.State().Profile.Mode)
	}
	p := RollProfile{
		Mode:    ProfileTiered,
		Scheme:  SchemeSuffix,
		Divisor: 100,
		Tiers:   []ProfileTier{{Match: 11, Label: "Pick"}, {Match: 22, Label: "Upgrade"}},
	}
	tr.SetProfile(p)
	got := tr.State().Profile
	if got.Mode != ProfileTiered || got.Scheme != SchemeSuffix || len(got.Tiers) != 2 {
		t.Fatalf("profile not round-tripped through State: %+v", got)
	}
}

func TestStopSession(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)

	id := tr.State().Sessions[0].ID
	if !tr.Stop(id) {
		t.Fatalf("Stop should return true for active session")
	}
	st := tr.State()
	if st.Sessions[0].Active {
		t.Fatalf("session should be inactive after Stop")
	}
	if tr.Stop(id) {
		t.Fatalf("Stop should return false when session already stopped")
	}
}

func TestRemoveSession(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)
	feedRoll(t, tr, "B", 444, 60, base.Add(time.Second))

	id := tr.State().Sessions[1].ID // older (333) session
	if !tr.Remove(id) {
		t.Fatalf("Remove should return true for known session")
	}
	st := tr.State()
	if len(st.Sessions) != 1 || st.Sessions[0].Max != 444 {
		t.Fatalf("Remove should leave only 444 session, got %+v", st.Sessions)
	}
	if tr.Remove(id) {
		t.Fatalf("Remove should return false for unknown session")
	}
}

func TestStopAndRemoveIndependentBuckets(t *testing.T) {
	tr := newTrackerForTest()
	base := time.Date(2026, 5, 10, 20, 25, 0, 0, time.Local)
	feedRoll(t, tr, "A", 333, 50, base)
	feedRoll(t, tr, "B", 444, 60, base.Add(time.Second))

	// Stop just the 333 session; the 444 session must remain Live so a
	// later roll on 444 still lands in it instead of opening a new one.
	st := tr.State()
	var id333 uint64
	for _, s := range st.Sessions {
		if s.Max == 333 {
			id333 = s.ID
		}
	}
	if !tr.Stop(id333) {
		t.Fatalf("Stop on 333 should succeed")
	}
	for _, s := range tr.State().Sessions {
		if s.Max == 444 && !s.Active {
			t.Fatalf("Stopping 333 should not affect 444 session")
		}
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

func TestTimerModeAutoStops(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetMode(ModeTimer, 1) // SetMode accepts >0 even though the API enforces ≥5
	base := time.Now()
	feedRoll(t, tr, "A", 333, 50, base)

	if tr.State().Sessions[0].AutoStopAt.IsZero() {
		t.Fatalf("timer-mode session should publish AutoStopAt")
	}
	// SetMode used a 1-second window above. Wait long enough for the
	// AfterFunc to fire, then verify the session is no longer Active.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !tr.State().Sessions[0].Active {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	st := tr.State()
	if st.Sessions[0].Active {
		t.Fatalf("session should have auto-stopped within 2s")
	}
	if !st.Sessions[0].AutoStopAt.IsZero() {
		t.Fatalf("AutoStopAt should clear once timer fires")
	}
}

func TestSetModePersistsAndSwitches(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetMode(ModeTimer, 30)
	st := tr.State()
	if st.Mode != ModeTimer || st.AutoStopSeconds != 30 {
		t.Fatalf("SetMode did not persist: %+v", st)
	}
	tr.SetMode(ModeManual, 0)
	if tr.State().Mode != ModeManual {
		t.Fatalf("SetMode back to manual did not stick")
	}
	if tr.State().AutoStopSeconds != 30 {
		t.Fatalf("AutoStopSeconds should be preserved across mode switches")
	}
}

func TestManualStopCancelsTimer(t *testing.T) {
	tr := newTrackerForTest()
	tr.SetMode(ModeTimer, 60)
	feedRoll(t, tr, "A", 333, 50, time.Now())
	id := tr.State().Sessions[0].ID
	if !tr.Stop(id) {
		t.Fatalf("Stop should succeed")
	}
	// Internally we expect the auto-stop timer to have been cancelled
	// so the inactive session is not re-broadcast a second time when
	// the AfterFunc would have fired.
	tr.mu.Lock()
	_, stillScheduled := tr.autoStops[id]
	tr.mu.Unlock()
	if stillScheduled {
		t.Fatalf("manual Stop must cancel the pending auto-stop timer")
	}
}

package respawn

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// openTestDB opens the shared quarm.db fixture used across backend tests.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestEngine builds an engine wired to a hub + the real DB. The hub's
// channel is buffered so broadcasts succeed without a Run() goroutine.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	return NewEngine(ws.NewHub(), openTestDB(t))
}

func killEvent(target string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventKill,
		Timestamp: ts,
		Data:      logparser.KillData{Killer: "You", Target: target},
	}
}

// TestOnKill_StartsTimer verifies a kill in a known zone produces a timer with
// the spawn data's respawn time and an ascending per-name label index.
func TestOnKill_StartsTimer(t *testing.T) {
	e := newTestEngine(t)
	// a_skeleton (level 4) spawns in nektulos with a 150s raw respawn. nektulos
	// is a standard reduced zone (reducedspawntimers=1, castdungeon=0), so a
	// newbie mob's 150s collapses to the 60s fast timer. Set the zone directly
	// to avoid depending on long-name text.
	e.logZoneShort = "nektulos"
	e.logZoneLong = "Nektulos Forest"

	now := time.Now()
	e.Handle(killEvent("a skeleton", now))

	st := e.GetState()
	if len(st.Timers) != 1 {
		t.Fatalf("want 1 timer, got %d", len(st.Timers))
	}
	tm := st.Timers[0]
	if tm.NPCName != "a skeleton" {
		t.Errorf("npc name: got %q", tm.NPCName)
	}
	if tm.LabelIndex != 1 {
		t.Errorf("label index: got %d, want 1", tm.LabelIndex)
	}
	if tm.DurationSeconds != 60 {
		t.Errorf("duration: got %v, want 60 (fast-respawn reduced)", tm.DurationSeconds)
	}
	if tm.RemainingSeconds <= 50 || tm.RemainingSeconds > 61 {
		t.Errorf("remaining out of range: got %v", tm.RemainingSeconds)
	}
	if tm.Zone != "nektulos" {
		t.Errorf("zone: got %q", tm.Zone)
	}

	// A second kill of the same name gets index 2.
	e.Handle(killEvent("a skeleton", now))
	st = e.GetState()
	if len(st.Timers) != 2 {
		t.Fatalf("want 2 timers after second kill, got %d", len(st.Timers))
	}
	maxIdx := 0
	for _, tt := range st.Timers {
		if tt.LabelIndex > maxIdx {
			maxIdx = tt.LabelIndex
		}
	}
	if maxIdx != 2 {
		t.Errorf("second label index: got %d, want 2", maxIdx)
	}
}

// TestHandle_ZoneThenKill verifies the full log-driven path: a zone-entry event
// resolves the short name from the DB, and a subsequent kill creates a timer.
func TestHandle_ZoneThenKill(t *testing.T) {
	e := newTestEngine(t)

	e.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "Nektulos Forest"},
	})
	e.Handle(killEvent("a skeleton", time.Now()))

	st := e.GetState()
	if len(st.Timers) != 1 {
		t.Fatalf("want 1 timer after zone+kill, got %d", len(st.Timers))
	}
	if st.Timers[0].Zone != "nektulos" {
		t.Errorf("zone resolved from long name: got %q, want nektulos", st.Timers[0].Zone)
	}
	if st.CurrentZone != "nektulos" {
		t.Errorf("current zone in state: got %q, want nektulos", st.CurrentZone)
	}
}

// TestOnKill_NoRespawnData verifies that a name with no spawn data in the zone
// (trash, a player slain by a mob, wrong zone) produces no timer.
func TestOnKill_NoRespawnData(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"

	e.Handle(killEvent("a totally nonexistent creature xyz", time.Now()))
	if st := e.GetState(); len(st.Timers) != 0 {
		t.Fatalf("want 0 timers for unknown name, got %d", len(st.Timers))
	}
}

// TestOnKill_UnknownZoneSkipped verifies that with no zone resolved, kills are
// ignored (we can't pick a respawn time without a zone).
func TestOnKill_UnknownZoneSkipped(t *testing.T) {
	e := newTestEngine(t)
	e.Handle(killEvent("a skeleton", time.Now()))
	if st := e.GetState(); len(st.Timers) != 0 {
		t.Fatalf("want 0 timers when zone unknown, got %d", len(st.Timers))
	}
}

// TestRemoveByID_ResetsIndex verifies manual removal works and that the label
// counter restarts at 1 once every timer for a name has cleared.
func TestRemoveByID_ResetsIndex(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	now := time.Now()

	e.Handle(killEvent("a skeleton", now))
	st := e.GetState()
	id := st.Timers[0].ID
	if !e.RemoveByID(id) {
		t.Fatalf("RemoveByID(%q) returned false", id)
	}
	if st := e.GetState(); len(st.Timers) != 0 {
		t.Fatalf("want 0 timers after removal, got %d", len(st.Timers))
	}
	if e.RemoveByID(id) {
		t.Errorf("RemoveByID of already-removed id returned true")
	}

	// Numbering restarts at 1 because no timer for that name remains.
	e.Handle(killEvent("a skeleton", now))
	if got := e.GetState().Timers[0].LabelIndex; got != 1 {
		t.Errorf("label index after reset: got %d, want 1", got)
	}
}

// TestSummarize covers the estimate / ambiguity / range reduction in isolation.
func TestSummarize(t *testing.T) {
	tests := []struct {
		name      string
		infos     []db.RespawnInfo
		wantEst   int
		wantAmbig bool
		wantMin   int
		wantMax   int
	}{
		{
			name:    "single value, not ambiguous",
			infos:   []db.RespawnInfo{{NPCID: 1, RespawnTime: 600}},
			wantEst: 600,
		},
		{
			name: "mode wins",
			infos: []db.RespawnInfo{
				{NPCID: 1, RespawnTime: 600},
				{NPCID: 1, RespawnTime: 600},
				{NPCID: 1, RespawnTime: 1200},
			},
			wantEst:   600,
			wantAmbig: true,
			wantMin:   600,
			wantMax:   1200,
		},
		{
			name: "tie breaks toward shorter",
			infos: []db.RespawnInfo{
				{NPCID: 1, RespawnTime: 1200},
				{NPCID: 1, RespawnTime: 240},
			},
			wantEst:   240,
			wantAmbig: true,
			wantMin:   240,
			wantMax:   1200,
		},
		{
			name:    "zero respawn rows ignored",
			infos:   []db.RespawnInfo{{NPCID: 1, RespawnTime: 0}},
			wantEst: 0,
		},
		{
			// Raid/named encounters with a script-controlled respawn use
			// this EQEmu sentinel in spawn2.respawntime instead of a real
			// natural timer; treating it as real produced the reported
			// "19d instead of 3d" bug for Luclin raid targets.
			name:    "script-controlled sentinel ignored",
			infos:   []db.RespawnInfo{{NPCID: 1, RespawnTime: scriptControlledRespawnSentinel}},
			wantEst: 0,
		},
		{
			name: "sentinel ignored alongside a real value",
			infos: []db.RespawnInfo{
				{NPCID: 1, RespawnTime: scriptControlledRespawnSentinel},
				{NPCID: 1, RespawnTime: 259200},
			},
			wantEst: 259200,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			est, ambig, minS, maxS, _ := summarize(tc.infos)
			if est != tc.wantEst {
				t.Errorf("estimate: got %d, want %d", est, tc.wantEst)
			}
			if ambig != tc.wantAmbig {
				t.Errorf("ambiguous: got %v, want %v", ambig, tc.wantAmbig)
			}
			if minS != tc.wantMin {
				t.Errorf("min: got %d, want %d", minS, tc.wantMin)
			}
			if maxS != tc.wantMax {
				t.Errorf("max: got %d, want %d", maxS, tc.wantMax)
			}
		})
	}
}

// --- Prioritized (pinned) respawn tracking ---
//
// "An Arcane Elementalist" is a second, unrelated-name Nektulos spawn used
// alongside "a skeleton" to exercise cross-name pin handoff by position.

// expireLocked directly rewrites a timer's RespawnAt into the past (beyond
// the grace window) so tests can simulate "already popped" without sleeping.
// White-box: same package, single-goroutine test, no ticker running.
func expireLocked(e *Engine, id string) {
	e.timers[id].RespawnAt = time.Now().Add(-2 * graceWindow)
}

// TestSnapshot_PinnedSortFirst verifies a pinned timer sorts ahead of an
// unpinned one regardless of remaining time or zone recency.
func TestSnapshot_PinnedSortFirst(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	now := time.Now()

	e.Handle(killEvent("a skeleton", now))
	e.Handle(killEvent("An Arcane Elementalist", now))
	st := e.GetState()
	if len(st.Timers) != 2 {
		t.Fatalf("want 2 timers, got %d", len(st.Timers))
	}
	// Pin whichever timer sorts second by the base ordering (current-zone,
	// remaining-ascending, ID) so pinning is what moves it, not luck.
	target := st.Timers[1].ID
	if !e.TogglePin(target, true) {
		t.Fatalf("TogglePin(%q, true) returned false", target)
	}

	st = e.GetState()
	if !st.Timers[0].Pinned || st.Timers[0].ID != target {
		t.Errorf("pinned timer should sort first: got order %+v", st.Timers)
	}
}

// TestPruneExpired_PinnedSurvives verifies a pinned, popped timer is exempt
// from the 60s grace-window prune, while an unpinned one is still removed.
func TestPruneExpired_PinnedSurvives(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	now := time.Now()

	e.Handle(killEvent("a skeleton", now))
	e.Handle(killEvent("An Arcane Elementalist", now))
	st := e.GetState()
	pinnedID, unpinnedID := st.Timers[0].ID, st.Timers[1].ID

	if !e.TogglePin(pinnedID, true) {
		t.Fatalf("TogglePin(%q, true) returned false", pinnedID)
	}
	expireLocked(e, pinnedID)
	expireLocked(e, unpinnedID)

	e.pruneExpired()

	st = e.GetState()
	if len(st.Timers) != 1 || st.Timers[0].ID != pinnedID {
		t.Fatalf("want only pinned timer to survive prune, got %+v", st.Timers)
	}
}

// TestTogglePin_UnknownID verifies TogglePin reports failure for an ID that
// isn't an active timer, without panicking or broadcasting.
func TestTogglePin_UnknownID(t *testing.T) {
	e := newTestEngine(t)
	if e.TogglePin("nektulos|nothing|1", true) {
		t.Error("TogglePin of unknown id returned true")
	}
}

// TestClaimPin_HandoffByPosition covers the Zeal-position handoff path: a
// pinned, popped timer hands its pin to the next kill within anchorRadius,
// regardless of name, and does not when the kill is farther away or the
// pinned timer hasn't popped yet.
func TestClaimPin_HandoffByPosition(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	t0 := time.Now()

	e.SetPipePlayerPos(0, 0)
	e.Handle(killEvent("a skeleton", t0))
	id1 := e.GetState().Timers[0].ID
	if !e.TogglePin(id1, true) {
		t.Fatalf("TogglePin(%q, true) returned false", id1)
	}
	if !e.timers[id1].hasAnchor {
		t.Fatalf("expected anchor to be stamped from the kill position")
	}

	t.Run("outside radius does not claim", func(t *testing.T) {
		expireLocked(e, id1)
		e.SetPipePlayerPos(1000, 1000) // ~1414 units away, well past anchorRadius
		e.Handle(killEvent("An Arcane Elementalist", t0))

		st := e.GetState()
		if len(st.Timers) != 2 {
			t.Fatalf("want 2 timers (no handoff), got %d", len(st.Timers))
		}
		for _, tm := range st.Timers {
			if tm.NPCName == "An Arcane Elementalist" && tm.Pinned {
				t.Errorf("far-away kill should not inherit the pin")
			}
			if tm.ID == id1 && !tm.Pinned {
				t.Errorf("original pinned timer should be untouched")
			}
		}
		// Clean up the unpinned timer this sub-test added.
		for _, tm := range st.Timers {
			if tm.NPCName == "An Arcane Elementalist" {
				e.RemoveByID(tm.ID)
			}
		}
	})

	t.Run("not yet popped does not claim", func(t *testing.T) {
		// Re-arm id1's RespawnAt into the future so it looks un-popped.
		e.timers[id1].RespawnAt = t0.Add(time.Hour)
		e.SetPipePlayerPos(10, 10) // well within radius of the (0,0) anchor
		e.Handle(killEvent("An Arcane Elementalist", t0))

		st := e.GetState()
		found := false
		for _, tm := range st.Timers {
			if tm.NPCName == "An Arcane Elementalist" {
				found = true
				if tm.Pinned {
					t.Errorf("still-running pinned timer should not hand off its pin")
				}
				e.RemoveByID(tm.ID)
			}
		}
		if !found {
			t.Fatalf("expected the new kill to still produce a timer")
		}
		expireLocked(e, id1) // restore popped state for the next sub-test
	})

	t.Run("within radius claims regardless of name", func(t *testing.T) {
		e.SetPipePlayerPos(50, 50) // distance ~70.7 from (0,0), within the 200 radius
		e.Handle(killEvent("An Arcane Elementalist", t0))

		st := e.GetState()
		if len(st.Timers) != 1 {
			t.Fatalf("want the popped pinned timer replaced by the new one, got %d timers: %+v", len(st.Timers), st.Timers)
		}
		got := st.Timers[0]
		if got.NPCName != "An Arcane Elementalist" {
			t.Fatalf("want the new kill's timer, got %q", got.NPCName)
		}
		if !got.Pinned {
			t.Fatalf("want the pin handed off to the new timer")
		}
		// The anchor stays at the original camp spot, not the new kill's
		// position, so the camp doesn't drift pull-to-pull.
		gotAnchor := e.timers[got.ID]
		if gotAnchor.anchorX != 0 || gotAnchor.anchorY != 0 {
			t.Errorf("anchor should be inherited from the original pin, got (%v, %v)", gotAnchor.anchorX, gotAnchor.anchorY)
		}
	})
}

// TestClaimPin_NearestOfTwoCandidates verifies that when two popped, pinned
// timers are both within range, the nearer one's pin is claimed.
func TestClaimPin_NearestOfTwoCandidates(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	t0 := time.Now()

	// "near" is pinned but left un-popped while "far" is set up, so its own
	// kill doesn't prematurely claim it (see the position-handoff test above
	// for why an un-popped pinned timer is never a candidate).
	e.SetPipePlayerPos(0, 0)
	e.Handle(killEvent("a skeleton", t0))
	near := e.GetState().Timers[0].ID
	e.TogglePin(near, true)

	e.SetPipePlayerPos(150, 0)
	e.Handle(killEvent("An Arcane Elementalist", t0))
	var far string
	for _, tm := range e.GetState().Timers {
		if tm.ID != near {
			far = tm.ID
		}
	}
	e.TogglePin(far, true)

	expireLocked(e, near)
	expireLocked(e, far)

	// New kill at (30, 0): distance 30 from "near" (0,0), distance 120 from
	// "far" (150,0). Both are within anchorRadius (200), so the nearer one
	// — "near" — should win, leaving "far" behind as its own untouched
	// pinned (now orphaned) timer.
	e.SetPipePlayerPos(30, 0)
	e.Handle(killEvent("An Arcane Elementalist", t0))

	st := e.GetState()
	if len(st.Timers) != 2 {
		t.Fatalf("want the claiming timer plus the untouched 'far' one, got %d timers: %+v", len(st.Timers), st.Timers)
	}
	if _, stillThere := e.timers[far]; !stillThere {
		t.Fatalf("the farther pinned timer should be left alone, not claimed")
	}
	if near == far {
		t.Fatal("test setup bug: near and far ids collided")
	}
	var claimed *RespawnTimer
	for id, tm := range e.timers {
		if id != far {
			claimed = tm
		}
	}
	if claimed == nil || !claimed.Pinned {
		t.Fatalf("want the new timer to have inherited the pin")
	}
	if claimed.anchorX != 0 || claimed.anchorY != 0 {
		t.Errorf("want the nearer anchor (0,0) inherited, got (%v, %v)", claimed.anchorX, claimed.anchorY)
	}
	if _, stillNear := e.timers[near]; stillNear {
		t.Errorf("the nearer pinned timer should have been claimed (removed), id %q still present", near)
	}
}

// TestClaimPin_HandoffBySameName covers the no-Zeal-position fallback: pin
// handoff matches on name only, oldest death first, and never crosses names.
func TestClaimPin_HandoffBySameName(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	t0 := time.Now()
	// No SetPipePlayerPos call: hasPipePos stays false for this whole test,
	// exercising the "no Zeal pipe connected" path.

	e.Handle(killEvent("a skeleton", t0))
	id1 := e.GetState().Timers[0].ID
	e.TogglePin(id1, true)
	if e.timers[id1].hasAnchor {
		t.Fatalf("pin without a known kill position should have no anchor")
	}
	expireLocked(e, id1)

	t.Run("different name does not claim", func(t *testing.T) {
		e.Handle(killEvent("An Arcane Elementalist", t0))
		st := e.GetState()
		if len(st.Timers) != 2 {
			t.Fatalf("want 2 timers (no handoff across names), got %d", len(st.Timers))
		}
		for _, tm := range st.Timers {
			if tm.NPCName == "An Arcane Elementalist" {
				if tm.Pinned {
					t.Errorf("different-name kill should not inherit the pin")
				}
				e.RemoveByID(tm.ID)
			}
		}
	})

	t.Run("same name claims", func(t *testing.T) {
		e.Handle(killEvent("a skeleton", t0))
		st := e.GetState()
		if len(st.Timers) != 1 {
			t.Fatalf("want the popped pinned timer replaced, got %d timers: %+v", len(st.Timers), st.Timers)
		}
		if !st.Timers[0].Pinned {
			t.Fatalf("want the pin handed off to the same-name kill")
		}
	})
}

// TestPinCommands covers PinLatestInCurrentZone, UnpinLatest and ClearPins —
// the actions behind the "/pipe respawn pin|unpin|clearpins" commands.
func TestPinCommands(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	t0 := time.Now()

	if e.PinLatestInCurrentZone() {
		t.Error("PinLatestInCurrentZone with no timers should return false")
	}

	e.Handle(killEvent("a skeleton", t0))
	first := e.GetState().Timers[0].ID
	e.Handle(killEvent("An Arcane Elementalist", t0.Add(time.Second)))

	if !e.PinLatestInCurrentZone() {
		t.Fatal("PinLatestInCurrentZone returned false with timers present")
	}
	st := e.GetState()
	var pinnedName string
	for _, tm := range st.Timers {
		if tm.Pinned {
			pinnedName = tm.NPCName
		}
	}
	if pinnedName != "An Arcane Elementalist" {
		t.Errorf("want the most recently died timer pinned, got %q", pinnedName)
	}

	// Also pin the first (older) timer, with a later pinnedAt so it's the
	// "most recently pinned" for UnpinLatest to target.
	e.TogglePin(first, true)
	e.timers[first].pinnedAt = time.Now().Add(time.Hour)

	if !e.UnpinLatest() {
		t.Fatal("UnpinLatest returned false with a pinned timer present")
	}
	if e.timers[first].Pinned {
		t.Error("UnpinLatest should have unpinned the most-recently-pinned timer")
	}
	st = e.GetState()
	pinnedCount := 0
	for _, tm := range st.Timers {
		if tm.Pinned {
			pinnedCount++
		}
	}
	if pinnedCount != 1 {
		t.Fatalf("want 1 timer still pinned after UnpinLatest, got %d", pinnedCount)
	}

	if !e.ClearPins() {
		t.Fatal("ClearPins returned false with a pinned timer present")
	}
	if e.ClearPins() {
		t.Error("ClearPins with nothing pinned should return false")
	}
	for _, tm := range e.GetState().Timers {
		if tm.Pinned {
			t.Errorf("timer %q still pinned after ClearPins", tm.ID)
		}
	}
}

// TestHandlePipeCommand covers the "/pipe respawn ..." text matching:
// case-insensitive, whitespace-trimmed, and only these three phrases.
func TestHandlePipeCommand(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	e.Handle(killEvent("a skeleton", time.Now()))

	if !e.HandlePipeCommand("  Respawn Pin  ") {
		t.Error(`HandlePipeCommand("  Respawn Pin  ") returned false`)
	}
	if !e.HandlePipeCommand("RESPAWN UNPIN") {
		t.Error(`HandlePipeCommand("RESPAWN UNPIN") returned false`)
	}
	e.HandlePipeCommand("respawn pin")
	if !e.HandlePipeCommand("respawn clearpins") {
		t.Error(`HandlePipeCommand("respawn clearpins") returned false`)
	}
	if e.HandlePipeCommand("respawn something else") {
		t.Error("HandlePipeCommand matched unrelated text")
	}
	if e.HandlePipeCommand("") {
		t.Error("HandlePipeCommand matched empty text")
	}
}

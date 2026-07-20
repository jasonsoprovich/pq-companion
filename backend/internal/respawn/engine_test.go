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

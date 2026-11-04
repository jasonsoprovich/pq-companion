package respawn

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestZoneCopies verifies kills are tagged with the dbg.txt zone copy, each
// copy numbers its own labels, and a snapshot marks timers from another copy
// of the current zone.
func TestZoneCopies(t *testing.T) {
	e := newTestEngine(t)
	e.logZoneShort = "nektulos"
	e.logZoneLong = "Nektulos Forest"
	now := time.Now()

	e.SetZoneCopy("nektulos", "host:7312")
	e.Handle(killEvent("a skeleton", now))
	e.SetZoneCopy("nektulos", "host:11243")
	e.Handle(killEvent("a skeleton", now))

	st := e.GetState()
	if st.CurrentInstance != "host:11243" {
		t.Fatalf("current instance = %q", st.CurrentInstance)
	}
	if len(st.Timers) != 2 {
		t.Fatalf("want 2 timers, got %d", len(st.Timers))
	}
	byCopy := map[string]RespawnTimer{}
	for _, tm := range st.Timers {
		byCopy[tm.Instance] = tm
	}
	a, b := byCopy["host:7312"], byCopy["host:11243"]
	if a.LabelIndex != 1 || b.LabelIndex != 1 || a.ID == b.ID {
		t.Fatalf("labels %d/%d ids %q/%q, want 1/1 and distinct", a.LabelIndex, b.LabelIndex, a.ID, b.ID)
	}
	if !a.OtherCopy || b.OtherCopy {
		t.Fatalf("other copy flags %v/%v, want true/false", a.OtherCopy, b.OtherCopy)
	}
	if st.Timers[0].Instance != "host:11243" {
		t.Fatalf("own copy should sort first, got %q", st.Timers[0].Instance)
	}

	// A copy for another zone doesn't apply here: the kill is untagged.
	e.SetZoneCopy("fearplane", "host:9000")
	if st := e.GetState(); st.CurrentInstance != "" {
		t.Fatalf("copy for another zone leaked: %q", st.CurrentInstance)
	}
	for _, tm := range e.GetState().Timers {
		if tm.OtherCopy {
			t.Fatalf("unknown copy marked %q as other", tm.ID)
		}
	}
	e.Handle(killEvent("a skeleton", now))
	for _, tm := range e.GetState().Timers {
		if tm.Instance == "" && tm.LabelIndex != 1 {
			t.Fatalf("untagged kill label = %d", tm.LabelIndex)
		}
	}
}

func TestPlaceSpawnCopies(t *testing.T) {
	now := time.Now()
	e := NewEngine(nil, nil)
	e.timers["x"] = &RespawnTimer{ID: "x", Zone: "z", Instance: "h:1", SpawnID: 1, RespawnAt: now.Add(time.Minute)}
	infos := spawnInfos(1, 2)
	e.mu.Lock()
	defer e.mu.Unlock()
	if got := e.placeSpawnLocked("z", "h:2", "a gnoll", infos, now); got != 0 {
		t.Fatalf("other copy's timer held a point: got %d with two free", got)
	}
	if got := e.placeSpawnLocked("z", "h:1", "a gnoll", infos, now); got != 2 {
		t.Fatalf("same copy: got %d, want 2", got)
	}
	if got := e.placeSpawnLocked("z", "", "a gnoll", infos, now); got != 2 {
		t.Fatalf("unknown copy: got %d, want 2", got)
	}
}

func spawnInfos(ids ...int) []db.RespawnInfo {
	out := make([]db.RespawnInfo, len(ids))
	for i, id := range ids {
		out[i] = db.RespawnInfo{SpawnID: id, X: float64(i * 100)}
	}
	return out
}

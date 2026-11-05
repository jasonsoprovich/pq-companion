package overlay

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/live"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

func intp(n int) *int { return &n }

// pipeTarget selects name with spawn id on a pipe-connected tracker the way
// the pipe does: name label first, then the player snapshot.
func pipeTarget(tr *NPCTracker, name string, id int) {
	tr.SetPipeTarget(name)
	tr.SetPipeTargetID(intp(id))
}

func pipeTracker() *NPCTracker {
	tr := newTestTracker()
	tr.SetPipeConnected(true)
	return tr
}

func TestConsider_PipeReadingAppliesToMatchingSpawn(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 9)
	c := tr.GetState().Consider
	if c == nil || c.Faction != 9 || c.Standing != "scowls" || c.Source != live.SourcePipe {
		t.Fatalf("Consider = %+v", c)
	}
	// A consider for some other spawn doesn't show.
	tr.SetPipeConsider(200, "a gnoll pup", 5)
	if tr.GetState().Consider != nil {
		t.Error("reading for another spawn id applied")
	}
}

func TestConsider_UnknownFactionIgnored(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 0)
	tr.SetPipeConsider(100, "a gnoll pup", 10)
	if tr.GetState().Consider != nil {
		t.Error("out-of-range faction applied")
	}
}

func TestConsider_ClearedOnRetargetToOtherSpawn(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 9)
	pipeTarget(tr, "a gnoll pup", 101) // same name, different spawn
	if tr.GetState().Consider != nil {
		t.Fatal("reading followed the name to another spawn")
	}
	pipeTarget(tr, "a gnoll pup", 100) // back to the first: stays cleared
	if tr.GetState().Consider != nil {
		t.Error("reading should be gone after a retarget")
	}
}

// With left-click con the server's reply can beat the retarget on the pipe.
func TestConsider_ArrivesBeforeRetarget(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(101, "a decaying skeleton", 8) // target still 100
	tr.SetPipeTargetID(intp(100))                     // another tick on the old target
	pipeTarget(tr, "a decaying skeleton", 101)
	c := tr.GetState().Consider
	if c == nil || c.Faction != 8 {
		t.Errorf("Consider = %+v, want threatening", c)
	}
}

func TestConsider_DeselectReselectSameSpawnKeeps(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 9)
	tr.ClearPipeTarget()
	tr.SetPipeTargetID(nil)
	if tr.GetState().Consider != nil {
		t.Fatal("no target should show no reading")
	}
	pipeTarget(tr, "a gnoll pup", 100)
	if c := tr.GetState().Consider; c == nil || c.Faction != 9 {
		t.Errorf("Consider = %+v after reselecting the same spawn", c)
	}
}

func TestConsider_InvalidateAndZone(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 5)
	tr.InvalidateConsider() // e.g. invis dropped
	if tr.GetState().Consider != nil {
		t.Fatal("invalidate should clear the reading")
	}
	tr.SetPipeConsider(100, "a gnoll pup", 9)
	tr.SetPipePlayerSnapshot(54, 0, 0, 0) // first snapshot: zone change
	if tr.GetState().Consider != nil {
		t.Error("zone change should clear the reading")
	}
}

func TestConsider_StampsWhile(t *testing.T) {
	tr := pipeTracker()
	tr.SetConsiderStateProvider(func() string { return "invisible" })
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 5)
	if c := tr.GetState().Consider; c == nil || c.While != "invisible" {
		t.Errorf("Consider = %+v, want While=invisible", c)
	}
}

func TestConsider_NotOnCorpse(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 9)
	tr.SetPipeTarget("a gnoll pup's corpse")
	tr.SetPipeTargetID(intp(100))
	if tr.GetState().Consider != nil {
		t.Error("corpse shows a reading")
	}
}

func logCon(tr *NPCTracker, name string, b logparser.FactionBucket) {
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: name, Bucket: b},
	})
}

// No Zeal at all: the log /con sets the target and the reading, by name.
func TestConsider_LogOnly(t *testing.T) {
	tr := newTestTracker()
	logCon(tr, "a gnoll pup", logparser.BucketScowling)
	c := tr.GetState().Consider
	if c == nil || c.Faction != 9 || c.Source != live.SourceLog {
		t.Fatalf("Consider = %+v", c)
	}
	// Retarget by combat to another name drops it.
	tr.Handle(logparser.LogEvent{Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "You", Skill: "slash", Target: "a bat", Damage: 5}})
	if tr.GetState().Consider != nil {
		t.Error("log reading kept across a retarget")
	}
}

// Zeal without consider messages: a log /con keys to the current target id.
func TestConsider_LogKeyedToPipeTarget(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	logCon(tr, "a gnoll pup", logparser.BucketThreatening)
	if c := tr.GetState().Consider; c == nil || c.Faction != 8 {
		t.Fatalf("Consider = %+v", c)
	}
	pipeTarget(tr, "a gnoll pup", 101)
	if tr.GetState().Consider != nil {
		t.Error("log reading followed the name to another spawn")
	}
	// A log /con of something other than the target is ignored.
	logCon(tr, "a bat", logparser.BucketScowling)
	if tr.GetState().Consider != nil {
		t.Error("log reading of a non-target applied")
	}
}

// Once Zeal sends considers, the log line no longer feeds the overlay.
func TestConsider_PipePrimaryOverLog(t *testing.T) {
	tr := pipeTracker()
	pipeTarget(tr, "a gnoll pup", 100)
	tr.SetPipeConsider(100, "a gnoll pup", 7)
	logCon(tr, "a gnoll pup", logparser.BucketDubious)
	if c := tr.GetState().Consider; c == nil || c.Source != live.SourcePipe {
		t.Errorf("Consider = %+v, want the pipe reading", c)
	}
	tr.ResetPipeFields() // Zeal gone: back to the log
	tr.SetPipeConnected(false)
	logCon(tr, "a gnoll pup", logparser.BucketDubious)
	if c := tr.GetState().Consider; c == nil || c.Source != live.SourceLog {
		t.Errorf("Consider = %+v, want the log reading after a pipe reset", c)
	}
}

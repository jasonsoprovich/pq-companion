package factiontracker

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// considerEngine returns an engine resolving every NPC to Priests of Life,
// with a controllable clock and deferred callbacks collected in *deferred.
func considerEngine(t *testing.T) (*Engine, *time.Time, *[]func(), *int) {
	t.Helper()
	e := newEngine()
	e.SetPrimaryFactionResolver(func(string) (int, string, bool) { return 341, "Priests of Life", true })
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	e.now = func() time.Time { return now }
	var deferred []func()
	e.after = func(_ time.Duration, fn func()) { deferred = append(deferred, fn) }
	persists := 0
	e.SetPersistFunc(func(int, Tally) { persists++ })
	return e, &now, &deferred, &persists
}

func runDeferred(d *[]func()) {
	fns := *d
	*d = nil
	for _, fn := range fns {
		fn()
	}
}

func TestPipeConsider_MapsAllLevels(t *testing.T) {
	want := map[int]logparser.FactionBucket{
		1: logparser.BucketAlly, 2: logparser.BucketWarmly, 3: logparser.BucketKindly,
		4: logparser.BucketAmiable, 5: logparser.BucketIndifferent, 6: logparser.BucketApprehensive,
		7: logparser.BucketDubious, 8: logparser.BucketThreatening, 9: logparser.BucketScowling,
	}
	for level, bucket := range want {
		e, _, _, _ := considerEngine(t)
		e.HandlePipeConsider(100, "a priest", level, time.Now())
		if got := tallyFor(t, e.State(), "Priests of Life").LastBucket; got != string(bucket) {
			t.Errorf("level %d: LastBucket = %q, want %q", level, got, bucket)
		}
	}
}

func TestPipeConsider_IgnoresUnknownLevels(t *testing.T) {
	e, _, _, persists := considerEngine(t)
	for _, level := range []int{0, 10, -1} {
		e.HandlePipeConsider(100, "a priest", level, time.Now())
	}
	if len(e.State().Tallies) != 0 || *persists != 0 {
		t.Errorf("unknown levels recorded: %+v", e.State().Tallies)
	}
}

// Without any pipe consider the log /con path works exactly as before.
func TestLogConsider_NoPipe_AppliesImmediately(t *testing.T) {
	e, _, deferred, persists := considerEngine(t)
	feedConsidered(e, "a priest", logparser.BucketDubious, time.Now())
	if len(*deferred) != 0 || *persists != 1 {
		t.Fatalf("deferred=%d persists=%d, want immediate", len(*deferred), *persists)
	}
	if got := tallyFor(t, e.State(), "Priests of Life").LastBucket; got != "dubious" {
		t.Errorf("LastBucket = %q", got)
	}
}

// Pipe event first, then the log line for the same /con: one reading.
func TestConsiderDedupe_PipeThenLog(t *testing.T) {
	e, now, deferred, persists := considerEngine(t)
	e.HandlePipeConsider(100, "a priest", 7, time.Now())
	*now = now.Add(800 * time.Millisecond)
	feedConsidered(e, "a priest", logparser.BucketDubious, time.Now())
	runDeferred(deferred)
	if *persists != 1 {
		t.Errorf("persists = %d, want 1 (log line dropped)", *persists)
	}
}

// Log line first while the pipe is primary: it waits, and the pipe event
// replaces it.
func TestConsiderDedupe_LogThenPipe(t *testing.T) {
	e, now, deferred, persists := considerEngine(t)
	e.HandlePipeConsider(99, "a gnoll", 9, time.Now()) // pipe is now primary
	*now = now.Add(10 * time.Second)
	feedConsidered(e, "a priest", logparser.BucketDubious, time.Now())
	if *persists != 1 || len(*deferred) != 1 {
		t.Fatalf("log reading should be held: persists=%d deferred=%d", *persists, len(*deferred))
	}
	*now = now.Add(500 * time.Millisecond)
	e.HandlePipeConsider(100, "a priest", 7, time.Now())
	runDeferred(deferred)
	if *persists != 2 {
		t.Errorf("persists = %d, want 2 (gnoll + pipe priest, held log dropped)", *persists)
	}
}

// The pipe event never arrives: the held log reading is applied.
func TestConsiderDedupe_PipeMissing_FallsBackToLog(t *testing.T) {
	e, now, deferred, persists := considerEngine(t)
	e.HandlePipeConsider(99, "a gnoll", 9, time.Now())
	*now = now.Add(10 * time.Second)
	feedConsidered(e, "a priest", logparser.BucketKindly, time.Now())
	*now = now.Add(considerDedupeWindow)
	runDeferred(deferred)
	if *persists != 2 {
		t.Fatalf("persists = %d, want 2", *persists)
	}
	if got := tallyFor(t, e.State(), "Priests of Life").LastBucket; got != "kindly" {
		t.Errorf("LastBucket = %q, want kindly", got)
	}
}

func TestConsiderDedupe_ResetPipe(t *testing.T) {
	e, now, deferred, _ := considerEngine(t)
	e.HandlePipeConsider(99, "a gnoll", 9, time.Now())
	e.ResetPipe()
	*now = now.Add(time.Second)
	feedConsidered(e, "a gnoll", logparser.BucketKindly, time.Now())
	if len(*deferred) != 0 {
		t.Error("after a pipe reset the log should apply immediately")
	}
	if got := tallyFor(t, e.State(), "Priests of Life").LastBucket; got != "kindly" {
		t.Errorf("LastBucket = %q, want kindly", got)
	}
}

func TestConsider_SuspectProvider(t *testing.T) {
	e, _, _, _ := considerEngine(t)
	var got ConsiderContext
	e.SetConsiderSuspectProvider(func(c ConsiderContext) bool {
		got = c
		return c.Bucket == logparser.BucketIndifferent
	})
	e.HandlePipeConsider(100, "a priest", 5, time.Now())
	pol := tallyFor(t, e.State(), "Priests of Life")
	if !pol.LastConsiderSuspect {
		t.Error("want suspect")
	}
	if got.SpawnID != 100 || got.NPCName != "a priest" {
		t.Errorf("context = %+v", got)
	}
	e.HandlePipeConsider(100, "a priest", 9, time.Now())
	if tallyFor(t, e.State(), "Priests of Life").LastConsiderSuspect {
		t.Error("clean reading should clear suspect")
	}
	// Log path consults it too.
	e.ResetPipe()
	feedConsidered(e, "a priest", logparser.BucketIndifferent, time.Now())
	if !tallyFor(t, e.State(), "Priests of Life").LastConsiderSuspect || got.SpawnID != 0 {
		t.Errorf("log reading: suspect=%v ctx=%+v", tallyFor(t, e.State(), "Priests of Life").LastConsiderSuspect, got)
	}
}

func TestConsider_PetsDropped(t *testing.T) {
	e, _, _, persists := considerEngine(t)
	e.SetConsiderPetProvider(func(c ConsiderContext) bool { return c.NPCName == "Gobaner" || c.SpawnID == 42 })
	e.HandlePipeConsider(42, "a gnoll pup", 4, time.Now())
	feedConsidered(e, "Gobaner", logparser.BucketAmiable, time.Now())
	if len(e.State().Tallies) != 0 || *persists != 0 {
		t.Errorf("pet readings recorded: %+v", e.State().Tallies)
	}
}

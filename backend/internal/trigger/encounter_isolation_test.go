package trigger_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Arrival never depends on an expected owner/result. Every gate is released
// and every worker joined on assertion/setup failure as well as success.
type isolationGate struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
	armed   atomic.Bool
	taken   atomic.Bool
}

func newIsolationGate() *isolationGate {
	return &isolationGate{arrived: make(chan struct{}), release: make(chan struct{})}
}
func (g *isolationGate) provider() string {
	if g.armed.Load() && g.taken.CompareAndSwap(false, true) {
		close(g.arrived)
		<-g.release
	}
	return "Tester"
}
func (g *isolationGate) open() { g.once.Do(func() { close(g.release) }) }
func isolationAwait(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(4 * time.Second):
		t.Fatalf("%s did not complete", label)
	}
}
func isolationWorker(t *testing.T, g *isolationGate, fn func()) func() {
	t.Helper()
	done := make(chan struct{})
	t.Cleanup(func() { g.open(); isolationAwait(t, done, "cleanup worker join") })
	g.armed.Store(true)
	go func() { defer close(done); fn() }()
	isolationAwait(t, g.arrived, "owner-independent provider arrival")
	return func() { g.open(); isolationAwait(t, done, "worker join") }
}
func isolationOwners(t *testing.T, char func() string) (*trigger.Store, *trigger.Engine, *spelltimer.Engine) {
	t.Helper()
	s, err := trigger.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := ws.NewHub()
	timers := spelltimer.NewEngine(h, nil, nil, nil, nil, nil, nil, nil, nil)
	e := trigger.NewEngine(s, h, timers, char)
	return s, e, timers
}
func isolationDefinition(id string) *trigger.Trigger {
	return &trigger.Trigger{ID: id, Name: "Same", Enabled: true, Pattern: "^START_" + id + " (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$", WornOffPattern: "^STOP_" + id + " (?P<key>[A-Za-z]+) (?P<target>[A-Za-z]+)$", TimerType: trigger.TimerTypeCustom, TimerDurationSecs: 350, TimerKeyCapture: "key", TimerTargetCapture: "target", TimerIfIdle: true, TimerClearOnContextChange: true, Actions: []trigger.Action{{Type: trigger.ActionTextToSpeech, Text: "initial {target}"}}}
}
func isolationReload(t *testing.T, s *trigger.Store, e *trigger.Engine, tr *trigger.Trigger, kind string) {
	t.Helper()
	var err error
	switch kind {
	case "stop":
		e.Handle(time.Now(), "STOP_"+tr.ID+" Main Alpha")
		return
	case "disable":
		tr.Enabled = false
		err = s.Update(tr)
	case "delete":
		err = s.Delete(tr.ID)
	case "material":
		tr.TimerDurationSecs = 400
		err = s.Update(tr)
	case "cosmetic":
		tr.BarColor = "#f00"
		tr.Pinned = true
		err = s.Update(tr)
	case "aba":
		tr.Enabled = false
		err = s.Update(tr)
		if err == nil {
			e.Reload()
			tr.Enabled = true
			err = s.Update(tr)
		}
	default:
		t.Fatal("unknown mutation", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	e.Reload()
}
func isolationAssertStart(t *testing.T, e *trigger.Engine, timers *spelltimer.Engine, at time.Time, want bool) {
	t.Helper()
	rows := timers.GetState().Timers
	hist := e.GetHistory()
	if !want {
		if len(rows) != 0 || len(hist) != 0 {
			t.Fatalf("revoked pending start fired: rows=%+v history=%+v", rows, hist)
		}
		return
	}
	if len(rows) != 1 || len(hist) != 1 {
		t.Fatalf("unrelated retirement lost valid A admission/actions: rows=%+v history=%+v", rows, hist)
	}
	row := rows[0]
	if row.SpellName != "Main" || row.TargetName != "Alpha" || !row.StartsAt.Equal(at) || !row.ExpiresAt.Equal(at.Add(350*time.Second)) || hist[0].TriggerID != "A" || len(hist[0].Actions) != 1 || hist[0].Actions[0].Text != "initial Alpha" {
		t.Fatalf("wrong admitted identity/deadline/actions: row=%+v history=%+v", row, hist)
	}
}

func TestEncounterIsolationPendingStart(t *testing.T) {
	for _, where := range []string{"target", "character"} {
		for _, mutation := range []string{"stop", "disable", "delete", "material", "cosmetic", "aba"} {
			t.Run(where+"/unrelated_"+mutation, func(t *testing.T) {
				g := newIsolationGate()
				char := func() string { return "Tester" }
				if where == "character" {
					char = g.provider
				}
				s, e, timers := isolationOwners(t, char)
				a, b := isolationDefinition("A"), isolationDefinition("B")
				for _, tr := range []*trigger.Trigger{a, b} {
					if err := s.Insert(tr); err != nil {
						t.Fatal(err)
					}
				}
				e.Reload()
				if where == "target" {
					e.SetTargetProvider(g.provider)
				}
				at := time.Now()
				join := isolationWorker(t, g, func() { e.Handle(at, "START_A Main Alpha") })
				isolationReload(t, s, e, b, mutation)
				join()
				isolationAssertStart(t, e, timers, at, true)
			})
		}
	}
}
func TestEncounterIsolationStopIdentity(t *testing.T) {
	for _, where := range []string{"target", "character"} {
		for _, tc := range []struct {
			name, line string
			want       bool
		}{{"other_target", "STOP_A Main Beta", true}, {"other_key", "STOP_A Other Alpha", true}, {"matching", "STOP_A Main Alpha", false}} {
			t.Run(where+"/"+tc.name, func(t *testing.T) {
				g := newIsolationGate()
				char := func() string { return "Tester" }
				if where == "character" {
					char = g.provider
				}
				s, e, timers := isolationOwners(t, char)
				a := isolationDefinition("A")
				if err := s.Insert(a); err != nil {
					t.Fatal(err)
				}
				e.Reload()
				if where == "target" {
					e.SetTargetProvider(g.provider)
				}
				at := time.Now()
				join := isolationWorker(t, g, func() { e.Handle(at, "START_A Main Alpha") })
				e.Handle(time.Now(), tc.line)
				join()
				isolationAssertStart(t, e, timers, at, tc.want)
				if !tc.want {
					fresh := time.Now()
					e.Handle(fresh, "START_A Main Alpha")
					isolationAssertStart(t, e, timers, fresh, true)
				}
			})
		}
	}
}
func TestEncounterIsolationOwnerLifetime(t *testing.T) {
	for _, where := range []string{"target", "character"} {
		for _, mutation := range []string{"disable", "delete", "material", "cosmetic", "aba"} {
			t.Run(where+"/"+mutation, func(t *testing.T) {
				g := newIsolationGate()
				char := func() string { return "Tester" }
				if where == "character" {
					char = g.provider
				}
				s, e, timers := isolationOwners(t, char)
				a := isolationDefinition("A")
				if err := s.Insert(a); err != nil {
					t.Fatal(err)
				}
				e.Reload()
				if where == "target" {
					e.SetTargetProvider(g.provider)
				}
				at := time.Now()
				join := isolationWorker(t, g, func() { e.Handle(at, "START_A Main Alpha") })
				isolationReload(t, s, e, a, mutation)
				join()
				isolationAssertStart(t, e, timers, at, mutation == "cosmetic")
			})
		}
	}
}
func TestEncounterIsolationPendingStop(t *testing.T) {
	for _, mutation := range []string{"disable", "delete", "material", "cosmetic", "aba"} {
		t.Run("unrelated_"+mutation, func(t *testing.T) {
			g := newIsolationGate()
			s, e, timers := isolationOwners(t, g.provider)
			a, b := isolationDefinition("A"), isolationDefinition("B")
			for _, tr := range []*trigger.Trigger{a, b} {
				if err := s.Insert(tr); err != nil {
					t.Fatal(err)
				}
			}
			e.Reload()
			at := time.Now()
			e.Handle(at, "START_A Main Alpha")
			isolationAssertStart(t, e, timers, at, true)
			join := isolationWorker(t, g, func() { e.Handle(time.Now(), "STOP_A Main Alpha") })
			isolationReload(t, s, e, b, mutation)
			join()
			if len(timers.GetState().Timers) != 0 || len(e.GetHistory()) != 1 {
				t.Fatalf("unrelated reload dropped valid A stop: %+v", timers.GetState())
			}
			fresh := time.Now()
			e.Handle(fresh, "START_A Main Alpha")
			if len(timers.GetState().Timers) != 1 || len(e.GetHistory()) != 2 {
				t.Fatal("stop failed to rearm next A attempt")
			}
		})
	}
	// A stop snapshot from a retired lifetime must not remove its ABA successor.
	t.Run("same_owner_aba", func(t *testing.T) {
		g := newIsolationGate()
		s, e, timers := isolationOwners(t, g.provider)
		a := isolationDefinition("A")
		if err := s.Insert(a); err != nil {
			t.Fatal(err)
		}
		e.Reload()
		e.Handle(time.Now(), "START_A Main Alpha")
		join := isolationWorker(t, g, func() { e.Handle(time.Now(), "STOP_A Main Alpha") })
		isolationReload(t, s, e, a, "aba")
		e.Handle(time.Now(), "START_A Main Alpha")
		join()
		if len(timers.GetState().Timers) != 1 || len(e.GetHistory()) != 2 {
			t.Fatal("retired stop removed successor", timers.GetState())
		}
	})
}
func TestEncounterIsolationContext(t *testing.T) {
	for _, where := range []string{"target", "character"} {
		for _, retire := range []string{"context", "source", "zone", "replay"} {
			t.Run(where+"/"+retire, func(t *testing.T) {
				g := newIsolationGate()
				char := func() string { return "Tester" }
				if where == "character" {
					char = g.provider
				}
				s, e, timers := isolationOwners(t, char)
				a := isolationDefinition("A")
				if err := s.Insert(a); err != nil {
					t.Fatal(err)
				}
				e.Reload()
				e.SourceChanged(1)
				if where == "target" {
					e.SetTargetProvider(g.provider)
				}
				at := time.Now()
				join := isolationWorker(t, g, func() { e.HandleSource(at, "START_A Main Alpha", 1) })
				switch retire {
				case "context":
					e.RetireContext()
				case "source":
					e.SourceChanged(2)
				case "zone":
					e.Handle(time.Now(), "You have entered The North Karana.")
				case "replay":
					e.SetReplaying(true)
				}
				join()
				isolationAssertStart(t, e, timers, at, false)
				generation := uint64(1)
				if retire == "source" {
					generation = 2
					e.HandleSource(time.Now(), "START_A Main Alpha", 1)
					isolationAssertStart(t, e, timers, at, false)
				}
				fresh := time.Now()
				e.HandleSource(fresh, "START_A Main Alpha", generation)
				isolationAssertStart(t, e, timers, fresh, true)
			})
		}
	}
}

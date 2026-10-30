package trigger_test

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

func TestEncounterIsolationEffectfulDraftStop(t *testing.T) {
	for _, where := range []string{"target", "character"} {
		for _, tc := range []struct {
			name, owner, line string
			effects, want     bool
		}{
			{"other_owner", "B", "STOP_B Main Alpha", true, true},
			{"other_target", "A", "STOP_A Main Beta", true, true},
			{"other_key", "A", "STOP_A Other Alpha", true, true},
			{"matching", "A", "STOP_A Main Alpha", true, false},
			{"silent_matching", "A", "STOP_A Main Alpha", false, true},
		} {
			t.Run(where+"/"+tc.name, func(t *testing.T) {
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
				draft := a
				if tc.owner == "B" {
					draft = b
				}
				report := e.RunTest(trigger.TestRequest{Trigger: draft, Lines: tc.line, Character: "Tester", FireEffects: tc.effects})
				wantFired := 0
				if tc.effects {
					wantFired = 1
				}
				if report.Fired != wantFired {
					t.Fatalf("wrong draft stop report: %+v", report)
				}
				join()
				isolationAssertStart(t, e, timers, at, tc.want)
			})
		}
	}
}

func TestEncounterIsolationEffectfulDraftPreservesPendingStop(t *testing.T) {
	for _, tc := range []struct{ name, owner, line string }{
		{"other_owner", "B", "STOP_B Main Alpha"},
		{"other_target", "A", "STOP_A Main Beta"},
		{"other_key", "A", "STOP_A Other Alpha"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newIsolationGate()
			s, e, timers := isolationOwners(t, g.provider)
			a, b := isolationDefinition("A"), isolationDefinition("B")
			for _, tr := range []*trigger.Trigger{a, b} {
				if err := s.Insert(tr); err != nil {
					t.Fatal(err)
				}
			}
			e.Reload()
			e.Handle(time.Now(), "START_A Main Alpha")
			if len(timers.GetState().Timers) != 1 {
				t.Fatal("missing active A control")
			}
			join := isolationWorker(t, g, func() { e.Handle(time.Now(), "STOP_A Main Alpha") })
			draft := a
			if tc.owner == "B" {
				draft = b
			}
			e.RunTest(trigger.TestRequest{Trigger: draft, Lines: tc.line, Character: "Tester", FireEffects: true})
			join()
			if len(timers.GetState().Timers) != 0 {
				t.Fatal("unrelated effectful draft lost valid A stop; row orphaned")
			}
		})
	}
}

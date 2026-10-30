package trigger_test

import (
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"sync/atomic"
	"testing"
	"time"
)

func TestEncounterRetirementRevokesPendingStart(t *testing.T) {
	for _, retire := range []string{"stop", "disable", "delete", "material_edit", "replay"} {
		t.Run(retire, func(t *testing.T) {
			s, e, timers := encounterOwners(t)
			tr := definition("one", true, true)
			if err := s.Insert(tr); err != nil {
				t.Fatal(err)
			}
			e.Reload()
			arrived := make(chan struct{})
			release := make(chan struct{})
			done := make(chan struct{})
			var once atomic.Bool
			e.SetTargetProvider(func() string {
				if once.CompareAndSwap(false, true) {
					close(arrived)
					<-release
				}
				return ""
			})
			go func() { defer close(done); e.Handle(time.Now(), "START") }()
			<-arrived
			switch retire {
			case "stop":
				e.Handle(time.Now(), "STOP")
			case "disable":
				tr.Enabled = false
				if err := s.Update(tr); err != nil {
					close(release)
					t.Fatal(err)
				}
				e.Reload()
			case "delete":
				if err := s.Delete(tr.ID); err != nil {
					close(release)
					t.Fatal(err)
				}
				e.Reload()
			case "material_edit":
				tr.TimerDurationSecs = 100
				if err := s.Update(tr); err != nil {
					close(release)
					t.Fatal(err)
				}
				e.Reload()
			case "replay":
				e.SetReplaying(true)
			}
			close(release)
			<-done
			if len(timers.GetState().Timers) != 0 || len(e.GetHistory()) != 0 {
				t.Fatal("retired pending start fired")
			}
			if retire == "disable" {
				tr.Enabled = true
				if err := s.Update(tr); err != nil {
					t.Fatal(err)
				}
				e.Reload()
			}
			if retire == "delete" {
				if err := s.Insert(tr); err != nil {
					t.Fatal(err)
				}
				e.Reload()
			}
			if retire == "replay" {
				e.SetReplaying(false)
			}
			e.Handle(time.Now(), "START")
			if len(timers.GetState().Timers) != 1 {
				t.Fatal("legitimate next attempt blocked")
			}
		})
	}
}
func TestEncounterNegativeAndFreshOwner(t *testing.T) {
	s, e, timers := encounterOwners(t)
	tr := definition("one", true, true)
	tr.Characters = []string{"Other"}
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(time.Now(), "START")
	if len(timers.GetState().Timers) != 0 {
		t.Fatal("wrong character admitted")
	}
	tr.Characters = []string{"Tester"}
	if err := s.Update(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	for _, line := range []string{"STOP", "Player says, 'START'", "Player says, 'STOP'", "unrelated"} {
		e.Handle(time.Now(), line)
	}
	if len(timers.GetState().Timers) != 0 || len(e.GetHistory()) != 0 {
		t.Fatal("negative/no-prior-start created timer")
	}
	e.Handle(time.Now(), "START")
	e.Handle(time.Now(), "Player says, 'STOP'")
	if len(timers.GetState().Timers) != 1 {
		t.Fatal("quoted stop retired row")
	}
	// A process restart creates no deadline until a real new start, never a
	// persisted Boolean latch reconstructed without a real timer row.
	_, fresh, newTimers := encounterOwners(t)
	report := fresh.RunTest(trigger.TestRequest{Trigger: tr, Lines: "START", FireEffects: false})
	if report.Fired != 0 || len(newTimers.GetState().Timers) != 0 {
		t.Fatal("fresh owner manufactured deadline")
	}
}

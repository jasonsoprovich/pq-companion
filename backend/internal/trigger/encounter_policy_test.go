package trigger_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

func definition(id string, idle, clear bool) *trigger.Trigger {
	return &trigger.Trigger{ID: id, Name: "Same", Enabled: true, Pattern: "^START(?: ([A-Za-z]+))?$", WornOffPattern: "^STOP(?: ([A-Za-z]+))?$", TimerType: trigger.TimerTypeCustom, TimerDurationSecs: 350, TimerTargetCapture: "1", TimerIfIdle: idle, TimerClearOnContextChange: clear}
}
func TestEncounterOwnershipOptionsAndReload(t *testing.T) {
	s, e, timers := encounterOwners(t)
	a := definition("a", true, true)
	b := definition("b", true, false)
	for _, tr := range []*trigger.Trigger{a, b} {
		if err := s.Insert(tr); err != nil {
			t.Fatal(err)
		}
	}
	e.Reload()
	now := time.Now()
	timers.StartExternal("Same", "custom", 900, 0, now, nil, 0, "", "", false, "", false)
	e.Handle(now, "START Alpha")
	e.Handle(now, "START Beta")
	if len(timers.GetState().Timers) != 5 {
		t.Fatal("same-name owner/target collision", timers.GetState())
	}
	e.Handle(now, "STOP Alpha")
	if len(timers.GetState().Timers) != 3 {
		t.Fatal("stop crossed target or legacy ownership")
	}
	e.Handle(now, "You have entered The North Karana.")
	if len(timers.GetState().Timers) != 2 {
		t.Fatal("context retirement crossed unopted ownership", timers.GetState())
	}
	e.Handle(now, "START Alpha")
	rows := timers.GetState().Timers
	a.BarColor = "#f00"
	a.Pinned = true
	if err := s.Update(a); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if len(timers.GetState().Timers) != len(rows) {
		t.Fatal("cosmetic reload retired row")
	}
	a.TimerDurationSecs = 400
	if err := s.Update(a); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if len(timers.GetState().Timers) != 3 {
		t.Fatal("material edit orphaned rows", timers.GetState())
	}
	b.Enabled = false
	if err := s.Update(b); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if len(timers.GetState().Timers) != 1 {
		t.Fatal("disable orphaned rows")
	}
	a.Enabled = true
	if err := s.Update(a); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(now, "START")
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if len(timers.GetState().Timers) != 1 {
		t.Fatal("delete orphaned row")
	}
}
func TestEncounterIndependentFlags(t *testing.T) {
	for _, idle := range []bool{false, true} {
		for _, clear := range []bool{false, true} {
			t.Run(fmt.Sprint(idle, clear), func(t *testing.T) {
				s, e, timers := encounterOwners(t)
				tr := definition("one", idle, clear)
				if err := s.Insert(tr); err != nil {
					t.Fatal(err)
				}
				e.Reload()
				now := time.Now().Add(-10 * time.Second)
				e.Handle(now, "START")
				e.Handle(now.Add(5*time.Second), "START")
				want := 2
				if idle {
					want = 1
				}
				if len(e.GetHistory()) != want {
					t.Fatal("incorrect duplicate policy")
				}
				e.Handle(time.Now(), "You have entered The North Karana.")
				count := 1
				if clear {
					count = 0
				}
				if len(timers.GetState().Timers) != count {
					t.Fatal("incorrect context policy")
				}
			})
		}
	}
}
func TestEncounterConcurrentAdmissionAndStalePlan(t *testing.T) {
	s, e, timers := encounterOwners(t)
	tr := definition("one", true, true)
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() { defer wg.Done(); e.Handle(time.Now(), "START") }()
	}
	wg.Wait()
	if len(e.GetHistory()) != 1 || len(timers.GetState().Timers) != 1 {
		t.Fatal("admission was not atomic")
	}
	e.RetireContext()
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
	e.RetireContext()
	close(release)
	<-done
	if len(e.GetHistory()) != 1 || len(timers.GetState().Timers) != 0 {
		t.Fatal("retired in-flight plan regained authority")
	}
	e.Handle(time.Now(), "START")
	if len(timers.GetState().Timers) != 1 {
		t.Fatal("fresh attempt rejected")
	}
	e.SourceChanged(4)
	e.HandleSource(time.Now(), "START", 3)
	e.SourceChanged(2)
	if len(timers.GetState().Timers) != 0 {
		t.Fatal("stale source regained authority")
	}
	e.HandleSource(time.Now(), "START", 4)
	if len(timers.GetState().Timers) != 1 {
		t.Fatal("fresh source rejected")
	}
}
func TestEncounterExpiryAndSilentDraft(t *testing.T) {
	s, e, timers := encounterOwners(t)
	tr := definition("one", true, true)
	tr.TimerDurationSecs = .01
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(time.Now(), "START")
	deadline := timers.GetState().Timers[0].ExpiresAt
	for time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.Handle(time.Now(), "START")
	if len(e.GetHistory()) != 2 {
		t.Fatal("expiry did not immediately rearm")
	}
	before := timers.GetState().Timers[0].ExpiresAt
	report := e.RunTest(trigger.TestRequest{Lines: "START\nSTOP\nSTART", Trigger: tr, FireEffects: false})
	if report.Fired != 0 || len(e.GetHistory()) != 2 || !timers.GetState().Timers[0].ExpiresAt.Equal(before) {
		t.Fatal("silent draft touched live owner")
	}
	info := report.Lines[0].Matches[0].Timer
	data, _ := json.Marshal(info)
	var fields map[string]any
	json.Unmarshal(data, &fields)
	if fields["timer_if_idle"] != true || fields["timer_clear_on_context_change"] != true {
		t.Fatal("draft policy missing", string(data))
	}
}

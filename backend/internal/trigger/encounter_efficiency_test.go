package trigger

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

func TestEncounterEfficiencyNoWorkAllocations(t *testing.T) {
	for _, kind := range []string{"legacy", "managed_no_match", "stale"} {
		t.Run(kind, func(t *testing.T) {
			e := NewEngine(nil, nil, nil, nil)
			for i := 0; i < 200; i++ {
				tr := &Trigger{ID: fmt.Sprint(i), Name: "Timer", TimerType: TimerTypeCustom, TimerIfIdle: kind != "legacy", TimerClearOnContextChange: kind != "legacy"}
				c := compiled{trigger: tr, re: regexp.MustCompile(`^START ([A-Za-z]+)$`), timerKey: "Timer"}
				if kind != "legacy" {
					c.lifetime = &managedLifetime{}
				}
				e.compiled = append(e.compiled, c)
			}
			ts := time.Now()
			message := "A yard trash NPC hits Someone for 50 points of damage."
			allocs := testing.AllocsPerRun(100, func() {
				if kind == "stale" {
					e.HandleSource(ts, message, 1)
				} else {
					e.Handle(ts, message)
				}
			})
			if allocs != 0 {
				t.Fatalf("no managed work allocated %.0f objects per line; want zero", allocs)
			}
			if len(e.GetHistory()) != 0 {
				t.Fatal("negative workload fired an action")
			}
			for _, c := range e.compiled {
				if c.lifetime != nil && len(c.lifetime.pending) != 0 {
					t.Fatal("negative workload retained a pending plan")
				}
			}
		})
	}
}

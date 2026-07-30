package chchain

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// fakeInterruptObserver records the casters InterruptWatcher extracts from
// log lines. The un-confirm behavior lives in the spelltimer engine and is
// exercised there; this file only covers line parsing and the settings
// gates.
type fakeInterruptObserver struct{ casters []string }

func (f *fakeInterruptObserver) NoteCastInterrupted(caster string, _ time.Time) {
	f.casters = append(f.casters, caster)
}

func newInterruptWatcher(o InterruptObserver, enabled, possibleMissEnabled, interruptEnabled bool) *InterruptWatcher {
	return NewInterruptWatcher(o, func() config.CHChainSettings {
		return config.CHChainSettings{
			Enabled:                   enabled,
			PossibleMissEnabled:       possibleMissEnabled,
			InterruptDetectionEnabled: interruptEnabled,
		}
	})
}

func TestInterruptWatcher_ReportsBystanderInterrupt(t *testing.T) {
	o := &fakeInterruptObserver{}
	w := newInterruptWatcher(o, true, true, true)

	w.HandleLine(time.Unix(1, 0), "Soandso's casting is interrupted!")
	if len(o.casters) != 1 || o.casters[0] != "Soandso" {
		t.Fatalf("casters = %v, want [Soandso]", o.casters)
	}
}

// TestInterruptWatcher_ReportsOwnInterruptAsYou pins the caster name the
// local player's own interrupt line reports, matching CastWatcher's "You"
// convention (see TestCastWatcher_ReportsOwnBeginCastAsYou) so the two agree
// on the local healer's own chain row.
func TestInterruptWatcher_ReportsOwnInterruptAsYou(t *testing.T) {
	o := &fakeInterruptObserver{}
	w := newInterruptWatcher(o, true, true, true)

	for _, line := range []string{
		"Your spell is interrupted.",
		"Your Complete Healing spell is interrupted.",
	} {
		o.casters = nil
		w.HandleLine(time.Unix(1, 0), line)
		if len(o.casters) != 1 || o.casters[0] != "You" {
			t.Fatalf("line %q: casters = %v, want [You]", line, o.casters)
		}
	}
}

func TestInterruptWatcher_IgnoresUnrelatedLines(t *testing.T) {
	o := &fakeInterruptObserver{}
	w := newInterruptWatcher(o, true, true, true)

	for _, line := range []string{
		"Soandso begins to cast a spell.",           // cast-begin, not interrupt
		"Soandso says, \"casting is interrupted!\"", // not the bystander form
		"Your spell did not take hold.",             // resist/overwrite, not an interrupt
		"Your target resisted the Mesmerization spell.",
	} {
		w.HandleLine(time.Unix(1, 0), line)
	}
	if len(o.casters) != 0 {
		t.Errorf("casters = %v, want none", o.casters)
	}
}

func TestInterruptWatcher_GatedBySettings(t *testing.T) {
	line := "Soandso's casting is interrupted!"

	o := &fakeInterruptObserver{}
	newInterruptWatcher(o, false, true, true).HandleLine(time.Unix(1, 0), line)
	if len(o.casters) != 0 {
		t.Errorf("chain disabled: casters = %v, want none", o.casters)
	}

	o2 := &fakeInterruptObserver{}
	newInterruptWatcher(o2, true, false, true).HandleLine(time.Unix(1, 0), line)
	if len(o2.casters) != 0 {
		t.Errorf("possible-miss disabled: casters = %v, want none", o2.casters)
	}

	o3 := &fakeInterruptObserver{}
	newInterruptWatcher(o3, true, true, false).HandleLine(time.Unix(1, 0), line)
	if len(o3.casters) != 0 {
		t.Errorf("interrupt detection off (default): casters = %v, want none", o3.casters)
	}
}

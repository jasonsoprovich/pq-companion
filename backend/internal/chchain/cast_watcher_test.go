package chchain

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// fakeObserver records the casters CastWatcher extracts from log lines. The
// pairing of a caster back to a chain callout lives in Matcher (exercised in
// matcher_test.go); this file only covers the line parsing and the settings
// gates.
type fakeObserver struct{ casters []string }

func (f *fakeObserver) NoteCastBegin(caster string, _ time.Time) {
	f.casters = append(f.casters, caster)
}

func newCastWatcher(o CastObserver, enabled, possibleMissEnabled bool) *CastWatcher {
	return NewCastWatcher(o, func() config.CHChainSettings {
		return config.CHChainSettings{
			Enabled:             enabled,
			PossibleMissEnabled: possibleMissEnabled,
		}
	})
}

func TestCastWatcher_ReportsBystanderBeginCast(t *testing.T) {
	o := &fakeObserver{}
	w := newCastWatcher(o, true, true)

	w.HandleLine(time.Unix(1, 0), "Soandso begins to cast a spell.")
	if len(o.casters) != 1 || o.casters[0] != "Soandso" {
		t.Fatalf("casters = %v, want [Soandso]", o.casters)
	}
}

// TestCastWatcher_ReportsOwnBeginCastAsYou pins the caster name the local
// player's own cast-begin line reports: Matcher records the player's own chain
// calls under the literal caster "You" (the default pattern's second-person
// branch), so the two must agree or the local healer's own row would never be
// confirmed.
func TestCastWatcher_ReportsOwnBeginCastAsYou(t *testing.T) {
	o := &fakeObserver{}
	w := newCastWatcher(o, true, true)

	w.HandleLine(time.Unix(1, 0), "You begin casting Complete Healing.")
	if len(o.casters) != 1 || o.casters[0] != "You" {
		t.Fatalf("casters = %v, want [You]", o.casters)
	}
}

func TestCastWatcher_IgnoresUnrelatedLines(t *testing.T) {
	o := &fakeObserver{}
	w := newCastWatcher(o, true, true)

	for _, line := range []string{
		"Soandso begins casting a spell.",        // wrong verb form
		"Krayziefoo is completely healed.",       // old landed-text mechanism, no longer watched
		"Soandso says, 'begins to cast a spell'", // not the bystander form
		"You begin casting.",                     // no spell name, doesn't match the self pattern
	} {
		w.HandleLine(time.Unix(1, 0), line)
	}
	if len(o.casters) != 0 {
		t.Errorf("casters = %v, want none", o.casters)
	}
}

func TestCastWatcher_DisabledOrMissDetectionOff(t *testing.T) {
	line := "Soandso begins to cast a spell."

	o := &fakeObserver{}
	newCastWatcher(o, false, true).HandleLine(time.Unix(1, 0), line)
	if len(o.casters) != 0 {
		t.Errorf("chain disabled: casters = %v, want none", o.casters)
	}

	o2 := &fakeObserver{}
	newCastWatcher(o2, true, false).HandleLine(time.Unix(1, 0), line)
	if len(o2.casters) != 0 {
		t.Errorf("possible-miss disabled: casters = %v, want none", o2.casters)
	}
}

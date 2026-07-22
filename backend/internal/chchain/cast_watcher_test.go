package chchain

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type fakeHealSink struct{ confirmed []string }

func (f *fakeHealSink) ConfirmHeal(targetName string) {
	f.confirmed = append(f.confirmed, targetName)
}

type fakeLookup struct {
	target string
	ok     bool
}

func (f fakeLookup) TargetForCaster(string, time.Time) (string, bool) {
	return f.target, f.ok
}

func newCastWatcher(s HealSink, lookup CasterLookup, enabled, possibleMissEnabled bool) *CastWatcher {
	return NewCastWatcher(s, lookup, func() config.CHChainSettings {
		return config.CHChainSettings{
			Enabled:             enabled,
			PossibleMissEnabled: possibleMissEnabled,
		}
	})
}

func TestCastWatcher_ConfirmsOnBystanderBeginCast(t *testing.T) {
	s := &fakeHealSink{}
	w := newCastWatcher(s, fakeLookup{target: "Krayziefoo", ok: true}, true, true)

	w.HandleLine(time.Unix(1, 0), "Soandso begins to cast a spell.")
	if len(s.confirmed) != 1 || s.confirmed[0] != "Krayziefoo" {
		t.Fatalf("confirmed = %v, want [Krayziefoo]", s.confirmed)
	}
}

func TestCastWatcher_ConfirmsOnOwnBeginCast(t *testing.T) {
	s := &fakeHealSink{}
	w := newCastWatcher(s, fakeLookup{target: "Krayziefoo", ok: true}, true, true)

	w.HandleLine(time.Unix(1, 0), "You begin casting Complete Healing.")
	if len(s.confirmed) != 1 || s.confirmed[0] != "Krayziefoo" {
		t.Fatalf("confirmed = %v, want [Krayziefoo]", s.confirmed)
	}
}

// TestCastWatcher_NoLookupMatch guards the case where the caster began
// casting something but has no recent chain callout on record (an unrelated
// player casting nearby, or a callout that already fell outside the
// correlation window) — must not confirm anything.
func TestCastWatcher_NoLookupMatch(t *testing.T) {
	s := &fakeHealSink{}
	w := newCastWatcher(s, fakeLookup{ok: false}, true, true)

	w.HandleLine(time.Unix(1, 0), "Soandso begins to cast a spell.")
	if len(s.confirmed) != 0 {
		t.Errorf("confirmed = %v, want none", s.confirmed)
	}
}

func TestCastWatcher_IgnoresUnrelatedLines(t *testing.T) {
	s := &fakeHealSink{}
	w := newCastWatcher(s, fakeLookup{target: "Krayziefoo", ok: true}, true, true)

	for _, line := range []string{
		"Soandso begins casting a spell.",        // wrong verb form
		"Krayziefoo is completely healed.",       // old landed-text mechanism, no longer watched
		"Soandso says, 'begins to cast a spell'", // not the bystander form
		"You begin casting.",                     // no spell name, doesn't match the self pattern
	} {
		w.HandleLine(time.Unix(1, 0), line)
	}
	if len(s.confirmed) != 0 {
		t.Errorf("confirmed = %v, want none", s.confirmed)
	}
}

func TestCastWatcher_DisabledOrMissDetectionOff(t *testing.T) {
	line := "Soandso begins to cast a spell."
	lookup := fakeLookup{target: "Krayziefoo", ok: true}

	s := &fakeHealSink{}
	newCastWatcher(s, lookup, false, true).HandleLine(time.Unix(1, 0), line)
	if len(s.confirmed) != 0 {
		t.Errorf("chain disabled: confirmed = %v, want none", s.confirmed)
	}

	s2 := &fakeHealSink{}
	newCastWatcher(s2, lookup, true, false).HandleLine(time.Unix(1, 0), line)
	if len(s2.confirmed) != 0 {
		t.Errorf("possible-miss disabled: confirmed = %v, want none", s2.confirmed)
	}
}

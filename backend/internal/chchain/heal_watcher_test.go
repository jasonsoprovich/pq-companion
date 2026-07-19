package chchain

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type fakeHealSink struct{ confirmed []string }

func (f *fakeHealSink) ConfirmHeal(targetName string) {
	f.confirmed = append(f.confirmed, targetName)
}

func newHealWatcher(s HealSink, enabled, possibleMissEnabled, includeDruid bool) *HealWatcher {
	return NewHealWatcher(s, func() config.CHChainSettings {
		return config.CHChainSettings{
			Enabled:                  enabled,
			PossibleMissEnabled:      possibleMissEnabled,
			PossibleMissIncludeDruid: includeDruid,
		}
	})
}

func TestHealWatcher_MatchesCompleteHealingLanded(t *testing.T) {
	s := &fakeHealSink{}
	w := newHealWatcher(s, true, true, false)

	w.HandleLine("Krayziefoo is completely healed.")
	if len(s.confirmed) != 1 || s.confirmed[0] != "Krayziefoo" {
		t.Fatalf("confirmed = %v, want [Krayziefoo]", s.confirmed)
	}
}

// TestHealWatcher_IgnoresUnrelatedHeals guards the default scope: with the
// Druid opt-in off, only Complete Healing's exact bystander text is watched.
// Superior Healing's "feels much better." (and any other heal spell's landed
// text) must NOT confirm a chain slot — that text is shared by over a dozen
// unrelated heal spells, so treating it as confirmation would let an
// off-chain healer mask an actual missed CH cast.
func TestHealWatcher_IgnoresUnrelatedHeals(t *testing.T) {
	s := &fakeHealSink{}
	w := newHealWatcher(s, true, true, false)

	for _, line := range []string{
		"Krayziefoo feels much better.",            // Superior Healing / many others — opt-in off
		"Krayziefoo is bathed in healing water.",   // Healing Water
		"Krayziefoo says, 'is completely healed.'", // not the bystander form
		"You are completely healed.",               // cast_on_you, not cast_on_other
		"is completely healed.",                    // no leading name
	} {
		w.HandleLine(line)
	}
	if len(s.confirmed) != 0 {
		t.Errorf("confirmed = %v, want none", s.confirmed)
	}
}

// TestHealWatcher_SuperiorHealingOnlyWhenOptedIn checks both sides of the
// PossibleMissIncludeDruid gate: off (default) ignores Superior Healing's
// landed text entirely; on, it confirms just like Complete Healing does,
// but other unrelated heal spells sharing the same text still confirm too
// (that's the accepted noise tradeoff of opting in) while genuinely
// different heal text still doesn't match.
func TestHealWatcher_SuperiorHealingOnlyWhenOptedIn(t *testing.T) {
	line := "Krayziefoo feels much better."

	off := &fakeHealSink{}
	newHealWatcher(off, true, true, false).HandleLine(line)
	if len(off.confirmed) != 0 {
		t.Errorf("opt-in off: confirmed = %v, want none", off.confirmed)
	}

	on := &fakeHealSink{}
	newHealWatcher(on, true, true, true).HandleLine(line)
	if len(on.confirmed) != 1 || on.confirmed[0] != "Krayziefoo" {
		t.Errorf("opt-in on: confirmed = %v, want [Krayziefoo]", on.confirmed)
	}

	unrelated := &fakeHealSink{}
	newHealWatcher(unrelated, true, true, true).HandleLine("Krayziefoo is bathed in healing water.")
	if len(unrelated.confirmed) != 0 {
		t.Errorf("still-unmatched text: confirmed = %v, want none", unrelated.confirmed)
	}
}

func TestHealWatcher_DisabledOrMissDetectionOff(t *testing.T) {
	line := "Krayziefoo is completely healed."

	s := &fakeHealSink{}
	newHealWatcher(s, false, true, false).HandleLine(line)
	if len(s.confirmed) != 0 {
		t.Errorf("chain disabled: confirmed = %v, want none", s.confirmed)
	}

	s2 := &fakeHealSink{}
	newHealWatcher(s2, true, false, false).HandleLine(line)
	if len(s2.confirmed) != 0 {
		t.Errorf("possible-miss disabled: confirmed = %v, want none", s2.confirmed)
	}

	// PossibleMissEnabled=false must also suppress the Druid opt-in path,
	// not just the Complete Healing one.
	s3 := &fakeHealSink{}
	newHealWatcher(s3, true, false, true).HandleLine("Krayziefoo feels much better.")
	if len(s3.confirmed) != 0 {
		t.Errorf("possible-miss disabled (druid opt-in on): confirmed = %v, want none", s3.confirmed)
	}
}

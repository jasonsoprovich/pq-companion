package chchain

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type fakeHealSink struct{ confirmed []string }

func (f *fakeHealSink) ConfirmHeal(targetName string) {
	f.confirmed = append(f.confirmed, targetName)
}

func newHealWatcher(s HealSink, enabled, possibleMissEnabled bool) *HealWatcher {
	return NewHealWatcher(s, func() config.CHChainSettings {
		return config.CHChainSettings{Enabled: enabled, PossibleMissEnabled: possibleMissEnabled}
	})
}

func TestHealWatcher_MatchesCompleteHealingLanded(t *testing.T) {
	s := &fakeHealSink{}
	w := newHealWatcher(s, true, true)

	w.HandleLine("Krayziefoo is completely healed.")
	if len(s.confirmed) != 1 || s.confirmed[0] != "Krayziefoo" {
		t.Fatalf("confirmed = %v, want [Krayziefoo]", s.confirmed)
	}
}

// TestHealWatcher_IgnoresUnrelatedHeals guards the deliberate scope decision:
// only Complete Healing's exact bystander text is watched. Superior
// Healing's "feels much better." (and any other heal spell's landed text)
// must NOT confirm a chain slot — that text is shared by over a dozen
// unrelated heal spells, so treating it as confirmation would let an
// off-chain healer mask an actual missed CH cast.
func TestHealWatcher_IgnoresUnrelatedHeals(t *testing.T) {
	s := &fakeHealSink{}
	w := newHealWatcher(s, true, true)

	for _, line := range []string{
		"Krayziefoo feels much better.",            // Superior Healing / many others
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

func TestHealWatcher_DisabledOrMissDetectionOff(t *testing.T) {
	line := "Krayziefoo is completely healed."

	s := &fakeHealSink{}
	newHealWatcher(s, false, true).HandleLine(line)
	if len(s.confirmed) != 0 {
		t.Errorf("chain disabled: confirmed = %v, want none", s.confirmed)
	}

	s2 := &fakeHealSink{}
	newHealWatcher(s2, true, false).HandleLine(line)
	if len(s2.confirmed) != 0 {
		t.Errorf("possible-miss disabled: confirmed = %v, want none", s2.confirmed)
	}
}

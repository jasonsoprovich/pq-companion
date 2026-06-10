package chchain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type capture struct {
	name     string
	category string
	dur      int
}

type fakeSink struct{ calls []capture }

func (f *fakeSink) StartExternal(name, category string, dur, _ int, _ time.Time, _ json.RawMessage, _ int) {
	f.calls = append(f.calls, capture{name, category, dur})
}

func newMatcher(s Sink, enabled bool, pattern string, interval float64) *Matcher {
	return New(s, func() config.CHChainSettings {
		return config.CHChainSettings{Enabled: enabled, Pattern: pattern, IntervalSecs: interval}
	})
}

func TestMatcher_DefaultPattern(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	line := "Soandso tells the raid, '--- 001 --- CH Winian with << 100% Mana >>'"
	m.HandleLine(time.Unix(1, 0), line)

	if len(s.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(s.calls))
	}
	c := s.calls[0]
	if c.category != "ch_chain" {
		t.Errorf("category = %q, want ch_chain", c.category)
	}
	// Bars now run the fixed CH cast time, not the configured cadence.
	if c.dur != config.CHCastSecs {
		t.Errorf("duration = %d, want %d", c.dur, config.CHCastSecs)
	}
	// Label carries chain position, target, and caster for the overlay.
	if want := "#1  Winian  ← Soandso"; c.name != want {
		t.Errorf("label = %q, want %q", c.name, want)
	}
}

// TestMatcher_RealRaidFormat locks in a real-world chain-call format observed
// in the wild: double-space after "raid,", "- - NNN - CH <Tank>" markers, and
// trailing mana/health notes. The speaker is the casting cleric.
func TestMatcher_RealRaidFormat(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	lines := []struct {
		in   string
		want string
	}{
		{"Luna tells the raid,  '- - 001 - CH Krayziefoo'", "#1  Krayziefoo  ← Luna"},
		{"Koramak tells the raid,  '- - 002 - CH Krayziefoo - 94% remaining'", "#2  Krayziefoo  ← Koramak"},
		{"Theofonias tells the raid,  '- - 003 - CH Krayziefoo, 90% mana'", "#3  Krayziefoo  ← Theofonias"},
	}
	for _, tc := range lines {
		s.calls = nil
		m.HandleLine(time.Unix(1, 0), tc.in)
		if len(s.calls) != 1 {
			t.Fatalf("%q: got %d calls, want 1", tc.in, len(s.calls))
		}
		if s.calls[0].name != tc.want {
			t.Errorf("%q: label = %q, want %q", tc.in, s.calls[0].name, tc.want)
		}
	}
}

func TestMatcher_DisabledAndNonMatching(t *testing.T) {
	s := &fakeSink{}
	// Disabled → no calls even on a matching line.
	off := newMatcher(s, false, config.DefaultCHChainPattern, 6)
	off.HandleLine(time.Unix(1, 0), "Soandso tells the raid, '--- 002 --- CH Bob'")
	if len(s.calls) != 0 {
		t.Fatalf("disabled matcher fired %d times, want 0", len(s.calls))
	}

	// Enabled but unrelated lines (guild chat, normal tells) must not match.
	on := newMatcher(s, true, config.DefaultCHChainPattern, 6)
	for _, line := range []string{
		"Soandso tells the guild, 'inc named'",
		"You tell your party, 'CH on me'",
		"Soandso tells the raid, 'rezzes incoming'",
	} {
		on.HandleLine(time.Unix(1, 0), line)
	}
	if len(s.calls) != 0 {
		t.Errorf("non-chain lines fired %d times, want 0", len(s.calls))
	}
}

func TestMatcher_BadPatternIsSafe(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, `(?P<caster>\w+`, 6) // unbalanced paren
	m.HandleLine(time.Unix(1, 0), "Soandso tells the raid, '--- 001 --- CH Winian'")
	if len(s.calls) != 0 {
		t.Errorf("bad pattern produced %d calls, want 0", len(s.calls))
	}
}

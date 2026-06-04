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

func newMatcher(s Sink, enabled bool, pattern string, interval int) *Matcher {
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
	if c.dur != 6 {
		t.Errorf("duration = %d, want 6", c.dur)
	}
	// Label carries chain position, target, and caster for the overlay.
	if want := "#1  Winian  ← Soandso"; c.name != want {
		t.Errorf("label = %q, want %q", c.name, want)
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

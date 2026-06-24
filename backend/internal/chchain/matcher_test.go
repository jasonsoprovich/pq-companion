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

func (f *fakeSink) StartExternal(name, category string, dur, _ int, _ time.Time, _ json.RawMessage, _ int, _, _ string) {
	f.calls = append(f.calls, capture{name, category, dur})
}

func newMatcher(s Sink, enabled bool, pattern string, interval float64) *Matcher {
	return New(s, func() config.CHChainSettings {
		return config.CHChainSettings{Enabled: enabled, Pattern: pattern, IntervalSecs: interval}
	})
}

func newSplitMatcher(s Sink, primary, secondary string) *Matcher {
	return New(s, func() config.CHChainSettings {
		return config.CHChainSettings{
			Enabled:          true,
			Pattern:          primary,
			SecondaryEnabled: true,
			SecondaryPattern: secondary,
			IntervalSecs:     6,
		}
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

// TestMatcher_OwnCastVerbConjugation guards the bug where own casts in shout
// and OOC never matched: your own messages use second-person verbs ("You
// shout", "You say out of character") while others use third person ("Soandso
// shouts", "Soandso says out of character"). Both conjugations must match.
func TestMatcher_OwnCastVerbConjugation(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	lines := []struct {
		in   string
		want string
	}{
		// shout: own (second person) and others (third person)
		{"You shout, '--- 001 --- CH Krayziefoo'", "#1  Krayziefoo  ← You"},
		{"Soandso shouts, '--- 002 --- CH Krayziefoo'", "#2  Krayziefoo  ← Soandso"},
		// OOC: own and others
		{"You say out of character, '--- 003 --- CH Krayziefoo'", "#3  Krayziefoo  ← You"},
		{"Soandso says out of character, '--- 004 --- CH Krayziefoo'", "#4  Krayziefoo  ← Soandso"},
		// raid say already worked (tells?), kept as a regression anchor
		{"You tell the raid, '--- 005 --- CH Krayziefoo'", "#5  Krayziefoo  ← You"},
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

// TestMatcher_LetterMarkersSingleChain: with the secondary chain disabled,
// the catch-all default routes letter calls to the main chain, and the first
// letter maps to a real position (A=1, B=2, …).
func TestMatcher_LetterMarkersSingleChain(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	lines := []struct {
		in       string
		want     string
		category string
	}{
		{"Luna tells the raid, '--- AAA --- CH Krayziefoo'", "#1  Krayziefoo  ← Luna", "ch_chain"},
		{"Koramak tells the raid, '--- BBB --- CH Krayziefoo'", "#2  Krayziefoo  ← Koramak", "ch_chain"},
		{"Theofonias tells the raid, '--- ccc --- CH Krayziefoo'", "#3  Krayziefoo  ← Theofonias", "ch_chain"},
	}
	for _, tc := range lines {
		s.calls = nil
		m.HandleLine(time.Unix(1, 0), tc.in)
		if len(s.calls) != 1 {
			t.Fatalf("%q: got %d calls, want 1", tc.in, len(s.calls))
		}
		if s.calls[0].name != tc.want || s.calls[0].category != tc.category {
			t.Errorf("%q: got (%q, %q), want (%q, %q)",
				tc.in, s.calls[0].name, s.calls[0].category, tc.want, tc.category)
		}
	}
}

// TestMatcher_SecondaryChainRouting: with the secondary chain enabled and the
// split defaults (numeric-only primary, letters-only secondary), numeric
// calls land in ch_chain and letter calls in ch_chain_2 — exactly one timer
// per line, never both.
func TestMatcher_SecondaryChainRouting(t *testing.T) {
	s := &fakeSink{}
	m := newSplitMatcher(s, config.DefaultCHChainNumericPattern, config.DefaultCHChainSecondaryPattern)

	lines := []struct {
		in       string
		want     string
		category string
	}{
		{"Luna tells the raid, '--- 001 --- CH Krayziefoo'", "#1  Krayziefoo  ← Luna", "ch_chain"},
		{"Koramak tells the raid, '--- 002 --- CH Krayziefoo'", "#2  Krayziefoo  ← Koramak", "ch_chain"},
		{"Dridelve tells the raid, '--- AAA --- CH Rampguy'", "#1  Rampguy  ← Dridelve", "ch_chain_2"},
		{"Theofonias tells the raid, '--- BBB --- CH Rampguy'", "#2  Rampguy  ← Theofonias", "ch_chain_2"},
	}
	for _, tc := range lines {
		s.calls = nil
		m.HandleLine(time.Unix(1, 0), tc.in)
		if len(s.calls) != 1 {
			t.Fatalf("%q: got %d calls, want 1", tc.in, len(s.calls))
		}
		if s.calls[0].name != tc.want || s.calls[0].category != tc.category {
			t.Errorf("%q: got (%q, %q), want (%q, %q)",
				tc.in, s.calls[0].name, s.calls[0].category, tc.want, tc.category)
		}
	}
}

// TestMatcher_SecondaryClaimsLettersFirst: even if the user keeps the
// catch-all primary pattern (which matches letters too), the secondary
// pattern is tried first, so letter calls still split off to ch_chain_2.
func TestMatcher_SecondaryClaimsLettersFirst(t *testing.T) {
	s := &fakeSink{}
	m := newSplitMatcher(s, config.DefaultCHChainPattern, config.DefaultCHChainSecondaryPattern)

	m.HandleLine(time.Unix(1, 0), "Luna tells the raid, '--- AAA --- CH Rampguy'")
	if len(s.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(s.calls))
	}
	if s.calls[0].category != "ch_chain_2" {
		t.Errorf("category = %q, want ch_chain_2", s.calls[0].category)
	}

	// And a numeric call still falls through to the primary chain.
	s.calls = nil
	m.HandleLine(time.Unix(1, 0), "Luna tells the raid, '--- 001 --- CH Krayziefoo'")
	if len(s.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(s.calls))
	}
	if s.calls[0].category != "ch_chain" {
		t.Errorf("category = %q, want ch_chain", s.calls[0].category)
	}
}

// TestMatcher_NumericPrimaryIgnoresLetters: with the split numeric-only
// primary and the secondary disabled, letter calls don't match at all.
func TestMatcher_NumericPrimaryIgnoresLetters(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainNumericPattern, 6)
	m.HandleLine(time.Unix(1, 0), "Luna tells the raid, '--- AAA --- CH Rampguy'")
	if len(s.calls) != 0 {
		t.Errorf("letter call matched numeric-only pattern %d times, want 0", len(s.calls))
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

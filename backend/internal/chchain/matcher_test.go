package chchain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type capture struct {
	name     string
	category string
	dur      float64
	target   string
}

type manaCall struct {
	name   string
	target string
	pct    int
}

type fakeSink struct {
	calls       []capture
	confirmed   []capture
	unconfirmed []string
	manas       []manaCall
}

func (f *fakeSink) StartExternal(name, category string, dur, _ float64, _ time.Time, _ json.RawMessage, _ int, targetName, _ string, _ bool, _ string, _ bool, _ ...bool) {
	f.calls = append(f.calls, capture{name, category, dur, targetName})
}

func (f *fakeSink) ConfirmCast(name, targetName string) {
	f.confirmed = append(f.confirmed, capture{name: name, target: targetName})
}

func (f *fakeSink) UnconfirmCast(caster string, _ time.Time) {
	f.unconfirmed = append(f.unconfirmed, caster)
}

func (f *fakeSink) SetCasterMana(name, targetName string, pct int) {
	f.manas = append(f.manas, manaCall{name: name, target: targetName, pct: pct})
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
		t.Errorf("duration = %v, want %v", c.dur, config.CHCastSecs)
	}
	// Label carries chain position, target, and caster for the overlay.
	if want := "#1  Winian  ← Soandso"; c.name != want {
		t.Errorf("label = %q, want %q", c.name, want)
	}
	// The captured target is also passed through as the timer's TargetName
	// (not just embedded in the label) so CastWatcher can correlate a
	// cast-begin line to this exact timer via Engine.ConfirmCast.
	if c.target != "Winian" {
		t.Errorf("target = %q, want %q", c.target, "Winian")
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

// TestMatcher_CasterMana covers extracting the caster's self-reported
// remaining mana from a callout's trailing note. It must arrive via
// SetCasterMana (a side-channel update keyed to the just-started timer),
// never baked into the label itself — see the Sink doc comment for why.
func TestMatcher_CasterMana(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	lines := []struct {
		in       string
		wantMana int // -1 = no SetCasterMana call expected
	}{
		{"Luna tells the raid,  '- - 001 - CH Krayziefoo'", -1},
		{"Koramak tells the raid,  '- - 002 - CH Krayziefoo - 94% remaining'", 94},
		{"Theofonias tells the raid,  '- - 003 - CH Krayziefoo, 90% mana'", 90},
		{"Soandso tells the raid, '--- 004 --- CH Krayziefoo with << 100% Mana >>'", 100},
		{"Baddy tells the raid, '--- 005 --- CH Krayziefoo, 994% mana'", -1}, // nonsensical >100%, ignored
	}
	for _, tc := range lines {
		s.calls, s.manas = nil, nil
		m.HandleLine(time.Unix(1, 0), tc.in)
		if len(s.calls) != 1 {
			t.Fatalf("%q: got %d StartExternal calls, want 1", tc.in, len(s.calls))
		}
		label := s.calls[0].name
		if tc.wantMana < 0 {
			if len(s.manas) != 0 {
				t.Errorf("%q: SetCasterMana called with %v, want none", tc.in, s.manas)
			}
			// No mana embedded in the label either.
			if strings.Contains(label, "%") {
				t.Errorf("%q: label %q unexpectedly contains a mana percentage", tc.in, label)
			}
			continue
		}
		if len(s.manas) != 1 {
			t.Fatalf("%q: got %d SetCasterMana calls, want 1", tc.in, len(s.manas))
		}
		if s.manas[0].pct != tc.wantMana {
			t.Errorf("%q: mana = %d, want %d", tc.in, s.manas[0].pct, tc.wantMana)
		}
		if s.manas[0].name != label || s.manas[0].target != "Krayziefoo" {
			t.Errorf("%q: SetCasterMana(%q, %q) doesn't match the started timer (%q, %q)",
				tc.in, s.manas[0].name, s.manas[0].target, label, "Krayziefoo")
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

// TestMatcher_ConfirmsOwnCallerNotAnotherClericOnSameTarget is the core
// regression guard for the possible-miss feature. Every cleric in a CH chain
// heals the SAME tank, so correlating a cast-begin line by target alone
// attributes it to whichever callout on that tank is oldest — a different
// cleric's. Confirmation must follow the CASTER.
func TestMatcher_ConfirmsOwnCallerNotAnotherClericOnSameTarget(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	base := time.Unix(1000, 0)
	m.HandleLine(base, "Bluecross tells the raid, '--- 001 --- CH Larzek'")
	m.HandleLine(base.Add(3*time.Second), "Eruna tells the raid, '--- 002 --- CH Larzek'")

	// Eruna casts. Bluecross's callout is older and shares the target, so the
	// old per-target FIFO would have confirmed Bluecross's row instead.
	m.NoteCastBegin("Eruna", base.Add(3*time.Second))

	if len(s.confirmed) != 1 {
		t.Fatalf("got %d confirmations, want 1: %+v", len(s.confirmed), s.confirmed)
	}
	got := s.confirmed[0]
	if got.target != "Larzek" || !strings.Contains(got.name, "Eruna") {
		t.Errorf("confirmed %+v, want Eruna's own callout on Larzek", got)
	}
	if strings.Contains(got.name, "Bluecross") {
		t.Error("confirmed another cleric's callout on the same target")
	}
}

// TestMatcher_ConfirmsCallerWhoseCastBeganFirst covers macros whose cast line
// beats their chain shout into the log — ~8% of callouts in real raid logs.
// Without the pending-cast side of the correlation those all flag as misses.
func TestMatcher_ConfirmsCallerWhoseCastBeganFirst(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	base := time.Unix(1000, 0)
	m.NoteCastBegin("Eruna", base) // cast-begin lands one second early
	m.HandleLine(base.Add(time.Second), "Eruna tells the raid, '--- 002 --- CH Larzek'")

	if len(s.confirmed) != 1 || !strings.Contains(s.confirmed[0].name, "Eruna") {
		t.Fatalf("confirmed = %+v, want Eruna's callout confirmed by the earlier cast", s.confirmed)
	}
}

// TestMatcher_StalePendingCastDoesNotConfirm keeps the early-cast path from
// becoming a blanket amnesty: a cast seen long before a callout (an unrelated
// spell last cycle) must not confirm it.
func TestMatcher_StalePendingCastDoesNotConfirm(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	base := time.Unix(1000, 0)
	m.NoteCastBegin("Eruna", base)
	m.HandleLine(base.Add(earlyCastWindow+time.Second), "Eruna tells the raid, '--- 002 --- CH Larzek'")

	if len(s.confirmed) != 0 {
		t.Errorf("confirmed = %+v, want none (pending cast too old)", s.confirmed)
	}
}

// TestMatcher_NoteCastBegin covers the ordinary path plus its two negative
// cases: an unrelated caster, and a callout that has aged out of the window
// (the next chain cycle must look like a fresh, unconfirmed callout).
func TestMatcher_NoteCastBegin(t *testing.T) {
	base := time.Unix(1000, 0)
	const line = "Soandso tells the raid, '--- 001 --- CH Winian'"

	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)
	m.HandleLine(base, line)
	m.NoteCastBegin("Soandso", base.Add(2*time.Second))
	if len(s.confirmed) != 1 || s.confirmed[0].target != "Winian" {
		t.Errorf("confirmed = %+v, want Winian confirmed", s.confirmed)
	}

	s2 := &fakeSink{}
	m2 := newMatcher(s2, true, config.DefaultCHChainPattern, 6)
	m2.HandleLine(base, line)
	m2.NoteCastBegin("Nobody", base.Add(2*time.Second))
	if len(s2.confirmed) != 0 {
		t.Errorf("confirmed = %+v, want none (caster never called)", s2.confirmed)
	}

	s3 := &fakeSink{}
	m3 := newMatcher(s3, true, config.DefaultCHChainPattern, 6)
	m3.HandleLine(base, line)
	m3.NoteCastBegin("Soandso", base.Add(recentCallWindow+time.Second))
	if len(s3.confirmed) != 0 {
		t.Errorf("confirmed = %+v, want none (callout aged out)", s3.confirmed)
	}
}

// TestMatcher_ConfirmationIsOneShot stops a caster's later, unrelated cast
// (a rez, a buff, a spot heal) from confirming a callout a second time — the
// next cycle's callout must stand on its own evidence.
func TestMatcher_ConfirmationIsOneShot(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	base := time.Unix(1000, 0)
	m.HandleLine(base, "Soandso tells the raid, '--- 001 --- CH Winian'")
	m.NoteCastBegin("Soandso", base.Add(time.Second))
	m.NoteCastBegin("Soandso", base.Add(2*time.Second))

	if len(s.confirmed) != 1 {
		t.Errorf("got %d confirmations, want 1: %+v", len(s.confirmed), s.confirmed)
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

// TestMatcher_MultiClericChainPrecisionAndRecall is the end-to-end guard for
// the false-possible-miss storm. It runs several cycles of a 6-cleric chain in
// the shape real raid logs take — every cleric healing the SAME tank, casts
// landing on the same log second as the callout, one cleric's cast line
// arriving a second EARLY — with exactly one genuine miss injected.
//
// Replayed against a real 6-cleric VT log, the previous target-keyed FIFO
// produced 378 possible-miss flags across 199 callouts in which every cleric
// cast every time; this correlation produces none, and still flags an injected
// miss with no collateral on the other five.
func TestMatcher_MultiClericChainPrecisionAndRecall(t *testing.T) {
	clerics := []string{"Bluecross", "Eruna", "Vortikai", "Roughnight", "Veia", "Syntra"}
	const (
		tank    = "Larzek"
		cadence = 3500 * time.Millisecond
		cycles  = 4
	)
	// Cycle 2, slot 3 (Vortikai) shouts but never casts.
	missCycle, missSlot := 2, 2

	s := &fakeSink{}
	m := newMatcher(s, true, config.DefaultCHChainPattern, 6)

	base := time.Unix(1_000_000, 0)
	at := base
	type call struct {
		label string
		at    time.Time
	}
	var wantMissed []call
	for cycle := 0; cycle < cycles; cycle++ {
		for slot, caster := range clerics {
			label := "#" + string(rune('1'+slot)) + "  " + tank + "  ← " + caster
			// Veia's macro casts a beat before it shouts; everyone else casts
			// on the same log second as their callout.
			early := caster == "Veia"
			if early {
				m.NoteCastBegin(caster, at.Add(-time.Second))
			}
			m.HandleLine(at, caster+" shouts, '00"+string(rune('1'+slot))+" - CH - "+tank+" - 90%'")
			if !early {
				if cycle == missCycle && slot == missSlot {
					wantMissed = append(wantMissed, call{label, at})
				} else {
					m.NoteCastBegin(caster, at)
				}
			}
			at = at.Add(cadence)
		}
	}

	if want := cycles * len(clerics); len(s.calls) != want {
		t.Fatalf("got %d callouts, want %d", len(s.calls), want)
	}

	// A callout counts as flagged when no confirmation arrived for it. The
	// engine addresses timers by (label, target) and rebuilds one per callout,
	// so count confirmations per label and compare against callouts per label.
	confirms := map[string]int{}
	for _, c := range s.confirmed {
		if c.target != tank {
			t.Errorf("confirmation carried target %q, want %q", c.target, tank)
		}
		confirms[c.name]++
	}
	callouts := map[string]int{}
	for _, c := range s.calls {
		callouts[c.name]++
	}
	for label, n := range callouts {
		want := n
		for _, mc := range wantMissed {
			if mc.label == label {
				want--
			}
		}
		if confirms[label] != want {
			t.Errorf("%q: %d confirmations, want %d (callouts=%d)", label, confirms[label], want, n)
		}
	}
	if len(wantMissed) != 1 {
		t.Fatalf("test setup: expected exactly 1 injected miss, got %d", len(wantMissed))
	}
}

// TestMatcher_NoteCastInterruptedForwardsToSink confirms Matcher's interrupt
// path needs no callout bookkeeping of its own (unlike NoteCastBegin) — it
// just forwards the caster straight to the sink, which has enough identity
// (the timer's caster-suffixed label) to find and un-confirm the right timer
// itself.
func TestMatcher_NoteCastInterruptedForwardsToSink(t *testing.T) {
	s := &fakeSink{}
	m := newMatcher(s, true, "", 0)

	m.NoteCastInterrupted("Soandso", time.Unix(5, 0))
	if len(s.unconfirmed) != 1 || s.unconfirmed[0] != "Soandso" {
		t.Fatalf("unconfirmed = %v, want [Soandso]", s.unconfirmed)
	}

	m.NoteCastInterrupted("", time.Unix(6, 0))
	if len(s.unconfirmed) != 1 {
		t.Errorf("empty caster should be a no-op, got %v", s.unconfirmed)
	}
}

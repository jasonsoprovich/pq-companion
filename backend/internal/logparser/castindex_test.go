package logparser

import "testing"

// fixtures used across the test cases. Names match real Project Quarm spell
// data so the test reflects production patterns.
var indexFixtures = []CastMessage{
	{
		SpellID:     2570,
		SpellName:   "Koadic's Endless Intellect",
		CastOnYou:   "Your mind expands beyond the bounds of space and time.",
		CastOnOther: "'s mind expands beyond the bounds of space and time.",
	},
	{
		SpellID:     1939,
		SpellName:   "Speed of the Shissar",
		CastOnYou:   "Your body pulses with the spirit of the Shissar.",
		CastOnOther: "'s body pulses with the spirit of the Shissar.",
	},
	{
		SpellID:     1710,
		SpellName:   "Visions of Grandeur",
		CastOnYou:   "You experience visions of grandeur.",
		CastOnOther: " experiences visions of grandeur.",
	},
	{
		SpellID:     307,
		SpellName:   "Mesmerization",
		CastOnYou:   "You are mesmerized.",
		CastOnOther: " has been mesmerized.",
	},
	// Two spells sharing the same self-cast text — the engine has to
	// disambiguate from the recently-cast spell.
	{
		SpellID:     1000,
		SpellName:   "Ultravision",
		CastOnYou:   "Your eyes tingle.",
		CastOnOther: "'s eyes tingle.",
	},
	{
		SpellID:     1001,
		SpellName:   "Plainsight",
		CastOnYou:   "Your eyes tingle.",
		CastOnOther: "'s eyes tingle.",
	},
}

func TestCastIndex_MatchSelf(t *testing.T) {
	idx := NewCastIndex(indexFixtures)

	got := idx.Match("Your mind expands beyond the bounds of space and time.")
	if got == nil {
		t.Fatalf("expected a match, got nil")
	}
	if got.Kind != MatchSelf {
		t.Errorf("kind: got %v, want MatchSelf", got.Kind)
	}
	if got.SpellID != 2570 {
		t.Errorf("spell id: got %d, want 2570", got.SpellID)
	}
	if got.SpellName != "Koadic's Endless Intellect" {
		t.Errorf("spell name: got %q", got.SpellName)
	}
	if got.TargetName != "" {
		t.Errorf("target name on MatchSelf should be empty, got %q", got.TargetName)
	}
}

func TestCastIndex_MatchOther(t *testing.T) {
	cases := []struct {
		line       string
		wantSpell  string
		wantTarget string
	}{
		{
			line:       "Tank's body pulses with the spirit of the Shissar.",
			wantSpell:  "Speed of the Shissar",
			wantTarget: "Tank",
		},
		{
			line:       "Healer experiences visions of grandeur.",
			wantSpell:  "Visions of Grandeur",
			wantTarget: "Healer",
		},
		{
			line:       "Pet has been mesmerized.",
			wantSpell:  "Mesmerization",
			wantTarget: "Pet",
		},
	}

	idx := NewCastIndex(indexFixtures)
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got := idx.Match(tc.line)
			if got == nil {
				t.Fatalf("expected a match for %q, got nil", tc.line)
			}
			if got.Kind != MatchOther {
				t.Errorf("kind: got %v, want MatchOther", got.Kind)
			}
			if got.SpellName != tc.wantSpell {
				t.Errorf("spell: got %q, want %q", got.SpellName, tc.wantSpell)
			}
			if got.TargetName != tc.wantTarget {
				t.Errorf("target: got %q, want %q", got.TargetName, tc.wantTarget)
			}
		})
	}
}

// Multiple spells share "Your eyes tingle." — the matcher must report all
// candidates and leave SpellID/SpellName zero/empty so the engine knows to
// disambiguate using context (e.g. lastCastSpell).
func TestCastIndex_AmbiguousSelf(t *testing.T) {
	idx := NewCastIndex(indexFixtures)
	got := idx.Match("Your eyes tingle.")
	if got == nil {
		t.Fatalf("expected a match")
	}
	if got.SpellID != 0 {
		t.Errorf("ambiguous match should have SpellID=0, got %d", got.SpellID)
	}
	if got.SpellName != "" {
		t.Errorf("ambiguous match should have SpellName empty, got %q", got.SpellName)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(got.Candidates))
	}
	wantIDs := map[int]bool{1000: true, 1001: true}
	for _, c := range got.Candidates {
		if !wantIDs[c.SpellID] {
			t.Errorf("unexpected candidate id %d", c.SpellID)
		}
	}
}

func TestCastIndex_AmbiguousOther(t *testing.T) {
	idx := NewCastIndex(indexFixtures)
	got := idx.Match("Tank's eyes tingle.")
	if got == nil {
		t.Fatalf("expected a match")
	}
	if got.Kind != MatchOther {
		t.Errorf("kind: got %v", got.Kind)
	}
	if got.TargetName != "Tank" {
		t.Errorf("target: got %q", got.TargetName)
	}
	if got.SpellID != 0 {
		t.Errorf("ambiguous: SpellID should be 0, got %d", got.SpellID)
	}
	if len(got.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(got.Candidates))
	}
}

// Combat hits, shouts, and arbitrary chatter must not be misidentified as
// spell lands. This is the most important false-positive guard.
func TestCastIndex_NoMatch(t *testing.T) {
	idx := NewCastIndex(indexFixtures)
	cases := []string{
		"You slash a gnoll for 50 points of damage.",
		"You begin casting Visions of Grandeur.",
		"a gnoll has been slain by Tank.",
		"You say, 'hello'",
		"Loading, Please Wait...",
		"",
	}
	for _, line := range cases {
		if got := idx.Match(line); got != nil {
			t.Errorf("line %q unexpectedly matched: kind=%v target=%q spell=%q",
				line, got.Kind, got.TargetName, got.SpellName)
		}
	}
}

// Multi-word NPC names ("a stone golem") and lower-case openings should not
// be captured as targets — the nameClass regex requires a capitalized leading
// token. Buffs are practically never cast on these targets, and accepting
// them would cause false positives on combat lines.
func TestCastIndex_RejectsLowercaseTarget(t *testing.T) {
	idx := NewCastIndex(indexFixtures)
	if got := idx.Match("a stone golem has been mesmerized."); got != nil {
		t.Errorf("lowercase target should not match: got %+v", got)
	}
}

func TestCastIndex_NilSafe(t *testing.T) {
	var idx *CastIndex
	if got := idx.Match("anything"); got != nil {
		t.Errorf("nil index Match should return nil, got %+v", got)
	}
}

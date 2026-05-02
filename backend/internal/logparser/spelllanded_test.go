package logparser

import (
	"testing"
)

// installTestCastIndex installs a small fixture index for the duration of a
// test, restoring whatever was previously installed (typically nil) on
// cleanup. The cast-index registry is process-wide, so any test that needs
// it must use this helper to avoid leaking state into sibling tests.
func installTestCastIndex(t *testing.T, msgs []CastMessage) {
	t.Helper()
	prev := activeCastIndex.Load()
	SetCastIndex(NewCastIndex(msgs))
	t.Cleanup(func() { activeCastIndex.Store(prev) })
}

var landedFixtures = []CastMessage{
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

func TestParseLine_SpellLanded_Self(t *testing.T) {
	installTestCastIndex(t, landedFixtures)

	ev, ok := ParseLine("[Mon Apr 13 06:00:00 2026] Your mind expands beyond the bounds of space and time.")
	if !ok {
		t.Fatalf("expected match")
	}
	if ev.Type != EventSpellLanded {
		t.Fatalf("type: got %v want EventSpellLanded", ev.Type)
	}
	d, ok := ev.Data.(SpellLandedData)
	if !ok {
		t.Fatalf("data type: %T", ev.Data)
	}
	if d.Kind != SpellLandedKindYou {
		t.Errorf("kind: got %v", d.Kind)
	}
	if d.SpellID != 2570 || d.SpellName != "Koadic's Endless Intellect" {
		t.Errorf("spell: got id=%d name=%q", d.SpellID, d.SpellName)
	}
	if d.TargetName != "" {
		t.Errorf("target should be empty for kind=you, got %q", d.TargetName)
	}
	if len(d.Candidates) != 0 {
		t.Errorf("unique match should not populate candidates, got %d", len(d.Candidates))
	}
}

func TestParseLine_SpellLanded_Other(t *testing.T) {
	installTestCastIndex(t, landedFixtures)

	ev, ok := ParseLine("[Mon Apr 13 06:00:00 2026] Tank's body pulses with the spirit of the Shissar.")
	if !ok {
		t.Fatalf("expected match")
	}
	if ev.Type != EventSpellLanded {
		t.Fatalf("type: %v", ev.Type)
	}
	d := ev.Data.(SpellLandedData)
	if d.Kind != SpellLandedKindOther {
		t.Errorf("kind: %v", d.Kind)
	}
	if d.SpellName != "Speed of the Shissar" {
		t.Errorf("spell: %q", d.SpellName)
	}
	if d.TargetName != "Tank" {
		t.Errorf("target: %q", d.TargetName)
	}
}

func TestParseLine_SpellLanded_Ambiguous(t *testing.T) {
	installTestCastIndex(t, landedFixtures)

	ev, ok := ParseLine("[Mon Apr 13 06:00:00 2026] Your eyes tingle.")
	if !ok {
		t.Fatalf("expected match")
	}
	d := ev.Data.(SpellLandedData)
	if d.SpellID != 0 || d.SpellName != "" {
		t.Errorf("ambiguous payload should leave id/name zero, got id=%d name=%q",
			d.SpellID, d.SpellName)
	}
	if len(d.Candidates) != 2 {
		t.Fatalf("candidate count: got %d, want 2", len(d.Candidates))
	}
	ids := map[int]bool{}
	for _, c := range d.Candidates {
		ids[c.SpellID] = true
		if c.SpellName == "" {
			t.Errorf("candidate name should be populated, got empty for id=%d", c.SpellID)
		}
	}
	if !ids[1000] || !ids[1001] {
		t.Errorf("candidate ids: got %v", ids)
	}
}

// Structured patterns must still win over the cast index. This is the
// safeguard against a spell's flavor text accidentally swallowing a combat,
// heal, or zone line.
func TestParseLine_StructuredEventsWinOverCastIndex(t *testing.T) {
	installTestCastIndex(t, landedFixtures)

	cases := []struct {
		line     string
		wantType EventType
	}{
		{"[Mon Apr 13 06:00:00 2026] You have entered The North Karana.", EventZone},
		{"[Mon Apr 13 06:00:00 2026] You begin casting Visions of Grandeur.", EventSpellCast},
		{"[Mon Apr 13 06:00:00 2026] Your Visions of Grandeur spell has worn off.", EventSpellFade},
		{"[Mon Apr 13 06:00:00 2026] You slash a gnoll for 50 points of damage.", EventCombatHit},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			ev, ok := ParseLine(tc.line)
			if !ok {
				t.Fatalf("expected match")
			}
			if ev.Type != tc.wantType {
				t.Errorf("type: got %v, want %v", ev.Type, tc.wantType)
			}
		})
	}
}

// Without a cast index installed (the default), spell-landed-looking lines
// must NOT classify as EventSpellLanded — they fall through to the unmatched
// path and ParseLine returns ok=false. Guards against state leaking from
// other tests in the package.
func TestParseLine_NoCastIndex_NoSpellLanded(t *testing.T) {
	prev := activeCastIndex.Load()
	SetCastIndex(nil)
	t.Cleanup(func() { activeCastIndex.Store(prev) })

	ev, ok := ParseLine("[Mon Apr 13 06:00:00 2026] Your mind expands beyond the bounds of space and time.")
	if ok {
		t.Errorf("expected no match without index, got type=%v", ev.Type)
	}
}

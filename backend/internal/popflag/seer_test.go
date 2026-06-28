package popflag

import (
	"sort"
	"testing"
)

func setOf(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func TestParseSeerCounters(t *testing.T) {
	// mavuin at 3 and fuirstel at 4 — counters print only their current line.
	transcript := `
Mavuin is grateful to you for taking his case before the Tribunal.
Bertoxxulous has been slain, the curse from Milyk now lifted.
`
	q := ParseSeer(transcript)
	if q["mavuin"] != "3" {
		t.Errorf("mavuin = %q, want 3", q["mavuin"])
	}
	if q["fuirstel"] != "4" {
		t.Errorf("fuirstel = %q, want 4", q["fuirstel"])
	}

	done := setOf(DeriveCompletion(q))
	for _, id := range []string{"poj_preflag", "poj_trial_mark", "poj_mavuin_return", "pod_preflag", "pod_postflag", "cod_bertox"} {
		if !done[id] {
			t.Errorf("expected %q complete from mavuin=3/fuirstel=4", id)
		}
	}
	if done["cod_postflag"] {
		t.Errorf("cod_postflag needs fuirstel 5, should not be complete at 4")
	}
}

func TestParseSeerBitmask(t *testing.T) {
	transcript := `
You have beaten Rydda` + "`" + `Dar in the first of Honor's Trials.
You have defeated the nomads in the third of Honor's Trials.
Xuzl's arcane wisdom pulses in your mind.
Jiva's strength fills your body.
`
	q := ParseSeer(transcript)
	if q["hohtrials"] != "101" {
		t.Errorf("hohtrials = %q, want 101", q["hohtrials"])
	}
	if q["sol_room"] != "10001" {
		t.Errorf("sol_room = %q, want 10001", q["sol_room"])
	}

	done := setOf(DeriveCompletion(q))
	if !done["hoh_trial_rydda"] || !done["hoh_trial_maidens"] {
		t.Errorf("rydda + maidens (bits 1,3) should be complete")
	}
	if done["hoh_trial_villagers"] {
		t.Errorf("villagers (bit 2) should NOT be complete")
	}
	if !done["solro_xuzl"] || !done["solro_jiva"] {
		t.Errorf("xuzl (bit1) + jiva (bit5) should be complete")
	}
	if done["solro_arlyxir"] {
		t.Errorf("arlyxir (bit2) should NOT be complete")
	}
}

func TestParseSeerHohAllTrials(t *testing.T) {
	q := ParseSeer("You have completed all of Honor's Trials.")
	if q["hohtrials"] != "111" {
		t.Fatalf("hohtrials = %q, want 111", q["hohtrials"])
	}
	done := setOf(DeriveCompletion(q))
	for _, id := range []string{"hoh_trial_rydda", "hoh_trial_villagers", "hoh_trial_maidens"} {
		if !done[id] {
			t.Errorf("%q should be complete from 'completed all'", id)
		}
	}
}

func TestParseSeerZeksMerge(t *testing.T) {
	cases := []struct {
		name       string
		transcript string
		wantZeks   string
		vallon     bool
		tallon     bool
	}{
		{"vallon only", "The pack of notes from Vallon are scribbled in a language that you cannot comprehend.", "3", true, false},
		{"tallon only", "The pack of notes from Tallon are scribbled in a language that you cannot comprehend.", "4", false, true},
		{
			"both -> 5",
			"The pack of notes from Vallon are scribbled in a language.\nThe pack of notes from Tallon are scribbled in a language.",
			"5", true, true,
		},
		{"maelin -> 6, both done", "The words of Maelin echo in your mind, 'The Zeks and Solusek are planning an invasion'", "6", true, true},
		{"rallos -> 7, both done", "The parchments of Rallos are scribed in a language that you cannot comprehend", "7", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := ParseSeer(tc.transcript)
			if q["zeks"] != tc.wantZeks {
				t.Errorf("zeks = %q, want %q", q["zeks"], tc.wantZeks)
			}
			done := setOf(DeriveCompletion(q))
			if done["potac_vallon"] != tc.vallon {
				t.Errorf("potac_vallon = %v, want %v", done["potac_vallon"], tc.vallon)
			}
			if done["potac_tallon"] != tc.tallon {
				t.Errorf("potac_tallon = %v, want %v", done["potac_tallon"], tc.tallon)
			}
		})
	}
}

func TestDeriveCipherReplacement(t *testing.T) {
	// Only the cipher line is present (server deleted saryrn + mmarr). The
	// saryrn- and mmarr-backed nodes must still resolve done via SatisfiedBy.
	q := ParseSeer("The Cipher of the Divine Language appears on your arms for a brief moment then fades.")
	if q["cipher"] != "1" {
		t.Fatalf("cipher = %q, want 1", q["cipher"])
	}
	done := setOf(DeriveCompletion(q))
	if !done["potor_saryrn"] {
		t.Errorf("potor_saryrn should be satisfied by cipher")
	}
	if !done["hoh_mithaniel"] {
		t.Errorf("hoh_mithaniel should be satisfied by cipher")
	}
}

func TestDeriveZebuxorukReplacement(t *testing.T) {
	// zebuxoruk deletes karana; karana-backed nodes resolve via SatisfiedBy.
	q := ParseSeer("The History translated for you reveals the fate of Zebuxoruk.")
	if q["zebuxoruk"] != "1" {
		t.Fatalf("zebuxoruk = %q, want 1", q["zebuxoruk"])
	}
	done := setOf(DeriveCompletion(q))
	if !done["post_bot_preflag"] || !done["post_bot_flag"] {
		t.Errorf("karana-backed BoT nodes should be satisfied by zebuxoruk")
	}
	if !done["pok_librarian"] {
		t.Errorf("pok_librarian (zebuxoruk>=1) should be complete")
	}
}

func TestMatchSeerLine(t *testing.T) {
	if !MatchSeerLine("Mavuin is grateful to you for taking his case before the Tribunal.") {
		t.Errorf("a Seer line should match")
	}
	if MatchSeerLine("You slash a gnoll for 150 points of damage.") {
		t.Errorf("a combat line should not match")
	}
	if MatchSeerLine("") {
		t.Errorf("empty line should not match")
	}
}

func TestDeriveCompletionDeterministic(t *testing.T) {
	// Sanity: derivation returns only known flag IDs and is stable.
	q := ParseSeer("Your soul has formed a bond with the Plane of Time.")
	done := DeriveCompletion(q)
	sort.Strings(done)
	if len(done) == 0 || done[0] == "" {
		t.Fatal("expected at least the potime flag")
	}
	for _, id := range done {
		if _, ok := ByID(id); !ok {
			t.Errorf("DeriveCompletion returned unknown flag id %q", id)
		}
	}
}

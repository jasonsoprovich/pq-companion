package popflag

import (
	"testing"
	"time"
)

// Lines as the quests Lua prints them (lua_modules/quintessence_progression
// and poinnovation/player.lua @ 05a3ec0), double spaces included.
const (
	quintLine = "The Quintessence of Elements completes your earlier planar progression. Activate the machine in the Plane of Innovation to attune yourself to the Plane of Time."
	timeLine  = "The ages begin to tear through your body.  You wake to find yourself in another time."
)

func TestConsumerQuintessenceCompletesEarlierProgression(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	c := NewConsumer(s, func() string { return char })
	c.HandleLine(time.Unix(7000, 0), quintLine)

	rows := doneByID(t, s, char)
	// Every required Tier 1-4 milestone is done; Time itself is not (the
	// machine grants it separately).
	for _, f := range Flags() {
		if f.Optional || f.Group != "" || f.Tier > 4 {
			continue
		}
		if row := rows[f.ID]; !row.Done || row.Source != SourceAuto {
			t.Errorf("%s = %+v, want done/auto", f.ID, row)
		}
	}
	if row, ok := rows["potime"]; ok {
		t.Errorf("potime = %+v, want no row from the Quintessence alone", row)
	}
	r := Resolve(mustStates(t, s, char))
	for _, f := range r.Flags {
		if f.ID == "potime" && f.Locked {
			t.Errorf("potime still locked after the Quintessence: missing %v", f.Missing)
		}
	}
}

func TestConsumerQuintessenceIgnoresSpoofs(t *testing.T) {
	s := openTempStore(t)
	c := NewConsumer(s, func() string { return "Osui" })
	c.HandleLine(time.Unix(7100, 0), "Darkclaw tells you, '"+quintLine+"'")
	c.HandleLine(time.Unix(7101, 0), "Darkclaw says, '"+quintLine+"'")
	if rows := doneByID(t, s, "Osui"); len(rows) != 0 {
		t.Errorf("spoofed lines wrote rows: %+v", rows)
	}
}

func TestConsumerTimeMachineGrant(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	c := NewConsumer(s, func() string { return char })
	ts := time.Unix(7200, 0)
	c.HandleLine(ts, quintLine)
	c.HandleLine(ts, "You have received a character flag!")
	c.HandleLine(ts, timeLine)
	if row := doneByID(t, s, char)["potime"]; !row.Done || row.Source != SourceAuto {
		t.Errorf("potime = %+v, want done/auto", row)
	}
}

func TestConsumerTimeGrantNeedsFlagLineFirst(t *testing.T) {
	cases := map[string][]string{
		"time line alone":   {timeLine},
		"reversed order":    {timeLine, "You have received a character flag!"},
		"interleaved":       {"You have received a character flag!", "Darkclaw says, 'hi'", timeLine},
		"checklist instead": {"You have received a new checklist flag!", timeLine},
	}
	for name, lines := range cases {
		t.Run(name, func(t *testing.T) {
			s := openTempStore(t)
			c := NewConsumer(s, func() string { return "Osui" })
			for _, l := range lines {
				c.HandleLine(time.Unix(7300, 0), l)
			}
			if row, ok := doneByID(t, s, "Osui")["potime"]; ok {
				t.Errorf("potime = %+v, want no row", row)
			}
		})
	}
}

// The Quintessence raises a partial earlier snapshot without lowering
// anything.
func TestApplyQuintessenceMergesSnapshot(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	if _, err := s.ApplySeer(char, map[string]string{"zeks": "7", "thelin": "2"}, "", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyQuintessence(char, time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	snap, err := s.GetSnapshot(char)
	if err != nil || snap == nil {
		t.Fatalf("snapshot: %v %v", snap, err)
	}
	if snap.Qglobals["zeks"] != "7" || snap.Qglobals["thelin"] != "4" || snap.Qglobals["pofire"] != "2" {
		t.Errorf("merged qglobals = %v", snap.Qglobals)
	}
}

// Every qglobal the Quintessence sets maps onto the dataset: one flag per
// scalar value, and every bit of each bitmask.
func TestQuintessenceStepsCoverEveryQglobal(t *testing.T) {
	grants, autos := quintessenceSteps()
	covered := map[string]bool{}
	for _, id := range append(grants, autos...) {
		f, ok := ByID(id)
		if !ok {
			t.Fatalf("unknown flag %q", id)
		}
		covered[f.Qglobal] = true
	}
	for qg := range quintessenceQglobals {
		if !covered[qg] {
			t.Errorf("qglobal %q maps to no dataset flag", qg)
		}
	}
	if len(grants) != len(quintessenceQglobals)-2 {
		t.Errorf("got %d scalar grants %v, want one per scalar qglobal", len(grants), grants)
	}
}

// The 2026-11-23 patch: Solusek Ro and Bertoxxulous credit no longer need the
// five minis / Carpryn's key. Those are now "bonus" rows: optional, counted
// only once done (TestResolveBonusRows covers the tally).
func TestSolRoAndBertoxNoLongerGatedByEntrySteps(t *testing.T) {
	states := []State{
		{FlagID: "solro_miak", Done: true, Source: SourceManual},
		{FlagID: "potac_rallos", Done: true, Source: SourceManual},
		{FlagID: "pod_postflag", Done: true, Source: SourceManual},
	}
	r := Resolve(states)
	for _, f := range r.Flags {
		switch f.ID {
		case "solro_solusek", "cod_bertox":
			if f.Locked {
				t.Errorf("%s locked, missing %v", f.ID, f.Missing)
			}
		case "solro_xuzl", "cod_carpryn":
			if !f.Optional || f.Role != "bonus" {
				t.Errorf("%s optional=%v role=%q, want a bonus row", f.ID, f.Optional, f.Role)
			}
		}
	}
}

func mustStates(t *testing.T, s *Store, char string) []State {
	t.Helper()
	st, err := s.Get(char)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

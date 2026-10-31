package popflag

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Real grant pairs captured from a live Quarm log (2026-10). The NPC line is
// always followed by the flag line, same timestamp.
var liveGrantPairs = []struct {
	flagID string
	npc    string
	flag   string
}{
	{
		"ponb_postflag",
		"Thelin tells you, 'I apologize but I cannot stand to greet you. I am still quite weak and recovery will be a long and painful road, though it is a most welcome venture. I want you to know how much I appreciate your bravery in helping me. I am forever in your debt.'",
		"You receive a character flag!",
	},
	{
		"potor_preflag",
		"Fahlia Shadyglade tells you, 'Wonderful. I did not think that an outsider was one that I could trust to aid me in this. One name that Tylis has mentioned in agony is that of Maareq. I do not know whom this is, but he must be instrumental in Tylis' suffering. You must find Maareq and do what you must to release Tylis from this torture.'",
		"You have received a character flag!",
	},
	{
		"pok_librarian",
		"Maelin tells you, 'This is quite interesting friends.  It is a detailed document on what has become of Zebuxoruk.  It seems that he had been tampering with translating works of the gods for mortals.  This knowledge was leading them to enlightenment and finally to ascention into dietyhood.  This angered the gods, for they felt that any diety should be chosen by them.  Ascension of any mortal was not something they were prepared to deal with.'",
		"You have received a character flag!",
	},
	{
		"solro_miak",
		"Miak the Searedsoul says 'I do not know if I will ever be able to correct the portal's destination, but if you have the true route for the portal in your mind I will be able to channel the portal to the correct location.'",
		"You have received a character flag!",
	},
}

func TestMatchGrantLineLive(t *testing.T) {
	for _, p := range liveGrantPairs {
		id, ok := MatchGrantLine(p.npc)
		if !ok || id != p.flagID {
			t.Errorf("MatchGrantLine(%.40q) = %q,%v, want %q", p.npc, id, ok, p.flagID)
		}
		if !IsFlagGrantLine(p.flag) {
			t.Errorf("IsFlagGrantLine(%q) = false", p.flag)
		}
	}
}

// Non-granting lines from the same NPCs' Lua (the hail/flavor branches) must
// never match — they print in states where the flag is NOT being granted.
func TestMatchGrantLineRejectsFlavor(t *testing.T) {
	for _, line := range []string{
		"Adroha Jezith tells you, 'This is wonderful! Thelin is starting to recover slowly.  He still needs to rest here so please try not to bother him too much!'",
		"Adroha gives a soft sigh of woe, 'You won't be getting any response from Thelin.  Ever since his injury he has been in a deep [coma] with no sign of ever recovering.'",
		"Adler Fuirstel tells you, 'Please you must hurry!  Take the ward that surrounds you back to my brother and lift the sickness that has come over him!'",
		"Adler Fuirstel tells you, 'Your service to me and my brother will never be forgotten.  We are concentrating on how to close this portal now.'",
		"Elder Fuirstel groans in agony.  His tainted breath surrounds your face.",
		"Fahlia Shadyglade tells you, 'This is wonderful! I am so happy that you could help to free Tylis from Saryrn's grasp!  Please try not to bother him too much. He will need time to recover.'",
		"Fahlia Shadyglade tells you, 'Greetings travelers. Please excuse me but I must attend to Tylis for his condition does not improve.'",
		"Mavuin tells you, 'Please it is the only hope that I live for.  Deliever me from this horrid fate.'",
		"Giwin Mirakon tells you, 'Well go on then, what are you cowering around for, let us see what kind of warrior you are!'",
		"Maelin tells you, 'Welcome to Myrist!  This is truly something for me to be proud of.'",
		"Maelin tells you, 'Welcome back my friends.  I assure you that I have been studying the Cipher of Druzzil very diligently.'",
		"Maelin tells you, 'What is this that you have found?  It appears to be written in the divine language.'",
		"Maelin tells you, 'There is no way to escape from the prison that is The Plane of Time.  I am sorry but your quest for information ends here.'",
		"Miak the Searedsoul says 'Greetings traveler.  I am Miak, guardian of the portal to the greater elemental [Plane of Fire.]'",
		"Miak the Searedsoul says 'You are the one that discovered the portal's destination.  All you need to do is concentrate on the correct path, the rest will be done by the foci and myself.'",
		"Mavuin tells the guild, 'So you have pleaded my case to the Tribunal, I am most thankful.'",
	} {
		if id, ok := MatchGrantLine(line); ok {
			t.Errorf("MatchGrantLine(%.50q) matched %q, want no match", line, id)
		}
	}
}

// A grant line must never be swallowed by the Seer or #popflags buffers —
// either would apply a (replacing) reading built from the wrong text.
func TestGrantLinesDisjointFromReadings(t *testing.T) {
	for _, f := range Flags() {
		for _, gl := range f.GrantLines {
			if MatchSeerLine(gl) {
				t.Errorf("%s grant line matches the Seer buffer: %q", f.ID, gl)
			}
			if MatchPopFlagsLine(gl) {
				t.Errorf("%s grant line matches the #popflags buffer: %q", f.ID, gl)
			}
		}
	}
	// The checklist-memory line a projection prints when hailed without its
	// preflag is a different grant and must never pair.
	if IsFlagGrantLine("You have received a new checklist flag!") {
		t.Error("checklist flag line accepted as a character flag line")
	}
	for l := range flagGrantLines {
		if MatchSeerLine(l) || MatchPopFlagsLine(l) {
			t.Errorf("flag line %q matches a reading buffer", l)
		}
	}
}

// Every annotated step must be grantable: an always-up required hail with a
// scalar qglobal (ApplyHailGrant floors it) and a reminder zone + NPC. Grant
// lines must also be unambiguous across the dataset.
func TestGrantDatasetInvariants(t *testing.T) {
	seen := map[string]string{}
	n := 0
	for _, f := range Flags() {
		if len(f.GrantLines) == 0 {
			if f.HailZone != "" {
				t.Errorf("%s has hail_zone but no grant_lines", f.ID)
			}
			continue
		}
		n++
		if f.StepKind != "hail" || f.Optional {
			t.Errorf("%s: grant lines on a non-required or non-hail step (kind=%q optional=%v)", f.ID, f.StepKind, f.Optional)
		}
		if f.Qglobal == "" || f.BitPosition > 0 || atoi(f.QglobalValue) < 1 {
			t.Errorf("%s: needs a scalar qglobal value, got %q=%q bit=%d", f.ID, f.Qglobal, f.QglobalValue, f.BitPosition)
		}
		if f.HailZone == "" || f.HailNPC == "" {
			t.Errorf("%s: missing hail_zone/hail_npc", f.ID)
		}
		for _, gl := range f.GrantLines {
			if prev, dup := seen[collapseSpaces(gl)]; dup {
				t.Errorf("grant line shared by %s and %s", prev, f.ID)
			}
			seen[collapseSpaces(gl)] = f.ID
			if id, _ := MatchGrantLine(gl); id != f.ID {
				t.Errorf("%s grant line resolves to %q (shadowed by an earlier prefix)", f.ID, id)
			}
		}
	}
	if n != 14 {
		t.Errorf("annotated grant steps = %d, want 14", n)
	}
}

func TestConsumerHailGrantPair(t *testing.T) {
	for _, p := range liveGrantPairs {
		t.Run(p.flagID, func(t *testing.T) {
			s := openTempStore(t)
			const char = "Osui"
			c := NewConsumer(s, func() string { return char })
			ts := time.Unix(5000, 0)
			c.HandleLine(ts, p.npc)
			c.HandleLine(ts, p.flag)
			if row := doneByID(t, s, char)[p.flagID]; !row.Done || row.Source != SourceAuto {
				t.Errorf("%s = %+v, want done/auto", p.flagID, row)
			}
		})
	}
}

func TestConsumerHailGrantRequiresAdjacentFlagLine(t *testing.T) {
	miak := liveGrantPairs[3]
	cases := []struct {
		name  string
		lines []string
		gap   time.Duration
	}{
		// A groupmate's keyword makes Miak Say the line to everyone nearby, but
		// only they receive the flag line.
		{"npc line alone", []string{miak.npc, "Tunedup says, 'portal's destination'"}, 0},
		{"interleaved line", []string{miak.npc, "Darkclaw tells Soslead:5, 'hi'", miak.flag}, 0},
		{"flag line alone", []string{miak.flag}, 0},
		{"too far apart", []string{miak.npc, miak.flag}, 3 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTempStore(t)
			c := NewConsumer(s, func() string { return "Osui" })
			ts := time.Unix(6000, 0)
			for i, l := range tc.lines {
				if i == len(tc.lines)-1 {
					ts = ts.Add(tc.gap)
				}
				c.HandleLine(ts, l)
			}
			if row, ok := doneByID(t, s, "Osui")["solro_miak"]; ok {
				t.Errorf("solro_miak = %+v, want no row", row)
			}
		})
	}
}

func TestConsumerHailGrantZoneGate(t *testing.T) {
	s := openTempStore(t)
	c := NewConsumer(s, func() string { return "Osui" })
	c.HandleEvent("zone", "Plane of Knowledge")
	ts := time.Unix(7000, 0)
	// Fahlia stands in Tranquility; a pair seen while the tracked zone is
	// Knowledge is rejected.
	c.HandleLine(ts, liveGrantPairs[1].npc)
	c.HandleLine(ts, liveGrantPairs[1].flag)
	if _, ok := doneByID(t, s, "Osui")["potor_preflag"]; ok {
		t.Fatal("potor_preflag committed outside its zone")
	}
	c.HandleEvent("zone", "Plane of Tranquility")
	c.HandleLine(ts, liveGrantPairs[1].npc)
	c.HandleLine(ts, liveGrantPairs[1].flag)
	if !doneByID(t, s, "Osui")["potor_preflag"].Done {
		t.Fatal("potor_preflag not committed in its zone")
	}
}

// A grant must never override a manual retraction, and must survive a later
// PARTIAL #popflags reading that doesn't cover its tier.
func TestApplyHailGrantPrecedenceAndSurvival(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	if err := s.SetManual(char, "pod_preflag", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyHailGrant(char, "pod_preflag", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if row := doneByID(t, s, char)["pod_preflag"]; row.Done || row.Source != SourceManual {
		t.Errorf("pod_preflag = %+v, want manual retraction kept", row)
	}

	if _, err := s.ApplyHailGrant(char, "ponb_postflag", time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	// thelin>=4 implies the whole Nightmare chain.
	rows := doneByID(t, s, char)
	for _, id := range []string{"ponb_preflag", "ponb_hedge", "ponb_terris", "ponb_postflag"} {
		if !rows[id].Done {
			t.Errorf("%s not done after thelin=4 grant", id)
		}
	}
	// A Tier 2 report knows nothing about thelin; the floor must carry over.
	report := ParsePopFlagsReport([]string{"=== Tier 2 Progression ===", "Lower Crypt access: Unlocked"})
	if _, err := s.ApplyPopFlagsReport(char, report, "raw", time.Unix(300, 0)); err != nil {
		t.Fatal(err)
	}
	if !doneByID(t, s, char)["ponb_postflag"].Done {
		t.Error("ponb_postflag retracted by a partial #popflags reading")
	}
	// A repeat hail is idempotent.
	changed, err := s.ApplyHailGrant(char, "ponb_postflag", time.Unix(400, 0))
	if err != nil || changed {
		t.Errorf("repeat grant changed=%v err=%v, want no change", changed, err)
	}
}

type recordHub struct{ events []ws.Event }

func (h *recordHub) Broadcast(e ws.Event) { h.events = append(h.events, e) }

func TestPendingHailsAndReminder(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	c := NewConsumer(s, func() string { return char })

	var got []HailReminder
	c.SetOnHailReminder(func(_, _ string, h []HailReminder) { got = h })

	// No stored rows yet: never-tracked characters get no reminder.
	c.HandleEvent("zone", "Plane of Tranquility")
	if got != nil {
		t.Fatalf("reminder fired for an untracked character: %+v", got)
	}

	// Terris Thule kill (auto) makes Elder Poxbourne available.
	for _, id := range []string{"ponb_preflag", "ponb_hedge"} {
		if err := s.SetManual(char, id, true); err != nil {
			t.Fatal(err)
		}
	}
	c.HandleEvent("kill", "Terris Thule")
	c.HandleEvent("zone", "Plane of Tranquility")
	ids := map[string]bool{}
	for _, h := range got {
		ids[h.FlagID] = true
	}
	if !ids["ponb_postflag"] || !ids["pod_preflag"] || !ids["solro_miak"] {
		t.Errorf("reminders = %+v, want ponb_postflag, pod_preflag, solro_miak", got)
	}
	if ids["ponb_preflag"] || ids["potor_preflag"] || ids["pod_postflag"] {
		t.Errorf("reminders include done/locked steps: %+v", got)
	}

	got = nil
	c.HandleEvent("zone", "Plane of Disease")
	if got != nil {
		t.Errorf("reminder fired in a zone with no hail NPCs: %+v", got)
	}
}

func TestReminderFireCooldownAndText(t *testing.T) {
	cfgMgr, err := config.LoadFrom(t.TempDir() + "/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := cfgMgr.Get()
	cfg.Preferences.PoPHailReminder.Enabled = true
	cfg.Preferences.PoPHailReminder.OverlayEnabled = true
	if err := cfgMgr.Update(cfg); err != nil {
		t.Fatal(err)
	}
	hub := &recordHub{}
	r := NewReminder(hub, cfgMgr)
	now := time.Unix(10000, 0)
	r.now = func() time.Time { return now }

	hails := []HailReminder{
		{FlagID: "ponb_postflag", NPC: "Elder Poxbourne"},
		{FlagID: "pod_postflag", NPC: "Elder Fuirstel"},
		{FlagID: "cod_postflag", NPC: "Elder Fuirstel"},
	}
	r.Fire("Osui", "Plane of Tranquility", hails)
	r.Fire("Osui", "Plane of Tranquility", hails) // within cooldown
	if len(hub.events) != 1 {
		t.Fatalf("events = %d, want 1", len(hub.events))
	}
	fired := hub.events[0].Data.(trigger.TriggerFired)
	if want := "Flag hail available: Elder Poxbourne and Elder Fuirstel"; fired.Actions[0].Text != want {
		t.Errorf("text = %q, want %q", fired.Actions[0].Text, want)
	}
	now = now.Add(time.Duration(config.DefaultPoPHailReminderCooldownSecs+1) * time.Second)
	r.Fire("Osui", "Plane of Tranquility", hails)
	if len(hub.events) != 2 {
		t.Errorf("events after cooldown = %d, want 2", len(hub.events))
	}
}

// A '#popflags' section only retracts flags whose qglobals it printed an
// exact value for: a Tier 2 run must leave a kill-detected Tier 1 row alone,
// while a Tier 1 run that reports the stage still retracts it.
func TestPopFlagsReportRetractionScopedToReportedQglobals(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	if _, err := s.SetAuto(char, "ponb_terris"); err != nil {
		t.Fatal(err)
	}
	tier2 := ParsePopFlagsReport([]string{"=== Tier 2 Progression ===", "Lower Crypt access: Unlocked"})
	if _, err := s.ApplyPopFlagsReport(char, tier2, "raw", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if row := doneByID(t, s, char)["ponb_terris"]; !row.Done || row.Source != SourceAuto {
		t.Fatalf("ponb_terris = %+v, want auto row kept across a Tier 2 report", row)
	}

	tier1 := newPopFlagsReport()
	tier1.Section = PopFlagsTier1
	tier1.Exact["thelin"] = "2"
	if _, err := s.ApplyPopFlagsReport(char, tier1, "raw", time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	rows := doneByID(t, s, char)
	if row, ok := rows["ponb_terris"]; ok && row.Done {
		t.Errorf("ponb_terris = %+v, want retracted by a report showing thelin=2", row)
	}
	if !rows["ponb_hedge"].Done {
		t.Error("ponb_hedge should be done at thelin=2")
	}
}

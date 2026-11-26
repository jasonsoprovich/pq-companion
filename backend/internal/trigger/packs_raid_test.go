package trigger

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// raidEventSamples pairs each raid pack event trigger with a line the game
// actually writes (quoted from Project Quarm's quest scripts). Every event
// trigger must have at least one sample, and every sample must match.
var raidEventSamples = map[string][]string{
	"Trial started":         {"The Tribunal says, 'Then begin.'"},
	"Trial already running": {"The Tribunal says, 'That trial is already underway.  You must wait.'"},
	"Trial still running":   {"Agent of The Tribunal says, 'The trial is yet underway.  You must wait.'"},
	"Trial reopens (failed)": {
		"The prisoners cry is cut off as his body crumples to the ground.  You have failed.",
		"The prisoner's screams cut off suddenly as he topples over, dead. You have failed.",
		"The prisoner collapses and lies still, no longer shielding herself from the stones.  You have failed.",
		"The prisoner gives one final twitch and suddenly becomes still, its limbs no longer flailing.  You have failed.",
		"Flames burst forth from the stake!",
	},
	"Trial reopens (completed)":       {"The Tribunal tells you, 'You have completed a trial - impressive for mortals.  You can tell Mavuin that we will hear his plea.  We will seek him out as time befits us.'"},
	"Execution — executioner walking": {"An executioner intones, 'Do you have any final words before your sentence is carried out?'"},
	"Hanging — prisoner choking":      {"The prisoner begins to choke as an invisible noose tightens around her neck."},
	"Seventh Hammer — Tremor of Judgment": {
		"The Seventh Hammer says, 'Prepare for judgement mortals!'",
		"The tremor of judgment rumbles around you.",
		"Larcen shudders as the ground shakes beneath them.",
	},
	"Seventh Hammer — Verdict of Eternity": {
		"The Seventh Hammer says, 'Very well mortals, Now begins the test of your very souls!'",
		"You feel your soul being judged.",
		"Larcen staggers under the judgment of the Tribunal.",
	},
	"Maze — Thelin walking":            {"Thelin Poxbourne says, 'Please stay close, I know not what horror Terris will unleash upon us.'"},
	"Maze — dagger piece found":        {"Thelin Poxbourne kneels down and picks up a dagger hilt.", "Thelin Poxbourne picks up a the top half of a dagger blade."},
	"Maze — final shard":               {"Thelin Poxbourne takes the final shard from you and places all of the pieces on the ground, with unseen hands the dagger moves together and fuses itself back together into one complete piece."},
	"Maze — Thelin asleep":             {"Thelin Poxbourne closes his eyes and falls asleep immediately. He looks peaceful for a moment, then screams in agony!"},
	"Mujaki — Etumer ready":            {"Etumer says, 'Perhaps you could find some way to overwhelm Mujaki?  I might be able to work an enchantment to transport you closer to my brother.'"},
	"Mujaki — Etumer can't port":       {"Etumer says, 'I... I don't think I can muster the force to work this teleportation. Allow me to rest and return to me later, perhaps I can work it then.'"},
	"Nitram — walking to Xanamech":     {"Nitram Anizok says, 'Excellent!  This is wonderful, please follow me!  I will show you the power of my greatest invention.'"},
	"Xanamech — awake":                 {"Nitram Anizok places the power unit right into the chest of the huge metallic beast.  It roars to life as steam begins pouring out of its huge gaping maw.  Suddenly it begins to attack!"},
	"Terris — engaged":                 {"The sound of a thousand terrified screams fills your head.  You feel yourself becoming a part of the fabric of this nightmare realm."},
	"Terris — 50% Direption of Dreams": {"As if in a waking nightmare, you feel your movements slow and your arms begin to fail you."},
	"Terris — 40% gargoyles":           {"The air grows thick with the smell of burning mana.  A rumbling sound draws your attention to the massive statues that rest above the ancient monoliths.  The statues begin to crumble, as they shift their attention from the heavens to you!"},
	"Bertox — waves begin":             {bertoxPrepare},
	"Bertox — event deadline":          {bertoxPrepare},
	"Bertox — waves":                   {"A foul wind is felt carrying on it the stench of death and decay.  Suddenly a thunderous bang is heard throughout the crypt and then these words, 'Great soldiers of decay you are summoned forth to do battle with these infidels!'"},
	"Bertox — king summoned":           {"An unsettling feeling of fear passes through you as the summoners cry, 'We call to you, corrupted King of Lxanvom, Darwol Adan! Your master has need of you!'"},
	"Bertox — last four kings":         {"An unsettling feeling of fear passes through you as you hear the summoners finish a dark incantation then cry out saying, 'We call to you the last corrupted Kings of Lxanvom. Meedo Adan! Qezzin Adan! Pzo Adan! Bhaly Adan! Your master has need of you!'"},
	"Bertox — Bertoxxulous up":         {"A sinister vision enters your mind of a faceless one handsome yet dead and decaying. The vision then shifts to that of a torn bestial creature and a loud shout is heard, 'Defilers death comes for you today!'"},
	"Bertox — 5 minutes left":          {"Dark voices whisper in your ear saying, 'Your time is close in coming to an end. Time to flee little ones!'"},
	"Bertox — event failed":            {"Harsh laughter echoes around the crypt and a voice then speaks saying, 'Perhaps you would care to try when you are more powerful fools.'  The harsh laughter continues softly as all of  the summoned minions of Bertoxxulous vanish."},
	"Bertox — defeated":                {"A nimbus of light floods through the crypt in one magnificent wave as an earth shattering howl is heard.  The Bringer of Plagues, Lord of All Disease and Decay, Bertoxxulous has been defeated."},
	"Terris — leashed":                 {"Terris Thule disappears into the ether and reforms at the center of her chamber, cleansed of your magic!"},
}

// bertoxPrepare is codecay/encounters/Bertox.lua RP_TEXT.prepare.
const bertoxPrepare = "Crazed laughter is heard as you notice a foul creature standing before you. The creature then speaks saying, 'Violators of the depths of Lxanvom shall pay with your lives!'  The foul minion of decay then begins chanting a dark ritual."

func openQuarmDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// raidNPCSpells returns spell id -> list entry for an npc_spells list and
// its parents, plus the list's melee proc.
type raidListEntry struct {
	proc        bool
	recastDelay int
}

func raidNPCSpells(t *testing.T, conn *sql.DB, listID int) map[int]raidListEntry {
	t.Helper()
	out := map[int]raidListEntry{}
	seen := map[int]bool{}
	for listID > 0 && !seen[listID] {
		seen[listID] = true
		var proc, parent int
		err := conn.QueryRow(`SELECT attack_proc, parent_list FROM npc_spells WHERE id = ?`, listID).Scan(&proc, &parent)
		if err != nil {
			break
		}
		rows, err := conn.Query(`SELECT spellid, recast_delay FROM npc_spells_entries WHERE npc_spells_id = ?`, listID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, rd int
			if err := rows.Scan(&id, &rd); err != nil {
				t.Fatal(err)
			}
			if _, ok := out[id]; !ok {
				out[id] = raidListEntry{recastDelay: rd}
			}
		}
		rows.Close()
		if proc > 0 {
			if _, ok := out[proc]; !ok {
				out[proc] = raidListEntry{proc: true}
			}
		}
		listID = parent
	}
	return out
}

var raidCounterSPA = map[int]string{35: "Disease", 36: "Poison", 116: "Curse"}

// TestRaidPacks_MatchQuarmDB checks every boss and spell in the raid tables
// against quarm.db: the boss exists with that level and resists, each spell
// is on its npc_spells list (or is its proc), and the spell's name, land
// messages, recast, cure type and beneficial flag match.
func TestRaidPacks_MatchQuarmDB(t *testing.T) {
	conn := openQuarmDB(t).DB

	effCols := make([]string, 0, 24)
	for i := 1; i <= 12; i++ {
		effCols = append(effCols, fmt.Sprintf("effectid%d", i), fmt.Sprintf("effect_base_value%d", i))
	}
	playerCast := make([]string, 15)
	for i := range playerCast {
		playerCast[i] = fmt.Sprintf("classes%d < 255", i+1)
	}
	sharedQ := `SELECT COUNT(*) FROM spells_new WHERE cast_on_other = ? AND id != ? AND (` + strings.Join(playerCast, " OR ") + `)`

	for _, d := range raidPackDefs() {
		for _, b := range d.Bosses {
			name := strings.ReplaceAll(b.Name, " ", "_")
			rows, err := conn.Query(`SELECT level, MR, CR, FR, npc_spells_id, COALESCE(special_abilities,'') FROM npc_types WHERE lower(name) IN (lower(?), lower(?))`, name, "#"+name)
			if err != nil {
				t.Fatal(err)
			}
			var listID int
			var abilities string
			found := false
			for rows.Next() {
				var lvl, mr, cr, fr, lid int
				var sa string
				if err := rows.Scan(&lvl, &mr, &cr, &fr, &lid, &sa); err != nil {
					t.Fatal(err)
				}
				if lvl == b.Level && mr == b.MR && cr == b.CR && fr == b.FR {
					found, listID, abilities = true, lid, sa
				}
			}
			rows.Close()
			if !found {
				t.Errorf("%s: no npc_types row for %q with level %d and resists %d/%d/%d", d.Name, b.Name, b.Level, b.MR, b.CR, b.FR)
				continue
			}
			has := map[int]bool{}
			for _, a := range db.ParseSpecialAbilities(abilities) {
				if a.Value > 0 {
					has[a.Code] = true
				}
			}
			var traits []string
			for _, tl := range raidTraitLabels {
				if has[tl.Code] {
					traits = append(traits, tl.Label)
				}
			}
			if got := strings.Join(traits, ", "); got != b.Traits {
				t.Errorf("%s / %s: Traits %q, quarm.db %q", d.Name, b.Name, b.Traits, got)
			}
			list := raidNPCSpells(t, conn, listID)
			for _, s := range b.Spells {
				where := fmt.Sprintf("%s / %s / %s (%d)", d.Name, b.Name, s.Name, s.ID)
				entry, ok := list[s.ID]
				if !ok {
					t.Errorf("%s: not on the boss's spell list", where)
					continue
				}
				if entry.proc != s.Proc {
					t.Errorf("%s: Proc = %v, quarm.db says %v", where, s.Proc, entry.proc)
				}
				var (
					spName, you, other string
					recastMs, good     int
				)
				effs := make([]int, 24)
				dest := []any{&spName, &you, &other, &recastMs, &good}
				for i := range effs {
					dest = append(dest, &effs[i])
				}
				err := conn.QueryRow(`SELECT name, COALESCE(cast_on_you,''), COALESCE(cast_on_other,''), recast_time, goodEffect, `+
					strings.Join(effCols, ", ")+` FROM spells_new WHERE id = ?`, s.ID).Scan(dest...)
				if err != nil {
					t.Errorf("%s: %v", where, err)
					continue
				}
				if spName != s.Name {
					t.Errorf("%s: name is %q in quarm.db", where, spName)
				}
				if (good != 0) != s.Buff {
					t.Errorf("%s: Buff = %v, goodEffect = %d", where, s.Buff, good)
				}
				if s.Buff {
					continue
				}
				if you != s.OnYou {
					t.Errorf("%s: OnYou %q, quarm.db %q", where, s.OnYou, you)
				}
				var shared int
				if err := conn.QueryRow(sharedQ, other, s.ID).Scan(&shared); err != nil {
					t.Fatal(err)
				}
				wantOther := other
				if other == "" || shared > 0 {
					wantOther = ""
				}
				if s.OnOther != wantOther {
					t.Errorf("%s: OnOther %q, want %q (player-shared: %v)", where, s.OnOther, wantOther, shared > 0)
				}
				counter := ""
				for i := 0; i < 24; i += 2 {
					if c, ok := raidCounterSPA[effs[i]]; ok && effs[i+1] > 0 {
						counter = c
						break
					}
				}
				if s.Counter != counter {
					t.Errorf("%s: Counter %q, quarm.db %q", where, s.Counter, counter)
				}
				want := 0.0
				if !entry.proc {
					switch rd := entry.recastDelay; {
					case rd > 0 && rd < 10000:
						want = float64(rd)
					case rd == -2 || rd >= 10000:
						want = 0
					default:
						want = float64(recastMs) / 1000
					}
				}
				if s.Recast != want {
					t.Errorf("%s: Recast %g, quarm.db %g", where, s.Recast, want)
				}
			}
		}
	}
}

// TestRaidPacks_Shape checks the built triggers: RE2-only patterns, no two
// triggers in a pack sharing a pattern, Who cards matching the /tell echo,
// every event matching its script lines, and the credit in descriptions.
func TestRaidPacks_Shape(t *testing.T) {
	sampled := map[string]bool{}
	for _, d := range raidPackDefs() {
		p := d.build()
		if !strings.Contains(p.Description, "Xanax <Former Glory>") {
			t.Errorf("%s: description lacks the Xanax <Former Glory> credit", p.PackName)
		}
		patterns := map[string]string{}
		names := map[string]bool{}
		for _, tr := range p.Triggers {
			if tr.PackName != p.PackName {
				t.Errorf("%s / %s: PackName %q", p.PackName, tr.Name, tr.PackName)
			}
			if names[tr.Name] {
				t.Errorf("%s: duplicate trigger name %q", p.PackName, tr.Name)
			}
			names[tr.Name] = true
			all := []string{tr.Pattern}
			for _, x := range tr.ExtraPatterns {
				all = append(all, x.Pattern)
			}
			var res []*regexp.Regexp
			for _, pat := range all {
				re, err := regexp.Compile(pat)
				if err != nil {
					t.Errorf("%s / %s: pattern is not RE2: %v", p.PackName, tr.Name, err)
					continue
				}
				res = append(res, re)
			}
			// Extra patterns may repeat (one aggro line starting two
			// countdowns); primary patterns may not.
			if prev, ok := patterns[tr.Pattern]; ok {
				t.Errorf("%s: %q and %q share a pattern", p.PackName, prev, tr.Name)
			}
			patterns[tr.Pattern] = tr.Name
			matchAny := func(line string) bool {
				for _, re := range res {
					if re.MatchString(line) {
						return true
					}
				}
				return false
			}
			switch {
			case strings.HasSuffix(tr.Name, " — Who"):
				boss := strings.TrimSuffix(tr.Name, " — Who")
				under := strings.ReplaceAll(boss, " ", "_")
				echoes := []string{boss, strings.ToLower(under)}
				if strings.Contains(boss, " ") {
					echoes = append(echoes, under[:len(under)-len(lastWord(boss))+1])
				} else if matchAny(boss[:1] + " is not online at this time.") {
					t.Errorf("%s: one-word Who card matches a one-letter tell", tr.Name)
				}
				for _, echo := range echoes {
					if line := echo + " is not online at this time."; !matchAny(line) {
						t.Errorf("%s: Who card doesn't match %q", p.PackName, line)
					}
				}
				if matchAny("Someoneelse is not online at this time.") {
					t.Errorf("%s: Who card matches an unrelated name", tr.Name)
				}
			case len(tr.BossCasters) > 0 || strings.HasSuffix(tr.Name, " on you"):
				// Spell triggers: built from the DB-checked table.
			default:
				samples, ok := raidEventSamples[tr.Name]
				if !ok {
					t.Errorf("%s / %s: event trigger has no sample line in raidEventSamples", p.PackName, tr.Name)
				}
				sampled[tr.Name] = true
				for _, line := range samples {
					if !matchAny(line) {
						t.Errorf("%s / %s: doesn't match its sample %q", p.PackName, tr.Name, line)
					}
				}
			}
		}
	}
	for name := range raidEventSamples {
		if !sampled[name] {
			t.Errorf("raidEventSamples has %q but no raid pack has that event trigger", name)
		}
	}
}

func lastWord(s string) string {
	f := strings.Fields(s)
	return f[len(f)-1]
}

// TestRaidPacks_SharedSpellTimer: a spell two bosses cast becomes one timer
// listing both casters, each with its own recast when they differ.
func TestRaidPacks_SharedSpellTimer(t *testing.T) {
	p := raidPoPTier1().build()
	var fetter *Trigger
	for i := range p.Triggers {
		if p.Triggers[i].SpellID == 3194 && len(p.Triggers[i].BossCasters) > 0 {
			fetter = &p.Triggers[i]
		}
	}
	if fetter == nil {
		t.Fatal("no Greater Fetter timer in PoP Tier 1")
	}
	if fetter.TimerDurationSecs != 15 || len(fetter.BossCasters) != 2 {
		t.Fatalf("Greater Fetter = %gs casters %+v; want 15s, Teion + Punisher of Flame", fetter.TimerDurationSecs, fetter.BossCasters)
	}
	if c := fetter.BossCasters[0]; c.Name != "Gallows Master Teion" || c.DurationSecs != 0 {
		t.Errorf("first caster %+v; want Teion at the trigger's 15s", c)
	}
	if c := fetter.BossCasters[1]; c.Name != "Punisher of Flame" || c.DurationSecs != 2.25 {
		t.Errorf("second caster %+v; want Punisher of Flame at 2.25s", c)
	}
}

// TestRaidPacks_InstalledPackBindsBosses replays a short Plane of Disease log
// through the engine with the real PoP Tier 1 pack installed: Grummus and
// Gryme both cast Stench of Decay, and each gets its own 40s countdown.
func TestRaidPacks_InstalledPackBindsBosses(t *testing.T) {
	s := openTestStore(t)
	sink := &recordSink{}
	e := NewEngine(s, ws.NewHub(), sink, nil)
	if err := InstallPack(s, raidPoPTier1().build()); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}
	e.Reload()

	t0 := time.Now()
	e.Handle(t0, "Grummus begins to cast a spell.")
	e.Handle(t0.Add(time.Second), "Gryme the Crypt Guardian begins to cast a spell.")
	e.Handle(t0.Add(3*time.Second), "Larcen begins to cough uncontrollably.")
	e.Handle(t0.Add(3*time.Second), "Your lungs are filled with the stench of decay.")
	e.Handle(t0.Add(4*time.Second), "Bob begins to cough uncontrollably.")
	e.Handle(t0.Add(4*time.Second), "Sue begins to cough uncontrollably.")

	if len(sink.starts) != 2 {
		t.Fatalf("got %d timers, want 2: %+v", len(sink.starts), sink.starts)
	}
	for i, want := range []string{"Grummus", "Gryme the Crypt Guardian"} {
		if st := sink.starts[i]; st.target != want || st.duration != 40 || !st.isCaster {
			t.Errorf("timer %d = %+v; want %s, 40s, boss cooldown", i, st, want)
		}
	}
}

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
	"Honor — Crazed trial failed":      {"A woman screams in anguish as she's forcibly taken away by a band of crazed norrathians."},
	"Honor — trial flag":               {"An ethereal mist descends from the air and surrounds your very being. Your body begins to emanate with the power of Alekson Garn."},
	"Agnarr — engaged":                 {"Agnarr the Storm Lord says 'Fool! This is the realm of the Storm Giants now, and you hope to defeat me here?'"},
	"Agnarr — portals": {
		"Agnarr strikes his staff to the ground, causing great ripples of energy to rage across the room.",
		"Agnarr the Storm Lord says 'Fool! This is the realm of the Storm Giants now, and you hope to defeat me here?'",
	},
	"Rallos — Warlord stirs":    {"The air of Drunder grows strangely cold as a rumble shakes through the fortress' walls. The Warlord stirs."},
	"Rallos — sons fallen":      {"Rallos Zek takes notice at the death of his sons. The Warlord is impressed. \"Come then, mortals! Raise your blades against the might of the Warlord!\""},
	"Rallos — engaged":          {"Rallos Zek raises his axe as flames explode from it! The Warlord has accepted your challenge!"},
	"Rallos — leaps to the pit": {"Rallos Zek leaps from the War Room into the cleared pit below! The force of his landing shakes the very foundations of Drunder, sending dust and stone crashing from the halls above."},
	"Rallos — corpse wave": {
		"The corpses of the unworthy begin to twitch and spasm as fallen champions rise again to try and prove their worth to the might and glory of Rallos Zek!",
		"The Warlord meets your challenge in the pit of Drunder. The might and fury of War are upon you!",
	},
	"Rallos — leashed":                   {"Rallos begins to laugh, causing the earth to rumble around you. 'Enough of this foolishness!  The warlord has better things to do then chase petty mortals all day!'"},
	"Rallos — defeated":                  {"Rallos Zek, the Warlord, has fallen! The halls of Drunder echo with cries of rage as the very foundations of the Plane of Tactics tremble and quake as his colossal form crashes to the ground before the might of the Norrathians!"},
	"Doomfire — Guardian engaged":        {"The Guardian of Doomfire unleashes an echoing howl. Across the scorched battlefield, weapons clash and frenzied voices rise as the army of the Tyrant answers your challenge."},
	"Doomfire — event deadline":          {"The Guardian of Doomfire crashes to the scorched earth. Frenzied cries erupt as an Army of Flames burns across the scorched realm of Doomfire."},
	"Doomfire — phase 2 commanders":      {"The first ranks fall, but the roar of battle grows louder. Four commanders rise amid the flames, driving fresh soldiers forward in a frenzy of rage."},
	"Doomfire — phase 3 Council of Fire": {"The commanders collapse beneath the onslaught. Columns of fire tear through the air as the Council of Fire advances through the wreckage of the army."},
	"Doomfire — Fennin Ro up":            {"The Council of Fire falls. A deafening roar shakes the Burning Lands as the flames gather into the towering form of Fennin Ro. The Tyrant of Fire rises from the inferno, his wrath spilling across Doomfire."},
	"Doomfire — Fennin engaged":          {"The inferno erupts as Fennin Ro meets your challenge. \"Norrathians! You sought the power of a god. Now face the frenzy of Doomfire and the vengeance of its Tyrant!\""},
	"Doomfire — failed":                  {"All of Doomfire trembles as the voice of Fennin Ro booms, \"Enough, Norrathians! Carry the memory of this failure back to your world. Should you return, my wrath will be waiting!\" The army withdraws into the raging flames, leaving the scorched battlefield silent."},
	"Doomfire — Fennin defeated":         {"Fennin Ro, the Tyrant of Fire, falls! His colossal form crashes into the scorched earth as shockwaves tear through the Burning Lands."},
	"Sol Ro tower — cauldron flag":       {"As you place your hand on the burning cauldron, you feel a wealth of knowledge enter your mind."},
	"Sol Ro tower — portal blocked":      {"The lava runes scorch your skin as you run your hand across them."},
	"Air — Dust island named up":         {"Avatar of Dust shouts, 'Elementals of Air and Dust, I call upon you to aid me in the defense of this land!'"},
	"Air — Mist island named up":         {"Melernil Faal`Armanna shouts, 'Leave now foolish mortal you have begun to send events in motion that you cannot possibly understand!'"},
	"Air — Smoke island named up":        {"An Elemental Masterpiece shouts, 'The time has come for all intruders to be tested I will not allow you to pillage this plane any longer!'"},
	"Air — Wind island named up":         {"Pherlondien Clawpike shouts, 'I will certainly make sure that no mortal is allowed to defile this sacred realm!'"},
	"Coirnav — event clock": {
		coirnavShout("Povar", "vapor"),
		"The waters churn as three voices rise as one through Coirnav, echoing throughout the reef. \"The powers of Povar, E`ci and Tarew Marr are bound within me. You will not break the will of the Triumvirate!\"",
		"Ice spreads across the reef as vapor coils around Coirnav's towering form. His voice rolls through the depths. \"Our power gathers, and your time dwindles. Struggle against the currents! Spend what strength you have left. These waters will yield to no invader!\"",
		"The reef trembles beneath the gathering fury of water, ice and vapor. Coirnav's voice resounds through the depths. \"Your time is nearly spent! Finish what you began. Soon the united power of the Triumvirate shall sweep you from this domain!\"",
	},
	"Coirnav — next minion wave":       {coirnavShout("Povar", "vapor"), coirnavShout("E`ci", "ice")},
	"Coirnav — ice minions":            {coirnavShout("E`ci", "ice")},
	"Coirnav — water minions":          {coirnavShout("Tarew Marr", "water")},
	"Coirnav — Pwelon, Nrinda, Vamuil": {"The currents wrench across the reef as Pwelon, Nrinda and Vamuil gather before Coirnav. The Triumvirate speaks as one through the Avatar of Water: \"We will permit no further advance. Pwelon, Nrinda, Vamuil - unite and drive these invaders from the depths!\""},
	"Coirnav — banished":               {"Three voices thunder as one through Coirnav. \"The Triumvirate has spoken! These waters reject you. Be banished from our domain!\" A crushing surge tears through the reef as his summoned defenders dissolve into water, ice and vapor."},
	"Coirnav — defeated":               {"Coirnav, the Avatar of Water, has fallen! His colossal form collapses, driving a thunderous shockwave through the reef."},
	"Earth ring — complete": {
		"The temple's final defender breaks apart, and the ancient force binding its earthen body falls silent. The trial of the Dust Ring is complete.",
		"The towering guardian ruptures in a thunderous splash, leaving only heaps of muck and motionless pools of grime. The trial of the Mud Ring is complete.",
	},
	"Earth ring — failed": {
		"The allotted time has passed. The Rathe's summons is withdrawn, and the temple's guardians unravel into dust.",
		"The temple's ancient summons stirs the dust, but a guardian fails to take form. The trial cannot continue.",
		"The stone temple's interrupted trial cannot be resumed. The weight of its guardians breaks their failing forms, leaving heaps of silent stone.",
	},
	"Dust Ring — started":        {"An ancient command rolls through the temple as the dusty warder bursts into grit. Three streams of earth sweep toward its halls, and the remaining dust rises as devotees of the Rathe."},
	"Dust Ring — Triumvirate":    {"The last dust devotee breaks apart. Bound by the Rathe's will, the Triumvirate of Soil advances to pass judgment."},
	"Dust Ring — final defender": {"The last of the Triumvirate sinks into loose earth. The temple trembles beneath an ancient command as its final defender takes form."},
	"Dust Ring — guardians":      {"Three earthen forms rise at the temple's summons, their bodies bound by an ancient command. They advance to uphold the defender's judgment."},
	"Mud Ring — Sludge Lurker": {
		"The last mudwalker bursts in a spray of wet clay. Within the temple, a great mound of sludge swells into the hulking Sludge Lurker. The mudwalkers surrounding it stir.",
		"The final mudwalker bursts in a spray of filth. The Sludge Lurker heaves free, dragging its bulk through the mire as ropes of sludge drip from its body.",
	},
	"Mud Ring — mudlets":                                 {"The Sludge Lurker ruptures in a heavy splash, spraying muck and grime across the ground. Quivering clots of sludge pull themselves upright as grasping mudlets."},
	"Mud Ring — gorgers":                                 {"The Sludge Lurker collapses into a churning pool of filth. Four bloated gorgers heave themselves from the muck, dripping grime as they lumber toward you."},
	"Mud Ring — final guardian":                          {"The last filth gorger bursts, drenching the stones in greasy sludge. The scattered muck swells into a towering, misshapen guardian, its body seeping filth."},
	"Stone Ring — fortifications":                        {"Massive slabs of stone wrench themselves from the earth and lock together. The living fortifications advance with crushing weight, each stride grinding rock beneath them."},
	"Stone Ring — rubble mounds":                         {"The last fortification shatters with a thunderous crack. The Rock Monstrosity's massive frame splits into four mounds of rubble, each grinding with the strength of the broken whole."},
	"Stone Ring — Rock Monstrosity":                      {"The rubble locks together with a roar of grinding stone. The Rock Monstrosity rises once more, its massive frame bearing the gathered strength of the temple."},
	"Stone Ring — stone heaps":                           {"The Rock Monstrosity crashes apart, its weight shaking the temple. A towering stone figure rises and folds into stillness as heaps of broken rock grind forward to crush the intruders."},
	"Stone Ring — final defender":                        {"The last stone heap shatters. The temple's remaining strength gathers into its final defender, a grinding mass of stone and packed earth that shakes the ground as it advances."},
	"Vine Ring — bloodthirsty creatures":                 {"The last tainted rock beast crashes down. Living vines wrench its remains upright as the bloodthirsty creatures uncoil and lash out from the temple."},
	"Vine Ring — final defender":                         {"The last bloodthirsty creature collapses in a tangle of severed vines. Thick roots split the earth as the temple's final defender emerges, trailing coils of living growth."},
	"Vine Ring — next adds":                              {"The temple's defender drives its roots into the earth. Mangled creatures claw free amid a thrashing mass of vines."},
	"Rathe Council — engaged":                            {"The stone beneath Ragrax trembles as the Rathe Council speaks as one: \"We stand together against you, Norrathians. The balance must be restored.\""},
	"Rathe Council — first kill expires":                 {"One of the Rathe Council crumbles into lifeless stone. The remaining voices answer without hesitation: \"We remain united. We shall rise again. You will not unravel the balance through the fall of one.\""},
	"Rathe Council — councilman returned":                {"Stone knits together as a fallen Councilman rises anew. The Council speaks as one: \"We stand together, Norrathians. We shall not yield because one has fallen.\""},
	"Rathe Council — Avatar of Earth":                    {"The Rathe speak through the towering Avatar of Earth: \"We remain as one while battle is joined. We shall tolerate thirty-five minutes of hesitation. No more.\""},
	"Time — phase 2 wall opening":                        {"Ethereal mists gather at the far wall, causing it fade in and out of focus."},
	"Quarm — red head down":                              {"Quarm roars in pain as his red head explodes in a shower of flaming ethereal energy!"},
	"Quarm — blue head down":                             {"Quarm roars in pain as his blue head explodes into pure ethereal energy!"},
	"Quarm — white head down":                            {"Quarm shakes the cavern with a bellow of pain as the white head explodes into a swirling vortex of ethereal energy!"},
	"Quarm — Druzzil appears":                            {"Suddenly in a bright flash a feminine figure appears before you.  Your ears ring from the loud burst as she slowly materializes."},
	"Shei Vinitras — next Tatrua adds":                   {"Shei Vinitras Sivuelaeus Rulour ans Rashan!"},
	"Vyzh`dra — first Touch of Vinitras":                 {"Vyzh`dra the Exiled shouts 'Bow before your only god! I will tear all others from the heavens!'"},
	"Emperor Ssraeshza — spawns":                         {"  The Emperor twitches.  He appears to have been weakened by the destruction of the golem."},
	"Emperor Ssraeshza — Rage of Ssraeshza":              {"Emperor Ssraeshza  hit Tankname for 4000 points of non-melee damage."},
	"a glyph covered serpent — Wave of Death (stand up)": {"Your mind fills with images of death."},
	"Ring War — first wave":                              {"Seneschal Aldikar shouts, 'Good citizens of Thurgadin, hear me! Our city, our people, our very lives are in danger this day.'"},
	"Ring War — Narandi":                                 {"Seneschal Aldikar shouts, 'Good citizens of Thurgadin, hear me! Our city, our people, our very lives are in danger this day.'"},
	"Ring War — wave called early":                       {"Seneschal Aldikar shouts, 'Tankname has spotted tha' next wave! Charge!'"},
	"Ring War — Narandi arrives":                         {"Narandi the Wretched shouts, 'Meddling fools, you are no match for the likes of me. Be warned... Engage me at your own peril!'"},
	"Plane of Growth — protector respawn":                {"You have slain a protector of growth!", "a protector of growth has been slain by Tankname!"},
	"Temple of Veeshan — Acid Jet trap":                  {"Acid begins to eat at your flesh."},
	"Temple of Veeshan — Flame Jet trap":                 {"Your body is engulfed in a jet of flames."},
	"Master Yael — Death Touch":                          {"Master Yael shouts, 'Tankname!'"},
	"Dread — Death Touch":                                {"Dread shouts, 'Tankname!'"},
	"Fright — Death Touch":                               {"Fright shouts, 'Tankname!'"},
	"Bazzt Zzzt — Death Touch":                           {"Bazzt Zzzt shouts, 'Tankname!'"},
	"Eye of Veeshan — Death Touch":                       {"Eye of Veeshan shouts, 'Tankname!'"},
	"Keeper of Souls — Death Touch":                      {"Keeper of Souls shouts, 'Tankname!'"},
	"Overseer of Air — Death Touch":                      {"Overseer of Air shouts, 'Tankname!'"},
	"Sister of the Spire — Death Touch":                  {"Sister of the Spire shouts, 'Tankname!'"},
	"the Hand of Veeshan — Death Touch":                  {"the Hand of Veeshan shouts, 'Tankname!'"},
	"The Spiroc Lord — Death Touch":                      {"The Spiroc Lord shouts, 'Tankname!'"},
	"Terris — leashed":                                   {"Terris Thule disappears into the ether and reforms at the center of her chamber, cleansed of your magic!"},
}

// bertoxPrepare is codecay/encounters/Bertox.lua RP_TEXT.prepare.
const bertoxPrepare = "Crazed laughter is heard as you notice a foul creature standing before you. The creature then speaks saying, 'Violators of the depths of Lxanvom shall pay with your lives!'  The foul minion of decay then begins chanting a dark ritual."

// coirnavShout is powater/encounters/Coirnav.lua's minion-wave emote.
func coirnavShout(god, element string) string {
	return "Coirnav the Avatar of Water shouts, \"Those that violate my domain will pay. I call upon the power imbued to me by " +
		god + "! Come forth my minions of " + element + " and destroy these intruders.\""
}

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
			rows, err := conn.Query(`SELECT level, MR, CR, FR, npc_spells_id, COALESCE(special_abilities,'') FROM npc_types WHERE lower(name) IN (lower(?), lower(?), lower(?))`, name, "#"+name, name+"_")
			if err != nil {
				t.Fatal(err)
			}
			// A boss can have several rows with the same level and resists
			// (Vallon Zek's own fight and the copy Rallos's script spawns);
			// the table must match one of them exactly.
			var listID int
			var candidates []string
			found := false
			for rows.Next() {
				var lvl, mr, cr, fr, lid int
				var sa string
				if err := rows.Scan(&lvl, &mr, &cr, &fr, &lid, &sa); err != nil {
					t.Fatal(err)
				}
				if lvl != b.Level || mr != b.MR || cr != b.CR || fr != b.FR {
					continue
				}
				traits := raidTraitsOf(sa)
				candidates = append(candidates, traits)
				if traits == b.Traits && !found {
					found, listID = true, lid
				}
			}
			rows.Close()
			if !found {
				t.Errorf("%s: no npc_types row for %q with level %d, resists %d/%d/%d and traits %q (rows at that level/resists have traits %q)",
					d.Name, b.Name, b.Level, b.MR, b.CR, b.FR, b.Traits, candidates)
				continue
			}
			list := raidNPCSpells(t, conn, listID)
			for _, s := range b.Spells {
				where := fmt.Sprintf("%s / %s / %s (%d)", d.Name, b.Name, s.Name, s.ID)
				entry, ok := list[s.ID]
				if s.Scripted {
					if ok {
						t.Errorf("%s: Scripted, but it is on the boss's spell list", where)
					}
					entry = raidListEntry{proc: true} // no AI recast: expect Recast 0
				} else if !ok {
					t.Errorf("%s: not on the boss's spell list", where)
					continue
				}
				if !s.Scripted && entry.proc != s.Proc {
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

// raidTraitsOf renders a special_abilities string as Who card traits.
func raidTraitsOf(abilities string) string {
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
	return strings.Join(traits, ", ")
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

// TestRaidPacks_SharedLandLinesDropped: two spells in one pack logging the
// same land line can't be told apart, so neither timer uses that line.
func TestRaidPacks_SharedLandLinesDropped(t *testing.T) {
	p := raidPoPTime().build()
	const convulses = " falls to the ground and convulses."
	for _, tr := range p.Triggers {
		if len(tr.BossCasters) == 0 {
			continue
		}
		if strings.Contains(tr.Pattern, regexp.QuoteMeta(convulses)) {
			t.Errorf("%s matches the shared %q line", tr.Name, convulses)
		}
		if tr.Name == "Tallon Zek — Barb of Tallon" && strings.Contains(tr.Pattern, "You resist") {
			t.Errorf("Barb of Tallon matches the resist line all four Barbs share")
		}
	}
	re := regexp.MustCompile(raidTriggerByName(t, p, "Rallos Zek — Vindictive Strike").Pattern)
	if !re.MatchString("You muscles spasm and refuse to obey you.") || !re.MatchString("You resist the Vindictive Strike spell!") {
		t.Error("Vindictive Strike lost its unique on-you or resist line")
	}
}

func raidTriggerByName(t *testing.T, p TriggerPack, name string) Trigger {
	t.Helper()
	for _, tr := range p.Triggers {
		if tr.Name == name {
			return tr
		}
	}
	t.Fatalf("%s: no trigger %q", p.PackName, name)
	return Trigger{}
}

// TestRaidPacks_IdenticalMessagesMerge: spells with different IDs but the
// same land lines become one timer listing every caster, so each dragon's
// breath still times (binding picks the dragon); procs stay out of it.
func TestRaidPacks_IdenticalMessagesMerge(t *testing.T) {
	p := raidVelious().build()
	tr := raidTriggerByName(t, p, "Lava Breath / Molten Breath (Nanzata the Warder, Telkorenar, Sontalak)")
	if len(tr.BossCasters) != 3 {
		t.Errorf("casters = %+v, want Nanzata, Telkorenar, Sontalak", tr.BossCasters)
	}
	re := regexp.MustCompile(tr.Pattern)
	for _, line := range []string{"Your body combusts as the lava hits you.",
		"You resist the Lava Breath spell!", "You resist the Molten Breath spell!"} {
		if !re.MatchString(line) {
			t.Errorf("merged breath timer misses %q", line)
		}
	}
	for _, tr := range raidLuclin().build().Triggers {
		if strings.Contains(tr.Name, "Stunning Strike") && len(tr.BossCasters) > 0 {
			t.Errorf("proc Stunning Strike merged into a timer: %s", tr.Name)
		}
	}
}

// TestRaidPacks_Group: the raid packs, and only they, carry the raid group
// the Packs tab lists under its own header.
func TestRaidPacks_Group(t *testing.T) {
	for _, p := range AllPacks() {
		isRaid := strings.HasPrefix(p.PackName, "Raid: ")
		if (p.Group == PackGroupRaid) != isRaid {
			t.Errorf("%s: group %q", p.PackName, p.Group)
		}
	}
}

package trigger

import (
	"fmt"
	"regexp"
	"strings"
)

// RaidEventTimersPackName is the display/category name of the built-in
// scripted-event timer pack. It sits with the "Raid:" packs, which leave
// these event clocks to it rather than carrying their own copies.
const RaidEventTimersPackName = "Raid: Scripted Event Timers"

// legacyRaidEventTimersPackName is the pack's name through v0.34;
// MigrateRaidEventTimersRename moves installs over to the new one.
const legacyRaidEventTimersPackName = "Raid Event Timers"

// RaidEventTimersPack returns custom-timer triggers for scripted raid events
// that run on server timers rather than kill counts (kill-count events live
// in the Kill Tracker's presets instead). Every timing and line here comes
// from Project Quarm's own quest scripts (SecretsOTheP/quests), which differ
// from upstream EQMacEmu for several of these encounters — re-check the
// script before changing a value.
//
// Events deliberately absent, because nothing a player can see marks their
// timing: Vulak`Aerr's ring (Quarm advances waves when the previous wave is
// dead, with no announcement), the Plane of Disease rat boss (no emote at
// start), the Ry`Gorr assault in Eastern Wastes (combat-paused despawns with
// no common anchor), and the Plane of Air Dust island's hidden hour.
func RaidEventTimersPack() TriggerPack {
	var triggers []Trigger
	triggers = append(triggers, paffaTimers()...)
	triggers = append(triggers, coirnavReinforcementTimer())
	triggers = append(triggers, poTimeClockTimers()...)
	triggers = append(triggers, airBossDespawnTimers()...)
	triggers = append(triggers, emperorTimers()...)
	triggers = append(triggers, doomshadeTimer())
	triggers = append(triggers, ringOfFearTimers()...)
	triggers = append(triggers, ringWarTimers()...)
	triggers = append(triggers, hollowshadeTimer())
	triggers = append(triggers, grimlingWarTimers()...)
	return TriggerPack{
		PackName: RaidEventTimersPackName,
		Group:    PackGroupRaid,
		Description: "Countdown timers for scripted raid events: Paffa, Coirnav's reinforcements, " +
			"the Plane of Time instance clock, Plane of Air boss despawns, the Emperor and his room " +
			"curse, Doomshade, Ring of Fear, the Great Divide Ring War, Hollowshade's invasions and " +
			"the Grimling war. Timings are from Project Quarm's quest scripts. Works " +
			"alongside the era raid packs, which leave these event clocks to it.",
		Triggers: triggers,
	}
}

// eventTimer builds one custom-timer trigger for the pack. pattern is a
// regexp; stop (optional) clears the timer early.
func eventTimer(name, pattern string, secs float64, stop string) Trigger {
	return Trigger{
		Name:              name,
		Enabled:           true,
		Pattern:           pattern,
		PackName:          RaidEventTimersPackName,
		TimerType:         TimerTypeCustom,
		TimerDurationSecs: secs,
		WornOffPattern:    stop,
		BarColor:          "#f59e0b",
	}
}

// warnAt returns an on-screen fade-soon alert at secs remaining.
func warnAt(id string, secs float64) TimerAlert {
	return TimerAlert{
		ID:           id,
		Seconds:      secs,
		Type:         TimerAlertTypeOverlayText,
		Text:         fmt.Sprintf("{spell} in %s", humanSecs(secs)),
		DurationSecs: 6,
		Color:        "#f59e0b",
	}
}

func humanSecs(secs float64) string {
	s := int(secs)
	if s >= 60 && s%60 == 0 {
		if s == 60 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", s/60)
	}
	return fmt.Sprintf("%d seconds", s)
}

// lit quotes a literal log fragment for use in a pattern.
func lit(s string) string { return regexp.QuoteMeta(s) }

// anyOf joins literal fragments into one alternation.
func anyOf(parts ...string) string {
	q := make([]string, len(parts))
	for i, p := range parts {
		q[i] = lit(p)
	}
	return strings.Join(q, "|")
}

// slain matches the log's death lines for one NPC display name without
// matching combat lines that also carry the name.
func slain(name string) string {
	q := lit(name)
	return fmt.Sprintf(`(?i)^(?:%s (?:has been slain by .+!|died\.)|(?:You have|\S+ has) slain %s!)$`, q, q)
}

// paffaTimers models Banord Paffa in the Crypt of Decay
// (codecay/encounters/Paffa.lua): walking into Banord starts the event; a
// pusling wave spawns every 234s until the event ends, and the third wave
// brings Overlord Banord Paffa, who must die within 40 minutes. Banord's
// lines only reach players within 125 units.
func paffaTimers() []Trigger {
	const start = "Come fools see if you can match the power of Banord"
	end := anyOf("A mocking laugh begins as Banord settles back") + "|" + slain("Overlord Banord Paffa")

	waves := eventTimer("Paffa: next pusling wave",
		lit(start)+"|"+lit("Banord calls for more minions to attack"), 234, end)
	waves.TimerRepeat = true
	waves.TimerAlerts = []TimerAlert{warnAt("paffa-wave-15s", 15)}

	overlord := eventTimer("Paffa: Overlord Banord Paffa", lit(start), 3*234,
		lit("Banord prepares to attack")+"|"+end)
	overlord.TimerAlerts = []TimerAlert{warnAt("paffa-overlord-30s", 30)}

	fail := eventTimer("Paffa: event fails",
		lit("A loud and maddened scream of rage is heard as Banord prepares to attack"), 2400, end)
	fail.TimerAlerts = []TimerAlert{warnAt("paffa-fail-5m", 300)}

	return []Trigger{waves, overlord, fail}
}

// coirnavReinforcementTimer models Coirnav's final phase in the Plane of
// Water (powater/encounters/Coirnav.lua): once Pwelon, Nrinda and Vamuil
// are dead, a reinforcement wave spawns every 49s until Coirnav dies or the
// 15-minute event clock (tracked by the Kill Tracker's Coirnav preset)
// runs out.
func coirnavReinforcementTimer() Trigger {
	t := eventTimer("Coirnav: reinforcements",
		lit("Water surges, ice fractures and vapor coils through the reef as Coirnav summons"), 49,
		anyOf("Coirnav, the Avatar of Water, has fallen!", "These waters reject you. Be banished from our domain!"))
	t.TimerRepeat = true
	t.TimerRepeatMax = 30
	t.TimerAlerts = []TimerAlert{warnAt("coirnav-ling-10s", 10)}
	return t
}

// poTimeClockTimers model the Plane of Time instance clock
// (potimeb/223077.lua): the controller announces the time left exactly on
// each hour boundary ("…You have three hours left.") and at thirty minutes,
// so each announcement pins the clock. Phase completions add hours with a
// different line; the next hourly announcement picks that up. When time
// runs out the instance gives a two-minute warning.
func poTimeClockTimers() []Trigger {
	words := []string{"one hour", "two hours", "three hours", "four hours", "five hours", "six hours",
		"seven hours", "eight hours", "nine hours", "ten hours", "eleven hours"}
	clock := eventTimer("Plane of Time: instance time left", lit("You have thirty minutes left."), 1800, "")
	for i, w := range words {
		clock.ExtraPatterns = append(clock.ExtraPatterns, ExtraPattern{
			Pattern:           lit("You have " + w + " left."),
			Enabled:           true,
			TimerDurationSecs: float64((i + 1) * 3600),
		})
	}
	clock.Pinned = true
	clock.TimerAlerts = []TimerAlert{warnAt("potime-clock-15m", 900)}

	shutdown := eventTimer("Plane of Time: instance shutting down",
		lit("Instance is shutting down in two minutes"), 120, "")
	shutdown.Pinned = true
	return []Trigger{clock, shutdown}
}

// airBossDespawnTimers model the four Plane of Air island bosses
// (poair/encounters/{Dust,Mist,Smoke,Wind}.lua): each shouts on spawn and
// despawns after 2.5 hours out of combat. The server pauses that clock
// while the boss is engaged, so the timer is the soonest it can despawn.
func airBossDespawnTimers() []Trigger {
	boss := func(name, shout, killName string) Trigger {
		t := eventTimer(name+": despawns (out of combat)", lit(shout), 9000, slain(killName))
		t.TimerAlerts = []TimerAlert{warnAt("poair-despawn-10m", 600)}
		return t
	}
	return []Trigger{
		boss("Avatar of Dust", "Elementals of Air and Dust, I call upon you to aid me in the defense of this land!", "Avatar of Dust"),
		boss("Melernil Faal`Armanna", "Leave now foolish mortal you have begun to send events in motion", "Melernil Faal`Armanna"),
		boss("An Elemental Masterpiece", "The time has come for all intruders to be tested", "An Elemental Masterpiece"),
		boss("Pherlondien Clawpike", "I will certainly make sure that no mortal is allowed to defile this sacred realm!", "Pherlondien Clawpike"),
	}
}

// emperorTimers model Emperor Ssraeshza (ssratemple/encounters/Emperor.lua):
// killing the Blood of Ssraeshza makes the Emperor twitch, and he becomes
// attackable 120s later. Separately, engaging the room's curse trap shows a
// blood splatter at 240s, a warning at 1113s, and Curse of Ssraeshza lands
// 312s after the warning — the two room lines pin one timer.
func emperorTimers() []Trigger {
	attackable := eventTimer("Emperor Ssraeshza: attackable",
		lit("The Emperor twitches.  He appears to have been weakened by the destruction of the golem."), 120, "")
	attackable.TimerAlerts = []TimerAlert{warnAt("emp-attackable-15s", 15)}

	curse := eventTimer("Emperor room: Curse of Ssraeshza",
		lit("You notice a splatter of oddly colored blood on the floor"), 1113-240+312, slain("Emperor Ssraeshza"))
	curse.ExtraPatterns = []ExtraPattern{{
		Pattern:           lit("Something tells you that this room isn't safe anymore"),
		Enabled:           true,
		TimerDurationSecs: 312,
	}}
	curse.TimerAlerts = []TimerAlert{warnAt("emp-curse-60s", 60)}
	return []Trigger{attackable, curse}
}

// doomshadeTimer models Doomshade in the Umbral Plains
// (umbral/encounters/doomshade.lua): after the four Dark Masters die he is
// released with a zone-wide scream and despawns after an hour out of combat
// (paused while engaged, so this is the soonest he can despawn).
func doomshadeTimer() Trigger {
	t := eventTimer("Doomshade: despawns (out of combat)",
		lit("a tortured scream echoes throughout the plains as a tormented spirit is released upon the land"),
		3600, slain("Doomshade"))
	t.TimerAlerts = []TimerAlert{warnAt("doomshade-despawn-10m", 600)}
	return t
}

// ringOfFearTimers model the Ring of Fear in Cazic-Thule
// (cazicthule/encounters/RingOfFear.lua): the Avatar of Fear walks a
// five-point loop pausing 52s at each, spawning a wave each lap (260s) —
// the first about 265s after his opening shout. The Dread, Fright and Terror
// waves pause the loop while the boss walks out, so each "ARISE!" shout
// re-pins the next-wave timer.
func ringOfFearTimers() []Trigger {
	first := eventTimer("Ring of Fear: first wave",
		lit("Minions of fear, hear me now! The temple of the great Cazic-Thule has been defiled."), 265,
		lit("the minions of fear, ARISE!"))
	next := eventTimer("Ring of Fear: next wave", lit("the minions of fear, ARISE!"), 260, "")
	next.TimerAlerts = []TimerAlert{warnAt("rof-wave-20s", 20)}
	boss := Trigger{
		Name:     "Ring of Fear: avatar summoned",
		Enabled:  true,
		Pattern:  `I call forth from the abyss\.\.\. (DREAD|FRIGHT|TERROR)!`,
		PackName: RaidEventTimersPackName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "{1} INCOMING", DurationSecs: 8, Color: "#ef4444"},
		},
	}
	return []Trigger{first, next, boss}
}

// ringWarTimers model the Great Divide Ring War
// (greatdivide/encounters/Ring10.lua): the Seneschal's first rally shout
// starts the schedule; wave 1 arrives 125s later and the rest follow a fixed
// but uneven cadence until Narandi the Wretched at 5475s. Waves aren't
// announced, so each wave gets its own timer, hidden until its last three
// minutes. Saying "the Kromrif are attacking" to Zrelik pulls the next wave
// in immediately and shifts everything after it, so that shout clears the
// schedule rather than leave it wrong.
func ringWarTimers() []Trigger {
	start := lit("Good citizens of Thurgadin, hear me!")
	charge := lit("has spotted tha' next wave! Charge!")
	// Seconds after the start shout, from WarTimer's StartWave cadence.
	offsets := []float64{125, 315, 505, 695, 885, 1075, 1265, 1275, 1795, 2080,
		2365, 2650, 2935, 3190, 3875, 4195, 4515, 4835, 5155}
	var out []Trigger
	for i, at := range offsets {
		t := eventTimer(fmt.Sprintf("Ring War: wave %02d", i+1), start, at, charge)
		t.DisplayThresholdSecs = 180
		out = append(out, t)
	}
	narandi := eventTimer("Ring War: Narandi the Wretched", start, 5475, charge+"|"+slain("Narandi the Wretched"))
	narandi.Pinned = true
	narandi.TimerAlerts = []TimerAlert{warnAt("ringwar-narandi-60s", 60)}
	reset := Trigger{
		Name:     "Ring War: schedule cleared by early charge",
		Enabled:  true,
		Pattern:  charge,
		PackName: RaidEventTimersPackName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "Early charge: Ring War wave timers cleared", DurationSecs: 6, Color: "#f59e0b"},
		},
	}
	return append(out, narandi, reset)
}

// hollowshadeTimer models the Hollowshade Moor war
// (hollowshade/encounters/war.lua): every invasion is announced with an
// attacker emote, and the battle starts 720s later; the controller re-rolls
// a new invasion (with a new emote) every 720s.
func hollowshadeTimer() Trigger {
	t := eventTimer("Hollowshade: invasion begins",
		anyOf("the Owlbear sharpen their claws on boulders",
			"the Grimlings make preparations to invade their enemy",
			"The sonic wolves are mounting an offensive"), 720, "")
	t.TimerAlerts = []TimerAlert{warnAt("hollowshade-1m", 60)}
	return t
}

// grimlingWarTimers model the Grimling war in Grimling Forest
// (grimling/encounters/GrimlingWar.lua): turning in Staginar's Battle Orders
// starts the war 20s later with a three-hour limit for the march; after
// Cullin claims the land, the assault begins 140s later. His lines only
// reach players nearby.
func grimlingWarTimers() []Trigger {
	end := anyOf("The invasion has failed", "We have failed!", "Our hopes are lost!",
		"The grimling high commander has been vanquished")
	limit := eventTimer("Grimling war: time limit",
		lit("Finally! An army with the courage to bring these scum to their knees!"), 3*3600+20, end)
	limit.TimerAlerts = []TimerAlert{warnAt("grimwar-limit-15m", 900)}
	cleansing := eventTimer("Grimling war: the cleansing begins",
		lit("I claim this land in the name of the king!"), 140, end)
	cleansing.TimerAlerts = []TimerAlert{warnAt("grimwar-cleanse-15s", 15)}
	return []Trigger{limit, cleansing}
}

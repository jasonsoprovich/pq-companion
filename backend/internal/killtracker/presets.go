package killtracker

import (
	"fmt"
	"regexp"
)

// Preset is a built-in, ready-made set of counters for a known event.
//
// Every preset is taken from Project Quarm's own quest scripts
// (SecretsOTheP/quests), not upstream EQMacEmu/quests — Quarm has rewritten
// several of these encounters (Bertox, Coirnav, Fennin) with different
// announcement text, and removed the guard-count gates from Neffiken, Gurebk
// and Jeplak entirely. Re-check the script before changing a preset.
type Preset struct {
	Key         string       `json:"key"`
	Name        string       `json:"name"`
	Zone        string       `json:"zone"`
	Description string       `json:"description"`
	Counters    []CounterDef `json:"counters"`
}

// Presets returns every built-in preset, in display order.
func Presets() []Preset {
	return []Preset{
		bertoxPreset(),
		coirnavPreset(),
		fenninPreset(),
		stoneRingPreset(),
		stonruakPreset(),
		drornokPreset(),
		poTimePhase3Preset(),
	}
}

func presetByKey(key string) (Preset, bool) {
	for _, p := range Presets() {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// stalePresetPatterns lists line patterns that older builds installed from a
// preset before the server changed its text. Installed counters keep their
// own copy of each pattern, so upgradePresetDef swaps in the current preset's
// pattern wherever the stored one is still one of these. Anything else is a
// user edit and is left alone.
var stalePresetPatterns = map[string]map[string]bool{
	// Quests 2026-10-25 Stone Ring rework replaced every ring emote.
	"stonering": {
		"": true,
		"The ground shudders as stone fortifications are summoned forth": true,
		`The Rock Monstrosity crumbles into mounds of rubble\.`:          true,
		"A gigantic formation of rocks appears":                          true,
		"As the last of the stone heaps collapses":                       true,
		`(?i)(?:Peregrin Rockskull|an Encrusted Dirt Cloud) (?:has been slain by .+!|died\.)|slain (?:Peregrin Rockskull|an Encrusted Dirt Cloud)!`: true,
	},
}

// upgradePresetDef refreshes stale preset patterns in an installed counter.
// It matches the counter to its preset counter by name and reports whether
// anything changed.
func upgradePresetDef(def CounterDef) (CounterDef, bool) {
	stale := stalePresetPatterns[def.Preset]
	if stale == nil {
		return def, false
	}
	p, _ := presetByKey(def.Preset)
	var cur *CounterDef
	for i := range p.Counters {
		if p.Counters[i].Name == def.Name {
			cur = &p.Counters[i]
		}
	}
	if cur == nil {
		return def, false
	}
	changed := false
	swap := func(have *string, want string) {
		if *have != want && stale[*have] {
			*have = want
			changed = true
		}
	}
	swap(&def.Pattern, cur.Pattern)
	swap(&def.ResetPattern, cur.ResetPattern)
	if len(def.Syncs) == len(cur.Syncs) {
		def.Syncs = append([]SyncRule(nil), def.Syncs...)
		for i := range def.Syncs {
			swap(&def.Syncs[i].Pattern, cur.Syncs[i].Pattern)
		}
	}
	if def.Clock != nil && cur.Clock != nil {
		clock := *def.Clock
		if len(clock.Marks) == len(cur.Clock.Marks) {
			clock.Marks = append([]ClockMark(nil), clock.Marks...)
			for i := range clock.Marks {
				swap(&clock.Marks[i].Pattern, cur.Clock.Marks[i].Pattern)
			}
		}
		if len(clock.StopPatterns) == len(cur.Clock.StopPatterns) {
			clock.StopPatterns = append([]string(nil), clock.StopPatterns...)
			for i := range clock.StopPatterns {
				swap(&clock.StopPatterns[i], cur.Clock.StopPatterns[i])
			}
		}
		def.Clock = &clock
	}
	return def, changed
}

// slainPattern matches any of the log's death forms for one NPC display name
// ("X has been slain by Y!", "You have slain X!", "X died.") without matching
// the combat lines that also carry the name.
func slainPattern(name string) string {
	q := regexp.QuoteMeta(name)
	return fmt.Sprintf(`(?i)^(?:%s (?:has been slain by .+!|died\.|has died\.)|(?:You have|\S+ has) slain %s!)$`, q, q)
}

// bertoxPreset models the Bertoxxulous event in the Crypt of Decay
// (codecay/encounters/Bertox.lua):
//
//   - Killing #Spectre_of_Corruption starts the event: trash spawns enable
//     350s later, the expiry warning lands 300s before the 7380s deadline.
//   - Each event Knight/Priest/Necromancer of Decay (and Elite) death bumps
//     the trash count; an Adan king spawns at 44/48/52/56 and 84/88/92/96
//     (nothing between 57 and 83), each announced zone-wide by name —
//     "We call to you, corrupted King of Lxanvom, <Name>! ..." (upstream
//     EQMacEmu ends the name with "," instead of "!"; both match).
//   - Every king death is announced zone-wide ("The corrupted king, <Name>,
//     falls."), so kings are counted from that line rather than kill lines,
//     which only reach players nearby. All eight first-tier kings dead
//     spawns the last four; all twelve spawns Bertoxxulous.
func bertoxPreset() Preset {
	trashBosses := []struct {
		at   int
		name string
	}{
		{44, "Darwol Adan"}, {48, "Feig Adan"}, {52, "Xhut Adan"}, {56, "Kavilis Adan"},
		{84, "Raddi Adan"}, {88, "Wavadozzik Adan"}, {92, "Zandal Adan"}, {96, "Akkapan Adan"},
	}
	var milestones []Milestone
	var syncs []SyncRule
	for _, b := range trashBosses {
		milestones = append(milestones, Milestone{At: b.at, Label: b.name + " spawns"})
		syncs = append(syncs, SyncRule{
			Pattern: fmt.Sprintf(`King of Lxanvom, %s[,!]`, regexp.QuoteMeta(b.name)),
			SetTo:   b.at,
		})
	}

	const (
		group = "Bertoxxulous"
		zone  = "The Crypt of Decay"
		// Spectre of Corruption's death emote — the event's real start.
		spectreDeath = `Crazed laughter is heard as you notice a foul creature standing before you`
		// Trash spawns enable 350s after the Spectre dies.
		trashStart = `Great soldiers of decay you are summoned forth`
		// The expiry warning, 300s before the 7380s deadline.
		expireWarning = `Your time is close in coming to an end`
		expired       = `Perhaps you would care to try when you are more powerful fools`
		bertoxDead    = `Bertoxxulous has been defeated`
	)
	reset := spectreDeath + "|" + trashStart

	return Preset{
		Key:  "bertox",
		Name: "Bertoxxulous",
		Zone: "Crypt of Decay",
		Description: "Event trash toward the eight trash-spawned Adan kings, Adan kills " +
			"toward the final four and Bertoxxulous, and the 123-minute event clock. " +
			"Resyncs from the zone-wide boss emotes.",
		Counters: []CounterDef{
			{
				Name:    "Event trash",
				Group:   group,
				Enabled: true,
				Mobs: []string{
					"Knight of Decay", "Priest of Decay", "Necromancer of Decay",
					"Elite Knight of Decay", "Elite Priest of Decay", "Elite Necromancer of Decay",
				},
				Zone:         zone,
				Goal:         96,
				Milestones:   milestones,
				WarnBefore:   2,
				Syncs:        syncs,
				ResetPattern: reset,
				Clock: &Clock{
					Label:        "Event time left",
					DurationSecs: 7380,
					Marks: []ClockMark{
						{Pattern: spectreDeath, AtSecs: 0},
						{Pattern: trashStart, AtSecs: 350},
						{Pattern: expireWarning, AtSecs: 7080},
					},
					StopPatterns: []string{expired, bertoxDead},
				},
				AlertOverlay: true,
			},
			{
				Name:    "Adan kings",
				Group:   group,
				Enabled: true,
				Pattern: `^The corrupted king, .+, falls\.`,
				Zone:    zone,
				Goal:    12,
				Milestones: []Milestone{
					{At: 8, Label: "Final four Adan kings spawn"},
					{At: 12, Label: "Bertoxxulous spawns"},
				},
				Syncs: []SyncRule{
					{Pattern: regexp.QuoteMeta("We call to you the last corrupted Kings of Lxanvom"), SetTo: 8},
					{Pattern: regexp.QuoteMeta("A sinister vision enters your mind of a faceless one"), SetTo: 12},
				},
				ResetPattern: reset,
				AlertOverlay: true,
			},
		},
	}
}

// coirnavPreset models Coirnav in the Plane of Water
// (powater/encounters/Coirnav.lua): killing the Guardian starts a hard
// 15-minute deadline for the whole event. Three timed waves (0s, 180s, 300s)
// of 25 vapor/ice/water fiends each — all 75 must die before Pwelon, Nrinda
// and Vamuil are summoned. Time warnings land at 10, 12 and 14 minutes.
func coirnavPreset() Preset {
	return Preset{
		Key:  "coirnav",
		Name: "Coirnav",
		Zone: "Plane of Water",
		Description: "The 75 fiends that unlock Pwelon, Nrinda and Vamuil, and the hard " +
			"15-minute event clock (resynced from each wave shout and time warning).",
		Counters: []CounterDef{
			{
				Name:    "Fiends",
				Group:   "Coirnav",
				Enabled: true,
				Mobs:    []string{"a triloun vaporfiend", "a hraquis icefiend", "a regrua waterfiend"},
				Zone:    "Plane of Water",
				Goal:    75,
				Milestones: []Milestone{
					{At: 75, Label: "Pwelon, Nrinda and Vamuil spawn"},
				},
				WarnBefore: 5,
				Syncs: []SyncRule{
					{Pattern: regexp.QuoteMeta("Pwelon, Nrinda and Vamuil gather before Coirnav"), SetTo: 75},
				},
				ResetPattern: regexp.QuoteMeta("imbued to me by Povar!"),
				Clock: &Clock{
					Label:        "Event time left",
					DurationSecs: 900,
					Marks: []ClockMark{
						{Pattern: regexp.QuoteMeta("imbued to me by Povar!"), AtSecs: 0},
						{Pattern: regexp.QuoteMeta("imbued to me by E`ci!"), AtSecs: 180},
						{Pattern: regexp.QuoteMeta("imbued to me by Tarew Marr!"), AtSecs: 300},
						{Pattern: regexp.QuoteMeta("The powers of Povar, E`ci and Tarew Marr are bound within me"), AtSecs: 600},
						{Pattern: regexp.QuoteMeta("Our power gathers, and your time dwindles"), AtSecs: 720},
						{Pattern: regexp.QuoteMeta("Your time is nearly spent! Finish what you began"), AtSecs: 840},
					},
					StopPatterns: []string{
						regexp.QuoteMeta("These waters reject you. Be banished from our domain!"),
						regexp.QuoteMeta("Coirnav, the Avatar of Water, has fallen!"),
					},
				},
				AlertOverlay: true,
			},
		},
	}
}

// fenninPreset models Fennin Ro in the Plane of Fire
// (pofire/encounters/Fennin.lua): the Guardian's death starts a 17500s
// running clock with a warning every 45 minutes. Each stage spawns when the
// previous one is fully dead: 41 army trash, then 20 (four commanders plus
// trash), then the four-member Council of Fire, then Fennin. Each stage
// change is announced zone-wide.
func fenninPreset() Preset {
	const prefix = `All of Doomfire trembles as the voice of Fennin Ro booms, "`
	warnings := []string{
		"Norrathians! Your hunger for power has carried you into my realm.",
		"You presume to command powers beyond your understanding.",
		"The balance of power will not bend to your greed!",
		"Hear the roar of Doomfire!",
		"My restraint is nearly spent!",
		"Your time dwindles, Norrathians!",
	}
	start := regexp.QuoteMeta("The Guardian of Doomfire crashes to the scorched earth")
	marks := []ClockMark{{Pattern: start, AtSecs: 0}}
	for i, w := range warnings {
		marks = append(marks, ClockMark{Pattern: regexp.QuoteMeta(prefix + w), AtSecs: (i + 1) * 2700})
	}
	return Preset{
		Key:  "fennin",
		Name: "Fennin Ro",
		Zone: "Plane of Fire",
		Description: "The Doomfire army stages (41 → 61 → Council of Fire → Fennin) and the " +
			"~4h52m event clock, resynced from the stage emotes and 45-minute warnings.",
		Counters: []CounterDef{
			{
				Name:    "Doomfire army",
				Group:   "Fennin Ro",
				Enabled: true,
				Mobs: []string{
					"a rage reaver of flame", "a chaos healer of flame", "a dark magus of flame",
					"a doomfire darkfiend", "a doomfire chaosfiend", "a doomfire ragefiend",
					"Azobian the Darklord", "Javonn the Overlord", "Hebabbilys the Ragelord", "Reaxnous the Chaoslord",
					"Chancellor Kirtra", "Chancellor Traxom", "Omni Magus Crato", "Warlord Prollaz",
				},
				Zone: "Plane of Fire",
				Goal: 65,
				Milestones: []Milestone{
					{At: 41, Label: "Commanders spawn (Azobian, Javonn, Hebabbilys, Reaxnous)"},
					{At: 61, Label: "Council of Fire spawns"},
					{At: 65, Label: "Fennin Ro spawns"},
				},
				WarnBefore: 3,
				Syncs: []SyncRule{
					{Pattern: regexp.QuoteMeta("The first ranks fall, but the roar of battle grows louder"), SetTo: 41},
					{Pattern: regexp.QuoteMeta("The commanders collapse beneath the onslaught"), SetTo: 61},
					{Pattern: regexp.QuoteMeta("The Council of Fire falls."), SetTo: 65},
				},
				ResetPattern: start,
				Clock: &Clock{
					Label:        "Event time left",
					DurationSecs: 17500,
					Marks:        marks,
					StopPatterns: []string{
						regexp.QuoteMeta("Fennin Ro, the Tyrant of Fire, falls!"),
						regexp.QuoteMeta("Carry the memory of this failure back to your world"),
					},
				},
				AlertOverlay: true,
			},
		},
	}
}

// stoneRingPreset models the Stone Ring in the Plane of Earth
// (poeartha/encounters/StoneRing.lua): each of the four trigger mobs
// summons a batch of fortifications (43 total); the 43rd death crumbles the
// Rock Monstrosity. Killing it starts 24 stone heaps in waves of four, then
// Peregrin Rockskull (or an Encrusted Dirt Cloud) rises. The ring fails 98
// minutes after it starts. The quests 2026-10-25 rework moved the ring's
// messages to zone-wide emotes and added a failure emote; each pattern also
// accepts the older temple-only (200 unit) text.
func stoneRingPreset() Preset {
	const (
		group  = "Stone Ring"
		zone   = "Plane of Earth"
		forts  = "Massive slabs of stone wrench themselves from the earth|The ground shudders as stone fortifications are summoned forth"
		crumbs = "The last fortification shatters with a thunderous crack|The Rock Monstrosity crumbles into mounds of rubble"
		heaps  = "The Rock Monstrosity crashes apart, its weight shaking the temple|A gigantic formation of rocks appears"
		boss   = "The last stone heap shatters\\. The temple's remaining strength|As the last of the stone heaps collapses"
		failed = "The Stone Ring has reset; you may attempt it again"
		finale = `(?i)(?:Peregrin Rockskull|an Encrusted Dirt Cloud) (?:has been slain by .+!|died\.)|slain (?:Peregrin Rockskull|an Encrusted Dirt Cloud)!|the trial of the Stone Ring is complete`
	)
	heapMilestones := []Milestone{}
	for at := 4; at < 24; at += 4 {
		heapMilestones = append(heapMilestones, Milestone{At: at, Label: "Next heap wave"})
	}
	heapMilestones = append(heapMilestones, Milestone{At: 24, Label: "Peregrin Rockskull rises"})
	return Preset{
		Key:  "stonering",
		Name: "Stone Ring",
		Zone: "Plane of Earth",
		Description: "Fortifications toward the Rock Monstrosity, stone heaps toward the ring " +
			"boss, and the 98-minute ring clock. A failed ring resets both counters.",
		Counters: []CounterDef{
			{
				Name:    "Stone fortifications",
				Group:   group,
				Enabled: true,
				Mobs:    []string{"A Stone Fortification"},
				Zone:    zone,
				Goal:    43,
				Milestones: []Milestone{
					{At: 43, Label: "Rock Monstrosity crumbles"},
				},
				WarnBefore: 3,
				Syncs: []SyncRule{
					{Pattern: crumbs, SetTo: 43},
				},
				ResetPattern: failed,
				Clock: &Clock{
					Label:        "Ring time left",
					DurationSecs: 5880,
					// Printed once per trigger mob; only the first starts the attempt.
					Marks:        []ClockMark{{Pattern: forts, AtSecs: 0, IfIdle: true, Reset: true}},
					StopPatterns: []string{finale},
				},
				AlertOverlay: true,
			},
			{
				Name:       "Stone heaps",
				Group:      group,
				Enabled:    true,
				Mobs:       []string{"A Stone Heap"},
				Zone:       zone,
				Goal:       24,
				Milestones: heapMilestones,
				Syncs: []SyncRule{
					{Pattern: boss, SetTo: 24},
				},
				// The first heap wave's emote — heaps only ever start from 0.
				ResetPattern: heaps + "|" + failed,
				AlertOverlay: true,
			},
		},
	}
}

// stonruakPreset models Ston`Ruak in the Plane of Storms
// (postorms/encounters/StonRuak.lua): after 38 Koka`Vor senvar/krovian
// deaths, once none are left up, eight Elders/Chieftains spawn; when they
// are all dead, Ston`Ruak spawns. The server count resets when he spawns.
func stonruakPreset() Preset {
	const group = "Ston`Ruak"
	reset := slainPattern("Ston`Ruak, Ancient of the Trees")
	return Preset{
		Key:  "stonruak",
		Name: "Ston`Ruak",
		Zone: "Plane of Storms",
		Description: "The 38 Koka`Vor guards (then clear the rest) that bring the eight " +
			"Elders and Chieftains, and those eight toward Ston`Ruak.",
		Counters: []CounterDef{
			{
				Name:    "Koka`Vor guards",
				Group:   group,
				Enabled: true,
				Mobs:    []string{"a Koka`Vor senvar", "a Koka`Vor krovian"},
				Zone:    "Plane of Storms",
				Goal:    38,
				Milestones: []Milestone{
					{At: 38, Label: "Clear the remaining guards to bring the Elders"},
				},
				WarnBefore:   3,
				ResetPattern: reset,
				AlertOverlay: true,
			},
			{
				Name:    "Elders and Chieftains",
				Group:   group,
				Enabled: true,
				Mobs:    []string{"a Koka`Vor Elder", "a Koka`Vor Chieftain"},
				Zone:    "Plane of Storms",
				Goal:    8,
				Milestones: []Milestone{
					{At: 8, Label: "Ston`Ruak spawns"},
				},
				ResetPattern: reset,
				AlertOverlay: true,
			},
		},
	}
}

// drornokPreset models Drornok Tok Vo`Lok in the Plane of Storms
// (postorms/encounters/Drornok.lua): more than 31 deaths of the cave's
// static amphans/toads/loktoles, with none of them left up at once, spawn
// Drornok. The server count resets when he spawns.
func drornokPreset() Preset {
	return Preset{
		Key:         "drornok",
		Name:        "Drornok Tok Vo`Lok",
		Zone:        "Plane of Storms",
		Description: "The 32 cave kills that, once the cave is fully clear, spawn Drornok.",
		Counters: []CounterDef{
			{
				Name:    "Cave guards",
				Group:   "Drornok",
				Enabled: true,
				Mobs: []string{
					"a typhoon amphan", "a tempest toad", "a hurricane loktole", "a vacillating loktole",
				},
				Zone: "Plane of Storms",
				Goal: 32,
				Milestones: []Milestone{
					{At: 32, Label: "Clear the cave to spawn Drornok"},
				},
				WarnBefore:   3,
				ResetPattern: slainPattern("Drornok Tok Vo`Lok"),
				AlertOverlay: true,
			},
		},
	}
}

// poTimePhase3Preset models Plane of Time phase 3
// (potimeb/encounters/Phase3.lua): eight waves of ten (two named plus
// eight), each spawned when the previous wave is fully dead; after 80 kills
// the Avatar of the Elements and Supernatural Guardian spawn. A wave's named
// spawn as "Shadow of ..." when that god's phase-1 trial was failed. Phase 3
// starts after the dialog that opens with "A bright aura streaks through
// the sky" (EMOTE_STRINGS[19] in potimeb/223077.lua).
func poTimePhase3Preset() Preset {
	waves := []string{
		"Ferocious Warboar and Blackheart",
		"Xeroan and Kraksmaal",
		"Deadly Warboar and Skullsmash",
		"Herlsoakian and Sinrunal",
		"Rianit and Needletusk Warboar",
		"Xerskel and Dersool",
		"Undead Squad Leader and Dark Knight of Terris",
		"Champion of Torment and Dreamwarp",
	}
	var milestones []Milestone
	for i := 1; i < len(waves); i++ {
		milestones = append(milestones, Milestone{At: i * 10, Label: fmt.Sprintf("Wave %d: %s", i+1, waves[i])})
	}
	milestones = append(milestones, Milestone{At: 80, Label: "Avatar of the Elements and Supernatural Guardian"})
	return Preset{
		Key:  "potime3",
		Name: "Plane of Time phase 3",
		Zone: "Plane of Time",
		Description: "Eight waves of ten toward the Avatar of the Elements and Supernatural " +
			"Guardian, with the named of each wave called out.",
		Counters: []CounterDef{
			{
				Name:    "Phase 3 waves",
				Group:   "Plane of Time",
				Enabled: true,
				Mobs: []string{
					// Wave named (normal and Shadow forms).
					"A Ferocious Warboar", "Deathbringer Blackheart", "Xeroan Xi`Geruonask", "Kraksmaal Fir`Dethsin",
					"A Deadly Warboar", "Deathbringer Skullsmash", "Herlsoakian", "Sinrunal Gorgedreal",
					"Deathbringer Rianit", "A Needletusk Warboar", "Xerskel Gerodnsal", "Dersool Fal`Giersnaol",
					"Undead Squad Leader", "Dark Knight of Terris", "Champion of Torment", "Dreamwarp",
					"Shadow of the Ferocious Warboar", "Shadow of Blackheart", "Shadow of Xeroan", "Shadow of Kraksmaal",
					"Shadow of the Deadly Warboar", "Shadow of Skullsmash", "Shadow of Herlsoakian", "Shadow of Sinrunal",
					"Shadow of Rianit", "Shadow of a Needletusk Warboar", "Shadow of Xerskel", "Shadow of Dersool",
					"Shadow of the Squad Leader", "Shadow of the Dark Knight", "Shadow of the Champion", "Shadow of Dreamwarp",
					// Wave trash.
					"A Dark Guardian of the Warlord", "A Pig of War", "Champion of War", "a deceiver of zek",
					"Archon of the Plaguebringer", "Footman of the Plaguebringer", "Incantator of the Plaguebringer",
					"Revenant of the Plaguebringer", "A Warlord of Zek", "A Zek Bloodtusk",
					"A Ferocious Cube Formation", "A Ferocious Cube Phantasmist", "An Arcane Cube Formation",
					"Battlemaster of the Faceless", "Vanquisher of the Faceless",
					"A Minion of the Warlord", "A warboar of suffering",
					"Assassin of Hate", "Dark Priest of Innoruuk", "Nightwalker of Hate",
					"A Hobgoblin Servant", "Knight of Nightmare", "Undead Pawn of Terris",
					"Cutthroat of Saryrn", "Dark Knight of Saryrn", "Doombringer of Saryrn",
					"Templar of Saryrn", "Thaumathurgist of Saryrn", "Warbeast of Saryrn",
				},
				Zone:         "Plane of Time",
				Goal:         80,
				Milestones:   milestones,
				WarnBefore:   2,
				ResetPattern: regexp.QuoteMeta("A bright aura streaks through the sky."),
				AlertOverlay: true,
			},
		},
	}
}

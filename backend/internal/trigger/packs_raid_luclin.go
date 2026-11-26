package trigger

// raidLuclin covers the Luclin raid targets: Akheva Ruins, Grieg's End,
// Katta Castellum, Sanctus Seru, Ssraeshza Temple, The Deep and Vex Thal.
// Event lines are from Project Quarm's akheva/encounters/SheiVinitras.lua,
// ssratemple/encounters/Emperor.lua and npc_emotes (Vyzh`dra's aggro shout).
// The five spells that used to ship in Raid Alerts are shared definitions
// (raidSignatureShared), so existing copies move here instead of doubling.
func raidLuclin() raidPackDef {
	return raidPackDef{
		Name: "Raid: Luclin",
		Description: "Luclin raid targets from Akheva Ruins to Vex Thal. Boss " +
			"spell recast timers that bind to the boss casting them (Aten Ha " +
			"Ra's Fling and Silence, Seru's Torturing Winds, Touch of " +
			"Vinitras...), cure alerts for curable debuffs on you, Shei's add " +
			"waves, Vyzh`dra's first Touch of Vinitras and the Emperor's spawn " +
			"and Rage of Ssraeshza. /tell <boss> x (e.g. /tell aten_ha_ra x) " +
			"shows the boss's level, resists, abilities, spells and what to " +
			"nuke with. " + raidCredit,
		Shared: raidSignatureShared("Fling", "Silence of the Shadows",
			"Torturing Winds", "Caustic Mist / Putrefy Flesh",
			"Touch of Vinitras"),
		Bosses: []raidBoss{
			{
				Name: "Shei Vinitras", Zone: "Akheva Ruins", Level: 63, MR: 180, CR: 165, FR: 60,
				Traits: "Rampage",
				Note:   "NUKES: Fire Tears + SS/Garrisons",
				// The real Shei (npc 179017), after the decoy dies.
				Spells: []raidSpell{
					{ID: 2859, Name: "Touch of Vinitras", OnYou: "Your soul fades into darkness.", OnOther: "'s soul fades into darkness.", Recast: 120},
					{ID: 2990, Name: "Chromatic Disruption", OnYou: "Your mystical protections weaken.", OnOther: "'s mystical protections weaken.", Proc: true},
				},
			},
			{
				Name: "The Insanity Crawler", Zone: "Akheva Ruins", Level: 63, MR: 175, CR: 350, FR: 165,
				Traits: "Rampage",
				Note:   "NUKES: Lure of Flame. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2817, Name: "Thought Vortex", OnYou: "You feel a consciousness invade your mind.", OnOther: "'s eyes roll back in their head.", Recast: 8},
				},
			},
			{
				Name: "The Itraer Vius", Zone: "Akheva Ruins", Level: 63, MR: 60, CR: 100, FR: 150,
				Traits: "Rampage, Unslowable",
				Note:   "NUKES: Magic + Cold Tears. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2905, Name: "Touch of the Mistress", OnYou: "Your soul is consumed by shadows of torment.", OnOther: "'s soul is consumed by shadows of torment.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "The Va`Dyn", Zone: "Akheva Ruins", Level: 63, MR: 75, CR: 150, FR: 50,
				Traits: "Flurry",
				Note:   "NUKES: Magic + Fire Tears. DEBUFFED = All 3 Tears",
				Spells: []raidSpell{
					{ID: 2816, Name: "Storm Tremor", OnYou: "You crack your skull on a block of stone.", OnOther: " cracks their skull on a black of stone.", Proc: true},
				},
			},
			{
				Name: "Grieg Veneficus", Zone: "Grieg's End", Level: 65, MR: 145, CR: 100, FR: 120,
				Traits: "Flurry",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear. DEBUFFED = Add Other Rains",
				Spells: []raidSpell{
					{ID: 2822, Name: "Upheaval", OnYou: "The ground buckles and heaves beneath your feet.", Recast: 12},
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Recast: 10, Counter: "Curse"},
					{ID: 2824, Name: "Spire Strike", OnYou: "Focused energy leaps from the spires in a blinding flash.", OnOther: " is stricken by a blast of focused energy.", Recast: 20},
				},
			},
			{
				Name: "Servitor of Luclin", Zone: "Grieg's End", Level: 65, MR: 45, CR: 180, FR: 40,
				Traits: "Flurry",
				Note:   "NUKES: Magic + Fire Tears",
				Spells: []raidSpell{},
			},
			{
				Name: "Lcea Katta", Zone: "Katta Castellum", Level: 60, MR: 150, CR: 60, FR: 55,
				Traits: "Flurry",
				Note:   "NUKES: Cold + Fire Tears. DEBUFFED = Add Magic Rain",
				Spells: []raidSpell{},
			},
			{
				Name: "Nathyn Illuminious", Zone: "Katta Castellum", Level: 64, MR: 50, CR: 50, FR: 50,
				Traits: "Flurry",
				Note:   "NUKES: All 3 Tears",
				Spells: []raidSpell{},
			},
			{
				Name: "Lord Inquisitor Seru", Zone: "Sanctus Seru", Level: 66, MR: 1000, CR: 100, FR: 100,
				Traits: "Flurry",
				Note:   "NUKES: Cold + Fire Tears. DEBUFFED = Add Ancient/IceSpear",
				Spells: []raidSpell{
					{ID: 2061, Name: "Torturing Winds", OnYou: "You are stricken by torturing winds.", OnOther: " is stricken by torturing winds.", Recast: 45, Counter: "Curse"},
					{ID: 1968, Name: "Stunning Strike", OnYou: "You are struck by a sudden force.", Proc: true},
				},
			},
			{
				Name: "a glyph covered serpent", Zone: "Ssraeshza Temple", Level: 63, MR: 185, CR: 100, FR: 65,
				Traits: "Flurry",
				Note:   "NUKES: Fire Tears + Ancient/IceSpear. DEBUFFED = Cold/Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2809, Name: "Wave of Death", OnYou: "Your mind fills with images of death.", OnOther: " falls to the ground and convulses.", Recast: 24},
				},
			},
			{
				Name: "Arch Lich Rhag`Zadune", Zone: "Ssraeshza Temple", Level: 63, MR: 160, CR: 60, FR: 60,
				Traits: "Rampage, Unslowable",
				Note:   "NUKES: Cold + Fire Tears. DEBUFFED = Add Magic Rain",
				Spells: []raidSpell{
					{ID: 2046, Name: "Deathly Chants", OnYou: "Your mind is filled with deathly chants.", OnOther: "'s mind is filled with Deathly chants.", Recast: 18, Counter: "Curse"},
					{ID: 2047, Name: "Bone Grasp", OnYou: "Your body is wracked with pain.", OnOther: "'s body is wracked with pain.", Proc: true},
				},
			},
			{
				Name: "Emperor Ssraeshza", Zone: "Ssraeshza Temple", Level: 66, MR: 1000, CR: 75, FR: 60,
				Traits: "Rampage",
				// The real Emperor (npc 162491); 162065 is the untargetable
				// placeholder. Rage of Ssraeshza logs no land text: see the
				// tankbuster event.
				Spells: []raidSpell{},
			},
			{
				Name: "High Priest of Ssraeshza", Zone: "Ssraeshza Temple", Level: 66, MR: 100, CR: 125, FR: 150,
				Traits: "Rampage, Unslowable",
				Note:   "NUKES: Magic Tears + Lure of Ice. DEBUFFED = Use Rains",
				Spells: []raidSpell{
					{ID: 2048, Name: "Ssraeshza's Command", OnYou: "You are struck by a sudden force.", Recast: 27},
					{ID: 2805, Name: "Sermon of Ssraeshza", OnYou: "You are halted by the stunning sermon of Ssraeshza.", OnOther: " is halted by the stunning sermon of Ssraeshza.", Proc: true},
				},
			},
			{
				Name: "Rhag`Mozdezh", Zone: "Ssraeshza Temple", Level: 63, MR: 60, CR: 60, FR: 60,
				Traits: "Rampage, Unslowable",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear. DEBUFFED = Add Other Rains",
				Spells: []raidSpell{
					{ID: 2788, Name: "Breath of Rag'Mozdezh", OnYou: "You are washed over by the decaying breath of Rhag`Mozdezh.", OnOther: " is washed over by the decaying breath of Rhag`Mozdezh.", Proc: true, Counter: "Disease"},
				},
			},
			{
				Name: "Rhag`Zhezum", Zone: "Ssraeshza Temple", Level: 63, MR: 60, CR: 60, FR: 60,
				Traits: "Flurry, Unslowable",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear. DEBUFFED = Add Other Rains",
				Spells: []raidSpell{
					{ID: 2787, Name: "Rag`Zhezum's Deathly Embrace", OnYou: "You feel the deathly embrace of Rhag`Zhezum.", OnOther: " spasms and shudders, feeling their strength ebb away.", Proc: true},
				},
			},
			{
				Name: "Vyzh`dra the Cursed", Zone: "Ssraeshza Temple", Level: 66, MR: 155, CR: 55, FR: 100,
				Traits: "Flurry",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear. DEBUFFED = Add Other Rains",
				Spells: []raidSpell{
					{ID: 2813, Name: "Mass Insanity", OnYou: "Your mind fills with hate.", OnOther: " begins to laugh maniacally.", Recast: 12},
					{ID: 2814, Name: "Caustic Mist", OnYou: "Your skin begins to rot.", OnOther: "'s flesh begins to liquefy.", Recast: 24},
				},
			},
			{
				Name: "Vyzh`dra the Exiled", Zone: "Ssraeshza Temple", Level: 63, MR: 165, CR: 155, FR: 165,
				Traits: "Flurry, Unslowable",
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2812, Name: "Pyrokinetic Hallucinations", OnYou: "You see your flesh melt away.", OnOther: " screams for no reason.", Recast: 12},
					{ID: 2859, Name: "Touch of Vinitras", OnYou: "Your soul fades into darkness.", OnOther: "'s soul fades into darkness.", Recast: 120},
				},
			},
			{
				Name: "Xerkizh The Creator", Zone: "Ssraeshza Temple", Level: 66, MR: 165, CR: 60, FR: 60,
				Traits: "Rampage",
				Note:   "NUKES: Cold + Fire Tears. DEBUFFED = Add Magic Rain",
				Spells: []raidSpell{
					{ID: 2045, Name: "Weakening Spray", OnYou: "Your muscles deteriorate.", OnOther: "'s muscles Deteriorate.", Recast: 18},
					{ID: 2913, Name: "Balance of Zebuxoruk", OnYou: "You have been balanced.", OnOther: " has been balanced.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "A burrower parasite", Zone: "The Deep", Level: 63, MR: 330, CR: 180, FR: 170,
				Traits: "Flurry, Unslowable",
				Note:   "NUKES: Lure of Ice",
				Spells: []raidSpell{},
			},
			{
				Name: "Thought Horror Overfiend", Zone: "The Deep", Level: 63, MR: 155, CR: 60, FR: 100,
				Traits: "Flurry",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear",
				Spells: []raidSpell{
					{ID: 2073, Name: "Overfiend's Fury", OnYou: "You are overcome with despair.", Recast: 12},
					{ID: 2899, Name: "Feeblemind", OnYou: "Your feel somewhat dimwitted.", OnOther: " looks somewhat dimwitted.", Recast: 30, Counter: "Curse"},
				},
			},
			{
				Name: "Aten Ha Ra", Zone: "Vex Thal", Level: 66, MR: 162, CR: 195, FR: 176,
				Traits: "Rampage",
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2157, Name: "Word of Command", OnYou: "Your mind is consumed by a terrible darkness.", OnOther: " writhes and staggers", Recast: 30},
					{ID: 2164, Name: "Silence of the Shadows", OnYou: "Your mind is assaulted by horrifying visions.", OnOther: "'s mind is assaulted by horrifying visions.", Recast: 30},
					{ID: 2167, Name: "Fling", OnYou: "A massive force knocks you into the air.", OnOther: " is knocked into the air by a massive force.", Recast: 45},
				},
			},
			{
				Name: "Diabo Xi Va", Zone: "Vex Thal", Level: 66, MR: 185, CR: 90, FR: 90,
				Traits: "Unslowable",
				Note:   "NUKES: Cold + Fire Tears OR Cold Tears + Ancient",
				Spells: []raidSpell{
					{ID: 2163, Name: "Lure of Shadows", OnYou: "Your body is assaulted by living shadows.", OnOther: "'s body is assaulted by living shadows.", Recast: 15},
				},
			},
			{
				Name: "Diabo Xi Va Temariel", Zone: "Vex Thal", Level: 66, MR: 125, CR: 80, FR: 80,
				Note: "NUKES: Cold + Fire Tears OR Cold Tears + Ancient",
				Spells: []raidSpell{
					{ID: 2162, Name: "Black Winds", OnYou: "Your body is consumed by howling black winds.", OnOther: "'s body is consumed by howling black winds.", Recast: 35},
				},
			},
			{
				Name: "Diabo Xi Xin", Zone: "Vex Thal", Level: 66, MR: 165, CR: 75, FR: 75,
				Traits: "Rampage",
				Note:   "NUKES: Cold + Fire Tears OR Cold Tears + Ancient",
				Spells: []raidSpell{},
			},
			{
				Name: "Diabo Xi Xin Thall", Zone: "Vex Thal", Level: 66, MR: 110, CR: 165, FR: 125,
				Traits: "Rampage, Unslowable",
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{
					{ID: 2165, Name: "Mark of Darkness", OnYou: "Your skin is covered in a glowing black aura.", OnOther: "'s skin is covered in a glowing black aura.", Proc: true},
				},
			},
			{
				Name: "Kaas Thox Xi Ans Dyek", Zone: "Vex Thal", Level: 66, MR: 120, CR: 60, FR: 190,
				Traits: "Flurry, Unslowable",
				Note:   "NUKES: Cold Tears + Ancient/IceSpear. DEBUFFED = Add Fire Tears",
				Spells: []raidSpell{},
			},
			{
				Name: "Kaas Thox Xi Aten Ha Ra", Zone: "Vex Thal", Level: 66, MR: 110, CR: 100, FR: 100,
				Note:   "NUKES: Cold + Fire Tears OR Lure of Ice. DEBUFFED = Use Tears",
				Spells: []raidSpell{},
			},
			{
				Name: "Thall Va Kelun", Zone: "Vex Thal", Level: 66, MR: 110, CR: 120, FR: 95,
				Traits: "Rampage",
				Note:   "NUKES: Fire Tears + Lure of Ice. DEBUFFED = Use Tears",
				Spells: []raidSpell{
					{ID: 2160, Name: "Greater Shadow Poison", OnYou: "You are surrounded by living shadows.", OnOther: " is entrapped by living shadows.", Proc: true, Counter: "Poison"},
				},
			},
			{
				Name: "Thall Va Xakra", Zone: "Vex Thal", Level: 60, MR: 125, CR: 170, FR: 170,
				Traits: "Rampage",
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{},
			},
			{
				Name: "Thall Xundraux Diabo", Zone: "Vex Thal", Level: 66, MR: 185, CR: 190, FR: 155,
				Traits: "Flurry, Unslowable",
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{},
			},
			{
				Name: "Va Dyn Khar", Zone: "Vex Thal", Level: 66, MR: 26, CR: 26, FR: 26,
				Note:   "NUKES: All 3 Tears",
				Spells: []raidSpell{},
			},
			{
				Name: "Va Xi Aten Ha Ra", Zone: "Vex Thal", Level: 66, MR: 150, CR: 140, FR: 125,
				Note:   "NUKES: Lure of Ice. DEBUFFED = Fire Tears + SS/Garrisons",
				Spells: []raidSpell{},
			},
		},
		Events: raidLuclinEvents(),
	}
}

func raidLuclinEvents() []Trigger {
	// Shei Vinitras emotes and calls the four Tatrua every 320s while
	// engaged (akheva/encounters/SheiVinitras.lua "aggro" timer).
	shei := raidEventTimer("Shei Vinitras — next Tatrua adds",
		lit("Sivuelaeus Rulour ans Rashan!"), 320)
	shei.TimerAlerts[0].TTSTemplate = "Shei adds in 10"
	shei.Actions = []Action{
		{Type: ActionOverlayText, Text: "Shei: Tatrua adds", DurationSecs: 5, Color: "#f59e0b"},
	}

	// Vyzh`dra the Exiled and the Banished shout on aggro (npc_emotes
	// 1560); the first Touch of Vinitras lands about two minutes later.
	// Later casts are timed by the shared Touch of Vinitras trigger.
	vyzh := raidEventTimer("Vyzh`dra — first Touch of Vinitras",
		lit("Bow before your only god!"), 120)
	vyzh.TimerAlerts[0].TTSTemplate = "Death touch in 10"

	// Blood of Ssraeshza dies → 10s → this emote → the Emperor spawns 120s
	// later (ssratemple/encounters/Emperor.lua "spawnemp").
	emp := raidEventTimer("Emperor Ssraeshza — spawns",
		lit("The Emperor twitches."), 120)
	emp.TimerAlerts[0].TTSTemplate = "Emperor in 10"
	emp.Actions = []Action{
		{Type: ActionOverlayText, Text: "Emperor spawns in 2 minutes", DurationSecs: 5, Color: "#f59e0b"},
	}

	// Rage of Ssraeshza (spell 2310, 4000 damage, 60s recast) logs no land
	// text, only the damage line.
	rage := raidEventTimer("Emperor Ssraeshza — Rage of Ssraeshza",
		`^Emperor Ssraeshza +hit [A-Za-z]+ for [0-9]{4} points of non-melee damage\.$`, 60)
	rage.TimerAlerts[0].Seconds = 15
	rage.TimerAlerts[0].TTSTemplate = "Tank buster in 15"
	rage.Actions = []Action{
		{Type: ActionOverlayText, Text: "Rage of Ssraeshza — tank hit", DurationSecs: 5, Color: "#ff4444"},
	}

	return []Trigger{
		shei,
		vyzh,
		emp,
		rage,
		raidAlert("a glyph covered serpent — Wave of Death (stand up)", lit("Your mind fills with images of death."),
			"Wave of Death — stand up", "Stand up"),
	}
}

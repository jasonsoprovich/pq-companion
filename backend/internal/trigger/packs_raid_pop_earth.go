package trigger

// raidPoPEarth is the Plane of Earth: the four Earth A ring trials and the
// Earth B war chieftains, Rathe Council and Avatar of Earth. Quarm rewrote
// the rings; lines are from poeartha/encounters/{Dust,Mud,Stone,Vine}Ring.lua
// and poearthb/encounters/RatheCouncil.lua.
func raidPoPEarth() raidPackDef {
	return raidPackDef{
		Name: "Raid: Plane of Earth",
		Description: "Plane of Earth: the Dust, Mud, Stone and Vine ring " +
			"trials, the war chieftains, the Rathe Council and the Avatar of " +
			"Earth. Boss spell recast timers that bind to the boss casting " +
			"them, cure alerts for curable debuffs on you, ring phase callouts, " +
			"the Vine Ring add timer and the Rathe Council kill-credit timer. " +
			"/tell <boss> x (e.g. /tell warlord_gintolaken x) shows the boss's " +
			"level, resists, abilities and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "A Monsterous Mudwalker", Zone: "Plane of Earth A", Level: 73, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 2860, Name: "Shackles", OnYou: "A shackle fastens around your ankle.", OnOther: " is caught in a shackle.", Recast: 2},
					{ID: 3163, Name: "Storm Avalanche", OnYou: "You are pummeled by a cascading avalanche", OnOther: " is caught in a cascading avalanche", Proc: true},
				},
			},
			{
				Name: "A Mystical Arbitor of Earth", Zone: "Plane of Earth A", Level: 74, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3155, Name: "Elemental Judgement", OnYou: "You have been judged by the elements.", OnOther: " has been judged by the elements.", Recast: 36},
					{ID: 794, Name: "Fist of Earth", OnYou: "You feel the ground shake.", Proc: true},
				},
			},
			{
				Name: "A Perfected Warder of Earth", Zone: "Plane of Earth A", Level: 73, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1050, Name: "Raging Blizzard", OnYou: "Your body is torn by a raging blizzard.", OnOther: " is surrounded in a raging blizzard.", Recast: 40},
					{ID: 794, Name: "Fist of Earth", OnYou: "You feel the ground shake.", Proc: true},
				},
			},
			{
				Name: "Derugoak Bloodwalker", Zone: "Plane of Earth A", Level: 73, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1040, Name: "Meteor Storm", OnYou: "A cascade of meteors falls down upon you.", OnOther: " is caught in a hail of meteors.", Recast: 40},
					{ID: 2137, Name: "Rain of Terror", OnYou: "Your skin blisters as poison rains down on you.", Proc: true},
				},
			},
			{
				Name: "Peregrin Rockskull", Zone: "Plane of Earth A", Level: 73, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 2897, Name: "Pebble Rain", OnYou: "A cloud of pebbles pelts you and covers the ground.", OnOther: " is pelted by a cloud of pebbles.", Recast: 10},
					{ID: 3164, Name: "Froglok Misery", OnYou: "Huge stones fly through the air toward your head.", OnOther: " is caught in a hail of stones.", Proc: true},
				},
			},
			{
				Name: "Tantisala Jaggedtooth", Zone: "Plane of Earth A", Level: 73, MR: 90, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 836, Name: "Diseased Cloud", OnYou: "Your body begins to rot.", OnOther: "'s body begins to rot.", Recast: 12, Counter: "Disease"},
					{ID: 1151, Name: "Raven Screech", OnYou: "Your ears ring in pain.", Proc: true},
				},
			},
			{
				Name: "Avatar of Earth", Zone: "Plane of Earth B", Level: 80, MR: 175, CR: 190, FR: 190,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3913, Name: "Ashen Form", OnYou: "You feel your body turn amorphic.", OnOther: "'s body becomes amorphic.", Recast: 60},
					{ID: 3914, Name: "Marl", OnYou: "Your body is covered in dark marl.", OnOther: " is covered in dark marl.", Recast: 60, Counter: "Curse"},
				},
			},
			{
				Name: "War Chieftan Awisano", Zone: "Plane of Earth B", Level: 75, MR: 290, CR: 230, FR: 125,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1081, Name: "Deathfog IV", OnYou: "Your lungs begin to burn!", OnOther: " begins to cough", Recast: 120},
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "War Chieftan Birak", Zone: "Plane of Earth B", Level: 75, MR: 235, CR: 290, FR: 125,
				Traits: "Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1081, Name: "Deathfog IV", OnYou: "Your lungs begin to burn!", OnOther: " begins to cough", Recast: 120},
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "War Chieftan Galronar", Zone: "Plane of Earth B", Level: 75, MR: 235, CR: 230, FR: 155,
				Traits: "Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1081, Name: "Deathfog IV", OnYou: "Your lungs begin to burn!", OnOther: " begins to cough", Recast: 120},
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "Warlord Gintolaken", Zone: "Plane of Earth B", Level: 77, MR: 365, CR: 290, FR: 150,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1042, Name: "Storm Comet", OnYou: "A great comet shatters your bones.", OnOther: " is struck by an enormous comet.", Recast: 30, Counter: "Curse"},
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Proc: true, Counter: "Curse"},
				},
			},
		},
		Events: raidPoPEarthEvents(),
	}
}

func raidPoPEarthEvents() []Trigger {
	// Vine Ring: the final defender roots and calls mangled adds every 60s
	// once engaged (poeartha/encounters/VineRing.lua).
	vineAdds := raidEventTimer("Vine Ring — next adds",
		lit("The temple's defender drives its roots into the earth."), 60)
	vineAdds.Actions = []Action{
		{Type: ActionOverlayText, Text: "Vine defender adds", DurationSecs: 5, Color: "#f59e0b"},
	}

	// Rathe Council: all twelve must be dead together; a councilman's kill
	// credit expires 7 minutes after it dies and it returns
	// (poearthb/encounters/RatheCouncil.lua).
	council := raidEventTimer("Rathe Council — first kill expires",
		lit("One of the Rathe Council crumbles into lifeless stone."), 420)
	council.TimerAlerts = []TimerAlert{{
		ID:          "raid-rathe-credit-60s",
		Seconds:     60,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "First councilman returns in one minute",
		TTSVolume:   100,
	}}
	council.Actions = []Action{
		{Type: ActionOverlayText, Text: "First councilman down — kill all twelve within 7 minutes of each death", DurationSecs: 8, Color: "#f59e0b"},
	}

	return []Trigger{
		// Earth A rings: shared complete / fail lines.
		raidAlert("Earth ring — complete", `The trial of the (?:Dust|Mud|Stone|Vine) Ring is complete\.`,
			"Ring trial complete", "Ring complete"),
		raidAlert("Earth ring — failed", anyOf("The allotted time has passed.", "The trial cannot continue.", "trial cannot be resumed."),
			"RING TRIAL FAILED", "Ring failed"),

		// Dust Ring.
		raidAlert("Dust Ring — started", lit("the dusty warder bursts into grit."),
			"Dust Ring: 37 devotees", "Dust ring"),
		raidAlert("Dust Ring — Triumvirate", lit("The last dust devotee breaks apart."),
			"Dust Ring: Triumvirate of Soil", "Triumvirate"),
		raidAlert("Dust Ring — final defender", lit("The last of the Triumvirate sinks into loose earth."),
			"Dust Ring: final defender", "Final defender"),
		raidAlert("Dust Ring — guardians", lit("Three earthen forms rise at the temple's summons"),
			"Dust Ring: three guardians — kill them to weaken the defender", "Guardians"),

		// Mud Ring.
		raidAlert("Mud Ring — Sludge Lurker", anyOf("The last mudwalker bursts in a spray of wet clay.", "The final mudwalker bursts in a spray of filth."),
			"Mud Ring: Sludge Lurker", "Sludge Lurker"),
		raidAlert("Mud Ring — mudlets", lit("The Sludge Lurker ruptures in a heavy splash"),
			"Mud Ring: mudlets — kill them all", "Mudlets"),
		raidAlert("Mud Ring — gorgers", lit("Four bloated gorgers heave themselves from the muck"),
			"Mud Ring: four gorgers", "Gorgers"),
		raidAlert("Mud Ring — final guardian", lit("The last filth gorger bursts"),
			"Mud Ring: final guardian", "Final guardian"),

		// Stone Ring.
		raidAlert("Stone Ring — fortifications", lit("Massive slabs of stone wrench themselves from the earth and lock together."),
			"Stone Ring: fortifications", "Stone ring"),
		raidAlert("Stone Ring — rubble mounds", lit("The Rock Monstrosity's massive frame splits into four mounds of rubble"),
			"Stone Ring: four rubble mounds", "Rubble"),
		raidAlert("Stone Ring — Rock Monstrosity", lit("The Rock Monstrosity rises once more"),
			"Stone Ring: Rock Monstrosity", "Rock Monstrosity"),
		raidAlert("Stone Ring — stone heaps", lit("The Rock Monstrosity crashes apart"),
			"Stone Ring: stone heaps", "Heaps"),
		raidAlert("Stone Ring — final defender", lit("The last stone heap shatters."),
			"Stone Ring: final defender", "Final defender"),

		// Vine Ring.
		raidAlert("Vine Ring — bloodthirsty creatures", lit("The last tainted rock beast crashes down."),
			"Vine Ring: bloodthirsty creatures", "Bloodthirsty"),
		raidAlert("Vine Ring — final defender", lit("The last bloodthirsty creature collapses in a tangle of severed vines."),
			"Vine Ring: final defender — adds every 60s", "Final defender"),
		vineAdds,

		// Earth B Rathe Council.
		raidAlert("Rathe Council — engaged", lit("The stone beneath Ragrax trembles as the Rathe Council speaks as one"),
			"Rathe Council engaged", "Council engaged"),
		council,
		raidAlert("Rathe Council — councilman returned", lit("Stone knits together as a fallen Councilman rises anew."),
			"A councilman returned — its kill no longer counts", "Councilman returned"),
		raidAlert("Rathe Council — Avatar of Earth", lit("The Rathe speak through the towering Avatar of Earth"),
			"Avatar of Earth — 35 minutes of idle allowed", "Avatar up"),
	}
}

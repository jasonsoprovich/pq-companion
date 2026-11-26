package trigger

// raidPoPWater is the Plane of Water: the reef nameds and Coirnav. Coirnav's
// lines and clock are from Project Quarm's powater/encounters/Coirnav.lua;
// his final-phase reinforcement timer lives in Raid Event Timers.
func raidPoPWater() raidPackDef {
	return raidPackDef{
		Name: "Raid: Plane of Water",
		Description: "Plane of Water: the reef nameds and Coirnav. Boss spell " +
			"recast timers that bind to the boss casting them, cure alerts for " +
			"curable debuffs on you, and Coirnav's 15-minute event clock, " +
			"minion waves and phase callouts. /tell <boss> x (e.g. /tell " +
			"hydrotha x) shows the boss's level, resists, abilities and " +
			"spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Coirnav the Avatar of Water", Zone: "Plane of Water", Level: 80, MR: 140, CR: 1000, FR: 175,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3061, Name: "Curse of the Triumvirate", OnYou: "The curse of the triumvirate consumes you.", OnOther: " has been cursed by the triumvirate.", Recast: 30, Counter: "Curse"},
				},
			},
			{
				Name: "Grioihin the Wise", Zone: "Plane of Water", Level: 75, MR: 165, CR: 1000, FR: 165,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3057, Name: "Tidal Freeze", OnYou: "You have been struck by a tidal freeze.", OnOther: " has been struck by a tidal freeze.", Recast: 60},
				},
			},
			{
				Name: "Guardian of Coirnav", Zone: "Plane of Water", Level: 70, MR: 95, CR: 1000, FR: 50,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Hydrotha", Zone: "Plane of Water", Level: 75, MR: 165, CR: 1000, FR: 165,
				Traits: "Flurry",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3058, Name: "Engulfing Ink", OnYou: "You have been consumed in dark ink.", OnOther: " is consumed in dark ink.", Recast: 30},
				},
			},
			{
				Name: "Krziik the Mighty", Zone: "Plane of Water", Level: 77, MR: 115, CR: 1000, FR: 165,
				Traits: "Rampage, Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 3059, Name: "Battle Cry of Regrua", OnYou: "You have been struck by the battle cry of Regrua", OnOther: " has been struck by the battle cry of Regrua.", Recast: 60},
				},
			},
			{
				Name: "Nrinda of Ice", Zone: "Plane of Water", Level: 75, MR: 95, CR: 1000, FR: 50,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Ofossaa the Enlightened", Zone: "Plane of Water", Level: 78, MR: 175, CR: 1000, FR: 115,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3060, Name: "Mists of Enlightenment", OnYou: "An Icy mist forms around you.", OnOther: " is surrounded by an icy mist.", Recast: 20, Counter: "Curse"},
				},
			},
			{
				Name: "Pwelon of Vapor", Zone: "Plane of Water", Level: 75, MR: 95, CR: 1000, FR: 50,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Vamuil of Water", Zone: "Plane of Water", Level: 75, MR: 95, CR: 1000, FR: 50,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
		},
		Events: raidPoPWaterEvents(),
	}
}

func raidPoPWaterEvents() []Trigger {
	// Coirnav (powater/encounters/Coirnav.lua): the Guardian's death starts a
	// 900s clock and the vapor minions; ice minions follow at 180s and water
	// minions at 300s. Warnings at 600s, 720s and 840s pin the clock; at 900s
	// the raid is banished.
	clock := raidEventTimer("Coirnav — event clock",
		lit("I call upon the power imbued to me by Povar!"), 900,
		ExtraPattern{Pattern: lit("You will not break the will of the Triumvirate!"), Enabled: true, TimerDurationSecs: 300},
		ExtraPattern{Pattern: lit("Our power gathers, and your time dwindles."), Enabled: true, TimerDurationSecs: 180},
		ExtraPattern{Pattern: lit("Your time is nearly spent! Finish what you began."), Enabled: true, TimerDurationSecs: 60},
	)
	clock.TimerAlerts = []TimerAlert{{
		ID:          "raid-coirnav-clock-60s",
		Seconds:     60,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "Banish in one minute",
		TTSVolume:   100,
	}}
	clock.WornOffPattern = anyOf("Coirnav, the Avatar of Water, has fallen!", "These waters reject you. Be banished from our domain!")
	clock.Actions = []Action{
		{Type: ActionOverlayText, Text: "Coirnav: vapor minions — ice at 3:00, water at 5:00, banish at 15:00", DurationSecs: 8, Color: "#f59e0b"},
		{Type: ActionTextToSpeech, Text: "Vapor adds", Volume: 1.0},
	}
	waves := raidEventTimer("Coirnav — next minion wave",
		lit("Come forth my minions of vapor"), 180,
		ExtraPattern{Pattern: lit("Come forth my minions of ice"), Enabled: true, TimerDurationSecs: 120})
	return []Trigger{
		clock,
		waves,
		raidAlert("Coirnav — ice minions", lit("I call upon the power imbued to me by E`ci!"), "Ice minions", "Ice adds"),
		raidAlert("Coirnav — water minions", lit("I call upon the power imbued to me by Tarew Marr!"), "Water minions", "Water adds"),
		raidAlert("Coirnav — Pwelon, Nrinda, Vamuil", lit("The currents wrench across the reef as Pwelon, Nrinda and Vamuil gather before Coirnav."),
			"Kill Pwelon, Nrinda and Vamuil", "Nameds"),
		raidAlert("Coirnav — banished", lit("These waters reject you. Be banished from our domain!"),
			"BANISHED — event failed", "Banished"),
		raidAlert("Coirnav — defeated", lit("Coirnav, the Avatar of Water, has fallen!"),
			"Coirnav defeated", ""),
	}
}

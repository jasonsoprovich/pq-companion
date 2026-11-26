package trigger

// raidPoPTier3 is the Planes of Power tier 3 raids: Halls of Honor
// (trials, Rydda`Dar, the Temple of Marr), Bastion of Thunder and Drunder
// (Tallon, Vallon and Rallos Zek). Event lines and timings are from Project
// Quarm's hohonora/, bothunder/ and potactics/encounters/Rallos.lua.
func raidPoPTier3() raidPackDef {
	return raidPackDef{
		Name: "Raid: PoP Tier 3",
		Description: "Planes of Power tier 3 raids: Halls of Honor and the " +
			"Temple of Marr, Bastion of Thunder, and Drunder. Boss spell " +
			"recast timers that bind to the boss casting them, cure alerts for " +
			"curable debuffs on you, Agnarr portal and Rallos Zek corpse-wave " +
			"timers, and encounter callouts. /tell <boss> x (e.g. /tell " +
			"vallon_zek x) shows the boss's level, resists, abilities and " +
			"spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Advocent Joran", Zone: "Halls of Honor", Level: 70, MR: 95, CR: 65, FR: 65,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Freegan Haun", Zone: "Halls of Honor", Level: 72, MR: 95, CR: 80, FR: 80,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Halgoz Rellinic", Zone: "Halls of Honor", Level: 72, MR: 65, CR: 90, FR: 90,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Rydda`Dar", Zone: "Halls of Honor", Level: 75, MR: 350, CR: 195, FR: 185,
				Traits: "Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3175, Name: "Crystal Roar", OnYou: "You flee in terror.", Recast: 60},
					{ID: 3174, Name: "Screeching Bellow", OnYou: "The bellow fills your ears.", OnOther: "holds their ears.", Proc: true, Counter: "Disease"},
				},
			},
			{
				Name: "Agnarr the Storm Lord", Zone: "Bastion of Thunder", Level: 75, MR: 140, CR: 90, FR: 90,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1043, Name: "Manastorm", OnYou: "Your mind is rent by the manastorm.", OnOther: "'s mind staggers from the manastorm.", Recast: 36},
					{ID: 1044, Name: "Rage of the Rainkeeper", OnYou: "You are consumed in the fury of the Rainkeeper.", OnOther: " is consumed in the fury of the Rainkeeper.", Recast: 40},
					{ID: 1045, Name: "Chain Lightning", OnYou: "A bolt of lightning arcs through your body.", OnOther: " screams as a bolt of lightning arcs over them.", Proc: true},
				},
			},
			{
				Name: "Auliffe Chaoswind", Zone: "Bastion of Thunder", Level: 67, MR: 200, CR: 125, FR: 65,
				Traits: "Flurry",
				Note:   "CH 4sec",
				Spells: []raidSpell{
					{ID: 1034, Name: "Cyclone", OnYou: "You are caught in a raging cyclone.", OnOther: " is caught in a raging cyclone.", Proc: true},
				},
			},
			{
				Name: "Brynju Thunderclap", Zone: "Bastion of Thunder", Level: 67, MR: 200, CR: 65, FR: 1000,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1026, Name: "Thunder Call", OnYou: "You are caught in a column of energy.", OnOther: " is struck by a bolt of planar lightning", Proc: true},
				},
			},
			{
				Name: "Eindride Icestorm", Zone: "Bastion of Thunder", Level: 67, MR: 200, CR: 1000, FR: 65,
				Traits: "Flurry",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1047, Name: "Deluge", OnYou: "A great wave knocks you to the ground.", OnOther: " is pummeled by a great wave.", Proc: true},
				},
			},
			{
				Name: "Emmerik Skyfury", Zone: "Bastion of Thunder", Level: 73, MR: 140, CR: 1000, FR: 65,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 665, Name: "Drifting Death", OnYou: "You feel the pain of a thousand stings.", Recast: 2.25},
					{ID: 1040, Name: "Meteor Storm", OnYou: "A cascade of meteors falls down upon you.", OnOther: " is caught in a hail of meteors.", Recast: 40},
					{ID: 1042, Name: "Storm Comet", OnYou: "A great comet shatters your bones.", OnOther: " is struck by an enormous comet.", Recast: 30, Counter: "Curse"},
					{ID: 1290, Name: "Chloroblast", Buff: true},
					{ID: 3192, Name: "Earthen Roots", OnYou: "Your feet become entwined.", Recast: 2.25},
					{ID: 3418, Name: "Frozen Wind", OnYou: "You are encased in a frozen wind.", OnOther: " is encased in a frozen wind.", Recast: 30},
					{ID: 3431, Name: "Brushfire", OnYou: "Brushfire rises around you.", OnOther: " is consumed in brushfire.", Recast: 30},
					{ID: 1041, Name: "Stone Thunder", OnYou: "You are surrounded by cosmic stone.", OnOther: " is encased in meteor dust.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "Evynd Firestorm", Zone: "Bastion of Thunder", Level: 70, MR: 140, CR: 65, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1036, Name: "Storm Flame", OnYou: "Burning embers descend around you.", OnOther: " is caught in a rain of fire.", Recast: 36},
					{ID: 1038, Name: "Flamewall", OnYou: "A wall of fire rises around you.", OnOther: " is surrounded by a wall of fire.", Proc: true},
				},
			},
			{
				Name: "Gaukr Sandstorm", Zone: "Bastion of Thunder", Level: 65, MR: 200, CR: 140, FR: 75,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1031, Name: "Stone Gale", OnYou: "Huge stones fly through the air toward your head.", OnOther: " is caught in a hail of stones.", Proc: true},
				},
			},
			{
				Name: "Hreidar Lynhillig", Zone: "Bastion of Thunder", Level: 65, MR: 200, CR: 65, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1026, Name: "Thunder Call", OnYou: "You are caught in a column of energy.", OnOther: " is struck by a bolt of planar lightning", Proc: true},
				},
			},
			{
				Name: "Kuanbyr Hailstorm", Zone: "Bastion of Thunder", Level: 67, MR: 200, CR: 80, FR: 85,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1031, Name: "Stone Gale", OnYou: "Huge stones fly through the air toward your head.", OnOther: " is caught in a hail of stones.", Proc: true},
				},
			},
			{
				Name: "Laef Windfall", Zone: "Bastion of Thunder", Level: 65, MR: 200, CR: 140, FR: 65,
				Note: "CH 4sec",
				Spells: []raidSpell{
					{ID: 1033, Name: "Tornado", OnYou: "You are caught in a tornado.", OnOther: " is trapped in a tornado.", Proc: true},
				},
			},
			{
				Name: "Oreen Wavecrasher", Zone: "Bastion of Thunder", Level: 65, MR: 200, CR: 1000, FR: 65,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Edium, Guardian of Marr", Zone: "Temple of Marr", Level: 75, MR: 95, CR: 95, FR: 95,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3173, Name: "Protection of Marr", OnYou: "The force of valor forces you back.", OnOther: " has been struck back.", Proc: true},
				},
			},
			{
				Name: "Halon of Marr", Zone: "Temple of Marr", Level: 75, MR: 95, CR: 95, FR: 95,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3170, Name: "Blinding Beauty", OnYou: "The radiant beauty blinds you.", OnOther: " has been blinded.", Recast: 60},
					{ID: 3172, Name: "Denial", OnYou: "Your existence has been denied.", OnOther: "'s existence has been denied.", Proc: true},
				},
			},
			{
				Name: "Lord Mithaniel Marr", Zone: "Temple of Marr", Level: 77, MR: 350, CR: 215, FR: 215,
				Traits: "Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1454, Name: "Flame of Light", OnYou: "A flame of divine light erupts around you.", OnOther: "'s body erupts in a flame of divine light.", Recast: 18},
					{ID: 3165, Name: "Silence of Marr", OnYou: "Your legs feel weak.", OnOther: " stumbles.", Recast: 3},
					{ID: 3166, Name: "Blinding Essence of Purity", OnYou: "The pure essence blinds you.", OnOther: " has been blinded.", Recast: 60},
					{ID: 3429, Name: "Touch of Nife", Buff: true},
					{ID: 3167, Name: "Strike of Marr", OnYou: "The blade of valor tears your soul apart.", OnOther: " has been stuck by the valor of Mithaniel.", Proc: true},
				},
			},
			{
				Name: "Ralthazor, Champion of Marr", Zone: "Temple of Marr", Level: 75, MR: 95, CR: 95, FR: 95,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3169, Name: "Champion's Protection", OnYou: "Your knees buckle.", OnOther: " 's knees buckle.", Recast: 180},
					{ID: 3168, Name: "Strike of the Champion", OnYou: "You have been stuck by valor.", OnOther: " has been struck by valor.", Proc: true},
				},
			},
			{
				Name: "Rallos Zek the Warlord", Zone: "Drunder, the Fortress of Zek", Level: 75, MR: 140, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 2444, Name: "Rage of Zek", OnYou: "The chaos of war consumes your soul.", OnOther: "'s eyes go wide with horror.", Recast: 35, Counter: "Curse"},
					{ID: 2445, Name: "Sedition", OnYou: "Your armor shatters.", OnOther: "'s armor crumbles to the ground.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "Tallon Zek", Zone: "Drunder, the Fortress of Zek", Level: 73, MR: 145, CR: 95, FR: 95,
				Traits: "Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 808, Name: "Avatar Power", OnYou: "You are knocked backward.", Recast: 35},
					{ID: 2448, Name: "Tallon's Balance", OnYou: "Chromatic runes surround you.", OnOther: " is surrounded by the runes of Zek.", Recast: 60},
					{ID: 2449, Name: "Barb of Tallon", OnYou: "The blood freezes in your veins.", OnOther: " has been struck by a huge frozen arrow.", Recast: 4},
					{ID: 2450, Name: "Barb of Tallon", OnYou: "Your veins incinerate.", OnOther: " screams as the arrow ignites their blood.", Recast: 4.5},
					{ID: 2451, Name: "Barb of Tallon", OnYou: "Your stomach lurches and refuses to obey you.", OnOther: " struggles to stay on their feet as the disease infests them.", Recast: 5},
					{ID: 2452, Name: "Barb of Tallon", OnYou: "The barb of Tallon sears your flesh.", OnOther: " has been poisoned by Tallon's barb.", Recast: 6},
				},
			},
			{
				Name: "Vallon Zek", Zone: "Drunder, the Fortress of Zek", Level: 73, MR: 140, CR: 90, FR: 90,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 2446, Name: "Vallon's Precision", OnYou: "You have been marked.", OnOther: "'s body glows with dark red runes.", Recast: 35},
					{ID: 2447, Name: "Vallon's Subversion", OnYou: "Vallon begins to chuckle at your pathetic skills.", OnOther: " struggles to keep Vallon's attention.", Proc: true},
				},
			},
		},
		Events: raidPoPTier3Events(),
	}
}

func raidPoPTier3Events() []Trigger {
	// Agnarr (bothunder/Agnarr_the_Storm_Lord.lua): portals 120s after the
	// pull, then every 120s — 90s below 50% health, 60s below 25%.
	agnarrPull := lit("Agnarr the Storm Lord says 'Fool! This is the realm of the Storm Giants now, and you hope to defeat me here?'")
	portals := raidEventTimer("Agnarr — portals",
		lit("Agnarr strikes his staff to the ground, causing great ripples of energy to rage across the room."), 120,
		ExtraPattern{Pattern: agnarrPull, Enabled: true})
	portals.Actions = []Action{
		{Type: ActionOverlayText, Text: "Agnarr portals — next in 120s (90s under 50%, 60s under 25%)", DurationSecs: 6, Color: "#f59e0b"},
	}

	// Rallos Zek in the pit (potactics/encounters/Rallos.lua): the corpses
	// rise 55s after the pull and every 55s after, with wraiths 10s later.
	corpses := raidEventTimer("Rallos — corpse wave",
		lit("The corpses of the unworthy begin to twitch and spasm"), 55,
		ExtraPattern{Pattern: lit("The Warlord meets your challenge in the pit of Drunder."), Enabled: true})
	corpses.Actions = []Action{
		{Type: ActionOverlayText, Text: "Corpses rising — wraiths in 10s", DurationSecs: 6, Color: "#f59e0b"},
		{Type: ActionTextToSpeech, Text: "Corpse wave", Volume: 1.0},
	}

	return []Trigger{
		// Halls of Honor trials (hohonora/encounters/*.lua).
		raidAlert("Honor — Crazed trial failed", lit("A woman screams in anguish as she's forcibly taken away by a band of crazed norrathians."),
			"TRIAL FAILED", "Trial failed"),
		raidAlert("Honor — trial flag", lit("An ethereal mist descends from the air and surrounds your very being."),
			"Trial flag received", "Trial flag"),

		raidAlert("Agnarr — engaged", agnarrPull, "Agnarr engaged — portals in 120s", "Agnarr engaged"),
		portals,

		// Drunder (potactics/encounters/Rallos.lua).
		raidAlert("Rallos — Warlord stirs", lit("The air of Drunder grows strangely cold as a rumble shakes through the fortress' walls."),
			"The Warlord stirs — Tallon and Vallon summoned", "Rallos stirring"),
		raidAlert("Rallos — sons fallen", lit("Rallos Zek takes notice at the death of his sons."),
			"Rallos Zek is coming", ""),
		raidAlert("Rallos — engaged", lit("Rallos Zek raises his axe as flames explode from it!"),
			"Rallos engaged — elites every 50s", "Rallos engaged"),
		raidAlert("Rallos — leaps to the pit", lit("Rallos Zek leaps from the War Room into the cleared pit below!"),
			"Rallos to the pit", "Rallos to the pit"),
		corpses,
		raidAlert("Rallos — leashed", lit("'Enough of this foolishness!  The warlord has better things to do then chase petty mortals all day!'"),
			"Rallos reset", "Rallos reset"),
		raidAlert("Rallos — defeated", lit("Rallos Zek, the Warlord, has fallen!"),
			"Rallos Zek defeated", ""),
	}
}

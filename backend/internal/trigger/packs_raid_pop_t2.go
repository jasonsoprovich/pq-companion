package trigger

// raidPoPTier2 is the Planes of Power tier 2 raids: Plane of Valor
// (Aerin`Dar), the Crypt of Decay (Bertoxxulous and his court), Plane of
// Storms and Torment (Saryrn). Bertoxxulous event lines and timings are
// from Project Quarm's codecay/encounters/Bertox.lua.
func raidPoPTier2() raidPackDef {
	return raidPackDef{
		Name: "Raid: PoP Tier 2",
		Description: "Planes of Power tier 2 raids: Aerin`Dar, the Crypt of " +
			"Decay, Plane of Storms and Torment. Boss spell recast timers that " +
			"bind to the boss casting them, cure alerts for curable debuffs on " +
			"you, and the Bertoxxulous event's preparation and deadline timers " +
			"and phase callouts. /tell <boss> x (e.g. /tell saryrn x) shows " +
			"the boss's level, resists, abilities and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Aerin`Dar", Zone: "Plane of Valor", Level: 70, MR: 80, CR: 75, FR: 65,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3225, Name: "Glass Roar", OnYou: "The fear of death overcomes you.", OnOther: " is overcome by fear.", Recast: 36},
					{ID: 3226, Name: "Glass Shards", OnYou: "You are pierced by several sharp pieces of shattered glass.", OnOther: " is pierced by glass.", Proc: true, Counter: "Disease"},
				},
			},
			{
				Name: "Avhi Escron", Zone: "The Crypt of Decay", Level: 70, MR: 100, CR: 100, FR: 100,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1061, Name: "Gathering of Souls", OnYou: "You feel a slight tug and then you are wracked with pain.", OnOther: " staggers as his soul is thrown out of balance.", Proc: true},
				},
			},
			{
				Name: "Bertoxxulous", Zone: "The Crypt of Decay", Level: 75, MR: 115, CR: 115, FR: 115,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3012, Name: "Rot of the Plaguebringer", OnYou: "Your system is assaulted by a deadly virus.", OnOther: " has been infected.", Recast: 60, Counter: "Disease"},
				},
			},
			{
				Name: "Bishop Toluwon", Zone: "The Crypt of Decay", Level: 70, MR: 80, CR: 80, FR: 80,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1062, Name: "Touch of Life", Buff: true},
				},
			},
			{
				Name: "Carprin Deatharn", Zone: "The Crypt of Decay", Level: 70, MR: 80, CR: 80, FR: 80,
				Traits: "Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1059, Name: "Theft of Life", OnYou: "You feel a pain deep down as your soul is drained of it's life.", Proc: true},
				},
			},
			{
				Name: "High Priest Ultor Szanvon", Zone: "The Crypt of Decay", Level: 70, MR: 80, CR: 80, FR: 80,
				Traits: "Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1064, Name: "Enveloping Entropy", OnYou: "You are enveloped with endless entropy.", Recast: 60, Counter: "Disease"},
				},
			},
			{
				Name: "Raex Pwodill", Zone: "The Crypt of Decay", Level: 70, MR: 80, CR: 80, FR: 80,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1065, Name: "Lifeshock", OnYou: "A hard shock comes over you as your life is drained away.", OnOther: " staggers as his lifeforce is torn.", Proc: true},
				},
			},
			{
				Name: "Vindor Mawnil", Zone: "The Crypt of Decay", Level: 70, MR: 80, CR: 80, FR: 80,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1065, Name: "Lifeshock", OnYou: "A hard shock comes over you as your life is drained away.", OnOther: " staggers as his lifeforce is torn.", Proc: true},
				},
			},
			{
				Name: "Falto, Lord of Thunder", Zone: "Plane of Storms", Level: 70, MR: 140, CR: 45, FR: 515,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3323, Name: "Maelstrom of Thunder", OnYou: "Great storm bolts arc through your body.", OnOther: " convulse as storm bolts arc across them.", Proc: true},
				},
			},
			{
				Name: "Faruek the Bold", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 80, FR: 65,
				Traits: "Rampage, Flurry, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Gurebk, Lord of Krendic", Zone: "Plane of Storms", Level: 70, MR: 140, CR: 145, FR: 85,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1031, Name: "Stone Gale", OnYou: "Huge stones fly through the air toward your head.", OnOther: " is caught in a hail of stones.", Proc: true},
				},
			},
			{
				Name: "Jeplak, Lord of Srerendi", Zone: "Plane of Storms", Level: 70, MR: 140, CR: 125, FR: 95,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3162, Name: "Wind Strike", OnYou: "You have been knocked to the ground.", OnOther: " has been knocked to the ground.", Proc: true},
				},
			},
			{
				Name: "Laruken the Rigid", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 195, FR: 65,
				Traits: "Rampage, Flurry, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Neffiken, Lord of Kelek`Vor", Zone: "Plane of Storms", Level: 70, MR: 140, CR: 515, FR: 85,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1046, Name: "Manabolt", OnYou: "Your mana fades away.", OnOther: " is struck by a bolt of mana.", Proc: true},
				},
			},
			{
				Name: "Paruek the Strong", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 80, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Pendubk the Turbulent", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 75, FR: 65,
				Traits: "Rampage, Flurry, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Solnebk the Unruly", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 85, FR: 85,
				Traits: "Rampage, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Ston`Ruak, Ancient of the Trees", Zone: "Plane of Storms", Level: 70, MR: 100, CR: 85, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Zertuken the Unyielding", Zone: "Plane of Storms", Level: 65, MR: 80, CR: 195, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Maareq the Prophet", Zone: "Torment, the Plane of Pain", Level: 70, MR: 80, CR: 75, FR: 75,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Saryrn", Zone: "Torment, the Plane of Pain", Level: 70, MR: 115, CR: 95, FR: 95,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3152, Name: "Will of Saryrn", OnYou: "You succumb to the will of Saryrn.", Recast: 45},
				},
			},
			{
				Name: "The Keeper of Sorrows", Zone: "Torment, the Plane of Pain", Level: 66, MR: 65, CR: 65, FR: 65,
				Traits: "AE Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{},
			},
		},
		Events: raidPoPTier2Events(),
	}
}

func raidPoPTier2Events() []Trigger {
	// Bertoxxulous (codecay/encounters/Bertox.lua): killing the Spectre of
	// Corruption starts a 350s preparation, then waves of trash and eight
	// Adan kings, the last four kings, then Bertoxxulous. The whole event
	// must finish within 7380s of the Spectre's death; a warning goes out
	// 300s before the deadline.
	deadline := raidEventTimer("Bertox — event deadline",
		lit("Violators of the depths of Lxanvom shall pay with your lives!"), 7380)
	deadline.TimerAlerts = []TimerAlert{{
		ID:          "raid-bertox-deadline-5m",
		Seconds:     300,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "Bertox event ends in 5 minutes",
		TTSVolume:   100,
	}}
	deadline.Actions = []Action{
		{Type: ActionOverlayText, Text: "Bertox event started — waves in 350s, 2h03m to finish", DurationSecs: 8, Color: "#f59e0b"},
	}
	return []Trigger{
		// Both timers key on the same preparation line (different fragments,
		// so each trigger keeps a unique pattern).
		raidEventTimer("Bertox — waves begin",
			lit("Crazed laughter is heard as you notice a foul creature standing before you."), 350),
		deadline,
		raidAlert("Bertox — waves", lit("A foul wind is felt carrying on it the stench of death and decay."),
			"Decay waves — stay in the main room", "Adds"),
		raidAlert("Bertox — king summoned", lit("'We call to you, corrupted King of Lxanvom,"),
			"Adan king up", "Named up"),
		raidAlert("Bertox — last four kings", lit("Meedo Adan! Qezzin Adan! Pzo Adan! Bhaly Adan!"),
			"Last four kings (Bhaly is unslowable)", "Last four kings"),
		raidAlert("Bertox — Bertoxxulous up", lit("'Defilers death comes for you today!'"),
			"Bertoxxulous up", "Bertox up"),
		raidAlert("Bertox — 5 minutes left", lit("'Your time is close in coming to an end. Time to flee little ones!'"),
			"Bertox event: 5 minutes left", ""),
		raidAlert("Bertox — event failed", lit("'Perhaps you would care to try when you are more powerful fools.'"),
			"BERTOX EVENT FAILED", "Event failed"),
		raidAlert("Bertox — defeated", lit("Bertoxxulous has been defeated."),
			"Bertoxxulous defeated — hail the Planar Projection", "Bertox down"),
	}
}

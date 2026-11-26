package trigger

// raidPoPFire is the Plane of Fire (Doomfire) and the Tower of Solusek Ro.
// Doomfire's lines and event budget are from Project Quarm's
// pofire/encounters/Fennin.lua; the tower's flag lines from
// solrotower/player.lua.
func raidPoPFire() raidPackDef {
	return raidPackDef{
		Name: "Raid: Plane of Fire",
		Description: "Plane of Fire (Doomfire) and the Tower of Solusek Ro. " +
			"Boss spell recast timers that bind to the boss casting them, cure " +
			"alerts for curable debuffs on you, the Doomfire event deadline " +
			"and phase callouts, and tower flag callouts. /tell <boss> x (e.g. " +
			"/tell solusek_ro x) shows the boss's level, resists, abilities " +
			"and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Arch Mage Yozanni", Zone: "Plane of Fire", Level: 78, MR: 170, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3019, Name: "Disjunction of Flame", OnYou: "You have been struck by a disjunction of flame.", OnOther: " has been struck with a disjunction of flame.", Recast: 60},
				},
			},
			{
				Name: "Azobian the Darklord", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3049, Name: "Infernal Darkness", OnYou: "You are consumed in a wave of endless darkness.", OnOther: " is consumed in a wave of endless darkness.", Recast: 30},
				},
			},
			{
				Name: "Babnoxis the Spider Queen", Zone: "Plane of Fire", Level: 75, MR: 115, CR: 165, FR: 1000,
				Traits: "Flurry",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3016, Name: "Poisoned Flames", OnYou: "Poison flames burn across your body.", OnOther: " has been consumed in poison flames.", Recast: 30, Counter: "Poison"},
				},
			},
			{
				Name: "Blazzax the Omnifiend", Zone: "Plane of Fire", Level: 78, MR: 170, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3018, Name: "Nova Inferno", OnYou: "You have been surrounded by a fiery inferno.", OnOther: " has been surrounded by a fiery inferno.", Recast: 30},
				},
			},
			{
				Name: "Chancellor Kirtra", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3053, Name: "Web of Flame", OnYou: "You are caught in a web of flame.", OnOther: " is caught in a web of flame.", Recast: 60},
				},
			},
			{
				Name: "Chancellor Traxom", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3052, Name: "Rising Suns", OnYou: "You are assaulted by rising suns.", OnOther: " is assaulted by rising suns.", Recast: 60},
				},
			},
			{
				Name: "Criare Sunmane", Zone: "Plane of Fire", Level: 75, MR: 165, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3014, Name: "Phoenix Charm", OnYou: "You have been struck with the power of the dawn.", OnOther: " has been struck with the power of the dawn.", Recast: 120},
				},
			},
			{
				Name: "Fennin Ro the Tyrant of Fire", Zone: "Plane of Fire", Level: 80, MR: 115, CR: 115, FR: 1000,
				Traits: "AE Rampage",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3055, Name: "Cataclysm of Ro", OnYou: "The earth shakes as fire erupts around you.", OnOther: " is caught in the cataclysm of Ro.", Recast: 60, Counter: "Curse"},
				},
			},
			{
				Name: "General Druav Flamesinger", Zone: "Plane of Fire", Level: 75, MR: 115, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3013, Name: "Fiery Strike", OnYou: "You have been struck by flames.", OnOther: " has been struck by flames.", Proc: true},
				},
			},
			{
				Name: "General Reparm", Zone: "Plane of Fire", Level: 78, MR: 170, CR: 165, FR: 1000,
				Traits: "Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 3013, Name: "Fiery Strike", OnYou: "You have been struck by flames.", OnOther: " has been struck by flames.", Proc: true},
				},
			},
			{
				Name: "Guardian of Doomfire", Zone: "Plane of Fire", Level: 70, MR: 125, CR: 50, FR: 1000,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Hebabbilys the Ragelord", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3050, Name: "Ravaging Rage of the Fiends", Buff: true},
				},
			},
			{
				Name: "Javonn the Overlord", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3051, Name: "Fiery Assault", OnYou: "You are caught in a fiery assault.", OnOther: " is caught in a fiery assault.", Recast: 30},
				},
			},
			{
				Name: "Jaxoliz Dawneyes", Zone: "Plane of Fire", Level: 75, MR: 165, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3014, Name: "Phoenix Charm", OnYou: "You have been struck with the power of the dawn.", OnOther: " has been struck with the power of the dawn.", Recast: 120},
				},
			},
			{
				Name: "Magmaton", Zone: "Plane of Fire", Level: 75, MR: 170, CR: 50, FR: 1000,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Omni Magus Crato", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3054, Name: "Comet of Flames", OnYou: "A comet of flames strikes you.", OnOther: " has been struck by a comet of flames.", Recast: 120},
				},
			},
			{
				Name: "Pyronis", Zone: "Plane of Fire", Level: 75, MR: 115, CR: 165, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3017, Name: "Mighty Bellow of Fire", OnYou: "A mighty bellow of fire strikes you.", OnOther: " has been struck by a bellow of fire.", Recast: 60},
				},
			},
			{
				Name: "Quavonis Firetail", Zone: "Plane of Fire", Level: 78, MR: 170, CR: 50, FR: 1000,
				Note: "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3015, Name: "Juxtapose of Fire", OnYou: "You have been struck by a shimmering strike of fire.", OnOther: " has been struck by shimmering fire.", Recast: 24},
				},
			},
			{
				Name: "Reaxnous the Chaoslord", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Note: "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3020, Name: "Rain of Burning Fire", OnYou: "Your skin seers as a rain of fire falls around you.", OnOther: "'s skin seers as a rain of fire falls around you.", Recast: 30},
				},
			},
			{
				Name: "Warlord Prollaz", Zone: "Plane of Fire", Level: 78, MR: 125, CR: 50, FR: 1000,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Arlyxir", Zone: "Tower of Solusek Ro", Level: 75, MR: 65, CR: 50, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 2673, Name: "Unity of Fire", Buff: true},
				},
			},
			{
				Name: "Jiva", Zone: "Tower of Solusek Ro", Level: 75, MR: 65, CR: 50, FR: 1000,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3009, Name: "Reckless Mutterings", OnYou: "Small flames dance across your body.", OnOther: " is covered in living flames.", Recast: 20},
					{ID: 1643, Name: "Draught of Jiva", OnYou: "You are caught in a torrent of reckless magic.", OnOther: " is caught in a torrent of reckless magic.", Proc: true},
				},
			},
			{
				Name: "Rizlona", Zone: "Tower of Solusek Ro", Level: 70, MR: 65, CR: 50, FR: 1000,
				Note: "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 820, Name: "FlameSongOfRo", OnYou: "You are immolated by flame.", Recast: 12},
				},
			},
			{
				Name: "Solusek Ro", Zone: "Tower of Solusek Ro", Level: 80, MR: 65, CR: 50, FR: 1000,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3006, Name: "Corona of Sol", Buff: true},
					{ID: 3007, Name: "Solar Flame", OnYou: "A bolt of solar flame tears through your body.", OnOther: "'s chest explodes.", Recast: 24},
					{ID: 3008, Name: "Solar Winds", OnYou: "Solar winds envelope your body.", OnOther: "'s body burns in solar winds.", Recast: 48},
				},
			},
			{
				Name: "The Protector of Dresolik", Zone: "Tower of Solusek Ro", Level: 75, MR: 65, CR: 50, FR: 1000,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3004, Name: "Fist of Lava", OnYou: "You are smashed by an enflamed fist.", OnOther: " is struck by an enflamed fist.", Proc: true},
				},
			},
			{
				Name: "Xuzl", Zone: "Tower of Solusek Ro", Level: 75, MR: 65, CR: 50, FR: 1000,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
		},
		Events: raidPoPFireEvents(),
	}
}

func raidPoPFireEvents() []Trigger {
	// Doomfire (pofire/encounters/Fennin.lua): killing the Guardian starts a
	// 17500s budget for the four phases; Fennin Ro's voice warns every 2700s.
	const fenninBooms = "All of Doomfire trembles as the voice of Fennin Ro booms, "
	deadline := raidEventTimer("Doomfire — event deadline",
		lit("The Guardian of Doomfire crashes to the scorched earth."), 17500)
	deadline.TimerAlerts = []TimerAlert{{
		ID:          "raid-doomfire-deadline-30m",
		Seconds:     1800,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "Doomfire ends in 30 minutes",
		TTSVolume:   100,
	}}
	deadline.Actions = []Action{
		{Type: ActionOverlayText, Text: "Doomfire phase 1 — Army of Flames (4h52m to finish)", DurationSecs: 8, Color: "#f59e0b"},
		{Type: ActionTextToSpeech, Text: "Phase one", Volume: 1.0},
	}
	return []Trigger{
		raidAlert("Doomfire — Guardian engaged", lit("The Guardian of Doomfire unleashes an echoing howl."),
			"Guardian of Doomfire engaged", "Guardian engaged"),
		deadline,
		raidAlert("Doomfire — phase 2 commanders", lit("Four commanders rise amid the flames"),
			"Phase 2 — Azobian, Javonn, Hebabbilys, Reaxnous", "Commanders"),
		raidAlert("Doomfire — phase 3 Council of Fire", lit("the Council of Fire advances through the wreckage of the army."),
			"Phase 3 — Council of Fire", "Council of Fire"),
		raidAlert("Doomfire — Fennin Ro up", lit("The Tyrant of Fire rises from the inferno"),
			"Fennin Ro up", "Fennin up"),
		raidAlert("Doomfire — Fennin engaged", lit("The inferno erupts as Fennin Ro meets your challenge."),
			"Fennin Ro engaged", ""),
		raidAlert("Doomfire — failed", lit(fenninBooms+"\"Enough, Norrathians! Carry the memory of this failure back to your world."),
			"DOOMFIRE FAILED", "Event failed"),
		raidAlert("Doomfire — Fennin defeated", lit("Fennin Ro, the Tyrant of Fire, falls!"),
			"Fennin Ro defeated", ""),

		// Tower of Solusek Ro (solrotower/player.lua).
		raidAlert("Sol Ro tower — cauldron flag", lit("As you place your hand on the burning cauldron, you feel"),
			"Cauldron flag received", "Flag"),
		raidAlert("Sol Ro tower — portal blocked", lit("The lava runes scorch your skin as you run your hand across them."),
			"Portal blocked — you need all five cauldron flags", "Need flags"),
	}
}

package trigger

// raidClassic covers the original-world and planar raid targets: Lord
// Nagafen, Lady Vox and the Planes of Fear, Hate, Sky and Growth.
func raidClassic() raidPackDef {
	return raidPackDef{
		Name: "Raid: Classic & Planar",
		Description: "Classic and planar raid targets: Lord Nagafen, Lady Vox " +
			"and the Planes of Fear, Hate, Sky and Growth. Boss breath and spell " +
			"recast timers that bind to the boss casting them, cure alerts for " +
			"curable debuffs on you, and Death Touch timers for the Plane of " +
			"Sky and Plane of Fear bosses that cast it. /tell <boss> x (e.g. " +
			"/tell vox x or /tell kos x) shows the boss's level, resists, " +
			"abilities, spells, healing plan and tactics. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Lord Nagafen", Zone: "Nagafen's Lair", Level: 55, MR: 340, CR: 70, FR: 425,
				Aliases: []string{"Naggy", "Nagafen"},
				Note:    "CH: As needed · Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 861, Name: "Lava Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
					{ID: 981, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
				},
			},
			{
				Name: "Lady Vox", Zone: "Permafrost", Level: 55, MR: 340, CR: 1000, FR: 70,
				Aliases: []string{"Vox"},
				Note:    "CH: As needed · Keep junk buff in top slot! Must be sieved!",
				Spells: []raidSpell{
					{ID: 862, Name: "Frost Breath", OnYou: "Your body freezes as the frost hits you.", OnOther: "'s body freezes as the frost hits them.", Recast: 12},
					{ID: 981, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
				},
			},
			{
				Name: "a dracoliche", Zone: "Plane of Fear", Level: 58, MR: 220, CR: 335, FR: 175,
				Aliases: []string{"Draco", "Dracoliche"},
				Note:    "Assist/fast heals · Tank should use evasive, enchanters spam rune",
				Spells: []raidSpell{
					{ID: 993, Name: "Deadly Lifetap", OnYou: "You feel a tugging at your soul.", Proc: true},
				},
			},
			{
				Name: "Cazic Thule", Zone: "Plane of Fear", Level: 70, MR: 155, CR: 100, FR: 380,
				Aliases: []string{"CT"},
				Traits:  "Rampage, Unslowable",
				Note:    "Assist/fast heals · Keep SoW/JBoots top slot and he will dispel his own snare",
				Spells: []raidSpell{
					{ID: 808, Name: "Avatar Power", OnYou: "You are knocked backward.", Recast: 45},
					{ID: 1308, Name: "Avatar Snare", OnYou: "Your knees buckle.", OnOther: " 's knees buckle.", Recast: 4},
					{ID: 956, Name: "Panic", OnYou: "You panic.", OnOther: " panics.", Proc: true},
				},
			},
			{
				Name: "Dread", Zone: "Plane of Fear", Level: 55, MR: 185, CR: 435, FR: 435,
				Note: "Assist/fast heals · Root Dread once in camp so he doesn't DT tank",
				Spells: []raidSpell{
					{ID: 1312, Name: "Scarab Storm", OnYou: "Scarabs burrow into your flesh.", OnOther: " shrieks as scarabs burrow into their skin.", Recast: 3, Counter: "Disease"},
				},
			},
			{
				Name: "Fright", Zone: "Plane of Fear", Level: 55, MR: 250, CR: 325, FR: 175,
				Note:   "Assist/fast heals · Root Fright once in camp so he doesn't DT tank",
				Spells: []raidSpell{},
			},
			{
				Name: "Terror", Zone: "Plane of Fear", Level: 55, MR: 435, CR: 190, FR: 335,
				Note: "Assist/fast heals · Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 862, Name: "Frost Breath", OnYou: "Your body freezes as the frost hits you.", OnOther: "'s body freezes as the frost hits them.", Recast: 12},
				},
			},
			{
				Name: "a gleaming sphere of light", Zone: "Plane of Growth", Level: 65, MR: 240, CR: 26, FR: 26,
				Aliases: []string{"Sphere"},
				Note:    "CH: 1s",
				Spells:  []raidSpell{},
			},
			{
				Name: "Ail the Elder", Zone: "Plane of Growth", Level: 60, MR: 100, CR: 100, FR: 100,
				Note: "CH: 6s+ · Must be sieved! Keep your top slot free to easily dispel silence after fight",
				Spells: []raidSpell{
					{ID: 1466, Name: "Cloud of Silence", OnYou: "You are in a cloud of silence.", OnOther: " is surrounded by a cloud of silence.", Recast: 15},
					{ID: 1480, Name: "Silver Breath", OnYou: "You feel your skin freeze.", Recast: 12},
				},
			},
			{
				Name: "Fayl Everstrong", Zone: "Plane of Growth", Level: 60, MR: 400, CR: 400, FR: 400,
				Note:   "CH: 6s+ · Must be sieved!",
				Spells: []raidSpell{},
			},
			{
				Name: "Guardian of Takish", Zone: "Plane of Growth", Level: 70, MR: 435, CR: 435, FR: 435,
				Aliases: []string{"Takish"},
				Note:    "CH: Instant · Hits like AoW, need a tight chain and bumps ready",
				Spells:  []raidSpell{},
			},
			{
				Name: "Guardian of Tunare", Zone: "Plane of Growth", Level: 60, MR: 40, CR: 65, FR: 40,
				Note: "CH: 6s+ · For #1, keep top slot free for silence, bards should be with clerics running group dispel song. For #2, keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 1466, Name: "Cloud of Silence", OnYou: "You are in a cloud of silence.", OnOther: " is surrounded by a cloud of silence.", Recast: 15},
				},
			},
			{
				Name: "keeper of the glades", Zone: "Plane of Growth", Level: 65, MR: 75, CR: 100, FR: 50,
				Note:   "CH: 3s",
				Spells: []raidSpell{},
			},
			{
				Name: "Prince Thirneg", Zone: "Plane of Growth", Level: 65, MR: 90, CR: 50, FR: 55,
				Aliases: []string{"Thirneg"},
				Traits:  "Unslowable",
				Note:    "CH: 3s",
				Spells:  []raidSpell{},
			},
			{
				Name: "Rumbleroot", Zone: "Plane of Growth", Level: 60, MR: 110, CR: 100, FR: 100,
				Note: "CH: 4s · Keep your top slot free to dispel DoT after fight or you will die",
				Spells: []raidSpell{
					{ID: 1424, Name: "Swarm of Retribution", OnYou: "You feel the pain of a million stings."},
				},
			},
			{
				Name: "Treah Greenroot", Zone: "Plane of Growth", Level: 60, MR: 45, CR: 45, FR: 45,
				Note:   "CH: 3s · Must be sieved!",
				Spells: []raidSpell{},
			},
			{
				Name: "Tunare", Zone: "Plane of Growth", Level: 70, MR: 385, CR: 385, FR: 390,
				Traits: "Flurry",
				Note:   "CH: 2s · Need a wall of ogres/trolls to stop MT from being punted out of range",
				Spells: []raidSpell{
					{ID: 1745, Name: "Vengeance of the Glades", OnYou: "Your feet adhere to the ground.", Recast: 4},
					{ID: 1746, Name: "Power of the Forests", OnYou: "Your body writhes in agony as the spirits of the forest attack.", OnOther: " writhes in agony s the spirits of the forest attack.", Recast: 4},
				},
			},
			{
				Name: "Tunarean Earthmelder", Zone: "Plane of Growth", Level: 60, MR: 35, CR: 35, FR: 35,
				Aliases: []string{"Earthmelder"},
				Note:    "CH: 10s · Must be sieved!",
				Spells:  []raidSpell{},
			},
			{
				Name: "a spite golem", Zone: "Plane of Hate", Level: 51, MR: 435, CR: 435, FR: 435,
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "Coercer T`vala", Zone: "Plane of Hate", Level: 56, MR: 80, CR: 125, FR: 75,
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "High Priest M`kari", Zone: "Plane of Hate", Level: 56, MR: 125, CR: 75, FR: 75,
				Traits: "Rampage",
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "Innoruuk", Zone: "Plane of Hate", Level: 70, MR: 200, CR: 250, FR: 150,
				Aliases: []string{"Inny"},
				Traits:  "Rampage",
				Note:    "CH: 4s, Slowed: 6s+ · Make sure everyone has levitation! Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 808, Name: "Avatar Power", OnYou: "You are knocked backward.", Recast: 35},
				},
			},
			{
				Name: "Lord of Loathing", Zone: "Plane of Hate", Level: 56, MR: 100, CR: 100, FR: 75,
				Traits: "Rampage",
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "Maestro of Rancor", Zone: "Plane of Hate", Level: 56, MR: 415, CR: 415, FR: 125,
				Traits: "Rampage",
				Note:   "Remedy/DL spam + CH · Tank must be max range to avoid other melee being hit by AOE",
				Spells: []raidSpell{},
			},
			{
				Name: "Magi P`tasa", Zone: "Plane of Hate", Level: 56, MR: 100, CR: 150, FR: 75,
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "Master of Spite", Zone: "Plane of Hate", Level: 56, MR: 435, CR: 435, FR: 435,
				Traits: "Rampage",
				Note:   "Remedy/DL spam · Must be sieved or DPSed down",
				Spells: []raidSpell{},
			},
			{
				Name: "Bazzt Zzzt", Zone: "Plane of Sky", Level: 63, MR: 435, CR: 435, FR: 435,
				Aliases: []string{"Bee Queen", "Queen Bee"},
				Note:    "CH: 3s",
				Spells:  []raidSpell{},
			},
			{
				Name: "Eye of Veeshan", Zone: "Plane of Sky", Level: 70, MR: 475, CR: 475, FR: 475,
				Note:   "Remedy/DL spam",
				Spells: []raidSpell{},
			},
			{
				Name: "Keeper of Souls", Zone: "Plane of Sky", Level: 60, MR: 450, CR: 425, FR: 440,
				Aliases: []string{"KoS"},
				Note:    "Remedy/DL spam",
				Spells:  []raidSpell{},
			},
			{
				Name: "Sister of the Spire", Zone: "Plane of Sky", Level: 65, MR: 450, CR: 450, FR: 460,
				Note: "Remedy/DL spam · Tank must be max range to avoid other melee being hit by AOE",
				Spells: []raidSpell{
					{ID: 989, Name: "Entomb in Ice", OnYou: "You are entombed in ice.", Recast: 18},
				},
			},
			{
				Name: "The Spiroc Lord", Zone: "Plane of Sky", Level: 63, MR: 435, CR: 435, FR: 435,
				Aliases: []string{"Spiroc Lord"},
				Note:    "Remedy/DL spam + CH · Hits hard when dual-wielding",
				Spells:  []raidSpell{},
			},
		},
		Events: raidClassicEvents(),
	}
}

func raidClassicEvents() []Trigger {
	// Cazic Touch (45s recast on each of these bosses' spell lists) logs no
	// land text; the server has the boss shout the target's name.
	var out []Trigger
	for _, boss := range []string{
		"Dread", "Fright",
		"Bazzt Zzzt", "Eye of Veeshan", "Keeper of Souls", "Overseer of Air",
		"Sister of the Spire", "the Hand of Veeshan", "The Spiroc Lord",
	} {
		dt := raidEventTimer(boss+" — Death Touch",
			`^`+lit(boss)+` shouts, '([A-Za-z]+)!'$`, 45)
		dt.TimerAlerts[0].TTSTemplate = "Death touch in 10"
		dt.Actions = []Action{
			{Type: ActionOverlayText, Text: boss + " Death Touch — {1}", DurationSecs: 5, Color: "#ff4444"},
		}
		out = append(out, dt)
	}
	return out
}

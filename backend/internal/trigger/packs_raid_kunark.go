package trigger

// raidKunark covers the Kunark raid targets: the open-world dragons,
// Trakanon, Venril Sathir, Chardok's Overking and Queen, Master Yael and
// Veeshan's Peak.
func raidKunark() raidPackDef {
	return raidPackDef{
		Name: "Raid: Kunark",
		Description: "Kunark raid targets: Gorenaire, Severilous, Talendor, " +
			"Faydedar, Trakanon, Venril Sathir, Chardok's Overking and Queen, " +
			"Master Yael and the Veeshan's Peak dragons. Boss breath and spell " +
			"recast timers that bind to the dragon casting them (Trakanon's " +
			"banish and Poison Breath, Dragon Charm...), cure alerts for " +
			"curable debuffs on you and Master Yael's Death Touch timer. " +
			"/tell <boss> x (e.g. /tell trakanon x or /tell pd x) shows the " +
			"boss's level, resists, abilities, spells, healing plan and " +
			"tactics. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Overking Bathezid", Zone: "Chardok", Level: 65, MR: 185, CR: 190, FR: 175,
				Aliases: []string{"Chardok King", "Bathezid", "King Bathezid"},
				Note:    "CH: As needed · Must be sieved!",
				Spells: []raidSpell{
					{ID: 951, Name: "Fiery Death", OnYou: "You are immolated by flame.", Recast: 24},
					{ID: 856, Name: "Blinding Fear", OnYou: "You freeze in terror.", Proc: true},
				},
			},
			{
				Name: "Queen Velazul Di`zok", Zone: "Chardok", Level: 62, MR: 60, CR: 60, FR: 60,
				Aliases: []string{"Chardok Queen", "Velazul", "Queen Velazul"},
				Note:    "CH: As needed",
				Spells:  []raidSpell{},
			},
			{
				Name: "Gorenaire", Zone: "Dreadlands", Level: 60, MR: 350, CR: 435, FR: 60,
				Note: "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 789, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 845, Name: "Freezing Breath", OnYou: "An icy cold shoots through your body, slowing your movements.", OnOther: " is slowed by the freezing blast.", Recast: 12},
				},
			},
			{
				Name: "Severilous", Zone: "Emerald Jungle", Level: 60, MR: 325, CR: 335, FR: 175,
				Note: "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 789, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 844, Name: "Ceticious Cloud", OnYou: "Your legs lock in pain as you choke on the noxious poison.", OnOther: " doubles over in pain as the noxious poison enters their lungs.", Recast: 3},
				},
			},
			{
				Name: "Venril Sathir", Zone: "Karnor's Castle", Level: 55, MR: 360, CR: 335, FR: 355,
				Aliases: []string{"VS"},
				Note:    "Assist/fast heals · Tank should use evasive, enchanters run a rune chain",
				Spells: []raidSpell{
					{ID: 993, Name: "Deadly Lifetap", OnYou: "You feel a tugging at your soul.", Proc: true},
				},
			},
			{
				Name: "Trakanon", Zone: "Old Sebilis", Level: 65, MR: 340, CR: 100, FR: 100,
				Note: "Remedy/DL spam · Snap agro tank w/ Rune + kiter required",
				Spells: []raidSpell{
					{ID: 855, Name: "Trakanon's Touch", OnYou: "You have been teleported.", Recast: 30},
					{ID: 856, Name: "Blinding Fear", OnYou: "You freeze in terror.", Recast: 60},
					{ID: 857, Name: "Poison Breath", OnYou: "A green mist seeps into your skin.", Recast: 35, Counter: "Poison"},
					{ID: 993, Name: "Deadly Lifetap", OnYou: "You feel a tugging at your soul.", Proc: true},
				},
			},
			{
				Name: "Talendor", Zone: "Skyfire", Level: 60, MR: 455, CR: 60, FR: 455,
				Note: "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 789, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 843, Name: "Immolating Breath", OnYou: "Your flesh is seared from your bones.", OnOther: "'s flesh is seared.", Recast: 12},
				},
			},
			{
				Name: "Faydedar", Zone: "Timorous Deep", Level: 55, MR: 425, CR: 50, FR: 450,
				Note: "CH: As needed · Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 789, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 861, Name: "Lava Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
				},
			},
			{
				Name: "Druushk", Zone: "Veeshan's Peak", Level: 65, MR: 425, CR: 425, FR: 75,
				Note: "Remedy/DL spam · Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 839, Name: "Lightning Breath", OnYou: "Your body is electrified as the lightning strikes you.", OnOther: "'s body is electrified as the lightning strikes.", Recast: 12},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Hoshkar", Zone: "Veeshan's Peak", Level: 65, MR: 425, CR: 425, FR: 75,
				Note: "Words of Restoration",
				Spells: []raidSpell{
					{ID: 836, Name: "Diseased Cloud", OnYou: "Your body begins to rot.", OnOther: "'s body begins to rot.", Recast: 12, Counter: "Disease"},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Nexona", Zone: "Veeshan's Peak", Level: 65, MR: 220, CR: 75, FR: 425,
				Note: "Words of Redemption, Remedy/DL spam, Wort pots · Keep SoW/JBoots top slot, dispel DoT ASAP",
				Spells: []raidSpell{
					{ID: 838, Name: "Stream of Acid", OnYou: "Your body burns as the acid hits you.", OnOther: "'s body burns as the acid hits them.", Recast: 12},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Phara Dar", Zone: "Veeshan's Peak", Level: 65, MR: 425, CR: 300, FR: 425,
				Aliases: []string{"PD"},
				Note:    "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 837, Name: "Stun Breath", OnYou: "Your eardrums rupture.", OnOther: " staggers with intense pain.", Recast: 12},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Silverwing", Zone: "Veeshan's Peak", Level: 65, MR: 425, CR: 300, FR: 425,
				Aliases: []string{"SW"},
				Note:    "Remedy/DL spam · Max MR",
				Spells: []raidSpell{
					{ID: 835, Name: "Chaos Breath", OnYou: "You experience chaotic weightlessness.", Recast: 12},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Xygoz", Zone: "Veeshan's Peak", Level: 65, MR: 425, CR: 300, FR: 425,
				Note: "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 840, Name: "Mesmerizing Breath", OnYou: "You are in a cloud of silence.", OnOther: " is surrounded by a cloud of silence.", Recast: 12},
					{ID: 841, Name: "Dragon Charm", OnYou: "You have been charmed.", Recast: 15},
				},
			},
			{
				Name: "Master Yael", Zone: "The Hole", Level: 56, MR: 175, CR: 435, FR: 435,
				// Cazic Touch (45s) logs only his shout of the target's name: see
				// the event.
				Spells: []raidSpell{},
			},
		},
		Events: raidKunarkEvents(),
	}
}

func raidKunarkEvents() []Trigger {
	// Master Yael's Cazic Touch (45s recast on his spell list) logs no
	// land text; the server has him shout the target's name as he casts.
	yael := raidEventTimer("Master Yael — Death Touch",
		`^Master Yael shouts, '([A-Za-z]+)!'$`, 45)
	yael.TimerAlerts[0].TTSTemplate = "Death touch in 10"
	yael.Actions = []Action{
		{Type: ActionOverlayText, Text: "Death Touch — {1}", DurationSecs: 5, Color: "#ff4444"},
	}
	return []Trigger{yael}
}

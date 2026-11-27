package trigger

// raidVelious covers the Velious raid targets: the open-world dragons,
// Temple of Veeshan, Sleeper's Tomb, Kael Drakkel, Skyshrine, Icewell Keep
// and Narandi's arrival in the Ring War. Event lines are from Project Quarm's
// greatdivide/encounters/Ring10.lua and growthplane/a_protector_of_growth.lua;
// timings from the same scripts and the protector's spawn2 respawn.
func raidVelious() raidPackDef {
	return raidPackDef{
		Name: "Raid: Velious",
		Description: "Velious raid targets: the dragons, Temple of Veeshan, " +
			"Sleeper's Tomb, Kael Drakkel, Skyshrine and Icewell Keep. Boss " +
			"breath and spell recast timers that bind to the dragon casting " +
			"them, cure alerts for curable debuffs on you, Narandi's arrival " +
			"(the Ring War clocks are in Raid: Scripted Event Timers), the " +
			"Plane of Growth protector respawn and the Temple of Veeshan jet traps. " +
			"/tell <boss> x (e.g. /tell vulak`aerr x) shows the boss's level, " +
			"resists, abilities, spells, CH cadence and tactics. " + raidCredit,
		Shared: raidSignatureShared("Caustic Mist / Putrefy Flesh"),
		Bosses: []raidBoss{
			{
				Name: "Kelorek`Dar", Zone: "Cobalt Scar", Level: 65, MR: 95, CR: 225, FR: 125,
				Note: "Remedy/DL",
				Spells: []raidSpell{
					{ID: 989, Name: "Entomb in Ice", OnYou: "You are entombed in ice.", Recast: 18},
				},
			},
			{
				Name: "Zlandicar", Zone: "Dragon Necropolis", Level: 70, MR: 235, CR: 155, FR: 140,
				Traits: "Rampage",
				Note:   "CH Unslowed: 8s, Slowed: 1 cleric · Max MR, wort pot fight! Do not get low HP agro!",
				Spells: []raidSpell{
					{ID: 897, Name: "Rotting Flesh", OnYou: "Your flesh begins to rot.", OnOther: "'s flesh begins to rot.", Recast: 35, Counter: "Disease"},
					{ID: 981, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 1956, Name: "Putrefy Flesh", OnYou: "Your flesh begins to liquefy.", OnOther: "'s flesh begins to liquefy.", Recast: 30, Counter: "Disease"},
					{ID: 837, Name: "Stun Breath", OnYou: "Your eardrums rupture.", OnOther: " staggers with intense pain.", Proc: true},
				},
			},
			{
				Name: "Narandi the Wretched", Zone: "Great Divide", Level: 65, MR: 425, CR: 340, FR: 50,
				Traits: "Flurry",
				Note:   "CH 3s",
				Spells: []raidSpell{},
			},
			{
				Name: "Dain Frostreaver IV", Zone: "Icewell Keep", Level: 70, MR: 155, CR: 1000, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH: 6s · Tank must be bound at zoneline and always have TL box (to bind)",
				Spells: []raidSpell{
					{ID: 1476, Name: "The Dain's Justice", OnYou: "The justice of Dain Frostreaver casts you into the pit.", OnOther: " is cast into the pit by Dain Frostreavers justice.", Recast: 30},
				},
			},
			{
				Name: "The Avatar of War", Zone: "Kael Drakkel", Level: 70, MR: 190, CR: 35, FR: 1000,
				Traits: "Flurry, Unslowable",
				Note:   "CH: 1s · AoW hits really hard, chain needs to be tight, ideal to have 14+ clerics. All classes patch heal, enchanters cast Rune V. Need several bumps. Need to be ready to swap tanks once defensive wears off.",
				Spells: []raidSpell{},
			},
			{
				Name: "Derakor the Vindicator", Zone: "Kael Drakkel", Level: 70, MR: 35, CR: 435, FR: 45,
				Traits: "Rampage, Unslowable",
				Note:   "CH: 4s",
				Spells: []raidSpell{},
			},
			{
				Name: "Keldor Dek`Torek", Zone: "Kael Drakkel", Level: 65, MR: 240, CR: 435, FR: 50,
				Note:   "Remedy/DL spam · Must be sieved!",
				Spells: []raidSpell{},
			},
			{
				Name: "King Tormax", Zone: "Kael Drakkel", Level: 70, MR: 105, CR: 1000, FR: 60,
				Traits: "Rampage, Unslowable",
				Note:   "CH: 4s",
				Spells: []raidSpell{},
			},
			{
				Name: "The Statue of Rallos Zek", Zone: "Kael Drakkel", Level: 59, MR: 260, CR: 195, FR: 195,
				Traits: "Flurry",
				Note:   "CH Unslowed: 2s, Slowed: 4s",
				Spells: []raidSpell{},
			},
			{
				Name: "Lord Yelinak", Zone: "Skyshrine", Level: 70, MR: 115, CR: 1000, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH: 4s, 2x ramp · Everyone needs levitate to counteract Mind Cloud, especially tank. Bard DA, cleric DA, bard DA, cleric DA, bard DA to avoid Ice Breath",
				Spells: []raidSpell{
					{ID: 1949, Name: "Ice breath", OnYou: "Shards of magical ice rend you.", OnOther: "'s body is cut by shards of magical ice.", Recast: 120},
					{ID: 1950, Name: "Mind Cloud", OnYou: "A cloud of pain covers you.", OnOther: " is covered by a cloud of pain.", Recast: 120},
				},
			},
			{
				Name: "Hraashna the Warder", Zone: "Sleeper's Tomb", Level: 70, MR: 80, CR: 260, FR: 260,
				Traits: "Rampage",
				Note:   "CH Unslowed: 2s, Slowed: 4s · Keep SoW/JBoots top slot, dispel DoT ASAP",
				Spells: []raidSpell{
					{ID: 838, Name: "Stream of Acid", OnYou: "Your body burns as the acid hits you.", OnOther: "'s body burns as the acid hits them.", Recast: 12},
				},
			},
			{
				Name: "Master of the Guard", Zone: "Sleeper's Tomb", Level: 69, MR: 200, CR: 425, FR: 200,
				Traits: "Flurry",
				Note:   "CH Unslowed: 5s, Slowed: 9s+",
				Spells: []raidSpell{},
			},
			{
				Name: "Nanzata the Warder", Zone: "Sleeper's Tomb", Level: 70, MR: 250, CR: 100, FR: 300,
				Note: "CH Unslowed: 3s, Slowed: 5s · Keep junk buff in top slot",
				Spells: []raidSpell{
					{ID: 861, Name: "Lava Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
				},
			},
			{
				Name: "The Final Arbiter", Zone: "Sleeper's Tomb", Level: 70, MR: 300, CR: 425, FR: 200,
				Traits: "Flurry",
				Note:   "CH Unslowed: 5s, Slowed: 9s+",
				Spells: []raidSpell{},
			},
			{
				Name: "The Progenitor", Zone: "Sleeper's Tomb", Level: 70, MR: 250, CR: 425, FR: 200,
				Traits: "Flurry, Unslowable",
				Note:   "CH: 5s",
				Spells: []raidSpell{},
			},
			{
				Name: "Tukaarak the Warder", Zone: "Sleeper's Tomb", Level: 70, MR: 250, CR: 260, FR: 100,
				Traits: "Rampage",
				Note:   "CH Unslowed: 3s, Slowed: 5s · Clerics hide behind pillar to dodge silence",
				Spells: []raidSpell{
					{ID: 840, Name: "Mesmerizing Breath", OnYou: "You are in a cloud of silence.", OnOther: " is surrounded by a cloud of silence.", Recast: 12},
				},
			},
			{
				Name: "Ventani the Warder", Zone: "Sleeper's Tomb", Level: 70, MR: 320, CR: 250, FR: 250,
				Traits: "Rampage",
				Note:   "CH Unslowed: 3s, Slowed: 5s · Melee must dodge Freezing Breath",
				Spells: []raidSpell{
					{ID: 840, Name: "Mesmerizing Breath", OnYou: "You are in a cloud of silence.", OnOther: " is surrounded by a cloud of silence.", Recast: 12},
					{ID: 845, Name: "Freezing Breath", OnYou: "An icy cold shoots through your body, slowing your movements.", OnOther: " is slowed by the freezing blast.", Recast: 12},
				},
			},
			{
				Name: "Aaryonar", Zone: "Temple of Veeshan", Level: 66, MR: 225, CR: 190, FR: 190,
				Note: "CH Unslowed: 3s, Slowed: 6s · Casters must be shrunk and down the stairs out of LoS",
				Spells: []raidSpell{
					{ID: 1485, Name: "Cloud of Disempowerment", OnYou: "You feel your skin freeze.", Recast: 12},
				},
			},
			{
				Name: "Cekenar", Zone: "Temple of Veeshan", Level: 66, MR: 200, CR: 1000, FR: 105,
				Note: "CH Unslowed: 4s, Slowed: 8s",
				Spells: []raidSpell{
					{ID: 1482, Name: "Electric Blast", OnYou: "Your body is electrified as lightning strikes you.", OnOther: "'s body is electrified as the lightning strikes.", Recast: 12},
				},
			},
			{
				Name: "Dagarn the Destroyer", Zone: "Temple of Veeshan", Level: 70, MR: 250, CR: 150, FR: 1000,
				Traits: "Rampage",
				Note:   "CH Unslowed: 4s, Slowed: 8s",
				Spells: []raidSpell{
					{ID: 1488, Name: "Rain of Molten Lava", OnYou: "Lava sears your skin.", Recast: 12},
					{ID: 1490, Name: "Wave of Heat", OnYou: "A blast of heat sears your flesh", OnOther: "'s skin sears.", Recast: 8},
				},
			},
			{
				Name: "Dozekar the Cursed", Zone: "Temple of Veeshan", Level: 66, MR: 190, CR: 65, FR: 65,
				Note: "CH Unslowed: 3s, Slowed: 6s · Keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 1480, Name: "Silver Breath", OnYou: "You feel your skin freeze.", Recast: 12},
				},
			},
			{
				Name: "Eashen of the Sky", Zone: "Temple of Veeshan", Level: 66, MR: 60, CR: 1000, FR: 60,
				Traits: "Unslowable",
				Note:   "CH: 3s & Remedy Spam",
				Spells: []raidSpell{
					{ID: 1488, Name: "Rain of Molten Lava", OnYou: "Lava sears your skin.", Recast: 12},
					{ID: 1489, Name: "Wave of Cold", OnYou: "A blast of cold freezes your skin.", Recast: 12},
				},
			},
			{
				Name: "Gozzrem", Zone: "Temple of Veeshan", Level: 66, MR: 1000, CR: 65, FR: 1000,
				Note: "CH: 6s · Must be sieved! Keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 862, Name: "Frost Breath", OnYou: "Your body freezes as the frost hits you.", OnOther: "'s body freezes as the frost hits them.", Recast: 12},
				},
			},
			{
				Name: "Ikatiar the Venom", Zone: "Temple of Veeshan", Level: 66, MR: 55, CR: 35, FR: 35,
				Traits: "Unslowable",
				Note:   "CH: 4s",
				Spells: []raidSpell{},
			},
			{
				Name: "Jorlleag", Zone: "Temple of Veeshan", Level: 66, MR: 200, CR: 1000, FR: 100,
				Note: "CH Unslowed: 4s, Slowed: 8s · Must be sieved! Keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 862, Name: "Frost Breath", OnYou: "Your body freezes as the frost hits you.", OnOther: "'s body freezes as the frost hits them.", Recast: 12},
				},
			},
			{
				Name: "Lady Mirenilla", Zone: "Temple of Veeshan", Level: 66, MR: 210, CR: 370, FR: 160,
				Note: "CH Unslowed: 4s, Slowed: 8s · Clerics must DA to avoid the first silence AOE while positioning",
				Spells: []raidSpell{
					{ID: 1483, Name: "Cloud of Fear", OnYou: "Your mind is wracked by fear.", Recast: 12},
				},
			},
			{
				Name: "Lady Nevederia", Zone: "Temple of Veeshan", Level: 66, MR: 185, CR: 80, FR: 105,
				Note: "CH Unslowed: 6s, Slowed: 10s · Make sure to dispel her at the start of the fight",
				Spells: []raidSpell{
					{ID: 1478, Name: "Bellowing Winds", OnYou: "You begin to spin.", Recast: 12},
				},
			},
			{
				Name: "Lendiniara the Keeper", Zone: "Temple of Veeshan", Level: 66, MR: 160, CR: 170, FR: 1000,
				Traits: "Unslowable",
				Note:   "CH 4s · Keep junk buff in top slot! Clerics shrink and hide on steps",
				Spells: []raidSpell{
					{ID: 1480, Name: "Silver Breath", OnYou: "You feel your skin freeze.", Recast: 12},
				},
			},
			{
				Name: "Lord Feshlak", Zone: "Temple of Veeshan", Level: 66, MR: 200, CR: 150, FR: 150,
				Note: "CH Unslowed: 4s, Slowed: 8s",
				Spells: []raidSpell{
					{ID: 1489, Name: "Wave of Cold", OnYou: "A blast of cold freezes your skin.", Recast: 12},
					{ID: 1490, Name: "Wave of Heat", OnYou: "A blast of heat sears your flesh", OnOther: "'s skin sears.", Recast: 8},
				},
			},
			{
				Name: "Lord Koi`Doken", Zone: "Temple of Veeshan", Level: 66, MR: 195, CR: 145, FR: 165,
				Note: "CH Unslowed: 4s, Slowed: 8s",
				Spells: []raidSpell{
					{ID: 1484, Name: "Tsunami", OnYou: "A tsunami crushes you.", OnOther: " is crushed by a wall of water.", Recast: 12},
				},
			},
			{
				Name: "Lord Kreizenn", Zone: "Temple of Veeshan", Level: 66, MR: 60, CR: 1000, FR: 1000,
				Traits: "Unslowable",
				Note:   "CH 2s · Clerics must be in the nook to avoid ministun",
				Spells: []raidSpell{
					{ID: 1479, Name: "Wave of Flame", OnYou: "You feel your skin burn.", OnOther: "'s skin burns.", Recast: 12},
				},
			},
			{
				Name: "Lord Vyemm", Zone: "Temple of Veeshan", Level: 66, MR: 1000, CR: 1000, FR: 85,
				Note: "CH: 3s & Remedy spam",
				Spells: []raidSpell{
					{ID: 1481, Name: "Scream of Chaos", OnYou: "You experience chaotic weightlessness.", Recast: 12},
				},
			},
			{
				Name: "Sevalak", Zone: "Temple of Veeshan", Level: 66, MR: 205, CR: 160, FR: 80,
				Note: "CH Unslowed: 3s, Slowed: 6s",
				Spells: []raidSpell{
					{ID: 835, Name: "Chaos Breath", OnYou: "You experience chaotic weightlessness.", Recast: 12},
				},
			},
			{
				Name: "Telkorenar", Zone: "Temple of Veeshan", Level: 66, MR: 1000, CR: 1000, FR: 70,
				Note: "CH 6s",
				Spells: []raidSpell{
					{ID: 1941, Name: "Lava Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
				},
			},
			{
				Name: "Vulak`Aerr", Zone: "Temple of Veeshan", Level: 70, MR: 80, CR: 100, FR: 100,
				Traits: "Flurry",
				Note:   "CH Unslowed: 1s, Slowed: 3s · Melee hide from Ancient Breath",
				Spells: []raidSpell{
					{ID: 2106, Name: "Gift of A'err", OnYou: "A small piece of your soul is sucked out of you.", OnOther: " screams in horror and pain.", Recast: 15},
					{ID: 1486, Name: "Ancient Breath", OnYou: "Your life force drains away.", Recast: 60, Counter: "Disease"},
				},
			},
			{
				Name: "Zlexak", Zone: "Temple of Veeshan", Level: 66, MR: 180, CR: 90, FR: 90,
				Note: "CH Unslowed: 4s, Slowed: 8s · Everyone must dodge Diseased Cloud",
				Spells: []raidSpell{
					{ID: 836, Name: "Diseased Cloud", OnYou: "Your body begins to rot.", OnOther: "'s body begins to rot.", Recast: 12, Counter: "Disease"},
				},
			},
			{
				Name: "Wuoshi", Zone: "The Wakening Land", Level: 64, MR: 250, CR: 175, FR: 170,
				Note: "Remedy/DL spam",
				Spells: []raidSpell{
					{ID: 789, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 844, Name: "Ceticious Cloud", OnYou: "Your legs lock in pain as you choke on the noxious poison.", OnOther: " doubles over in pain as the noxious poison enters their lungs.", Recast: 3},
				},
			},
			{
				Name: "Velketor the Sorcerer", Zone: "Velketor's Labyrinth", Level: 66, MR: 345, CR: 1000, FR: 140,
				Note:   "CH: 3s",
				Spells: []raidSpell{},
			},
			{
				Name: "Klandicar", Zone: "Western Wastes", Level: 70, MR: 230, CR: 1000, FR: 105,
				Traits: "Rampage",
				Note:   "Heroic Bond heals or group heals · Wort pot fight, do not get low HP agro. Keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 981, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 1480, Name: "Silver Breath", OnYou: "You feel your skin freeze.", Recast: 12},
				},
			},
			{
				Name: "Sontalak", Zone: "Western Wastes", Level: 70, MR: 190, CR: 180, FR: 1000,
				Traits: "Flurry",
				Note:   "Heroic Bond heals or group heals · Rune is very important to keep the tank from being dispelled. Wort pot fight, do not get low HP agro. Keep junk buff in top slot!",
				Spells: []raidSpell{
					{ID: 1943, Name: "Molten Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
					{ID: 981, Name: "Dragon Roar", OnYou: "You flee in terror.", Recast: 36},
					{ID: 1660, Name: "Char", OnYou: "You feel your skin ignite and char.", OnOther: "'s skin ignites and chars.", Proc: true},
				},
			},
		},
		Events: raidVeliousEvents(),
	}
}

func raidVeliousEvents() []Trigger {
	// The Ring War's wave and Narandi clocks (greatdivide/encounters/
	// Ring10.lua), and clearing them on an early charge, live in Raid:
	// Scripted Event Timers; this pack only calls Narandi's arrival.

	// a protector of growth respawns in 25 minutes (spawn2 1500s); half
	// its spawns also bring the #protector that starts Tunare's event.
	growth := raidEventTimer("Plane of Growth — protector respawn",
		`^(?:You have slain a protector of growth!|a protector of growth has been slain by [A-Za-z]+!)$`, 1500)
	growth.TimerAlerts[0].Seconds = 30
	growth.TimerAlerts[0].TTSTemplate = "Protector of growth in 30"

	return []Trigger{
		raidAlert("Ring War — Narandi arrives", lit("Meddling fools, you are no match for the likes of me."),
			"Narandi — kill him before he reaches the end of his path", "Narandi"),
		growth,
		raidAlert("Temple of Veeshan — Acid Jet trap", lit("Acid begins to eat at your flesh."),
			"Acid Jet trap!", "Acid jet"),
		raidAlert("Temple of Veeshan — Flame Jet trap", lit("Your body is engulfed in a jet of flames."),
			"Flame Jet trap!", "Flame jet"),
	}
}

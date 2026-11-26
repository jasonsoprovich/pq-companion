package trigger

// raidPoPTier1 is the Planes of Power tier 1 raids: Plane of Justice
// (trials, Tribunal mini-bosses, the Seventh Hammer), Plane of Disease,
// Plane of Nightmare, Plane of Innovation and the Lair of Terris Thule.
// Event lines are quoted from Project Quarm's quest scripts
// (pojustice/, ponightmare/, poinnovation/, nightmareb/).
func raidPoPTier1() raidPackDef {
	return raidPackDef{
		Name: "Raid: PoP Tier 1",
		Description: "Planes of Power tier 1 raids: Justice, Disease, Nightmare, " +
			"Innovation and Terris Thule. Boss spell recast timers that bind to " +
			"the boss casting them, cure alerts for curable debuffs on you, " +
			"Tribunal trial and Seventh Hammer timers, and encounter callouts. " +
			"/tell <boss> x (e.g. /tell grummus x) shows the boss's level, " +
			"resists, abilities and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "a construct of nightmares", Zone: "Plane of Nightmares", Level: 65, MR: 65, CR: 65, FR: 50,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Aramin the Spider Guardian", Zone: "Plane of Disease", Level: 68, MR: 85, CR: 65, FR: 65,
				Traits: "Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3048, Name: "Plague Silk", OnYou: "You are entangled in a plagued web.", OnOther: " is covered in a plagued web.", Recast: 90, Counter: "Disease"},
					{ID: 1114, Name: "Viral Decay", OnYou: "Your system is assaulted by a deadly virus.", OnOther: " has been infected.", Proc: true},
				},
			},
			{
				Name: "Grummus", Zone: "Plane of Disease", Level: 70, MR: 85, CR: 65, FR: 65,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1113, Name: "Stench of Decay", OnYou: "Your lungs are filled with the stench of decay.", OnOther: " begins to cough uncontrollably.", Recast: 40, Counter: "Disease"},
					{ID: 1115, Name: "Plasma Decay", OnYou: "Your blood cells begin to dissolve.", OnOther: "'s veins turn black.", Recast: 240, Counter: "Curse"},
					{ID: 1112, Name: "Decay of the Plaguebringer", OnYou: "Your skin decays into a field of gangrene.", OnOther: "'s skin begins to melt into black decay.", Proc: true, Counter: "Disease"},
				},
			},
			{
				Name: "Gryme the Crypt Guardian", Zone: "Plane of Disease", Level: 66, MR: 75, CR: 65, FR: 65,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1113, Name: "Stench of Decay", OnYou: "Your lungs are filled with the stench of decay.", OnOther: " begins to cough uncontrollably.", Recast: 40, Counter: "Disease"},
				},
			},
			{
				Name: "Gallows Master Teion", Zone: "Plane of Justice", Level: 65, MR: 80, CR: 75, FR: 75,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 73, Name: "Gravity Flux", OnYou: "You experience chaotic weightlessness.", Recast: 10},
					{ID: 191, Name: "Feedback", Buff: true},
					{ID: 1691, Name: "Glamour of Kintaz", OnYou: "You are mesmerized by the Glamour of Kintaz.", OnOther: " has been mesmerized by the Glamour of Kintaz.", Recast: 2.25},
					{ID: 1697, Name: "Recant Magic", Buff: true},
					{ID: 1698, Name: "Dementia", OnYou: "Twisted logic warps your mind.", OnOther: "'s mind warps.", Recast: 8},
					{ID: 1703, Name: "Asphyxiate", OnYou: "You feel a shortness of breath.", Recast: 2.25},
					{ID: 1704, Name: "Wind of Tashanian", OnYou: "You hear the barking of Tashania.", Recast: 2.25, Counter: "Poison"},
					{ID: 1705, Name: "Boltran`s Agacerie", OnYou: "You have been charmed.", Recast: 15},
					{ID: 1709, Name: "Wonderous Rapidity", Buff: true},
					{ID: 1712, Name: "Forlorn Deeds", OnYou: "You slow down.", Recast: 2.25, Counter: "Disease"},
					{ID: 1715, Name: "Largarn's Lamentation", OnYou: "You are overcome by sadness.", OnOther: " begins to weep.", Recast: 24},
					{ID: 2016, Name: "Dementing Visions", OnYou: "Insidious imagery clouds your mind.", OnOther: "'s mind is clouded by insidious imagery.", Recast: 8},
					{ID: 3194, Name: "Greater Fetter", OnYou: "Your feet adhere to the ground.", Recast: 15},
					{ID: 3199, Name: "Arcane Rune", Buff: true},
					{ID: 3342, Name: "Howl of Tashan", OnYou: "You hear the barking of Tashania.", Recast: 2.25, Counter: "Poison"},
				},
			},
			{
				Name: "Lashman Azakal", Zone: "Plane of Justice", Level: 65, MR: 85, CR: 75, FR: 85,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1024, Name: "Thunderclap", OnYou: "A clap of thunder fills your ears.", OnOther: "  is stunned by a clap of thunder.", Recast: 8},
				},
			},
			{
				Name: "Prime Executioner Vathoch", Zone: "Plane of Justice", Level: 65, MR: 80, CR: 75, FR: 75,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1126, Name: "Armor Break", OnYou: "Your armor cracks and breaks.", OnOther: "'s armor cracks and breaks.", Recast: 10},
				},
			},
			{
				Name: "Punisher of Flame", Zone: "Plane of Justice", Level: 65, MR: 85, CR: 50, FR: 1000,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1488, Name: "Rain of Molten Lava", OnYou: "Lava sears your skin.", Recast: 12},
					{ID: 1526, Name: "Annul Magic", Buff: true},
					{ID: 1631, Name: "Atol's Spectral Shackles", OnYou: "Spectral shackles bind your feet to the ground.", OnOther: " is shackled to the ground.", Recast: 10},
					{ID: 1635, Name: "Markar's Discord", OnYou: "You are struck by a sudden force.", Recast: 24},
					{ID: 1636, Name: "Invert Gravity", OnYou: "You feel gravity reverse.", Recast: 12},
					{ID: 1638, Name: "Lure of Flame", OnYou: "You succumb to the lure of flame.", OnOther: " succumbs to the lure of flame.", Recast: 2.25},
					{ID: 1641, Name: "Draught of Ice", OnYou: "You are caught in a torrent of jagged ice.", OnOther: " is caught in a torrent of jagged ice.", Recast: 2.25},
					{ID: 1643, Name: "Draught of Jiva", OnYou: "You are caught in a torrent of reckless magic.", OnOther: " is caught in a torrent of reckless magic.", Recast: 2.25},
					{ID: 1644, Name: "Pillar of Flame", OnYou: "You are immolated by a pillar of flame.", OnOther: " is immolated by a pillar of flame.", Recast: 25},
					{ID: 1650, Name: "Inferno of Al'Kabor", OnYou: "You burn within the inferno of Al'Kabor.", OnOther: " burns within the inferno of Al'Kabor.", Recast: 9},
					{ID: 1658, Name: "Sunstrike", OnYou: "Your body is consumed by the flames of the sun.", OnOther: " is consumed by the flames of the sun.", Recast: 20},
					{ID: 1769, Name: "Lure of Ice", OnYou: "You succumb to the lure of ice.", OnOther: " succumbs to the lure of ice.", Recast: 2.25},
					{ID: 2554, Name: "Elnerick's Entombment of Ice", OnYou: "You are entombed in elemental ice.", OnOther: " is entombed by elemental ice.", Recast: 15},
					{ID: 2884, Name: "Garrison's Superior Sundering", OnYou: "Your body is torn asunder by a blast of intense heat.", OnOther: "'s body is torn asunder by a blast of intense heat.", Recast: 20},
					{ID: 3191, Name: "Shock of Magic", OnYou: "You are blasted by pure magic.", OnOther: " is blasted by pure magic.", Recast: 20},
					{ID: 3194, Name: "Greater Fetter", OnYou: "Your feet adhere to the ground.", Recast: 2.25},
					{ID: 3301, Name: "Force Shield", Buff: true},
				},
			},
			{
				Name: "Punisher Veshtaq", Zone: "Plane of Justice", Level: 65, MR: 80, CR: 75, FR: 75,
				Traits: "AE Rampage, Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{
					{ID: 2101, Name: "Pain and Suffering", OnYou: "You are mauled by Pain and Suffering.", OnOther: " is mauled by Pain and Suffering.", Recast: 8},
					{ID: 6, Name: "Ignite Blood", OnYou: "Your blood ignites.", Recast: 2.25},
					{ID: 346, Name: "Grim Aura", Buff: true},
					{ID: 364, Name: "Banshee Aura", Buff: true},
					{ID: 394, Name: "Diamondskin", Buff: true},
					{ID: 1508, Name: "Asystole", OnYou: "Your heart stops.", Recast: 2.25, Counter: "Disease"},
					{ID: 3400, Name: "Festering Darkness", OnYou: "You are engulfed by a festering darkness.", OnOther: " is engulfed by a festering darkness.", Recast: 4},
					{ID: 3403, Name: "Aura of Pain", OnYou: "Pain stabs at your soul.", OnOther: " is overcome with pain.", Recast: 72},
					{ID: 3413, Name: "Touch of Innoruuk", OnYou: "A piece of your spirit is torn from your body.", Recast: 10},
				},
			},
			{
				Name: "The Seventh Hammer", Zone: "Plane of Justice", Level: 73, MR: 265, CR: 190, FR: 190,
				Traits: "AE Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 1109, Name: "Justice", OnYou: "You have been judged.", OnOther: " has been judged.", Proc: true},
				},
			},
			{
				Name: "Yurae Zhaleem", Zone: "Plane of Justice", Level: 65, MR: 80, CR: 80, FR: 80,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 2823, Name: "Gravel Rain", OnYou: "A cloud of gravel pelts you and covers the ground.", OnOther: " is pelted by a cloud of gravel.", Recast: 10, Counter: "Curse"},
					{ID: 6, Name: "Ignite Blood", OnYou: "Your blood ignites.", Recast: 2.25},
					{ID: 346, Name: "Grim Aura", Buff: true},
					{ID: 364, Name: "Banshee Aura", Buff: true},
					{ID: 394, Name: "Diamondskin", Buff: true},
					{ID: 1508, Name: "Asystole", OnYou: "Your heart stops.", Recast: 2.25, Counter: "Disease"},
					{ID: 3400, Name: "Festering Darkness", OnYou: "You are engulfed by a festering darkness.", OnOther: " is engulfed by a festering darkness.", Recast: 4},
					{ID: 3403, Name: "Aura of Pain", OnYou: "Pain stabs at your soul.", OnOther: " is overcome with pain.", Recast: 72},
					{ID: 3413, Name: "Touch of Innoruuk", OnYou: "A piece of your spirit is torn from your body.", Recast: 10},
				},
			},
			{
				Name: "Mujaki the Devourer", Zone: "Plane of Nightmare", Level: 70, MR: 65, CR: 65, FR: 65,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 3001, Name: "Instill Nightmare", OnYou: "You run in terror from the horrible imagery before you", Recast: 24},
					{ID: 3000, Name: "Devouring Nightmare", OnYou: "You feel a terrifying grasp on your mind.", Proc: true},
				},
			},
			{
				Name: "Manaetic Behemoth", Zone: "Plane of Innovation", Level: 70, MR: 250, CR: 200, FR: 180,
				Traits: "Unslowable",
				Note:   "CH 3sec",
				Spells: []raidSpell{
					{ID: 1077, Name: "Mana Beam", OnYou: "You are pierced by a beam of pure mana!", OnOther: " is pierced by a beam of light!", Recast: 30},
					{ID: 1078, Name: "Gyrosonic Disruption", OnYou: "You are knocked into the air!", Recast: 60},
					{ID: 1079, Name: "Wave of White Noise", OnYou: "Your ears ring painfully", OnOther: " is unable to speak.", Recast: 45},
				},
			},
			{
				Name: "Xanamech Nezmirthafen", Zone: "Plane of Innovation", Level: 66, MR: 250, CR: 150, FR: 185,
				Note: "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 1074, Name: "Steam Blast", OnYou: "You are enveloped in blistering steam!", OnOther: " is enveloped in blistering steam!", Recast: 60},
					{ID: 1075, Name: "Electrical Short", OnYou: "You are filled with an electrical current!", OnOther: " convulses in pain.", Recast: 45},
					{ID: 1076, Name: "Oil Spray", OnYou: "You are covered in oil.", OnOther: " is covered in oil.", Recast: 30},
				},
			},
			{
				Name: "Terris Thule", Zone: "The Lair of Terris Thule", Level: 70, MR: 250, CR: 250, FR: 250,
				Traits: "Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1618, Name: "Touch of Night", OnYou: "Your life force drains away at the Touch of Night.", Proc: true},
				},
			},
		},
		Events: popTier1Events(),
	}
}
func popTier1Events() []Trigger {
	// The Seventh Hammer: the invisible Tribunal casts Tremor of Judgment
	// 86–95s and Verdict of Eternity 181–190s after aggro, then each every
	// 187s (pojustice/The_Seventh_Hammer.lua). The aggro say starts both
	// countdowns at their earliest; each land line restarts its own.
	hammerAggro := `^The Seventh Hammer says, '(?:Prepare for judgement mortals!|Very well mortals, Now begins the test of your very souls!)'$`
	tremor := raidEventTimer("Seventh Hammer — Tremor of Judgment",
		`^(?:The tremor of judgment rumbles around you\.|You resist the Tremor of Judgment spell!|[A-Za-z`+"`"+`' ]+ shudders as the ground shakes beneath them\.)$`,
		187, ExtraPattern{Pattern: hammerAggro, Enabled: true, TimerDurationSecs: 86})
	tremor.SpellID = 1107
	tremor.RefireCooldownSecs = 3
	verdict := raidEventTimer("Seventh Hammer — Verdict of Eternity",
		`^(?:You feel your soul being judged\.|You resist the Verdict of Eternity spell!|[A-Za-z`+"`"+`' ]+ staggers under the judgment of the Tribunal\.)$`,
		187, ExtraPattern{Pattern: hammerAggro, Enabled: true, TimerDurationSecs: 181})
	verdict.SpellID = 1108
	verdict.RefireCooldownSecs = 3
	return []Trigger{
		// Plane of Justice trials (pojustice/The_Tribunal.lua and
		// encounters/*Trial.lua). A failed trial reopens after 60s, a
		// completed one after 10 minutes.
		raidAlert("Trial started", `^The Tribunal says, 'Then begin\.'$`, "Trial started", "Trial started"),
		raidAlert("Trial already running", lit("That trial is already underway."), "Trial already running — wait", ""),
		raidAlert("Trial still running", lit("The trial is yet underway."), "Can't leave — trial still running", ""),
		func() Trigger {
			t := raidEventTimer("Trial reopens (failed)",
				anyOf("You have failed.", "Flames burst forth from the stake!"), 60)
			t.Actions = []Action{
				{Type: ActionOverlayText, Text: "TRIAL FAILED", DurationSecs: 6, Color: "#ef4444"},
				{Type: ActionTextToSpeech, Text: "Trial failed", Volume: 1.0},
			}
			return t
		}(),
		func() Trigger {
			t := raidEventTimer("Trial reopens (completed)",
				lit("The Tribunal tells you, 'You have completed a trial"), 600)
			t.Actions = []Action{
				{Type: ActionOverlayText, Text: "Trial complete — tell Mavuin the Tribunal will hear his plea", DurationSecs: 8, Color: "#22c55e"},
			}
			return t
		}(),
		raidAlert("Execution — executioner walking", lit("intones, 'Do you have any final words before your sentence is carried out?'"),
			"Executioner walking — kill the wave", "Kill the wave"),
		raidAlert("Hanging — prisoner choking", lit("The prisoner begins to choke as an invisible noose tightens around"),
			"Prisoner choking — kill the spirits of suffocation", "Kill spirits"),
		tremor,
		verdict,
		// Plane of Nightmare hedge maze (ponightmare/encounters/Maze.lua).
		raidAlert("Maze — Thelin walking", lit("Please stay close, I know not what horror Terris will unleash upon us."),
			"Thelin walking — keep him alive", "Thelin walking"),
		raidAlert("Maze — dagger piece found", `^Thelin Poxbourne (?:kneels down and picks up a dagger hilt|slowly picks up a shard of a dagger blade|picks up a the top half of a dagger blade|picks up a dagger handle with a small hole in it|picks up a small gem to place inside the handle|kneels down and picks up a dagger blade tip)\.$`,
			"Thelin found a dagger piece — adds incoming", "Adds"),
		raidAlert("Maze — final shard", lit("Thelin Poxbourne takes the final shard from you"),
			"Dagger complete — hail Thelin to leave", ""),
		raidAlert("Maze — Thelin asleep", lit("closes his eyes and falls asleep immediately."),
			"Thelin is dreaming — keep him alive", "Keep Thelin alive"),
		// Mujaki (ponightmare/encounters/Mujaki.lua).
		raidAlert("Mujaki — Etumer ready", lit("Perhaps you could find some way to overwhelm Mujaki?"),
			"Form ranks (24) — group leaders tell Etumer ready", "Form ranks"),
		raidAlert("Mujaki — Etumer can't port", lit("I don't think I can muster the force to work this teleportation."),
			"Etumer can't port yet — come back later", ""),
		// Plane of Innovation (poinnovation/Nitram_Anizok.lua).
		raidAlert("Nitram — walking to Xanamech", lit("This is wonderful, please follow me!"),
			"Nitram walking — keep him alive", "Keep Nitram alive"),
		raidAlert("Xanamech — awake", lit("places the power unit right into the chest of the huge metallic beast."),
			"Xanamech is awake", "Xanamech up"),
		// Terris Thule (nightmareb/Terris_Thule.lua).
		raidAlert("Terris — engaged", lit("The sound of a thousand terrified screams fills your head."),
			"Terris engaged — defilers at 79% and 69%", ""),
		raidAlert("Terris — 50% Direption of Dreams", lit("As if in a waking nightmare, you feel your movements slow"),
			"Terris 50% — Direption of Dreams (dispel)", "Direption"),
		raidAlert("Terris — 40% gargoyles", lit("The statues begin to crumble, as they shift their attention from the heavens to you!"),
			"Terris 40% — gargoyles awaken", "Gargoyles"),
		raidAlert("Terris — leashed", lit("Terris Thule disappears into the ether and reforms at the center of her chamber"),
			"Terris leashed — reset to full health", "Terris reset"),
	}
}

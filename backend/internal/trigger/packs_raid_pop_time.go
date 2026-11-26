package trigger

// raidPoPTime is the Plane of Time: the phase 1 elemental and undead
// nameds, the gods of phases 2–5 and Quarm. Event lines are from Project
// Quarm's potimeb/encounters/Phase2.lua and Phase6.lua; the instance clock
// lives in Raid Event Timers.
func raidPoPTime() raidPackDef {
	return raidPackDef{
		Name: "Raid: Plane of Time",
		Description: "Plane of Time: phase 1 nameds, the gods and Quarm. Boss " +
			"spell recast timers that bind to the god casting them (from the " +
			"server's NPC recasts, not the spell data), cure alerts for " +
			"curable debuffs on you including Quarm's scripted spells, and " +
			"phase 2 and Quarm head callouts (install Raid Event Timers for " +
			"the instance clock). /tell <boss> x (e.g. /tell cazic_thule x) " +
			"shows the boss's level, resists, abilities and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Anar of Water", Zone: "Plane of Time", Level: 68, MR: 170, CR: 550, FR: 150,
				Traits: "Rampage, AE Rampage, Flurry, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3744, Name: "Aqueous flux", OnYou: "Raging water surrounds your body.", OnOther: " is surrounded by raging water.", Recast: 300},
				},
			},
			{
				Name: "Avatar of the Elements", Zone: "Plane of Time", Level: 72, MR: 100, CR: 100, FR: 100,
				Note: "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 2390, Name: "Crumbling Earth Breath", OnYou: "You mind goes numb.", OnOther: " staggers around shivering.", Proc: true},
				},
			},
			{
				Name: "Bertoxxulous", Zone: "Plane of Time", Level: 75, MR: 450, CR: 375, FR: 375,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3763, Name: "Black Plague", OnYou: "A black plague assaults your body.", OnOther: "'s body is assaulted by a black plague.", Recast: 58, Counter: "Disease"},
					{ID: 3764, Name: "Rain of Bile", OnYou: "Bile falls from the sky around you.", OnOther: " has been caught in a storm of bile.", Recast: 60},
				},
			},
			{
				Name: "Cazic Thule", Zone: "Plane of Time", Level: 75, MR: 250, CR: 170, FR: 165,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3759, Name: "Call of the Faceless", OnYou: "Fear freezes your muscles.", OnOther: " has been frozen in fear.", Recast: 88},
					{ID: 3760, Name: "Aura of Fear", OnYou: "An aura of fear envelopes you.", OnOther: " has been enveloped in an aura of fear.", Recast: 58},
					{ID: 3070, Name: "Timeless Panic", OnYou: "You flee in complete panic.", Proc: true},
				},
			},
			{
				Name: "Earthen Overseer", Zone: "Plane of Time", Level: 67, MR: 125, CR: 110, FR: 120,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Gutripping War Beast", Zone: "Plane of Time", Level: 67, MR: 125, CR: 110, FR: 120,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Innoruuk", Zone: "Plane of Time", Level: 75, MR: 250, CR: 200, FR: 200,
				Traits: "Rampage, Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 3765, Name: "Seething Hatred", OnYou: "You are consumed by hatred.", OnOther: " has been consumed by hatred.", Recast: 59},
					{ID: 3766, Name: "Barrier of Hatred", Buff: true},
				},
			},
			{
				Name: "Kazrok of Fire", Zone: "Plane of Time", Level: 68, MR: 170, CR: 150, FR: 600,
				Traits: "Rampage, AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3741, Name: "Pyrokinetic Aura", Buff: true},
					{ID: 3742, Name: "Black Pyre", OnYou: "Your body begins to blister.", OnOther: "'s body begins to blister.", Recast: 180},
				},
			},
			{
				Name: "Neimon of Air", Zone: "Plane of Time", Level: 68, MR: 170, CR: 150, FR: 150,
				Traits: "Rampage, AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3745, Name: "Caustic Aura", Buff: true},
					{ID: 3746, Name: "Caustic Atmosphere", OnYou: "Your mind and body begin to slow.", OnOther: "'s mind and body slow.", Recast: 180},
				},
			},
			{
				Name: "Quarm", Zone: "Plane of Time", Level: 80, MR: 300, CR: 300, FR: 400,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3936, Name: "Dark Aura", OnYou: "A dark aura surrounds you.", OnOther: " is surrounded in a dark aura.", Proc: true, Counter: "Curse"},
					{ID: 3767, Name: "Epoch Conviction", OnYou: "Time and reality begin to tear around you.", OnOther: " is wrapped in the curse of inevitability.", Scripted: true, Counter: "Curse"},
					{ID: 3771, Name: "Plagued Earth", OnYou: "Putrid earth engulfs your body.", OnOther: "'s body is engulfed in putrid earth.", Scripted: true, Counter: "Disease"},
					{ID: 3772, Name: "Earthen Collaboration II", OnYou: "The poison soaked earth rises up to engulf you.", Scripted: true, Counter: "Poison"},
				},
			},
			{
				Name: "Rallos Zek", Zone: "Plane of Time", Level: 75, MR: 250, CR: 170, FR: 170,
				Traits: "Rampage, Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 2444, Name: "Rage of Zek", OnYou: "The chaos of war consumes your soul.", OnOther: "'s eyes go wide with horror.", Recast: 35, Counter: "Curse"},
					{ID: 3761, Name: "Blind Rage", Buff: true},
					{ID: 3762, Name: "Vindictive Strike", OnYou: "You muscles spasm and refuse to obey you.", OnOther: " falls to the ground and convulses.", Recast: 19},
				},
			},
			{
				Name: "Ralthos Enrok", Zone: "Plane of Time", Level: 69, MR: 165, CR: 150, FR: 130,
				Traits: "Flurry, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3749, Name: "Unholy Aura", OnYou: "An aura of death overcomes your mind and body.", OnOther: " has been surrounded by an aura of death.", Recast: 300},
					{ID: 6, Name: "Ignite Blood", OnYou: "Your blood ignites.", Recast: 2.25},
					{ID: 1508, Name: "Asystole", OnYou: "Your heart stops.", Recast: 2.25, Counter: "Disease"},
					{ID: 3400, Name: "Festering Darkness", OnYou: "You are engulfed by a festering darkness.", OnOther: " is engulfed by a festering darkness.", Recast: 4},
					{ID: 3403, Name: "Aura of Pain", OnYou: "Pain stabs at your soul.", OnOther: " is overcome with pain.", Recast: 72},
					{ID: 3413, Name: "Touch of Innoruuk", OnYou: "A piece of your spirit is torn from your body.", Recast: 10},
				},
			},
			{
				Name: "Rythor of the Undead", Zone: "Plane of Time", Level: 68, MR: 170, CR: 150, FR: 150,
				Traits: "Rampage, AE Rampage, Flurry",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3747, Name: "Insidious Calamity", OnYou: "You feel a cold hand touch your heart.", OnOther: " becomes very cold.", Recast: 180},
					{ID: 3748, Name: "Insipid Decay", OnYou: "Your body begins to decay.", OnOther: "'s body begins to decay.", Recast: 300},
				},
			},
			{
				Name: "Saryrn", Zone: "Plane of Time", Level: 71, MR: 235, CR: 180, FR: 170,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3753, Name: "Torrent of Agony", OnYou: "Agony assaults your soul.", OnOther: " trembles in agony.", Recast: 24, Counter: "Disease"},
					{ID: 3754, Name: "Horrifying affliction", OnYou: "Your mind pulses in pain.", OnOther: "staggers to the ground.", Recast: 30},
					{ID: 3072, Name: "Torrential Torment", OnYou: "Your skin begins to bubble and seethe.", OnOther: "'a skin begins to bubble and seethe.", Proc: true, Counter: "Curse"},
				},
			},
			{
				Name: "Supernatural Guardian", Zone: "Plane of Time", Level: 72, MR: 100, CR: 100, FR: 100,
				Traits: "Flurry, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{},
			},
			{
				Name: "Tallon Zek", Zone: "Plane of Time", Level: 71, MR: 200, CR: 175, FR: 200,
				Traits: "Rampage, Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 2448, Name: "Tallon's Balance", OnYou: "Chromatic runes surround you.", OnOther: " is surrounded by the runes of Zek.", Recast: 60},
					{ID: 2449, Name: "Barb of Tallon", OnYou: "The blood freezes in your veins.", OnOther: " has been struck by a huge frozen arrow.", Recast: 4},
					{ID: 2450, Name: "Barb of Tallon", OnYou: "Your veins incinerate.", OnOther: " screams as the arrow ignites their blood.", Recast: 4.5},
					{ID: 2451, Name: "Barb of Tallon", OnYou: "Your stomach lurches and refuses to obey you.", OnOther: " struggles to stay on their feet as the disease infests them.", Recast: 5},
					{ID: 2452, Name: "Barb of Tallon", OnYou: "The barb of Tallon sears your flesh.", OnOther: " has been poisoned by Tallon's barb.", Recast: 6},
					{ID: 3757, Name: "Strategic Blow", OnYou: "Your muscles twitch and tremble.", OnOther: " falls to the ground and convulses.", Recast: 420},
					{ID: 3758, Name: "Planeshift", OnYou: "Reality shifts around you.", OnOther: " has been caught in a reality sphere.", Recast: 420},
				},
			},
			{
				Name: "Terlok of Earth", Zone: "Plane of Time", Level: 68, MR: 150, CR: 150, FR: 140,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Terris Thule", Zone: "Plane of Time", Level: 71, MR: 235, CR: 170, FR: 175,
				Traits: "Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 3751, Name: "Quivering Nightmares", OnYou: "Nightmares tear at your thoughts.", OnOther: "'s mind  is torn my nightmares.", Recast: 24},
					{ID: 3752, Name: "Phantasmal Torment", OnYou: "Phantasms burn your mind.", OnOther: "'s mind is burned by phantasms.", Recast: 58},
					{ID: 3071, Name: "Insipid Dreams", OnYou: "Your mind is wracked by terrible nightmares.", OnOther: " screams as they are consumed by nightmares.", Proc: true},
				},
			},
			{
				Name: "Vallon Zek", Zone: "Plane of Time", Level: 71, MR: 200, CR: 180, FR: 185,
				Traits: "Rampage, Unslowable",
				Note:   "CH 1sec",
				Spells: []raidSpell{
					{ID: 2446, Name: "Vallon's Precision", OnYou: "You have been marked.", OnOther: "'s body glows with dark red runes.", Recast: 35},
					{ID: 3755, Name: "Tactical Strike", OnYou: "You are stuck by the force of the Zeks.", OnOther: " has been struck by the force of the Zeks.", Recast: 24},
				},
			},
			{
				Name: "War Shapen Emissary", Zone: "Plane of Time", Level: 67, MR: 125, CR: 110, FR: 110,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
			{
				Name: "Windshapen Warlord of Air", Zone: "Plane of Time", Level: 67, MR: 125, CR: 120, FR: 75,
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{},
			},
		},
		Events: raidPoPTimeEvents(),
	}
}

func raidPoPTimeEvents() []Trigger {
	return []Trigger{
		raidAlert("Time — phase 2 wall opening", lit("Ethereal mists gather at the far wall, causing it fade in and out of focus."),
			"Phase 2 wall opening", "Wall opening"),

		// Quarm (potimeb/encounters/Phase6.lua): each head's death is
		// followed by Balance of the Nameless, which strips his debuffs.
		raidAlert("Quarm — red head down", lit("Quarm roars in pain as his red head explodes in a shower of flaming ethereal energy!"),
			"Red head down — re-slow Quarm", "Red head down"),
		raidAlert("Quarm — blue head down", lit("Quarm roars in pain as his blue head explodes into pure ethereal energy!"),
			"Blue head down — re-slow Quarm", "Blue head down"),
		raidAlert("Quarm — white head down", lit("Quarm shakes the cavern with a bellow of pain as the white head explodes into a swirling vortex of ethereal energy!"),
			"White head down — re-slow Quarm", "White head down"),
		raidAlert("Quarm — Druzzil appears", lit("Suddenly in a bright flash a feminine figure appears before you."),
			"Druzzil Ro — the event is ending", "Druzzil"),
	}
}

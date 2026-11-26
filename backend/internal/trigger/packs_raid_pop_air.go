package trigger

// raidPoPAir is the Plane of Air: the island nameds, the four avatars and
// Xegony. Island spawn shouts are from Project Quarm's
// poair/encounters/{Dust,Mist,Smoke,Wind}.lua; their 2.5-hour despawn
// timers live in Raid Event Timers.
func raidPoPAir() raidPackDef {
	return raidPackDef{
		Name: "Raid: Plane of Air",
		Description: "Plane of Air: island nameds, the four avatars and " +
			"Xegony. Boss spell recast timers that bind to the boss casting " +
			"them, cure alerts for curable debuffs on you, and island event " +
			"callouts (install Raid Event Timers for the despawn countdowns). " +
			"/tell <boss> x (e.g. /tell xegony_the_queen_of_air x) shows the " +
			"boss's level, resists, abilities and spells. " + raidCredit,
		Bosses: []raidBoss{
			{
				Name: "Arch Mage Alchtonion", Zone: "Plane of Air", Level: 65, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 2263, Name: "Dominion of Force", OnYou: "Scintillating flames stun your mind", OnOther: " is dazzled by flickering flames", Proc: true},
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
				Name: "Avatar of Dust", Zone: "Plane of Air", Level: 75, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1079, Name: "Wave of White Noise", OnYou: "Your ears ring painfully", OnOther: " is unable to speak.", Recast: 45},
					{ID: 1045, Name: "Chain Lightning", OnYou: "A bolt of lightning arcs through your body.", OnOther: " screams as a bolt of lightning arcs over them.", Proc: true},
				},
			},
			{
				Name: "Avatar of Mist", Zone: "Plane of Air", Level: 75, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 1031, Name: "Stone Gale", OnYou: "Huge stones fly through the air toward your head.", OnOther: " is caught in a hail of stones.", Recast: 1},
					{ID: 1033, Name: "Tornado", OnYou: "You are caught in a tornado.", OnOther: " is trapped in a tornado.", Recast: 18},
					{ID: 1034, Name: "Cyclone", OnYou: "You are caught in a raging cyclone.", OnOther: " is caught in a raging cyclone.", Proc: true},
				},
			},
			{
				Name: "Avatar of Smoke", Zone: "Plane of Air", Level: 75, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1033, Name: "Tornado", OnYou: "You are caught in a tornado.", OnOther: " is trapped in a tornado.", Recast: 18},
					{ID: 1034, Name: "Cyclone", OnYou: "You are caught in a raging cyclone.", OnOther: " is caught in a raging cyclone.", Recast: 18},
					{ID: 1032, Name: "Hail Storm", OnYou: "You are pummeled by cascading hail stones.", OnOther: " is caught in a hail storm.", Proc: true},
				},
			},
			{
				Name: "Avatar of Wind", Zone: "Plane of Air", Level: 75, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1034, Name: "Cyclone", OnYou: "You are caught in a raging cyclone.", OnOther: " is caught in a raging cyclone.", Recast: 18},
					{ID: 1035, Name: "Lure of the Storm", OnYou: "An inescapable wind throws you into the air.", OnOther: " is thrown high into the air.", Recast: 36},
					{ID: 1033, Name: "Tornado", OnYou: "You are caught in a tornado.", OnOther: " is trapped in a tornado.", Proc: true},
				},
			},
			{
				Name: "Baltaldor the Cursed", Zone: "Plane of Air", Level: 67, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 4sec, 6sec slowed",
				Spells: []raidSpell{
					{ID: 836, Name: "Diseased Cloud", OnYou: "Your body begins to rot.", OnOther: "'s body begins to rot.", Recast: 12, Counter: "Disease"},
					{ID: 1032, Name: "Hail Storm", OnYou: "You are pummeled by cascading hail stones.", OnOther: " is caught in a hail storm.", Proc: true},
				},
			},
			{
				Name: "Gakamenial Fir`Disralsi", Zone: "Plane of Air", Level: 67, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 861, Name: "Lava Breath", OnYou: "Your body combusts as the lava hits you.", OnOther: "'s body combusts as the lava hits them.", Recast: 12},
					{ID: 1043, Name: "Manastorm", OnYou: "Your mind is rent by the manastorm.", OnOther: "'s mind staggers from the manastorm.", Proc: true},
				},
			},
			{
				Name: "Melernil Faal`Armanna", Zone: "Plane of Air", Level: 73, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1079, Name: "Wave of White Noise", OnYou: "Your ears ring painfully", OnOther: " is unable to speak.", Recast: 45},
					{ID: 988, Name: "Greater Spiroc Thunder", OnYou: "Lightning surges through your body.", Proc: true},
				},
			},
			{
				Name: "Muzlakh the Chosen", Zone: "Plane of Air", Level: 73, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1308, Name: "Avatar Snare", OnYou: "Your knees buckle.", OnOther: " 's knees buckle."},
					{ID: 692, Name: "Life Leech", OnYou: "You feel your life force drain away.", Proc: true},
				},
			},
			{
				Name: "Queen Silandria", Zone: "Plane of Air", Level: 68, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage, Unslowable",
				Note:   "CH 2sec",
				Spells: []raidSpell{
					{ID: 2096, Name: "Crushing Fist of the Wind", OnYou: "You are trapped within a whirling wind.", OnOther: " is trapped within a whirling wind.", Proc: true},
				},
			},
			{
				Name: "Rinturion Windblade", Zone: "Plane of Air", Level: 70, MR: 100, CR: 90, FR: 90,
				Traits: "Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 3153, Name: "Blade Frenzy", OnYou: "You are engulfed with an unstoppable whirlwind of blades.", Proc: true},
				},
			},
			{
				Name: "Sigismond Windwalker", Zone: "Plane of Air", Level: 74, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 2sec, 3sec slowed",
				Spells: []raidSpell{
					{ID: 1079, Name: "Wave of White Noise", OnYou: "Your ears ring painfully", OnOther: " is unable to speak.", Recast: 45},
					{ID: 3154, Name: "Air Strike", OnYou: "You are struck by the spirits of air.", OnOther: " is struck by the spirits of air.", Proc: true},
				},
			},
			{
				Name: "Xegony the Queen of Air", Zone: "Plane of Air", Level: 80, MR: 100, CR: 90, FR: 90,
				Traits: "AE Rampage",
				Note:   "CH 1sec, 2sec slowed",
				Spells: []raidSpell{
					{ID: 3075, Name: "Wind of Xegony", OnYou: "Spirits of the wind dance around your body.", OnOther: " is surrounded by spirits of the air.", Recast: 30, Counter: "Curse"},
				},
			},
		},
		Events: raidPoPAirEvents(),
	}
}

func raidPoPAirEvents() []Trigger {
	return []Trigger{
		raidAlert("Air — Dust island named up", lit("Elementals of Air and Dust, I call upon you to aid me in the defense of this land!"),
			"Dust island named up — adds incoming", "Adds"),
		raidAlert("Air — Mist island named up", lit("Leave now foolish mortal you have begun to send events in motion that you cannot possibly understand!"),
			"Mist island named up", "Named up"),
		raidAlert("Air — Smoke island named up", lit("The time has come for all intruders to be tested I will not allow you to pillage this plane any longer!"),
			"Smoke island named up", "Named up"),
		raidAlert("Air — Wind island named up", lit("I will certainly make sure that no mortal is allowed to defile this sacred realm!"),
			"Wind island named up", "Named up"),
	}
}

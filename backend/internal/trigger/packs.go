package trigger

import "time"

// EnchanterPack returns the pre-built enchanter trigger pack: critical
// crowd-control break alerts (mez/charm/root), casting-failure alerts
// (resist, immunities, interrupt), and timer-creating triggers for the
// standard enchanter buff/debuff/mez lines.
//
// The timer triggers run alongside the spell-landed pipeline in
// internal/spelltimer; the engine's same-name dedup window (3s) keeps
// the two from creating duplicate entries when both fire for the same
// cast. Triggers carry SpellID so the engine can apply item/AA duration
// focuses just like the spell-landed path.
func EnchanterPack() TriggerPack {
	return TriggerPack{
		PackName:    "Enchanter",
		Description: "CC break + cast-failure alerts plus spell timers for the enchanter buff (VoG, KEI, IS, GRM, Speed of the Shissar/Brood), debuff (Tashanian, Cripple, Asphyxiate), root (Root, Fetter, Greater Fetter), mez (Mesmerize, Mesmerization, Dazzle, Enthrall, Entrance, Glamour of Kintaz, Rapture / Ancient: Eternal Rapture), charm (Charm, Beguile, Cajoling Whispers, Allure, Dictate, Boltran's Agacerie), and pacify (Lull, Calm, Soothe, Pacify, Wake of Tranquility) lines.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			{
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  `Your (?:Mesmerize|Mesmerization|Enthrall|Entrance|Dazzle|Wake of Tranquility|Glamour of Kintaz|Instill|Rapture|Ancient: Eternal Rapture) spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5, Color: "#ff4444"},
				},
			},
			{
				Name:     "Charm Broke",
				Enabled:  true,
				Pattern:  `Your (?:Charm|Beguile|Beguile Animals|Beguile Plants|Cajoling Whispers|Allure|Dictate|Boltran's Agacerie) spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CHARM BROKE!", DurationSecs: 6, Color: "#ff0000"},
				},
			},
			{
				Name:     "Root Broke",
				Enabled:  true,
				Pattern:  `Your (?:Root|Engulfing Roots|Engulfing Darkness|Fetter|Greater Fetter) spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
				},
			},

			// ── Resists, immunities, and interrupts ──────────────────────
			{
				Name:     "Spell Resisted",
				Enabled:  true,
				Pattern:  `Your target resisted the .+ spell\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "RESIST!", DurationSecs: 4, Color: "#ff8800"},
				},
			},
			{
				Name:     "Cannot Be Mezzed",
				Enabled:  true,
				Pattern:  `Your target cannot be mesmerized\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CANNOT MEZ", DurationSecs: 4, Color: "#ff8800"},
				},
			},
			{
				Name:     "Cannot Be Charmed",
				Enabled:  true,
				Pattern:  `Your target cannot be charmed\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CANNOT CHARM", DurationSecs: 4, Color: "#ff8800"},
				},
			},
			{
				Name:     "Spell Interrupted",
				Enabled:  true,
				Pattern:  `Your spell is interrupted\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INTERRUPTED!", DurationSecs: 3, Color: "#ffcc00"},
				},
			},

			// ── Group buffs (timers) ─────────────────────────────────────
			{
				Name:              "Visions of Grandeur",
				Enabled:           true,
				Pattern:           `^(?:You experience visions of grandeur\.|[A-Z][a-zA-Z']{2,14} experiences visions of grandeur\.)$`,
				WornOffPattern:    `^Your visions fade\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2520,
				SpellID:           1710,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Koadic's Endless Intellect",
				Enabled:           true,
				Pattern:           `^(?:Your mind expands beyond the bounds of space and time\.|[A-Z][a-zA-Z']{2,14}'s mind expands beyond the bounds of space and time\.)$`,
				WornOffPattern:    `^Your mind returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           2570,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Intellectual Superiority",
				Enabled:           true,
				Pattern:           `^(?:Your mind sharpens\.|[A-Z][a-zA-Z']{2,14}'s mind sharpens\.)$`,
				WornOffPattern:    `^The intellectual advancement fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 1620,
				SpellID:           2562,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Group Resist Magic",
				Enabled:           true,
				Pattern:           `^(?:You feel protected from magic\.|[A-Z][a-zA-Z']{2,14} is resistant to magic\.)$`,
				WornOffPattern:    `^Your protection fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2160,
				SpellID:           72,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Speed of the Shissar/Brood",
				Enabled:           true,
				Pattern:           `^(?:Your body pulses with the spirit of the Shissar\.|[A-Z][a-zA-Z']{2,14}'s body pulses with the spirit of the Shissar\.)$`,
				WornOffPattern:    `^Your body slows\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 1800,
				SpellID:           1939,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Debuffs (timers) ─────────────────────────────────────────
			{
				Name:              "Tashanian",
				Enabled:           true,
				Pattern:           `^(?:You hear the barking of Tashania\.|[A-Z][a-zA-Z']{2,14} glances nervously about\.)$`,
				WornOffPattern:    `^The barking fades\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 720,
				SpellID:           1702,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Cripple",
				Enabled:           true,
				Pattern:           `^(?:You have been crippled\.|[A-Z][a-zA-Z']{2,14} has been crippled\.)$`,
				WornOffPattern:    `^You feel your strength return\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 810,
				SpellID:           1592,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Asphyxiate",
				Enabled:           true,
				Pattern:           `^(?:You feel a shortness of breath\.|[A-Z][a-zA-Z']{2,14} begins to choke\.)$`,
				WornOffPattern:    `^You can breathe again\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           1703,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Root (timers) ────────────────────────────────────────────
			// All three enchanter roots share "<name> adheres to the ground."
			// land text, so each is matched on "You begin casting <SpellName>."
			// to disambiguate. Resists clear via the spell-specific resist line.
			{
				Name:              "Root",
				Enabled:           true,
				Pattern:           `^You begin casting Root\.$`,
				WornOffPattern:    `^(?:Your Root spell has worn off\.|Your target resisted the Root spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 48,
				SpellID:           230,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Fetter",
				Enabled:           true,
				Pattern:           `^You begin casting Fetter\.$`,
				WornOffPattern:    `^(?:Your Fetter spell has worn off\.|Your target resisted the Fetter spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 180,
				SpellID:           1633,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Greater Fetter",
				Enabled:           true,
				Pattern:           `^You begin casting Greater Fetter\.$`,
				WornOffPattern:    `^(?:Your Greater Fetter spell has worn off\.|Your target resisted the Greater Fetter spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 180,
				SpellID:           3194,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Mez (timers) ─────────────────────────────────────────────
			// Mesmerize / Mesmerization / Dazzle share the "<name> has been
			// mesmerized." land text but have three different base durations
			// (24 / 24 / 96 s), so they're matched on "You begin casting
			// <SpellName>." instead — the cast-begin message uniquely names
			// each spell. The trade-off is that the timer starts ~2-3 s before
			// the spell actually lands; on a resist the WornOffPattern catches
			// it via the spell-specific resist line so the stale timer clears.
			{
				Name:              "Mesmerize",
				Enabled:           true,
				Pattern:           `^You begin casting Mesmerize\.$`,
				WornOffPattern:    `^(?:Your Mesmerize spell has worn off\.|Your target resisted the Mesmerize spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 24,
				SpellID:           292,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Mesmerization",
				Enabled:           true,
				Pattern:           `^You begin casting Mesmerization\.$`,
				WornOffPattern:    `^(?:Your Mesmerization spell has worn off\.|Your target resisted the Mesmerization spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 24,
				SpellID:           307,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Dazzle",
				Enabled:           true,
				Pattern:           `^You begin casting Dazzle\.$`,
				WornOffPattern:    `^(?:Your Dazzle spell has worn off\.|Your target resisted the Dazzle spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 96,
				SpellID:           190,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Enthrall",
				Enabled:           true,
				Pattern:           `^(?:You have been enthralled\.|[A-Z][a-zA-Z']{2,14} has been enthralled\.)$`,
				WornOffPattern:    `^Your Enthrall spell has worn off\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 144,
				SpellID:           187,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Entrance",
				Enabled:           true,
				Pattern:           `^(?:You have been entranced\.|[A-Z][a-zA-Z']{2,14} has been entranced\.)$`,
				WornOffPattern:    `^Your Entrance spell has worn off\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 72,
				SpellID:           188,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Glamour of Kintaz",
				Enabled:           true,
				Pattern:           `^(?:You are mesmerized by the Glamour of Kintaz\.|[A-Z][a-zA-Z']{2,14} has been mesmerized by the Glamour of Kintaz\.)$`,
				WornOffPattern:    `^Your Glamour of Kintaz spell has worn off\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           1691,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			// Rapture and Ancient: Eternal Rapture share both cast text and
			// duration (7 ticks / 42 s), so a single trigger covers both.
			// SpellID is set to the base Rapture; the AA version's duration
			// modifiers extend the same way.
			{
				Name:              "Rapture",
				Enabled:           true,
				Pattern:           `^(?:You swoon, overcome by rapture\.|[A-Z][a-zA-Z']{2,14} swoons in raptured bliss\.)$`,
				WornOffPattern:    `^Your (?:Rapture|Ancient: Eternal Rapture) spell has worn off\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 42,
				SpellID:           1692,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Charm (timers) ───────────────────────────────────────────
			// Every enchanter charm has an empty cast_on_other (the caster
			// never sees a "<name> has been charmed." line in their log) and
			// the charm line shares "You have been charmed." / "You are no
			// longer charmed." across spells with very different durations,
			// so each trigger matches on "You begin casting <SpellName>."
			// instead. Resists clear the stale timer via the spell-specific
			// resist line.
			{
				Name:              "Charm",
				Enabled:           true,
				Pattern:           `^You begin casting Charm\.$`,
				WornOffPattern:    `^(?:Your Charm spell has worn off\.|Your target resisted the Charm spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           300,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Beguile",
				Enabled:           true,
				Pattern:           `^You begin casting Beguile\.$`,
				WornOffPattern:    `^(?:Your Beguile spell has worn off\.|Your target resisted the Beguile spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           182,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Cajoling Whispers",
				Enabled:           true,
				Pattern:           `^You begin casting Cajoling Whispers\.$`,
				WornOffPattern:    `^(?:Your Cajoling Whispers spell has worn off\.|Your target resisted the Cajoling Whispers spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           183,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Allure",
				Enabled:           true,
				Pattern:           `^You begin casting Allure\.$`,
				WornOffPattern:    `^(?:Your Allure spell has worn off\.|Your target resisted the Allure spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           184,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Dictate",
				Enabled:           true,
				Pattern:           `^You begin casting Dictate\.$`,
				WornOffPattern:    `^(?:Your Dictate spell has worn off\.|Your target resisted the Dictate spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 48,
				SpellID:           1707,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Boltran's Agacerie",
				Enabled:           true,
				Pattern:           `^You begin casting Boltran's Agacerie\.$`,
				WornOffPattern:    `^(?:Your Boltran's Agacerie spell has worn off\.|Your target resisted the Boltran's Agacerie spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           1706,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Pacify line (timers) ─────────────────────────────────────
			// Lull / Calm / Soothe / Pacify / Wake of Tranquility share most
			// of their cast text and have empty spell_fades, so each trigger
			// matches on "You begin casting <SpellName>." to get the right
			// per-spell duration. WornOffPattern uses the caster's
			// "Your <SpellName> spell has worn off." line plus the
			// spell-specific resist line so the timer clears on resist.
			{
				Name:              "Lull",
				Enabled:           true,
				Pattern:           `^You begin casting Lull\.$`,
				WornOffPattern:    `^(?:Your Lull spell has worn off\.|Your target resisted the Lull spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           208,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Calm",
				Enabled:           true,
				Pattern:           `^You begin casting Calm\.$`,
				WornOffPattern:    `^(?:Your Calm spell has worn off\.|Your target resisted the Calm spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 126,
				SpellID:           47,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Soothe",
				Enabled:           true,
				Pattern:           `^You begin casting Soothe\.$`,
				WornOffPattern:    `^(?:Your Soothe spell has worn off\.|Your target resisted the Soothe spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 450,
				SpellID:           501,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Pacify",
				Enabled:           true,
				Pattern:           `^You begin casting Pacify\.$`,
				WornOffPattern:    `^(?:Your Pacify spell has worn off\.|Your target resisted the Pacify spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 720,
				SpellID:           45,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Wake of Tranquility",
				Enabled:           true,
				Pattern:           `^You begin casting Wake of Tranquility\.$`,
				WornOffPattern:    `^(?:Your Wake of Tranquility spell has worn off\.|Your target resisted the Wake of Tranquility spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 126,
				SpellID:           1541,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
		},
	}
}

// ClericPack returns the pre-built cleric trigger pack: timer-creating
// triggers for the standard cleric raid-buff (Aegolism, Blessing of Aegolism,
// Ancient: Gift of Aegolism, Naltron's Mark), emergency-buff (Divine
// Intervention), self-defensive (Divine Aura), and target-debuff (Mark of
// Karn) lines. Generic resist/interrupt overlays are intentionally omitted
// — they live in the Enchanter pack and would duplicate if both are
// installed.
func ClericPack() TriggerPack {
	return TriggerPack{
		PackName:    "Cleric",
		Description: "Spell timers for Aegolism / Blessing of Aegolism / Ancient: Gift of Aegolism, Naltron's Mark, Divine Intervention, Mark of Karn, and Divine Aura.",
		Triggers: []Trigger{
			// ── Raid buffs (timers) ──────────────────────────────────────
			// Aegolism, Blessing of Aegolism, and Ancient: Gift of Aegolism
			// share the same fade text ("Your aegolism fades.") and the
			// first two share identical cast_on_you/cast_on_other anchors,
			// so each timer is keyed on "You begin casting <SpellName>."
			// to disambiguate. The fade pattern matches the shared line so
			// any of the three clears the right timer when stacking is not
			// in play.
			{
				Name:              "Aegolism",
				Enabled:           true,
				Pattern:           `^You begin casting Aegolism\.$`,
				WornOffPattern:    `^Your aegolism fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           1447,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Blessing of Aegolism",
				Enabled:           true,
				Pattern:           `^You begin casting Blessing of Aegolism\.$`,
				WornOffPattern:    `^Your aegolism fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           2510,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Ancient: Gift of Aegolism",
				Enabled:           true,
				Pattern:           `^You begin casting Ancient: Gift of Aegolism\.$`,
				WornOffPattern:    `^Your aegolism fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           2122,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Naltron's Mark",
				Enabled:           true,
				Pattern:           `^(?:A mystic symbol flashes before your eyes\.|[A-Z][a-zA-Z']{2,14} is cloaked in a shimmer of glowing symbols\.)$`,
				WornOffPattern:    `^The mystic symbol fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3240,
				SpellID:           1774,
				PackName:          "Cleric",
				Actions:           []Action{},
			},

			// ── Emergency self-buff (timer) ──────────────────────────────
			{
				Name:              "Divine Intervention",
				Enabled:           true,
				Pattern:           `^(?:You feel the watchful eyes of the gods upon you\.|[A-Z][a-zA-Z']{2,14} feels the watchful eyes of the gods upon them\.)$`,
				WornOffPattern:    `^You are no longer watched\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           1546,
				PackName:          "Cleric",
				Actions:           []Action{},
			},

			// ── Self-defensive (timer) ───────────────────────────────────
			// Divine Aura's empty cast_on_other means only the caster sees
			// the land message; matching cast_on_you is sufficient.
			{
				Name:              "Divine Aura",
				Enabled:           true,
				Pattern:           `^The gods have rendered you invulnerable\.$`,
				WornOffPattern:    `^Your invulnerability fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 18,
				SpellID:           207,
				PackName:          "Cleric",
				Actions:           []Action{},
			},

			// ── Target debuff (timer) ────────────────────────────────────
			{
				Name:              "Mark of Karn",
				Enabled:           true,
				Pattern:           `^(?:Your skin gleams with a pure aura\.|[A-Z][a-zA-Z']{2,14}'s skin gleams with a pure aura\.)$`,
				WornOffPattern:    `^The aura fades\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 570,
				SpellID:           1548,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
		},
	}
}

// DruidPack returns the pre-built druid trigger pack: timer-creating
// triggers for the standard druid self/group buff (Protection of the
// Glades, Blessing of Replenishment, Legacy of Thorn), target debuff
// (Hand of Ro), DoT (Winged Death), and snare/root (Ensnare,
// Entrapping Roots) lines. Generic resist/interrupt overlays are
// intentionally omitted — they live in the Enchanter pack and would
// duplicate if both are installed.
func DruidPack() TriggerPack {
	return TriggerPack{
		PackName:    "Druid",
		Description: "Spell timers for Protection of the Glades, Blessing of Replenishment, Legacy of Thorn, Hand of Ro, Winged Death, Ensnare, and Entrapping Roots.",
		Triggers: []Trigger{
			// ── Self / group buffs (timers) ──────────────────────────────
			{
				Name:              "Protection of the Glades",
				Enabled:           true,
				Pattern:           `^(?:Your skin shimmers\.|[A-Z][a-zA-Z']{2,14}'s skin shimmers\.)$`,
				WornOffPattern:    `^Your skin returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 6000,
				SpellID:           1442,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Blessing of Replenishment",
				Enabled:           true,
				Pattern:           `^(?:You begin to regenerate\.|[A-Z][a-zA-Z']{2,14} begins to regenerate\.)$`,
				WornOffPattern:    `^You have stopped regenerating\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           3441,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Legacy of Thorn",
				Enabled:           true,
				Pattern:           `^(?:You are surrounded by a thorny barrier\.|[A-Z][a-zA-Z']{2,14} is surrounded by a thorny barrier\.)$`,
				WornOffPattern:    `^The brambles fall away\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 390,
				SpellID:           1561,
				PackName:          "Druid",
				Actions:           []Action{},
			},

			// ── Target debuff / DoT (timers) ─────────────────────────────
			{
				Name:              "Hand of Ro",
				Enabled:           true,
				Pattern:           `^(?:You are immolated by blazing flames\.|[A-Z][a-zA-Z']{2,14} is immolated by blazing flames\.)$`,
				WornOffPattern:    `^The flames die down\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 180,
				SpellID:           3435,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Winged Death",
				Enabled:           true,
				Pattern:           `^(?:You feel the pain of a million stings\.|[A-Z][a-zA-Z']{2,14} is engulfed by a swarm of deadly insects\.)$`,
				WornOffPattern:    `^The swarm departs\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           1601,
				PackName:          "Druid",
				Actions:           []Action{},
			},

			// ── Snare / root (timers) ────────────────────────────────────
			{
				Name:              "Ensnare",
				Enabled:           true,
				Pattern:           `^(?:You are ensnared\.|[A-Z][a-zA-Z']{2,14} has been ensnared\.)$`,
				WornOffPattern:    `^You are no longer ensnared\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 720,
				SpellID:           512,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Entrapping Roots",
				Enabled:           true,
				Pattern:           `^(?:Your feet become entwined\.|[A-Z][a-zA-Z']{2,14} is entrapped by roots\.)$`,
				WornOffPattern:    `^(?:The roots fall from your feet\.|Your Entrapping Roots spell has worn off\.|Your target resisted the Entrapping Roots spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 540,
				SpellID:           1608,
				PackName:          "Druid",
				Actions:           []Action{},
			},
		},
	}
}

// ShamanPack returns the pre-built shaman trigger pack: timer-creating
// triggers for the standard slow (Turgur's Insects, Plague of Insects),
// resist debuff (Malo, Malosini), self/tank buff (Torpor, Avatar, Focus
// of Spirit, Regrowth of Dar Khura) lines. Instant utilities like
// Cannibalize and Kragg's Mending are intentionally omitted (no timer
// to track), and generic resist/interrupt overlays remain in the
// Enchanter pack.
func ShamanPack() TriggerPack {
	return TriggerPack{
		PackName:    "Shaman",
		Description: "Spell timers for Turgur's Insects, Plague of Insects, Malo, Malosini, Torpor, Avatar, Focus of Spirit, and Regrowth of Dar Khura.",
		Triggers: []Trigger{
			// ── Slows (timers) ──────────────────────────────────────────
			{
				Name:              "Turgur's Insects",
				Enabled:           true,
				Pattern:           `^(?:You feel drowsy\.|[A-Z][a-zA-Z']{2,14} yawns\.)$`,
				WornOffPattern:    `^You feel less drowsy\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 390,
				SpellID:           1588,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			{
				Name:              "Plague of Insects",
				Enabled:           true,
				Pattern:           `^(?:You're motions slow as a plague of insects chew at your skin\.|[A-Z][a-zA-Z']{2,14}'s motions slow as a plague of insects chews at their skin\.)$`,
				WornOffPattern:    `^The plague of insects subsides\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 630,
				SpellID:           2527,
				PackName:          "Shaman",
				Actions:           []Action{},
			},

			// ── Resist debuffs (timers) ─────────────────────────────────
			// Malo and Malosini share identical anchors and fade text, so
			// each is keyed on "You begin casting <SpellName>." The
			// shared "Your vulnerability fades." fade clears whichever
			// is active.
			{
				Name:              "Malo",
				Enabled:           true,
				Pattern:           `^You begin casting Malo\.$`,
				WornOffPattern:    `^(?:Your vulnerability fades\.|Your target resisted the Malo spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 720,
				SpellID:           1578,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			{
				Name:              "Malosini",
				Enabled:           true,
				Pattern:           `^You begin casting Malosini\.$`,
				WornOffPattern:    `^(?:Your vulnerability fades\.|Your target resisted the Malosini spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           1577,
				PackName:          "Shaman",
				Actions:           []Action{},
			},

			// ── Tank-heal buff (timer) ──────────────────────────────────
			{
				Name:              "Torpor",
				Enabled:           true,
				Pattern:           `^(?:You fall into a state of torpor\.|[A-Z][a-zA-Z']{2,14} falls into a state of torpor\.)$`,
				WornOffPattern:    `^Your state of torpor ends\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 24,
				SpellID:           1576,
				PackName:          "Shaman",
				Actions:           []Action{},
			},

			// ── Melee buff (timer) ──────────────────────────────────────
			// Two Avatar rows in spells_new (1598 = AA/raid version, 2434
			// = epic 1.0 proc) share identical land/fade anchors and have
			// nearly identical durations (60 vs 65 ticks). A single
			// trigger covers both — the AA version's duration is used.
			{
				Name:              "Avatar",
				Enabled:           true,
				Pattern:           `^(?:Your body screams with the power of an Avatar\.|[A-Z][a-zA-Z']{2,14} has been infused with the power of an Avatar\.)$`,
				WornOffPattern:    `^The Avatar departs\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           1598,
				PackName:          "Shaman",
				Actions:           []Action{},
			},

			// ── Self / group buffs (timers) ─────────────────────────────
			{
				Name:              "Focus of Spirit",
				Enabled:           true,
				Pattern:           `^(?:You feel focused\.|[A-Z][a-zA-Z']{2,14} looks focused\.)$`,
				WornOffPattern:    `^Your focus fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 6000,
				SpellID:           1432,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			// Regrowth of Dar Khura shares "You begin to regenerate." /
			// "You have stopped regenerating." with the druid's Blessing
			// of Replenishment, so this trigger keys on "You begin
			// casting Regrowth of Dar Khura." to disambiguate.
			{
				Name:              "Regrowth of Dar Khura",
				Enabled:           true,
				Pattern:           `^You begin casting Regrowth of Dar Khura\.$`,
				WornOffPattern:    `^You have stopped regenerating\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           2528,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
		},
	}
}

// PaladinPack returns the pre-built paladin trigger pack: a Lay on
// Hands overlay alert and timer-creating triggers for Immobilize,
// Holyforge Discipline, and Sanctification Discipline. Pacify (Spell
// 45) and Divine Aura are intentionally omitted — they live in the
// Enchanter and Cleric packs respectively. Instant stuns (Stun,
// Force, Force of Akilae) are also skipped: no duration to track and
// generic resist alerts already live in the Enchanter pack.
func PaladinPack() TriggerPack {
	return TriggerPack{
		PackName:    "Paladin",
		Description: "Lay on Hands alert plus spell timers for Immobilize, Holyforge Discipline, and Sanctification Discipline.",
		Triggers: []Trigger{
			// ── Emergency burst (overlay alert) ──────────────────────────
			// Lay on Hands is instant with a 72-minute recast; the cast
			// message ("Your hands shimmer with holy light.") only fires
			// when the paladin uses the ability, making it a clean alert
			// without timer support.
			{
				Name:     "Lay on Hands",
				Enabled:  true,
				Pattern:  `^Your hands shimmer with holy light\.$`,
				PackName: "Paladin",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "LAY ON HANDS!", DurationSecs: 5, Color: "#ffdd33"},
				},
			},

			// ── Root (timer) ────────────────────────────────────────────
			{
				Name:              "Immobilize",
				Enabled:           true,
				Pattern:           `^(?:Your feet adhere to the ground\.|[A-Z][a-zA-Z']{2,14} adheres to the ground\.)$`,
				WornOffPattern:    `^(?:Your feet come free\.|Your Immobilize spell has worn off\.|Your target resisted the Immobilize spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 180,
				SpellID:           132,
				PackName:          "Paladin",
				Actions:           []Action{},
			},

			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Holyforge Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapon is bathed in a holy light\.|[A-Z][a-zA-Z']{2,14}'s weapon is bathed in a holy light\.)$`,
				WornOffPattern:    `^The holy light fades from your weapon\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 300,
				SpellID:           4500,
				PackName:          "Paladin",
				Actions:           []Action{},
			},
			{
				Name:              "Sanctification Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your body is surrounded in an aura of sanctification\.|[A-Z][a-zA-Z']{2,14} is surrounded in an aura of sanctification\.)$`,
				WornOffPattern:    `^Your sanctification fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 18,
				SpellID:           4518,
				PackName:          "Paladin",
				Actions:           []Action{},
			},
		},
	}
}

// ShadowknightPack returns the pre-built shadowknight trigger pack: a
// Harm Touch overlay alert, a Feign Death skill alert, and timer-creating
// triggers for Engulfing Darkness, Disease Cloud, Voice of Terris,
// Asystole, and Unholy Aura Discipline. Drain Soul, Death Peace, and the
// Feign Death spell (id 366) are intentionally omitted — instant or
// unreliable to detect from the log without false positives.
func ShadowknightPack() TriggerPack {
	return TriggerPack{
		PackName:    "Shadowknight",
		Description: "Harm Touch and Feign Death alerts plus spell timers for Engulfing Darkness, Disease Cloud, Voice of Terris, Asystole, and Unholy Aura Discipline.",
		Triggers: []Trigger{
			// ── Emergency burst (overlay alert) ──────────────────────────
			{
				Name:     "Harm Touch",
				Enabled:  true,
				Pattern:  `^Your hands glow with malignant power\.$`,
				PackName: "Shadowknight",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "HARM TOUCH!", DurationSecs: 5, Color: "#aa00ff"},
				},
			},

			// ── Feign Death skill (overlay alert) ────────────────────────
			// SK and monk both have the FD skill (skill_id 35). The classic
			// EQ client emits "You feign death." on success — same line for
			// both classes. The skill's strings are hardcoded client-side
			// rather than DB anchors; if the exact text differs on Quarm,
			// users can adjust the pattern in the trigger editor.
			{
				Name:     "Feign Death",
				Enabled:  true,
				Pattern:  `^You feign death\.$`,
				PackName: "Shadowknight",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "FEIGN DEATH", DurationSecs: 4, Color: "#888888"},
				},
			},

			// ── Snare/DoT (timer) ────────────────────────────────────────
			{
				Name:              "Engulfing Darkness",
				Enabled:           true,
				Pattern:           `^(?:You are engulfed by darkness\.|[A-Z][a-zA-Z']{2,14} is engulfed by darkness\.)$`,
				WornOffPattern:    `^(?:The darkness fades\.|Your Engulfing Darkness spell has worn off\.|Your target resisted the Engulfing Darkness spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           355,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},

			// ── Aggro debuff (timer) ─────────────────────────────────────
			{
				Name:              "Disease Cloud",
				Enabled:           true,
				Pattern:           `^(?:Your stomach begins to cramp\.|[A-Z][a-zA-Z']{2,14} doubles over in pain\.)$`,
				WornOffPattern:    `^Your stomach feels better\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
				SpellID:           340,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},

			// ── Self aggro buff (timer) ──────────────────────────────────
			{
				Name:              "Voice of Terris",
				Enabled:           true,
				Pattern:           `^(?:Your voice fills with the power of nightmares\.|[A-Z][a-zA-Z']{2,14} speaks with the voice of nightmares\.)$`,
				WornOffPattern:    `^The voice of nightmares fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 600,
				SpellID:           1228,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},

			// ── DoT debuff (timer) ───────────────────────────────────────
			{
				Name:              "Asystole",
				Enabled:           true,
				Pattern:           `^(?:Your heart stops\.|[A-Z][a-zA-Z']{2,14} clutches their chest\.)$`,
				WornOffPattern:    `^Your heartbeat resumes\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           1508,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},

			// ── Discipline (timer) ───────────────────────────────────────
			{
				Name:              "Unholy Aura Discipline",
				Enabled:           true,
				Pattern:           `^(?:An unholy aura envelopes your body\.|[A-Z][a-zA-Z']{2,14} is enveloped in an unholy aura\.)$`,
				WornOffPattern:    `^The unholy aura fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 72,
				SpellID:           4520,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},
		},
	}
}

// GroupAwarenessPack returns the pre-built group awareness trigger pack with
// alerts for incoming tells, player deaths, and group member deaths.
func GroupAwarenessPack() TriggerPack {
	return TriggerPack{
		PackName:    "Group Awareness",
		Description: "Alerts for incoming tells, your death, and group member deaths.",
		Triggers: []Trigger{
			{
				Name:     "Incoming Tell",
				Enabled:  true,
				Pattern:  `\w+ tells you,`,
				PackName: "Group Awareness",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INCOMING TELL!", DurationSecs: 5, Color: "#44aaff"},
				},
			},
			{
				Name:     "You Died",
				Enabled:  true,
				Pattern:  `You have been slain by`,
				PackName: "Group Awareness",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "YOU DIED!", DurationSecs: 8, Color: "#ff0000"},
				},
			},
			{
				Name:     "Group Member Died",
				Enabled:  true,
				Pattern:  `\w+ has been slain by`,
				PackName: "Group Awareness",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "GROUP DEATH!", DurationSecs: 5, Color: "#ff6600"},
				},
			},
		},
	}
}

// AllPacks returns all built-in trigger packs.
func AllPacks() []TriggerPack {
	return []TriggerPack{
		EnchanterPack(),
		ClericPack(),
		DruidPack(),
		ShamanPack(),
		PaladinPack(),
		ShadowknightPack(),
		GroupAwarenessPack(),
	}
}

// InstallPack replaces any existing triggers for pack.PackName with the pack's
// triggers, assigning fresh IDs and creation timestamps.
func InstallPack(store *Store, pack TriggerPack) error {
	if err := store.DeleteByPack(pack.PackName); err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range pack.Triggers {
		t := &pack.Triggers[i]
		id, err := NewID()
		if err != nil {
			return err
		}
		t.ID = id
		t.CreatedAt = now
		if err := store.Insert(t); err != nil {
			return err
		}
	}
	return nil
}

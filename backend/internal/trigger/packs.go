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
		Description: "CC break + cast-failure alerts plus spell timers for the enchanter buff (VoG, KEI, IS, GRM, Speed of the Shissar/Brood), debuff (Tashanian, Cripple, Asphyxiate), and mez (Mesmerize, Mesmerization, Dazzle, Enthrall, Entrance, Glamour of Kintaz, Rapture / Ancient: Eternal Rapture) lines.",
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

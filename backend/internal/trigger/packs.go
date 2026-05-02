package trigger

import "time"

// EnchanterPack returns the pre-built enchanter trigger pack covering the
// crowd-control and raid-buff workflow: mez/charm/root breaks, resist and
// immunity warnings, debuff-fade recast prompts (Tashanian, Cripple,
// Asphyxiate, Overwhelming Splendor), and self-buff fade alerts for the
// common raid buffs (VoG, KEI, Group Resist Magic, Speed of the Shissar/
// Brood, Intellectual Superiority).
func EnchanterPack() TriggerPack {
	return TriggerPack{
		PackName:    "Enchanter",
		Description: "Mez/charm/root breaks, resist and immunity alerts, and fade prompts for enchanter debuffs and raid buffs.",
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

			// ── Debuff fades (recast prompts) ────────────────────────────
			{
				Name:     "Tashanian Faded",
				Enabled:  true,
				Pattern:  `Your Tashanian spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "TASH FADED", DurationSecs: 4, Color: "#ffaa44"},
				},
			},
			{
				Name:     "Cripple Faded",
				Enabled:  true,
				Pattern:  `Your Cripple spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CRIPPLE FADED", DurationSecs: 4, Color: "#ffaa44"},
				},
			},
			{
				Name:     "Asphyxiate Faded",
				Enabled:  true,
				Pattern:  `Your Asphyxiate spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ASPHYX FADED", DurationSecs: 4, Color: "#ffaa44"},
				},
			},
			{
				Name:     "Overwhelming Splendor Faded",
				Enabled:  true,
				Pattern:  `Your Overwhelming Splendor spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "SPLENDOR FADED", DurationSecs: 4, Color: "#ffaa44"},
				},
			},

			// ── Self-buff fades (recast prompts) ─────────────────────────
			{
				Name:     "Visions of Grandeur Faded",
				Enabled:  true,
				Pattern:  `Your visions fade\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "VoG FADED", DurationSecs: 5, Color: "#44aaff"},
				},
			},
			{
				Name:     "KEI Faded",
				Enabled:  true,
				Pattern:  `Your mind returns to normal\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "KEI FADED", DurationSecs: 5, Color: "#44aaff"},
				},
			},
			{
				Name:     "Group Resist Magic Faded",
				Enabled:  true,
				Pattern:  `Your protection fades\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "GRM FADED", DurationSecs: 5, Color: "#44aaff"},
				},
			},
			{
				Name:     "Haste Faded (Shissar/Brood)",
				Enabled:  true,
				Pattern:  `Your body slows\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "HASTE FADED", DurationSecs: 5, Color: "#44aaff"},
				},
			},
			{
				Name:     "Intellectual Superiority Faded",
				Enabled:  true,
				Pattern:  `The intellectual advancement fades\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "IS FADED", DurationSecs: 5, Color: "#44aaff"},
				},
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

package trigger

import "time"

// EnchanterPack returns the pre-built enchanter trigger pack: critical
// crowd-control break alerts (mez/charm/root) and casting-failure alerts
// (resist, immunities, interrupt). Buff and debuff timers — self-buffs cast
// on the group, debuffs cast on enemies — are tracked per-target by the
// spell-landed pipeline (with item/AA duration extensions) and don't need
// a fade alert; the user reads them off the buff/detrim overlays.
func EnchanterPack() TriggerPack {
	return TriggerPack{
		PackName:    "Enchanter",
		Description: "Mez/charm/root break alerts and casting-failure alerts. Buff and debuff durations are tracked per-target on the spell timer overlays automatically — no fade alerts needed.",
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

package trigger

import (
	"fmt"
	"log/slog"
	"time"
)

// Default audio-alert thresholds applied to every built-in class pack timer
// trigger that doesn't carry its own TimerAlerts. Values mirror what the
// frontend trigger editor would produce for a "fading soon" / "expiring"
// TTS preset, with TTS volume at 100% and the system-default voice. Users
// can override these per-trigger after installing a pack.
const (
	// buffFadeShortSecs fires the "fading soon" TTS for buffs whose total
	// duration is under one hour. 60s is enough warning to reapply during
	// downtime without interrupting active combat.
	buffFadeShortSecs = 60
	// buffFadeLongSecs fires the "fading soon" TTS for buffs an hour or
	// longer (e.g. KEI, Aegolism). Five minutes gives time to corral the
	// group for a re-buff before the song line drops.
	buffFadeLongSecs = 300
	// detrimentalExpireSecs fires the "expiring" TTS for any detrimental
	// timer (mez, root, charm, debuff). Ten seconds is the tightest useful
	// warning before re-cast windows close.
	detrimentalExpireSecs = 10
)

// buffFadingAlert returns the standard "fading soon" TimerAlert used by
// built-in class packs. The threshold (seconds) is picked per-trigger by
// duration band; see applyDefaultTimerAlerts.
func buffFadingAlert(seconds int) TimerAlert {
	return TimerAlert{
		ID:          fmt.Sprintf("pack-fade-%ds", seconds),
		Seconds:     seconds,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "{spell} fading soon",
		TTSVolume:   100,
	}
}

// detrimentalExpiringAlert returns the standard 10-second "expiring"
// TimerAlert used by built-in class packs for detrimental timers.
func detrimentalExpiringAlert() TimerAlert {
	return TimerAlert{
		ID:          fmt.Sprintf("pack-expire-%ds", detrimentalExpireSecs),
		Seconds:     detrimentalExpireSecs,
		Type:        TimerAlertTypeTextToSpeech,
		TTSTemplate: "{spell} expiring",
		TTSVolume:   100,
	}
}

// applyDefaultTimerAlerts fills in a default TimerAlerts list for every
// timer-bound trigger in p that doesn't already declare one. Buffs get a
// fading-soon TTS at 60s (or 300s if the buff runs an hour or longer);
// detrimentals get an expiring TTS at 10s. Triggers whose duration is too
// short for the chosen threshold to ever fire (e.g. a 12-second discipline
// vs. a 60-second buff threshold) are left untouched — the threshold would
// trip on the first tick and produce a useless alert.
func applyDefaultTimerAlerts(p TriggerPack) TriggerPack {
	for i := range p.Triggers {
		t := &p.Triggers[i]
		if len(t.TimerAlerts) > 0 || t.TimerDurationSecs <= 0 {
			continue
		}
		switch t.TimerType {
		case TimerTypeBuff:
			if t.TimerDurationSecs >= 3600 {
				t.TimerAlerts = []TimerAlert{buffFadingAlert(buffFadeLongSecs)}
			} else if t.TimerDurationSecs > buffFadeShortSecs {
				t.TimerAlerts = []TimerAlert{buffFadingAlert(buffFadeShortSecs)}
			}
		case TimerTypeDetrimental:
			if t.TimerDurationSecs > detrimentalExpireSecs {
				t.TimerAlerts = []TimerAlert{detrimentalExpiringAlert()}
			}
		}
	}
	return p
}

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
		Class:       ClassPtr(ClassEnchanter),
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
					{Type: ActionTextToSpeech, Text: "Mez broke", Volume: 1.0},
				},
			},
			{
				Name:     "Charm Broke",
				Enabled:  true,
				// EQ emits the same generic line for every charm spell:
				// "Your charm spell has worn off." (lowercase 'charm',
				// regardless of whether the underlying spell was Charm,
				// Beguile, Cajoling Whispers, Boltran's Agacerie, etc.).
				// The previous per-name alternation never matched anything
				// in real logs and was the reason charm-break alerts never
				// fired for the user.
				Pattern:  `^Your charm spell has worn off\.$`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CHARM BROKE!", DurationSecs: 6, Color: "#ff0000"},
					{Type: ActionTextToSpeech, Text: "Charm broke", Volume: 1.0},
				},
			},
			{
				Name:     "Root Broke",
				Enabled:  true,
				Pattern:  `Your (?:Root|Engulfing Roots|Engulfing Darkness|Fetter|Greater Fetter) spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
					{Type: ActionTextToSpeech, Text: "Root broke", Volume: 1.0},
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
				// 780s = 130 ticks per the corrected formula 9
				// (level*2+10, base 140) at level 60. PQDI shows max 140
				// ticks but that requires level 65+ which Quarm caps out
				// before. Was 720s (120 ticks) under the old formula 9.
				TimerDurationSecs: 780,
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
				// 450s = 75 ticks per the corrected formula 8 (fixed base)
				// matching PQDI for spell 1592 (base=75). Was 810s (135
				// ticks) under the old formula 8.
				TimerDurationSecs: 450,
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
			// Charm/Beguile/Cajoling Whispers/Allure/Boltran's Agacerie all
			// share spells_new buffduration=205 / buffdurationformula=10. With
			// formula 10 = min(level*3 + 10, base), level 60 yields 190 ticks
			// = 1140s. Old formula 10 (`min(level, base)`) returned 60 ticks
			// = 360s, which is what these were calibrated against; updated in
			// lockstep with the duration.go fix.
			{
				Name:              "Charm",
				Enabled:           true,
				Pattern:           `^You begin casting Charm\.$`,
				WornOffPattern:    `^(?:Your Charm spell has worn off\.|Your target resisted the Charm spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
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
				TimerDurationSecs: 1140,
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
				TimerDurationSecs: 1140,
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
				TimerDurationSecs: 1140,
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
				TimerDurationSecs: 1140,
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
				// 360s = 6 minutes per PQDI (spell 45, buffduration=60 ticks).
				// Was 720 — that came from the EQEmu-canonical formula 8
				// reading; Quarm uses fixed base, fixed here in lockstep with
				// the spelltimer.CalcDurationTicks formula-8 fix.
				TimerDurationSecs: 360,
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
		Class:       ClassPtr(ClassCleric),
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
		Class:       ClassPtr(ClassDruid),
		Description: "Crowd-control break alerts (root, snare) plus spell timers for Protection of the Glades, Blessing of Replenishment, Legacy of Thorn, Hand of Ro, Winged Death, Ensnare, and Entrapping Roots.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Caster's "Your <SpellName> spell has worn off." line fires both
			// on damage-induced break and on natural expiry — either way the
			// druid needs to know the mob is loose.
			{
				Name:     "Root Broke",
				Enabled:  true,
				Pattern:  `Your Entrapping Roots spell has worn off\.`,
				PackName: "Druid",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
					{Type: ActionTextToSpeech, Text: "Root broke", Volume: 1.0},
				},
			},
			{
				Name:     "Snare Broke",
				Enabled:  true,
				Pattern:  `Your Ensnare spell has worn off\.`,
				PackName: "Druid",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "SNARE BROKE!", DurationSecs: 5, Color: "#ff8833"},
					{Type: ActionTextToSpeech, Text: "Snare broke", Volume: 1.0},
				},
			},

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
		Class:       ClassPtr(ClassShaman),
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
		Class:       ClassPtr(ClassPaladin),
		Description: "Lay on Hands alert, root break alert, and spell timers for Immobilize, Holyforge Discipline, and Sanctification Discipline.",
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

			// ── Crowd-control break ──────────────────────────────────────
			{
				Name:     "Root Broke",
				Enabled:  true,
				Pattern:  `Your Immobilize spell has worn off\.`,
				PackName: "Paladin",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
					{Type: ActionTextToSpeech, Text: "Root broke", Volume: 1.0},
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
		Class:       ClassPtr(ClassShadowknight),
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

// WarriorPack returns the pre-built warrior trigger pack: timer-creating
// triggers for the four core warrior disciplines (Defensive, Evasive,
// Aggressive, Furious). The Taunt skill is intentionally omitted — its
// success/failure is not directly logged in classic-era EQ, only the
// resulting aggro shift, which we can't detect from the log alone.
func WarriorPack() TriggerPack {
	return TriggerPack{
		PackName:    "Warrior",
		Class:       ClassPtr(ClassWarrior),
		Description: "Spell timers for Defensive, Evasive, Aggressive, and Furious Disciplines.",
		Triggers: []Trigger{
			// ── Disciplines (timers) ────────────────────────────────────
			// Defensive / Aggressive / Evasive share the fade text "You
			// return to your normal fighting style." but have unique cast
			// messages, so each is distinguishable on its land anchor.
			{
				Name:              "Defensive Discipline",
				Enabled:           true,
				Pattern:           `^(?:You assume a defensive fighting style\.|[A-Z][a-zA-Z']{2,14} assumes a defensive fighting style\.)$`,
				WornOffPattern:    `^You return to your normal fighting style\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 180,
				SpellID:           4499,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Evasive Discipline",
				Enabled:           true,
				Pattern:           `^(?:You assume an evasive fighting style\.|[A-Z][a-zA-Z']{2,14} assumes an evasive fighting style\.)$`,
				WornOffPattern:    `^You return to your normal fighting style\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 180,
				SpellID:           4503,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Aggressive Discipline",
				Enabled:           true,
				Pattern:           `^(?:You assume an aggressive fighting style\.|[A-Z][a-zA-Z']{2,14} assumes an aggressive fighting style\.?)$`,
				WornOffPattern:    `^You return to your normal fighting style\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 180,
				SpellID:           4498,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Furious Discipline",
				Enabled:           true,
				Pattern:           `^(?:A consuming rage takes over your weapons\.|[A-Z][a-zA-Z']{2,14}'s body is consumed in rage\.)$`,
				WornOffPattern:    `^The rage leaves you\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4674,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
		},
	}
}

// MonkPack returns the pre-built monk trigger pack: a Feign Death skill
// overlay alert and timers for the four core monk disciplines
// (Stonestance, Innerflame, Thunderkick, Ashenhand). Mend is intentionally
// omitted — its exact log strings vary by classic-era client and we'd
// risk false positives without verifying against real Quarm logs.
//
// The Feign Death entry overlaps with the Shadowknight pack — installing
// both packs will cause duplicate overlay fires on FD; users can disable
// one in the trigger editor if needed.
func MonkPack() TriggerPack {
	return TriggerPack{
		PackName:    "Monk",
		Class:       ClassPtr(ClassMonk),
		Description: "Feign Death alert plus spell timers for Stonestance, Innerflame, Thunderkick, and Ashenhand Disciplines.",
		Triggers: []Trigger{
			// ── Feign Death skill (overlay alert) ────────────────────────
			{
				Name:     "Feign Death",
				Enabled:  true,
				Pattern:  `^You feign death\.$`,
				PackName: "Monk",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "FEIGN DEATH", DurationSecs: 4, Color: "#888888"},
				},
			},

			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Stonestance Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your body becomes one with the earth\.|[A-Z][a-zA-Z']{2,14}'s feet become one with the earth\.)$`,
				WornOffPattern:    `^You are no longer one with the earth\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4510,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Innerflame Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your muscles bulge with the force of will\.|[A-Z][a-zA-Z']{2,14}'s muscles bulge with the force of will\.)$`,
				WornOffPattern:    `^Your strength of will fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4512,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			// Thunderkick and Ashenhand are single-use "next strike" discs
			// — they consume on the first melee hit, expiring the buff via
			// spell_fades. The 72-second timer is the natural max lifetime
			// (formula 50 = level/5 ticks); using before then clears it
			// via the worn_off line.
			{
				Name:              "Thunderkick Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your feet glow with mystic power\.|[A-Z][a-zA-Z']{2,14}'s feet glow with mystic power\.)$`,
				WornOffPattern:    `^The glow fades from your feet\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 72,
				SpellID:           4511,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Ashenhand Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your hands clench with fatal fervor\.|[A-Z][a-zA-Z']{2,14}'s fist clenches with fatal fervor\.)$`,
				WornOffPattern:    `^Your fists lose their fervor\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 72,
				SpellID:           4508,
				PackName:          "Monk",
				Actions:           []Action{},
			},
		},
	}
}

// RoguePack returns the pre-built rogue trigger pack: timer-creating
// triggers for Duelist, Deadeye, and Counterattack disciplines. Escape
// (AA) and Evasion (skill) are intentionally omitted — their exact log
// strings aren't in spells_new and we'd risk false positives without
// verifying against real Quarm logs.
func RoguePack() TriggerPack {
	return TriggerPack{
		PackName:    "Rogue",
		Class:       ClassPtr(ClassRogue),
		Description: "Spell timers for Duelist, Deadeye, and Counterattack Disciplines.",
		Triggers: []Trigger{
			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Duelist Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your muscles quiver with power\.|[A-Z][a-zA-Z']{2,14}'s eyes gleam with energy\.)$`,
				WornOffPattern:    `^Your fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4676,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Deadeye Discipline",
				Enabled:           true,
				Pattern:           `^(?:You feel unstoppable\.|[A-Z][a-zA-Z']{2,14} feels unstoppable\.)$`,
				WornOffPattern:    `^Your confidence fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4505,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Counterattack Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons move with uncanny grace\.|[A-Z][a-zA-Z']{2,14}'s weapons move with uncanny grace\.)$`,
				WornOffPattern:    `^Your weapons lose their uncanny grace\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4673,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
		},
	}
}

// RangerPack returns the pre-built ranger trigger pack: timer-creating
// triggers for Trueshot and Weapon Shield disciplines, the Call of
// Sky / Call of Fire elemental weapon-proc buffs, and the Flame Lick
// aggro tool. Ensnare and Entrapping Roots are intentionally omitted —
// they're already covered by the Druid pack with the same SpellIDs.
func RangerPack() TriggerPack {
	return TriggerPack{
		PackName:    "Ranger",
		Class:       ClassPtr(ClassRanger),
		Description: "Spell timers for Trueshot Discipline, Weapon Shield Discipline, Call of Sky, Call of Fire, and Flame Lick.",
		Triggers: []Trigger{
			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Trueshot Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your bow crackles with natural energy\.|[A-Z][a-zA-Z']{2,14}'s bow crackles with natural energy\.)$`,
				WornOffPattern:    `^The natural energy fades from your bow\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 120,
				SpellID:           4506,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
			{
				Name:              "Weapon Shield Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons begin to spin\.|[A-Z][a-zA-Z']{2,14}'s weapons begin to spin\.)$`,
				WornOffPattern:    `^Your weapons slow down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 24,
				SpellID:           4519,
				PackName:          "Ranger",
				Actions:           []Action{},
			},

			// ── Elemental weapon-proc buffs (timers) ────────────────────
			// Call of Sky and Call of Fire share "'s weapons gleam." as
			// cast_on_other, so each is keyed on its unique cast_on_you
			// to disambiguate when the caster sees their own cast.
			{
				Name:              "Call of Sky",
				Enabled:           true,
				Pattern:           `^The Call of Sky fills your weapons with power\.$`,
				WornOffPattern:    `^The Call of Sky leaves\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           1461,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
			{
				Name:              "Call of Fire",
				Enabled:           true,
				Pattern:           `^The Call of Fire fills your weapons with power\.$`,
				WornOffPattern:    `^The Call of Fire leaves\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           1463,
				PackName:          "Ranger",
				Actions:           []Action{},
			},

			// ── Aggro tool (timer) ───────────────────────────────────────
			{
				Name:              "Flame Lick",
				Enabled:           true,
				Pattern:           `^(?:You are surrounded by flickering flames\.|[A-Z][a-zA-Z']{2,14} is surrounded by flickering flames\.)$`,
				WornOffPattern:    `^The flames are extinguished\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 48,
				SpellID:           239,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
		},
	}
}

// BardPack returns the pre-built bard trigger pack: timer-creating
// triggers for the standard bard mana/HP/melee/resist song line
// (Cantata of Replenishment, Warsong of Zek, Niv's Melody of
// Preservation, Psalm of Veeshan, Elemental Rhythms, Guardian Rhythms)
// plus the mez (Kelin's Lucid Lullaby), lull (Kelin's Lugubrious
// Lament), charm (Solon's Bewitching Bravura), and slow (Largo's
// Absonant Binding) target debuffs.
//
// Bard songs pulse on every tick while sung; the spelltimer engine's
// same-name dedup window keeps duplicate timers from forming, and each
// pulse refreshes the existing timer so the post-singing countdown
// (typically 54 seconds at level 60) starts when the bard stops.
//
// Cassindra's Chorus of Clarity is omitted — it has formula 0 / base 0
// in spells_new (no buff duration to track) and empty cast_on_other /
// spell_fades, so there's nothing reliable to time from log alone.
func BardPack() TriggerPack {
	return TriggerPack{
		PackName:    "Bard",
		Class:       ClassPtr(ClassBard),
		Description: "Crowd-control break alerts (mez, charm) plus spell timers for Cantata of Replenishment, Warsong of Zek, Niv's Melody of Preservation, Psalm of Veeshan, Elemental Rhythms, Guardian Rhythms, Kelin's Lucid Lullaby / Lugubrious Lament, Solon's Bewitching Bravura, and Largo's Absonant Binding.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Bard songs pulse every tick, so the worn-off line only fires
			// once the bard stops singing (or the song's natural duration
			// elapses post-stop). Either case means the mez/charm is gone.
			{
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  `Your Kelin's Lucid Lullaby spell has worn off\.`,
				PackName: "Bard",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5, Color: "#ff4444"},
					{Type: ActionTextToSpeech, Text: "Mez broke", Volume: 1.0},
				},
			},
			{
				Name:     "Charm Broke",
				Enabled:  true,
				Pattern:  `Your Solon's Bewitching Bravura spell has worn off\.`,
				PackName: "Bard",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CHARM BROKE!", DurationSecs: 6, Color: "#ff0000"},
					{Type: ActionTextToSpeech, Text: "Charm broke", Volume: 1.0},
				},
			},

			// ── Group buff songs (timers, self-only land) ────────────────
			// These songs have empty cast_on_other in spells_new, so only
			// the song's targets see "You feel ...". Each timer triggers
			// on the unique cast_on_you and clears on the song's fade
			// line. Cantata's spell_fades is empty in the DB, so its
			// timer simply expires after the natural duration.
			{
				Name:              "Cantata of Replenishment",
				Enabled:           true,
				Pattern:           `^You feel replenished\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           1759,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Warsong of Zek",
				Enabled:           true,
				Pattern:           `^You hear the war horns of Zek echo in your mind\.$`,
				WornOffPattern:    `^The warsong of Zek fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           3374,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Niv's Melody of Preservation",
				Enabled:           true,
				Pattern:           `^You feel an aura of protection engulf you\.$`,
				WornOffPattern:    `^Your protection fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           748,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Psalm of Veeshan",
				Enabled:           true,
				Pattern:           `^Crystalline scales gather around you\.$`,
				WornOffPattern:    `^The crystalline scales fall away\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           3368,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Elemental Rhythms",
				Enabled:           true,
				Pattern:           `^You feel an aura of elemental protection surrounding you\.$`,
				WornOffPattern:    `^The aura of protection fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           710,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			// Guardian Rhythms shares "The aura of protection fades." with
			// Elemental Rhythms; the trigger engine's same-name dedup keeps
			// the wrong-song timer from being cleared inadvertently when
			// both run side-by-side, since each timer keys on its own name.
			{
				Name:              "Guardian Rhythms",
				Enabled:           true,
				Pattern:           `^You feel an aura of mystic protection surrounding you\.$`,
				WornOffPattern:    `^The aura of protection fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 54,
				SpellID:           709,
				PackName:          "Bard",
				Actions:           []Action{},
			},

			// ── Mez / lull / charm / slow songs (timers) ────────────────
			{
				Name:              "Kelin's Lucid Lullaby",
				Enabled:           true,
				Pattern:           `^(?:You feel quite drowsy\.|[A-Z][a-zA-Z']{2,14}'s head nods\.)$`,
				WornOffPattern:    `^You no longer feel sleepy\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           724,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Kelin's Lugubrious Lament",
				Enabled:           true,
				Pattern:           `^(?:You feel a strong sense of loss\.|[A-Z][a-zA-Z']{2,14} looks sad\.)$`,
				WornOffPattern:    `^You no longer feel sad\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           728,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Solon's Bewitching Bravura",
				Enabled:           true,
				Pattern:           `^(?:You are captivated by the bewitching tune\.|[A-Z][a-zA-Z']{2,14}'s eyes glaze over\.)$`,
				WornOffPattern:    `^You are no longer captivated\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           750,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Largo's Absonant Binding",
				Enabled:           true,
				Pattern:           `^(?:Strands of solid music bind your body\.|[A-Z][a-zA-Z']{2,14} is bound by strands of solid music\.)$`,
				WornOffPattern:    `^The strands of fade away\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           1751,
				PackName:          "Bard",
				Actions:           []Action{},
			},
		},
	}
}

// MagicianPack returns the pre-built magician trigger pack: timer-creating
// triggers for Burnout IV (pet haste) and Shield of Lava (damage shield).
//
// Malo / Malosini are intentionally omitted — already covered by the
// Shaman pack with the same SpellIDs. Call of the Hero, Modulating Rod,
// and pet summon/death tracking are skipped: instant utilities or
// state changes that don't fit the two-purpose timer/alert model.
func MagicianPack() TriggerPack {
	return TriggerPack{
		PackName:    "Magician",
		Class:       ClassPtr(ClassMagician),
		Description: "Spell timers for Burnout IV (pet haste) and Shield of Lava (damage shield).",
		Triggers: []Trigger{
			// ── Pet haste (timer) ───────────────────────────────────────
			// Burnout IV has empty cast_on_you and "<name> goes berserk."
			// cast_on_other; the latter could clash with NPC enrage-skill
			// log lines, so this trigger keys on the unambiguous cast-
			// start instead. The 900-second timer is the natural max
			// (formula 11 / base 150 = 150 ticks).
			{
				Name:              "Burnout IV",
				Enabled:           true,
				Pattern:           `^You begin casting Burnout IV\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 900,
				SpellID:           1472,
				PackName:          "Magician",
				Actions:           []Action{},
			},

			// ── Damage shield (timer) ───────────────────────────────────
			{
				Name:              "Shield of Lava",
				Enabled:           true,
				Pattern:           `^(?:You are enveloped by lava\.|[A-Z][a-zA-Z']{2,14} is enveloped by flame\.)$`,
				WornOffPattern:    `^The flames die down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 390,
				SpellID:           412,
				PackName:          "Magician",
				Actions:           []Action{},
			},
		},
	}
}

// NecromancerPack returns the pre-built necromancer trigger pack:
// timer-creating triggers for Arch Lich (self-buff), Splurt / Ignite
// Blood / Pyrocruor (DoTs), Bond of Death (DoT lifetap), and Harmshield
// (defensive). Engulfing Darkness is intentionally omitted — already
// covered by the Shadowknight pack with the same SpellID. Death Peace
// is skipped: cast_on_other " dies." would match every mob death.
// Pet summon/death and Call to Corpse AA are skipped — log strings
// are environment-dependent and risk false positives.
func NecromancerPack() TriggerPack {
	return TriggerPack{
		PackName:    "Necromancer",
		Class:       ClassPtr(ClassNecromancer),
		Description: "Spell timers for Arch Lich, Splurt, Ignite Blood, Pyrocruor, Bond of Death, and Harmshield.",
		Triggers: []Trigger{
			// ── Self buff (timer) ───────────────────────────────────────
			{
				Name:              "Arch Lich",
				Enabled:           true,
				Pattern:           `^(?:You feel the skin peel from your bones\.|[A-Z][a-zA-Z']{2,14}'s skin peels away\.)$`,
				WornOffPattern:    `^Your flesh returns\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           1416,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},

			// ── DoTs (timers) ───────────────────────────────────────────
			{
				Name:              "Splurt",
				Enabled:           true,
				Pattern:           `^(?:Your body begins to splurt\.|[A-Z][a-zA-Z']{2,14}'s body begins to splurt\.)$`,
				WornOffPattern:    `^Your body stops splurting\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 96,
				SpellID:           1620,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
			// Ignite Blood (id 6) and Pyrocruor (id 1617) share identical
			// cast_on_you / cast_on_other / spell_fades anchors, so each
			// is keyed on cast-start to disambiguate. The shared "Your
			// blood cools." fade clears whichever is active.
			{
				Name:              "Ignite Blood",
				Enabled:           true,
				Pattern:           `^You begin casting Ignite Blood\.$`,
				WornOffPattern:    `^(?:Your blood cools\.|Your target resisted the Ignite Blood spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 126,
				SpellID:           6,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
			{
				Name:              "Pyrocruor",
				Enabled:           true,
				Pattern:           `^You begin casting Pyrocruor\.$`,
				WornOffPattern:    `^(?:Your blood cools\.|Your target resisted the Pyrocruor spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 108,
				SpellID:           1617,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
			{
				Name:              "Bond of Death",
				Enabled:           true,
				Pattern:           `^(?:You feel your life force drain away\.|[A-Z][a-zA-Z']{2,14} staggers\.)$`,
				WornOffPattern:    `^The bond fades\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           456,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},

			// ── Defensive (timer) ───────────────────────────────────────
			{
				Name:              "Harmshield",
				Enabled:           true,
				Pattern:           `^You no longer feel pain\.$`,
				WornOffPattern:    `^Your invulnerability fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 18,
				SpellID:           199,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
		},
	}
}

// WizardPack returns the pre-built wizard trigger pack: a Harvest overlay
// alert and timer-creating triggers for Manaskin and Atol's Spectral
// Shackles.
//
// Evacuate / Exodus and Familiar are intentionally omitted — Evac/Exodus
// are instant utilities (no duration), and Familiar uses formula 3600
// in spells_new which the spelltimer engine treats as no-timer
// (permanent / song-pulse semantics).
func WizardPack() TriggerPack {
	return TriggerPack{
		PackName:    "Wizard",
		Class:       ClassPtr(ClassWizard),
		Description: "Harvest alert, root break alert, and spell timers for Manaskin and Atol's Spectral Shackles.",
		Triggers: []Trigger{
			// ── Crowd-control break ──────────────────────────────────────
			{
				Name:     "Root Broke",
				Enabled:  true,
				Pattern:  `Your Atol's Spectral Shackles spell has worn off\.`,
				PackName: "Wizard",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
					{Type: ActionTextToSpeech, Text: "Root broke", Volume: 1.0},
				},
			},

			// ── Mana tool (overlay alert) ────────────────────────────────
			// Harvest is instant with a 10-minute recast; the cast message
			// "You gather mana from your surroundings." only fires when
			// the wizard uses the spell, making it a clean alert without
			// timer support.
			{
				Name:     "Harvest",
				Enabled:  true,
				Pattern:  `^You gather mana from your surroundings\.$`,
				PackName: "Wizard",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "HARVEST", DurationSecs: 4, Color: "#33ccff"},
				},
			},

			// ── Self buff (timer) ───────────────────────────────────────
			// Manaskin runs ~2 hours at level 60 (1200 ticks formula 3).
			{
				Name:              "Manaskin",
				Enabled:           true,
				Pattern:           `^(?:Your skin gleams with an incandescent glow\.|[A-Z][a-zA-Z']{2,14}'s skin gleams with an incandescent glow\.)$`,
				WornOffPattern:    `^Your skin returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 7200,
				SpellID:           1609,
				PackName:          "Wizard",
				Actions:           []Action{},
			},

			// ── Snare/root (timer) ──────────────────────────────────────
			// Atol's shares "Your feet come free." with the paladin's
			// Immobilize, but each trigger keys on its own timer so the
			// shared fade only clears the matching pack's timer.
			{
				Name:              "Atol's Spectral Shackles",
				Enabled:           true,
				Pattern:           `^(?:Spectral shackles bind your feet to the ground\.|[A-Z][a-zA-Z']{2,14} is shackled to the ground\.)$`,
				WornOffPattern:    `^(?:Your feet come free\.|Your Atol's Spectral Shackles spell has worn off\.|Your target resisted the Atol's Spectral Shackles spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 150,
				SpellID:           1631,
				PackName:          "Wizard",
				Actions:           []Action{},
			},
		},
	}
}

// BeastlordPack returns the pre-built beastlord trigger pack: timer-creating
// triggers for Spiritual Purity / Spiritual Dominion (group mana/HP),
// Paragon of Spirit (regen burst), Ferocity (melee buff), and Sha's
// Advantage (slow).
//
// The spec's "Sha's Legacy" doesn't exist in spells_new on Quarm; Sha's
// Advantage (id 2942, level-60 BL slow) is used as the canonical
// equivalent. Spirit of Arag is omitted — it's an instant pet summon
// with empty cast_on_you / spell_fades, so there's no buff timer to
// track.
func BeastlordPack() TriggerPack {
	return TriggerPack{
		PackName:    "Beastlord",
		Class:       ClassPtr(ClassBeastlord),
		Description: "Spell timers for Spiritual Purity, Spiritual Dominion, Paragon of Spirit, Ferocity, and Sha's Advantage.",
		Triggers: []Trigger{
			// ── Group mana/HP buffs (timers) ────────────────────────────
			{
				Name:              "Spiritual Purity",
				Enabled:           true,
				Pattern:           `^(?:An aura of spiritual purity envelops you\.|[A-Z][a-zA-Z']{2,14} is enveloped by an aura of spiritual purity\.)$`,
				WornOffPattern:    `^The aura of purity fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2700,
				SpellID:           2629,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},
			{
				Name:              "Spiritual Dominion",
				Enabled:           true,
				Pattern:           `^(?:An aura of spiritual command envelops you\.|[A-Z][a-zA-Z']{2,14} is enveloped by an aura of spiritual dominion\.)$`,
				WornOffPattern:    `^The dominion fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2700,
				SpellID:           3460,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},

			// ── Regen burst (timer) ─────────────────────────────────────
			// Paragon of Spirit shares "The aura of purity fades." with
			// Spiritual Purity; each timer keys on its own name so the
			// shared fade only clears the matching pack's timer.
			{
				Name:              "Paragon of Spirit",
				Enabled:           true,
				Pattern:           `^(?:Your spirit transcends\.|[A-Z][a-zA-Z']{2,14} becomes a paragon of spirit\.)$`,
				WornOffPattern:    `^The aura of purity fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 36,
				SpellID:           3291,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},

			// ── Melee buff (timer) ──────────────────────────────────────
			// Ferocity's cast_on_other in spells_new omits the leading
			// apostrophe ("s lips curl...") which is a DB quirk; the
			// pattern allows the apostrophe as optional to match either
			// form in the actual log.
			{
				Name:              "Ferocity",
				Enabled:           true,
				Pattern:           `^(?:Your lips curl into a feral snarl as you descend into ferocity\.|[A-Z][a-zA-Z']{2,14}'?s lips curl into a feral snarl as they descend into ferocity\.)$`,
				WornOffPattern:    `^The ferocity fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 390,
				SpellID:           3463,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},

			// ── Slow (timer) ────────────────────────────────────────────
			{
				Name:              "Sha's Advantage",
				Enabled:           true,
				Pattern:           `^(?:You lose your fighting edge\.|[A-Z][a-zA-Z']{2,14} loses their fighting edge\.)$`,
				WornOffPattern:    `^You regain your fighting edge\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 630,
				SpellID:           2942,
				PackName:          "Beastlord",
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
				// Suppress tells from pets and NPC merchants/bankers/trainers
				// so only real player tells fire the alert. Bazaar trader
				// PCs are indistinguishable from regular players in the log;
				// add their character names here to silence them too.
				ExcludePatterns: []string{
					`\b[Mm]aster[.!]`,                  // pet command responses (Attacking ... Master., By your command, master., Following you, Master.)
					`tells you, '[Tt]hat'll be `,       // NPC merchant: selling price
					`tells you, '[Ii]'ll give you `,    // NPC merchant: buying offer
					`tells you, 'I'?m not interested in buying`,
					`tells you, 'Welcome to my bank`,
					`tells you, 'Come back soon`,
					`tells you, 'You cannot afford `,
					`tells you, '?Hold your horses`,
					`tells you, 'I'?m busy`,
					`tells you, 'You have learned the basics`,
					`tells you, 'You have increased your `,
					`tells you, 'You are already browsing`,
					`tells you, 'I charge `,
					`tells you, 'I am unable to wake `,
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

// AllPacks returns all built-in trigger packs with default audio alerts
// applied. Class packs run through applyDefaultTimerAlerts so every timer
// trigger gets a fading/expiring TTS without each pack having to spell it
// out per-spell. GroupAwareness has no timers and is returned as-is.
func AllPacks() []TriggerPack {
	classPacks := []TriggerPack{
		EnchanterPack(),
		ClericPack(),
		DruidPack(),
		ShamanPack(),
		PaladinPack(),
		ShadowknightPack(),
		WarriorPack(),
		MonkPack(),
		RoguePack(),
		RangerPack(),
		BardPack(),
		MagicianPack(),
		NecromancerPack(),
		WizardPack(),
		BeastlordPack(),
	}
	out := make([]TriggerPack, 0, len(classPacks)+1)
	for _, p := range classPacks {
		out = append(out, applyDefaultTimerAlerts(p))
	}
	out = append(out, GroupAwarenessPack())
	return out
}

// DefaultUpdate is a one-time additive change to an installed built-in
// pack. It runs at most once per Key (recorded in pack_default_updates)
// and only ever appends entries to list-typed fields on an existing
// trigger — nothing scalar gets overwritten. Lets us ship hotfixes (e.g.
// "the Incoming Tell trigger should also exclude pet/merchant lines")
// without forcing users to reinstall the pack and lose customizations.
//
// If the target trigger doesn't exist (the user removed it or never
// installed the pack), the update is silently skipped but still recorded
// as applied — we don't want to retry forever.
type DefaultUpdate struct {
	Key                   string   // unique migration id
	PackName              string   // built-in pack the trigger belongs to
	TriggerName           string   // trigger within the pack
	AppendExcludePatterns []string // appended if not already present
}

// DefaultUpdates is the list of one-time additive pack updates to apply on
// startup. Append new entries here when shipping a fix that should land
// for users who already have the pack installed. Keys are immutable —
// rename = re-run the migration on every install.
func DefaultUpdates() []DefaultUpdate {
	return []DefaultUpdate{
		{
			// 2026-05-07: filter pet command responses + NPC merchant
			// canned phrases out of the broad "Incoming Tell" pattern.
			// Existing installs don't get this from a fresh InstallPack
			// (they'd lose customizations); this migration adds them
			// additively to whatever the user has now.
			Key:         "GroupAwareness:IncomingTell:pet-merchant-excludes-v1",
			PackName:    "Group Awareness",
			TriggerName: "Incoming Tell",
			AppendExcludePatterns: []string{
				`\b[Mm]aster[.!]`,
				`tells you, '[Tt]hat'll be `,
				`tells you, '[Ii]'ll give you `,
				`tells you, 'I'?m not interested in buying`,
				`tells you, 'Welcome to my bank`,
				`tells you, 'Come back soon`,
				`tells you, 'You cannot afford `,
				`tells you, '?Hold your horses`,
				`tells you, 'I'?m busy`,
				`tells you, 'You have learned the basics`,
				`tells you, 'You have increased your `,
				`tells you, 'You are already browsing`,
				`tells you, 'I charge `,
				`tells you, 'I am unable to wake `,
			},
		},
	}
}

// ApplyDefaultUpdates runs every DefaultUpdate that hasn't already been
// applied. Idempotent — each Key runs at most once. Reports the number of
// updates that mutated a trigger so the caller knows whether to broadcast
// or reload. Errors on any individual update are logged and skipped so a
// single bad migration doesn't block the rest.
func ApplyDefaultUpdates(store *Store, updates []DefaultUpdate) (int, error) {
	mutated := 0
	for _, u := range updates {
		applied, err := store.IsDefaultUpdateApplied(u.Key)
		if err != nil {
			return mutated, err
		}
		if applied {
			continue
		}
		changed, err := applyDefaultUpdate(store, u)
		if err != nil {
			// Don't bail — record as applied so we don't loop on a bad
			// migration. The slog warning is the breadcrumb.
			slog.Warn("trigger: default update failed", "key", u.Key, "err", err)
		}
		if err := store.MarkDefaultUpdateApplied(u.Key); err != nil {
			return mutated, err
		}
		if changed {
			mutated++
		}
	}
	return mutated, nil
}

// applyDefaultUpdate looks up the named trigger and appends any missing
// AppendExcludePatterns. Returns whether the trigger was actually mutated
// (false if the trigger doesn't exist, or all entries were already present).
func applyDefaultUpdate(store *Store, u DefaultUpdate) (bool, error) {
	if u.TriggerName == "" || u.PackName == "" {
		return false, nil
	}
	t, err := store.FindByPackAndName(u.PackName, u.TriggerName)
	if err != nil {
		return false, err
	}
	if t == nil {
		// User uninstalled the pack or removed the trigger; nothing to do.
		// Marked applied by the caller so we don't re-check forever.
		slog.Info("trigger: default update target missing, skipping", "key", u.Key, "pack", u.PackName, "trigger", u.TriggerName)
		return false, nil
	}
	existing := make(map[string]struct{}, len(t.ExcludePatterns))
	for _, p := range t.ExcludePatterns {
		existing[p] = struct{}{}
	}
	added := 0
	for _, p := range u.AppendExcludePatterns {
		if _, ok := existing[p]; ok {
			continue
		}
		t.ExcludePatterns = append(t.ExcludePatterns, p)
		existing[p] = struct{}{}
		added++
	}
	if added == 0 {
		return false, nil
	}
	if err := store.Update(t); err != nil {
		return false, err
	}
	slog.Info("trigger: default update applied", "key", u.Key, "pack", u.PackName, "trigger", u.TriggerName, "added_excludes", added)
	return true, nil
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

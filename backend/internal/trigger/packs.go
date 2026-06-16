package trigger

import (
	"fmt"
	"log/slog"
	"strings"
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

// npcNameClass matches a spell target's name inside a pack's cast_on_other
// land pattern. Mirrors backend logparser.nameClass (castindex.go): a
// lowercase-leading articled mob ("a sand giant"), a multi-word named NPC,
// or an apostrophe/backtick possessive ("Gygr`s warder"). The pack patterns
// historically used an uppercase-only, single-word class which only ever
// matched player names and single-word named mobs — so a detrimental trigger
// never fired on the overwhelming majority of targets ("a gnoll", "an ice
// goblin"). Defined as a const (not a raw-string literal in each pattern)
// because it contains a backtick. Pack triggers fire without the spell-landed
// pipeline's scope gate, so only splice this into patterns whose land-text
// suffix is specific enough not to false-match unrelated emotes.
const npcNameClass = "[a-zA-Z][a-zA-Z' `]{2,29}"

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
// playerNameClass matches a player (or single-word named) character at the
// start of a buff's "lands on other" branch — the name shown after the
// alternation pipe in patterns like
// `^(?:You feel X\.|<name> feels X\.)$`. Kept as a const so the same literal
// drives both applyBuffTargetCapture and its migration.
const playerNameClass = `[A-Z][a-zA-Z']{2,14}`

// targetCaptureGroup wraps playerNameClass in a named group so the trigger
// engine's TimerTargetCapture="target" can pull the recipient out.
const targetCaptureGroup = `(?P<target>` + playerNameClass + `)`

// applyBuffTargetCapture turns every built-in BUFF trigger whose pattern has a
// "lands on other" branch (a player name right after the alternation pipe)
// into one that captures that name as the timer's target — so the buff overlay
// shows the grey "on <target>" suffix when you cast a group buff on someone,
// exactly like the spell-landed pipeline does. The self-cast branch ("You
// feel…") carries no name, so casting on yourself shows no suffix.
//
// Done generically (one transform, applied in AllPacks) rather than editing
// ~50 pattern literals by hand: the cast-on-other branch is uniformly shaped
// `|<playerNameClass>…`, so wrapping the single occurrence that follows a pipe
// is safe and consistent. Triggers that already set TimerTargetCapture, or
// whose pattern has no such branch, are left untouched.
func applyBuffTargetCapture(p TriggerPack) TriggerPack {
	marker := "|" + playerNameClass
	for i := range p.Triggers {
		t := &p.Triggers[i]
		if t.TimerType != TimerTypeBuff || t.TimerTargetCapture != "" {
			continue
		}
		if !strings.Contains(t.Pattern, marker) {
			continue
		}
		// Wrap only the first pipe-prefixed name (the cast-on-other branch);
		// Go regexp forbids duplicate group names, so never wrap twice.
		t.Pattern = strings.Replace(t.Pattern, marker, "|"+targetCaptureGroup, 1)
		t.TimerTargetCapture = "target"
	}
	return p
}

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

// meleeSharedDisciplines returns the two disciplines available to every
// melee class (Resistant + Fearless). Each carries a stable dedup_key so
// the install-time skip logic ensures they show up exactly once in the
// user's trigger list regardless of how many melee class packs they
// install. Promote-on-uninstall keeps them around as long as any melee
// pack is still installed.
//
// Pack name is parameterized so the trigger row records which pack
// installed it (used by promote-on-uninstall to find a fallback).
func meleeSharedDisciplines(packName string) []Trigger {
	return []Trigger{
		{
			Name:    "Resistant Discipline",
			Enabled: true,
			// Self-cast: "You channel your will into magical resistance."
			// Other-cast: "<name> has become more resistant."
			Pattern:           `^(?:You channel your will into magical resistance\.|[A-Z][a-zA-Z']{2,14} has become more resistant\.)$`,
			WornOffPattern:    `^Your resistance fades\.$`,
			TimerType:         TimerTypeBuff,
			TimerDurationSecs: 300, // 50 ticks
			SpellID:           4585,
			CooldownSecs:      1800,
			DedupKey:          "disc_resistant",
			PackName:          packName,
			Actions:           []Action{},
		},
		{
			Name:    "Fearless Discipline",
			Enabled: true,
			// Self-cast: "Your will drives fear from your mind."
			// Other-cast: "<name>'s eyes gleam with iron will."
			Pattern:           `^(?:Your will drives fear from your mind\.|[A-Z][a-zA-Z']{2,14}'s eyes gleam with iron will\.)$`,
			WornOffPattern:    `^The specter of fear returns to your mind\.$`,
			TimerType:         TimerTypeBuff,
			TimerDurationSecs: 60, // 10 ticks
			SpellID:           4587,
			CooldownSecs:      1800,
			DedupKey:          "disc_fearless",
			PackName:          packName,
			Actions:           []Action{},
		},
	}
}

// Shared crowd-control break alert patterns. EQ's worn-off lines read the
// same for every class, so the break alerts are defined once and shipped by
// every pack that wants them (class packs + the Spell Breaks pack), each
// carrying a stable dedup_key like the shared melee disciplines — install
// any combination of those packs and exactly one copy exists; uninstalling
// one promotes a copy from another still-installed pack.
//
// Charm is special: EQ emits the same generic line for every charm spell —
// "Your charm spell has worn off." (lowercase 'charm', regardless of whether
// the underlying spell was Charm, Beguile, Boltran`s Agacerie, etc.). The
// per-name alternates are kept as a defensive tail for any charm that does
// log under its own name (e.g. bard song charms).
//
// Spell-name apostrophes are matched as [`'] — quarm.db mixes backtick and
// ASCII apostrophe across rows (it carries both "Boltran`s Agacerie" and
// "Boltran's Agacerie"). Defined as double-quoted consts because of the
// backtick.
// Each list is the full set of player-castable spells with that effect in
// quarm.db (SPA 22 charm / 99 root / 3 snare / 31 mez, any class level
// 1–60), audited 2026-06-11. Self-only root buffs (Treeform, Illusion:
// Tree, Spirit of Ash/Oak) are deliberately absent — their fade is not a
// loose mob. Greater Fetter (61) and Word of Terris (65) are PoP spells
// included ahead of the expansion launch; Dominating Gaze is the level-254
// spell the Dire Charm AA casts. Undead Pact / Entrancing Lights /
// Elnerick's Entombment of Ice / Insidious Retrogression are Quarm-custom
// spells.
const (
	charmBreakPattern = "^Your (?:charm|Charm|Beguile|Cajoling Whispers|Allure|Boltran[`']s Agacerie|Dictate|Befriend Animal|Charm Animals|Beguile Plants|Beguile Animals|Allure of the Wild|Call of Karana|Tunare[`']s Request|Dominate Undead|Beguile Undead|Cajole Undead|Thrall of Bones|Enslave Death|Word of Terris|Undead Pact|Dominating Gaze|Solon[`']s Bewitching Bravura|Solon[`']s Song of the Sirens) spell has worn off\\.$"
	rootBreakPattern  = "^Your (?:Root|Instill|Fetter|Greater Fetter|Paralyzing Earth|Immobilize|Grasping Roots|Ensnaring Roots|Enveloping Roots|Engulfing Roots|Engorging Roots|Entrapping Roots|Hungry Earth|Elnerick[`']s Entombment of Ice) spell has worn off\\.$"
	snareBreakPattern = "^Your (?:Snare|Ensnare|Tangling Weeds|Atol[`']s Spectral Shackles|Engulfing Darkness|Dooming Darkness|Cascading Darkness|Clinging Darkness|Devouring Darkness|Bonds of Force|Bonds of Tunare|Insidious Retrogression|Largo[`']s Absonant Binding|Selo[`']s Consonant Chain|Selo[`']s Assonant Strane|Song of Midnight) spell has worn off\\.$"
	// mezWornOffPattern is only used as an ExcludePattern on the Spell Breaks
	// catch-all — mez break alerts stay class-specific (Enchanter, Bard,
	// Necromancer). Kept in sync with the union of those packs' Mez Broke
	// patterns so a line is suppressed here exactly when a class alert
	// covers it; lull-type spells (Wake of Tranquility, Lugubrious Lament)
	// are not mezzes and fall through to the generic worn-off overlay.
	mezWornOffPattern = "^Your (?:Mesmerize|Mesmerization|Enthrall|Entrance|Dazzle|Fascination|Entrancing Lights|Glamour of Kintaz|Rapture|Ancient: Eternal Rapture|Screaming Terror|Kelin[`']s Lucid Lullaby|Crission[`']s Pixie Strike|Sionachie[`']s Dreams|Song of Twilight|Dreams of Ayonae|Ancient: Lullaby of Shadow) spell has worn off\\.$"
	// petWornOffPattern matches a pet buff wearing off ("Your pet's <Spell>
	// spell has worn off."). It's both its own "Pet Spell Worn Off" trigger and
	// an ExcludePattern on the generic "Spell Worn Off" catch-all, so a pet line
	// fires only the pet trigger and never the player one.
	petWornOffPattern = "^Your pet's (.+) spell has worn off\\.$"
)

// sharedCharmBreak returns the shared charm-break alert. Pack name is
// parameterized so the trigger row records which pack installed it (used by
// promote-on-uninstall to find a fallback).
func sharedCharmBreak(packName string) Trigger {
	return Trigger{
		Name:     "Charm Broke",
		Enabled:  true,
		Pattern:  charmBreakPattern,
		DedupKey: "charm_broke",
		PackName: packName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "CHARM BROKE!", DurationSecs: 6, Color: "#ff0000"},
			{Type: ActionTextToSpeech, Text: "Charm broke", Volume: 1.0},
		},
	}
}

// sharedRootBreak returns the shared root-break alert. The worn-off line
// fires both on damage-induced break and on natural expiry — either way the
// mob is loose.
func sharedRootBreak(packName string) Trigger {
	return Trigger{
		Name:     "Root Broke",
		Enabled:  true,
		Pattern:  rootBreakPattern,
		DedupKey: "root_broke",
		PackName: packName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "ROOT BROKE!", DurationSecs: 5, Color: "#ff6633"},
			{Type: ActionTextToSpeech, Text: "Root broke", Volume: 1.0},
		},
	}
}

// sharedSnareBreak returns the shared snare-break alert.
func sharedSnareBreak(packName string) Trigger {
	return Trigger{
		Name:     "Snare Broke",
		Enabled:  true,
		Pattern:  snareBreakPattern,
		DedupKey: "snare_broke",
		PackName: packName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "SNARE BROKE!", DurationSecs: 5, Color: "#ff8833"},
			{Type: ActionTextToSpeech, Text: "Snare broke", Volume: 1.0},
		},
	}
}

// EnchanterPack returns the pre-built enchanter trigger pack: critical
// crowd-control break alerts (mez/charm/root), mez/charm immunity alerts,
// and timer-creating triggers for the standard enchanter
// buff/debuff/mez lines. Generic spell resist/interrupt overlays live in
// the General Triggers pack — they are not class-specific.
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
		Description: "CC break + cast-failure alerts plus spell timers for the enchanter buff (VoG, KEI, Gift of Brilliance, IS, GRM, Speed of the Shissar/Brood), debuff (Tashanian, Cripple, Asphyxiate), root (Root, Fetter, Greater Fetter), mez (Mesmerize, Mesmerization, Dazzle, Enthrall, Entrance, Glamour of Kintaz, Rapture / Ancient: Eternal Rapture), charm (Charm, Beguile, Cajoling Whispers, Allure, Dictate, Boltran's Agacerie), and pacify (Lull, Calm, Soothe, Pacify, Wake of Tranquility) lines.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			{
				// Mezzes only: Instill is a root (sharedRootBreak covers it —
				// listing it here double-fired MEZ BROKE on top of ROOT
				// BROKE) and Wake of Tranquility is a lull, not a mez.
				// Fascination and Entrancing Lights (Quarm-custom) are the
				// enchanter AE mezzes.
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  `Your (?:Mesmerize|Mesmerization|Enthrall|Entrance|Dazzle|Fascination|Entrancing Lights|Glamour of Kintaz|Rapture|Ancient: Eternal Rapture) spell has worn off\.`,
				PackName: "Enchanter",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5, Color: "#ff4444"},
					// "Mezz" (not "Mez") so Windows SAPI pronounces it as the EQ term
					// instead of the prefix "mehz-". Pattern and overlay text remain "Mez".
					{Type: ActionTextToSpeech, Text: "Mezz broke", Volume: 1.0},
				},
			},
			// Charm/root breaks are the shared cross-pack alerts (see
			// sharedCharmBreak for why charm matches the generic lowercase
			// "Your charm spell has worn off." line).
			sharedCharmBreak("Enchanter"),
			sharedRootBreak("Enchanter"),

			// ── Immunities ──────────────────────────────────────────────
			// Generic Spell Resisted / Spell Interrupted overlays live in
			// the General Triggers pack — installing both packs without
			// dedup would fire two overlays per event.
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
				Name:              "Gift of Brilliance",
				Enabled:           true,
				Pattern:           `^(?:Your thoughts begin to race and flow faster\.|[A-Z][a-zA-Z']{2,14} appears to be staring into nothingness\.)$`,
				WornOffPattern:    `^Your gift of brilliance fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 6000,
				SpellID:           1410,
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
				Name:           "Tashanian",
				Enabled:        true,
				Pattern:        `^(?:You hear the barking of Tashania\.|` + npcNameClass + ` glances nervously about\.)$`,
				WornOffPattern: `^The barking fades\.$`,
				TimerType:      TimerTypeDetrimental,
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
				Name:           "Cripple",
				Enabled:        true,
				Pattern:        `^(?:You have been crippled\.|` + npcNameClass + ` has been crippled\.)$`,
				WornOffPattern: `^You feel your strength return\.$`,
				TimerType:      TimerTypeDetrimental,
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
				Pattern:           `^(?:You feel a shortness of breath\.|` + npcNameClass + ` begins to choke\.)$`,
				WornOffPattern:    `^You can breathe again\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           1703,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Root (merged timer) ──────────────────────────────────────
			// All three enchanter roots share "<name> adheres to the ground."
			// land text, so the merged trigger matches "You begin casting
			// <SpellName>." — one pattern row per spell, each carrying its
			// own duration/spell-id override. TimerKeyCapture keys the
			// countdown by the captured spell name, so each root runs an
			// independent timer; the worn-off pattern captures the same name
			// (one group covering both the worn-off and resist forms) to
			// clear the right one.
			{
				Name:    "Root",
				Enabled: true,
				Pattern: `^You begin casting (Root)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Fetter)\.$`, Enabled: true, TimerDurationSecs: 180, SpellID: 1633},
					{Pattern: `^You begin casting (Greater Fetter)\.$`, Enabled: true, TimerDurationSecs: 180, SpellID: 3194},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Greater Fetter|Fetter|Root) spell(?: has worn off)?\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 48,
				SpellID:           230,
				TimerKeyCapture:   "1",
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Mez (merged timer) ───────────────────────────────────────
			// Mesmerize / Mesmerization / Dazzle share the "<name> has been
			// mesmerized." land text but have different base durations
			// (24 / 24 / 96 s), so the merged trigger matches "You begin
			// casting <SpellName>." — one pattern row per spell with its own
			// duration/spell-id. The trade-off is that the timer starts
			// ~2-3 s before the spell actually lands; on a resist the
			// WornOffPattern catches it via the spell-specific resist line so
			// the stale timer clears. TimerKeyCapture keys each countdown by
			// the captured spell name — which also keeps the spelltimer
			// engine's deferred-render handling for these three spells
			// working (it's keyed by spell name).
			//
			// Enthrall / Entrance / Glamour of Kintaz / Rapture stay separate
			// below: they match on unique land text, which doesn't contain
			// the spell name to capture.
			{
				Name:    "Mez",
				Enabled: true,
				Pattern: `^You begin casting (Mesmerize)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Mesmerization)\.$`, Enabled: true, TimerDurationSecs: 24, SpellID: 307},
					{Pattern: `^You begin casting (Dazzle)\.$`, Enabled: true, TimerDurationSecs: 96, SpellID: 190},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Mesmerization|Mesmerize|Dazzle) spell(?: has worn off)?\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 24,
				SpellID:           292,
				TimerKeyCapture:   "1",
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Enthrall",
				Enabled:           true,
				Pattern:           `^(?:You have been enthralled\.|` + npcNameClass + ` has been enthralled\.)$`,
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
				Pattern:           `^(?:You have been entranced\.|` + npcNameClass + ` has been entranced\.)$`,
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
				Pattern:           `^(?:You are mesmerized by the Glamour of Kintaz\.|` + npcNameClass + ` has been mesmerized by the Glamour of Kintaz\.)$`,
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
				Pattern:           `^(?:You swoon, overcome by rapture\.|` + npcNameClass + ` swoons in raptured bliss\.)$`,
				WornOffPattern:    `^Your (?:Rapture|Ancient: Eternal Rapture) spell has worn off\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 42,
				SpellID:           1692,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Charm (merged timer) ─────────────────────────────────────
			// Every enchanter charm has an empty cast_on_other (the caster
			// never sees a "<name> has been charmed." line in their log) and
			// the charm line shares "You have been charmed." / "You are no
			// longer charmed." across spells with very different durations,
			// so the merged trigger matches on "You begin casting
			// <SpellName>.". Resists clear the stale timer via the
			// spell-specific resist line.
			// Charm/Beguile/Cajoling Whispers/Allure/Boltran's Agacerie all
			// share spells_new buffduration=205 / buffdurationformula=10. With
			// formula 10 = min(level*3 + 10, base), level 60 yields 190 ticks
			// = 1140s. Old formula 10 (`min(level, base)`) returned 60 ticks
			// = 360s, which is what these were calibrated against; updated in
			// lockstep with the duration.go fix.
			// One merged trigger covers the five standard charms;
			// TimerKeyCapture keys each countdown by the captured spell name.
			// Dictate stays separate below — its reuse cooldown (CooldownSecs)
			// is trigger-level and would otherwise be shared by the whole line.
			{
				Name:    "Charm",
				Enabled: true,
				Pattern: `^You begin casting (Charm)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Beguile)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 182},
					{Pattern: `^You begin casting (Cajoling Whispers)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 183},
					{Pattern: `^You begin casting (Allure)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 184},
					// quarm.db carries both apostrophe spellings (1705 backtick,
					// 1706 ASCII); accept either so the captured key always matches
					// what the log emits. Double-quoted for the backtick.
					{Pattern: "^You begin casting (Boltran[`']s Agacerie)\\.$", Enabled: true, TimerDurationSecs: 1140, SpellID: 1706},
				},
				WornOffPattern:    "^Your (?:target resisted the )?(Cajoling Whispers|Boltran[`']s Agacerie|Beguile|Allure|Charm) spell(?: has worn off)?\\.$",
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           300,
				TimerKeyCapture:   "1",
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
				CooldownSecs:      300,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},

			// ── Pacify line (merged timer) ───────────────────────────────
			// Lull / Calm / Soothe / Pacify / Wake of Tranquility share most
			// of their cast text and have empty spell_fades, so the merged
			// trigger matches "You begin casting <SpellName>." with one
			// pattern row per spell. TimerKeyCapture keys each countdown by
			// the captured spell name; the worn-off pattern captures the same
			// name from the caster's worn-off line or the spell-specific
			// resist line.
			// Pacify's 360s = 6 minutes per PQDI (spell 45, buffduration=60
			// ticks). EQMac formula 8 = min(level+10, base); at level 60
			// that's the full 60-tick base. Was 720 (from the modern-EQEmu
			// reading); kept in lockstep with spelltimer.CalcDurationTicks.
			{
				Name:    "Pacify",
				Enabled: true,
				Pattern: `^You begin casting (Lull)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Calm)\.$`, Enabled: true, TimerDurationSecs: 126, SpellID: 47},
					{Pattern: `^You begin casting (Soothe)\.$`, Enabled: true, TimerDurationSecs: 450, SpellID: 501},
					{Pattern: `^You begin casting (Pacify)\.$`, Enabled: true, TimerDurationSecs: 360, SpellID: 45},
					{Pattern: `^You begin casting (Wake of Tranquility)\.$`, Enabled: true, TimerDurationSecs: 126, SpellID: 1541},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Wake of Tranquility|Soothe|Pacify|Lull|Calm) spell(?: has worn off)?\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           208,
				TimerKeyCapture:   "1",
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
// Karn) lines. Generic resist/interrupt overlays are intentionally
// omitted — they live in the General Triggers pack.
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
				CooldownSecs:      90,
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
				CooldownSecs:      900,
				PackName:          "Cleric",
				Actions:           []Action{},
			},

			// ── Target debuff (timer) ────────────────────────────────────
			{
				Name:              "Mark of Karn",
				Enabled:           true,
				Pattern:           `^(?:Your skin gleams with a pure aura\.|` + npcNameClass + `'s skin gleams with a pure aura\.)$`,
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
// intentionally omitted — they live in the General Triggers pack.
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
			sharedRootBreak("Druid"),
			sharedSnareBreak("Druid"),

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
				Pattern:           `^(?:You are immolated by blazing flames\.|` + npcNameClass + ` is immolated by blazing flames\.)$`,
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
				Pattern:           `^(?:You feel the pain of a million stings\.|` + npcNameClass + ` is engulfed by a swarm of deadly insects\.)$`,
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
				Pattern:           `^(?:You are ensnared\.|` + npcNameClass + ` has been ensnared\.)$`,
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
				Pattern:           `^(?:Your feet become entwined\.|` + npcNameClass + ` is entrapped by roots\.)$`,
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
// General Triggers pack.
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
				Pattern:           `^(?:You feel drowsy\.|` + npcNameClass + ` yawns\.)$`,
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
				Pattern:           `^(?:You're motions slow as a plague of insects chew at your skin\.|` + npcNameClass + `'s motions slow as a plague of insects chews at their skin\.)$`,
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
				CooldownSecs:      180,
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
// Hands overlay alert and timer-creating triggers for Immobilize, both
// paladin disciplines (Holyforge, Sanctification), and the two shared
// melee disciplines (Resistant, Fearless). Pacify (Spell 45) and Divine
// Aura are intentionally omitted — they live in the Enchanter and Cleric
// packs respectively. Instant stuns (Stun, Force, Force of Akilae) are
// also skipped: no duration to track and generic resist alerts already
// live in the Enchanter pack.
func PaladinPack() TriggerPack {
	return TriggerPack{
		PackName:    "Paladin",
		Class:       ClassPtr(ClassPaladin),
		Description: "Lay on Hands alert, root break alert, and spell timers for Immobilize, Holyforge Discipline, Sanctification Discipline, plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
			// ── Emergency burst (overlay alert) ──────────────────────────
			// Lay on Hands is instant with a 72-minute recast; the cast
			// message ("Your hands shimmer with holy light.") only fires
			// when the paladin uses the ability, making it a clean alert
			// without timer support.
			{
				Name:         "Lay on Hands",
				Enabled:      true,
				Pattern:      `^Your hands shimmer with holy light\.$`,
				CooldownSecs: 4320,
				PackName:     "Paladin",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "LAY ON HANDS!", DurationSecs: 5, Color: "#ffdd33"},
				},
			},

			// ── Crowd-control break ──────────────────────────────────────
			sharedRootBreak("Paladin"),

			// ── Root (timer) ────────────────────────────────────────────
			{
				Name:              "Immobilize",
				Enabled:           true,
				Pattern:           `^(?:Your feet adhere to the ground\.|` + npcNameClass + ` adheres to the ground\.)$`,
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
				CooldownSecs:      4050,
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
				CooldownSecs:      4050,
				PackName:          "Paladin",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Paladin")...),
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
				Pattern:           `^(?:You are engulfed by darkness\.|` + npcNameClass + ` is engulfed by darkness\.)$`,
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
				Pattern:           `^(?:Your stomach begins to cramp\.|` + npcNameClass + ` doubles over in pain\.)$`,
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
				CooldownSecs:      540,
				PackName:          "Shadowknight",
				Actions:           []Action{},
			},

			// ── DoT debuff (timer) ───────────────────────────────────────
			{
				Name:              "Asystole",
				Enabled:           true,
				Pattern:           `^(?:Your heart stops\.|` + npcNameClass + ` clutches their chest\.)$`,
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
				CooldownSecs:      3780,
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
		Description: "Spell timers for all 11 warrior disciplines: Defensive, Evasive, Aggressive, Furious, Precision, Charge, Mighty Strike, Fellstrike, Fortitude, plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
			// ── Stance disciplines (timers) ─────────────────────────────
			// Defensive / Aggressive / Evasive / Precision share the fade
			// text "You return to your normal fighting style." but have
			// unique cast messages, so each is distinguishable on its
			// land anchor.
			{
				Name:              "Defensive Discipline",
				Enabled:           true,
				Pattern:           `^(?:You assume a defensive fighting style\.|[A-Z][a-zA-Z']{2,14} assumes a defensive fighting style\.)$`,
				WornOffPattern:    `^You return to your normal fighting style\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 180,
				SpellID:           4499,
				CooldownSecs:      630,
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
				CooldownSecs:      468,
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
				CooldownSecs:      1800,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:    "Precision Discipline",
				Enabled: true,
				// The DB carries the cast_on_other as "'s assumes a
				// precise fighting style." (literal apostrophe-s plus
				// "assumes" — a known dump quirk); pattern allows both
				// the apostrophe-s and a plain-name form.
				Pattern:           `^(?:You assume a precise fighting style\.|[A-Z][a-zA-Z']{2,14}'?s? ?assumes a precise fighting style\.)$`,
				WornOffPattern:    `^You return to your normal fighting style\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 60,
				SpellID:           4501,
				CooldownSecs:      1500,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			// ── Offensive disciplines (timers) ──────────────────────────
			{
				Name:              "Furious Discipline",
				Enabled:           true,
				Pattern:           `^(?:A consuming rage takes over your weapons\.|[A-Z][a-zA-Z']{2,14}'s body is consumed in rage\.)$`,
				WornOffPattern:    `^The rage leaves you\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4674,
				CooldownSecs:      2400,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Charge Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your focus becomes perfect\.|[A-Z][a-zA-Z']{2,14}'s focus becomes perfect\.)$`,
				WornOffPattern:    `^Your perfect focus fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4672,
				CooldownSecs:      1500,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Mighty Strike Discipline",
				Enabled:           true,
				Pattern:           `^(?:You feel like a killing machine\.|[A-Z][a-zA-Z']{2,14} feels like a killing machine\.)$`,
				WornOffPattern:    `^Your killer instinct fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4514,
				CooldownSecs:      1500,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Fellstrike Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons strike true\.|[A-Z][a-zA-Z']{2,14}'s weapons strike true\.)$`,
				WornOffPattern:    `^Your weapons lose their accuracy\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4675,
				CooldownSecs:      1500,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			// ── Defensive (cooldown-style) disciplines ──────────────────
			{
				Name:    "Fortitude Discipline",
				Enabled: true,
				// DB grammar quirk on cast_on_you: "You instincts take
				// over..." (missing 'r'). Matched verbatim so the live
				// log line — which mirrors the DB — fires the trigger.
				Pattern:           `^(?:You instincts take over as you avoid every attack\.|[A-Z][a-zA-Z']{2,14}'s body begins to move with instinctual grace\.)$`,
				WornOffPattern:    `^Your battle instinct leaves you\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4670,
				CooldownSecs:      2400,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Warrior")...),
	}
}

// MonkPack returns the pre-built monk trigger pack: a Feign Death skill
// overlay alert and timers for all 8 monk disciplines plus the two shared
// melee disciplines (Resistant, Fearless). Mend is intentionally omitted
// — its exact log strings vary by classic-era client and we'd risk false
// positives without verifying against real Quarm logs.
//
// The Feign Death entry overlaps with the Shadowknight pack — installing
// both packs will cause duplicate overlay fires on FD; users can disable
// one in the trigger editor if needed.
func MonkPack() TriggerPack {
	return TriggerPack{
		PackName:    "Monk",
		Class:       ClassPtr(ClassMonk),
		Description: "Feign Death alert plus spell timers for all 8 monk disciplines (Stonestance, Innerflame, Thunderkick, Ashenhand, Silentfist, Whirlwind, Voiddance, Hundred Fists) plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
			// ── Feign Death skill (overlay alert + reuse CD) ─────────────
			// The FD skill has a 9s base reuse on Quarm (the client emits
			// "You must wait longer before you can feign death." when used
			// early; the Rapid Feign AA shortens it — not auto-applied here).
			// CooldownSecs shows a brief "Feign Death CD" countdown on the
			// buff overlay alongside the text alert, like Lay on Hands.
			{
				Name:         "Feign Death",
				Enabled:      true,
				Pattern:      `^You feign death\.$`,
				CooldownSecs: 9,
				PackName:     "Monk",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "FEIGN DEATH", DurationSecs: 4, Color: "#888888"},
				},
			},

			// ── Defensive/utility disciplines (timers) ──────────────────
			{
				Name:              "Stonestance Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your body becomes one with the earth\.|[A-Z][a-zA-Z']{2,14}'s feet become one with the earth\.)$`,
				WornOffPattern:    `^You are no longer one with the earth\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4510,
				CooldownSecs:      234,
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
				CooldownSecs:      1320,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:    "Voiddance Discipline",
				Enabled: true,
				// DB carries fade as "You movements return to normal."
				// (a known dump quirk — missing 'r'); matched verbatim.
				Pattern:           `^(?:You become untouchable\.|[A-Z][a-zA-Z']{2,14} becomes untouchable\.)$`,
				WornOffPattern:    `^You movements return to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4502,
				CooldownSecs:      2400,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Whirlwind Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your instincts take over as you turn aside every attack\.|[A-Z][a-zA-Z']{2,14}'s face becomes twisted with fury\.)$`,
				WornOffPattern:    `^Your fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4509,
				CooldownSecs:      2400,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			// ── Offensive disciplines (timers) ──────────────────────────
			{
				Name:              "Hundred Fists Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your fists begin to blur\.|[A-Z][a-zA-Z']{2,14}'s fists begin to blur\.)$`,
				WornOffPattern:    `^Your hands slow down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4513,
				CooldownSecs:      1320,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			// Thunderkick / Ashenhand / Silentfist are single-use "next
			// strike" discs — they consume on the first melee hit,
			// expiring the buff via spell_fades. The 72-second timer is
			// the natural max lifetime (formula 50 = level/5 ticks);
			// using before then clears it via the worn_off line.
			{
				Name:              "Thunderkick Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your feet glow with mystic power\.|[A-Z][a-zA-Z']{2,14}'s feet glow with mystic power\.)$`,
				WornOffPattern:    `^The glow fades from your feet\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 72,
				SpellID:           4511,
				CooldownSecs:      600,
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
				CooldownSecs:      1800,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Silentfist Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your body is filled with silent fury\.|[A-Z][a-zA-Z']{2,14}'s body is filled with silent fury\.)$`,
				WornOffPattern:    `^The silent fury fades away\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 72,
				SpellID:           4507,
				CooldownSecs:      600,
				PackName:          "Monk",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Monk")...),
	}
}

// RoguePack returns the pre-built rogue trigger pack: timer-creating
// triggers for all 6 rogue disciplines plus the two shared melee
// disciplines (Resistant, Fearless). Escape (AA) and Evasion (skill) are
// intentionally omitted — their exact log strings aren't in spells_new
// and we'd risk false positives without verifying against real Quarm logs.
func RoguePack() TriggerPack {
	return TriggerPack{
		PackName:    "Rogue",
		Class:       ClassPtr(ClassRogue),
		Description: "Spell timers for all 6 rogue disciplines (Duelist, Deadeye, Counterattack, Nimble, Kinesthetics, Blinding Speed) plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
			// ── Offensive disciplines (timers) ──────────────────────────
			{
				Name:              "Duelist Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your muscles quiver with power\.|[A-Z][a-zA-Z']{2,14}'s eyes gleam with energy\.)$`,
				WornOffPattern:    `^Your fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4676,
				CooldownSecs:      1320,
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
				CooldownSecs:      1200,
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
				CooldownSecs:      2400,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			// ── Evasive / speed disciplines (timers) ────────────────────
			{
				Name:    "Nimble Discipline",
				Enabled: true,
				// DB carries fade as "You movements return to normal."
				// (missing 'r' — a known dump quirk shared with
				// Voiddance); matched verbatim.
				Pattern:           `^(?:You bounce about nimbly\.|[A-Z][a-zA-Z']{2,14} bounces about nimbly\.)$`,
				WornOffPattern:    `^You movements return to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4515,
				CooldownSecs:      1260,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Kinesthetics Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your arms feel alive with mystic energy\.|[A-Z][a-zA-Z']{2,14}'s arms feel alive with mystic energy\.)$`,
				WornOffPattern:    `^The mystic energy fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4517,
				CooldownSecs:      300,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:    "Blinding Speed Discipline",
				Enabled: true,
				// "Your hands speeds up." is the DB cast_on_you verbatim
				// (grammar quirk — singular verb with plural subject).
				Pattern:           `^(?:Your hands speeds up\.|[A-Z][a-zA-Z']{2,14}'s hands speeds up\.)$`,
				WornOffPattern:    `^Your hands slow down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4677,
				CooldownSecs:      1200,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Rogue")...),
	}
}

// RangerPack returns the pre-built ranger trigger pack: timer-creating
// triggers for both ranger disciplines (Trueshot, Weapon Shield), the
// Call of Sky / Call of Fire elemental weapon-proc buffs, the Flame Lick
// aggro tool, and the two shared melee disciplines (Resistant, Fearless).
// Ensnare and Entrapping Roots are intentionally omitted — they're
// already covered by the Druid pack with the same SpellIDs.
func RangerPack() TriggerPack {
	return TriggerPack{
		PackName:    "Ranger",
		Class:       ClassPtr(ClassRanger),
		Description: "Snare break alert plus spell timers for Trueshot Discipline, Weapon Shield Discipline, Call of Sky, Call of Fire, Flame Lick, plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
			// ── Crowd-control break ──────────────────────────────────────
			// Covers the ranger snare line (Snare, Ensnare, Tangling
			// Weeds, Bonds of Tunare).
			sharedSnareBreak("Ranger"),

			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Trueshot Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your bow crackles with natural energy\.|[A-Z][a-zA-Z']{2,14}'s bow crackles with natural energy\.)$`,
				WornOffPattern:    `^The natural energy fades from your bow\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 120,
				SpellID:           4506,
				CooldownSecs:      4050,
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
				CooldownSecs:      4050,
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
				Pattern:           `^(?:You are surrounded by flickering flames\.|` + npcNameClass + ` is surrounded by flickering flames\.)$`,
				WornOffPattern:    `^The flames are extinguished\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 48,
				SpellID:           239,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Ranger")...),
	}
}

// BardPack returns the pre-built bard trigger pack: timer-creating
// triggers for the standard bard mana/HP/melee/resist song line
// (Cantata of Replenishment, Warsong of Zek, Niv's Melody of
// Preservation, Psalm of Veeshan, Elemental Rhythms, Guardian Rhythms)
// plus the mez (Kelin's Lucid Lullaby), lull (Kelin's Lugubrious
// Lament), charm (Solon's Bewitching Bravura), and slow (Largo's
// Absonant Binding) target debuffs. Also tracks the Tiny Cloak of
// Darkest Night clicky (Shroud of Stealth), a common bard utility.
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
		Description: "Crowd-control break alerts (mez, charm, snare) plus spell timers for Cantata of Replenishment, Warsong of Zek, Niv's Melody of Preservation, Psalm of Veeshan, Elemental Rhythms, Guardian Rhythms, Kelin's Lucid Lullaby / Lugubrious Lament, Solon's Bewitching Bravura, Largo's Absonant Binding, and the Tiny Cloak of Darkest Night clicky (Shroud of Stealth).",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Bard songs pulse every tick, so the worn-off line only fires
			// once the bard stops singing (or the song's natural duration
			// elapses post-stop). Either case means the mez/charm is gone.
			{
				// All bard mez songs. Apostrophes are [`'] — the log emits
				// the backtick form (Kelin`s), so a plain ASCII apostrophe
				// here never matches. Double-quoted for the backtick.
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  "Your (?:Kelin[`']s Lucid Lullaby|Crission[`']s Pixie Strike|Sionachie[`']s Dreams|Song of Twilight|Dreams of Ayonae|Ancient: Lullaby of Shadow) spell has worn off\\.",
				PackName: "Bard",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5, Color: "#ff4444"},
					// "Mezz" (not "Mez") so Windows SAPI pronounces it as the EQ term
					// instead of the prefix "mehz-". Pattern and overlay text remain "Mez".
					{Type: ActionTextToSpeech, Text: "Mezz broke", Volume: 1.0},
				},
			},
			// Shared charm break — covers the generic charm worn-off line
			// plus the bard charm songs by name (see sharedCharmBreak).
			sharedCharmBreak("Bard"),
			// Shared snare break — covers the bard snare songs (Selo's
			// Consonant Chain / Assonant Strane, Song of Midnight, Largo's
			// Absonant Binding). Like the mez/charm breaks above, the
			// worn-off line only fires once the song actually fades.
			sharedSnareBreak("Bard"),

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
				TimerDurationSecs: 18,
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
				TimerDurationSecs: 18,
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
				TimerDurationSecs: 18,
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
				TimerDurationSecs: 18,
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
				TimerDurationSecs: 18,
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
				TimerDurationSecs: 18,
				SpellID:           709,
				PackName:          "Bard",
				Actions:           []Action{},
			},

			// ── Mez / lull / charm / slow songs (timers) ────────────────
			{
				Name:              "Kelin's Lucid Lullaby",
				Enabled:           true,
				Pattern:           `^(?:You feel quite drowsy\.|` + npcNameClass + `'s head nods\.)$`,
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
				Pattern:           `^(?:You feel a strong sense of loss\.|` + npcNameClass + ` looks sad\.)$`,
				WornOffPattern:    `^You no longer feel sad\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           728,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Solon's Bewitching Bravura",
				Enabled:           true,
				Pattern:           `^(?:You are captivated by the bewitching tune\.|` + npcNameClass + `'s eyes glaze over\.)$`,
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
				Pattern:           `^(?:Strands of solid music bind your body\.|` + npcNameClass + ` is bound by strands of solid music\.)$`,
				WornOffPattern:    `^The strands of fade away\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           1751,
				PackName:          "Bard",
				Actions:           []Action{},
			},

			// ── Item clicky (timer) ─────────────────────────────────────
			// Tiny Cloak of Darkest Night (item 28766) clicks Shroud of
			// Stealth (spell 2039) at clicklevel 55. Formula 4 base 20:
			// 55*2 + 20 = 130, capped at base*3 = 60 ticks → 360s.
			//
			// Shroud of Stealth has classes1..15 = 255 (no class can cast
			// it as a spell), so the clicky-class-match guard in buffmod
			// already skips item/AA duration extensions; bards are also
			// universally exempt from duration extensions. Either guard
			// alone keeps the timer at the base 360s.
			{
				Name:              "Shroud of Stealth",
				Enabled:           true,
				Pattern:           `^(?:You hide yourself from view\.|[A-Z][a-zA-Z']{2,14} disappears into the shadows\.)$`,
				WornOffPattern:    `^You are no longer hidden from view\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           2039,
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
		Description: "Mez/charm/snare break alerts plus spell timers for Arch Lich, Splurt, Ignite Blood, Pyrocruor, Bond of Death, and Harmshield.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Charm covers the undead-charm line (Dominate/Beguile/Cajole
			// Undead, Thrall of Bones, Enslave Death); snare covers the
			// darkness DoT-snares (Clinging through Devouring Darkness).
			sharedCharmBreak("Necromancer"),
			sharedSnareBreak("Necromancer"),
			// Screaming Terror is the necro single-target mez; mirrors the
			// Enchanter/Bard packs' Mez Broke alert (and is excluded from
			// the Spell Breaks catch-all via mezWornOffPattern).
			{
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  `Your Screaming Terror spell has worn off\.`,
				PackName: "Necromancer",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5, Color: "#ff4444"},
					// "Mezz" (not "Mez") so Windows SAPI pronounces it as the EQ term
					// instead of the prefix "mehz-". Pattern and overlay text remain "Mez".
					{Type: ActionTextToSpeech, Text: "Mezz broke", Volume: 1.0},
				},
			},

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
				Pattern:           `^(?:Your body begins to splurt\.|` + npcNameClass + `'s body begins to splurt\.)$`,
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
				Pattern:           `^(?:You feel your life force drain away\.|` + npcNameClass + ` staggers\.)$`,
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
				CooldownSecs:      600,
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
		Description: "Harvest alert, root/snare break alerts, and spell timers for Manaskin and Atol's Spectral Shackles.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Atol's Spectral Shackles is a snare (SPA 3 movement-speed
			// -35%), not a root — despite the flavour text about feet
			// being bound. Label the break accordingly. The root break
			// covers the wizard root line (Root, Instill, Fetter,
			// Paralyzing Earth, Elnerick's Entombment of Ice).
			sharedSnareBreak("Wizard"),
			sharedRootBreak("Wizard"),

			// ── Mana tool (overlay alert) ────────────────────────────────
			// Harvest is instant with a 10-minute recast; the cast message
			// "You gather mana from your surroundings." only fires when
			// the wizard uses the spell, making it a clean alert without
			// timer support.
			{
				Name:         "Harvest",
				Enabled:      true,
				Pattern:      `^You gather mana from your surroundings\.$`,
				CooldownSecs: 600,
				PackName:     "Wizard",
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

			// ── Snare (timer) ───────────────────────────────────────────
			// Atol's shares "Your feet come free." with the paladin's
			// Immobilize, but each trigger keys on its own timer so the
			// shared fade only clears the matching pack's timer.
			{
				Name:              "Atol's Spectral Shackles",
				Enabled:           true,
				Pattern:           `^(?:Spectral shackles bind your feet to the ground\.|` + npcNameClass + ` is shackled to the ground\.)$`,
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
		Description: "Spell timers for Spiritual Purity, Spiritual Dominion, Paragon of Spirit, Ferocity, Sha's Advantage, both beastlord disciplines (Protective Spirit, Bestial Rage), plus the shared melee disciplines Resistant and Fearless.",
		Triggers: append([]Trigger{
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
				CooldownSecs:      120,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},

			// ── Slow (timer) ────────────────────────────────────────────
			{
				Name:              "Sha's Advantage",
				Enabled:           true,
				Pattern:           `^(?:You lose your fighting edge\.|` + npcNameClass + ` loses their fighting edge\.)$`,
				WornOffPattern:    `^You regain your fighting edge\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 630,
				SpellID:           2942,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},

			// ── Disciplines (timers) ────────────────────────────────────
			{
				Name:              "Protective Spirit Discipline",
				Enabled:           true,
				Pattern:           `^(?:A protective spirit guards you\.|[A-Z][a-zA-Z']{2,14} is guarded by a protective spirit\.)$`,
				WornOffPattern:    `^The protective spirit leaves you\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4671,
				CooldownSecs:      234,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},
			{
				Name:              "Bestial Rage Discipline",
				Enabled:           true,
				Pattern:           `^(?:A bestial fury consumes you\.|[A-Z][a-zA-Z']{2,14} is consumed in a bestial fury\.)$`,
				WornOffPattern:    `^The bestial fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 30,
				SpellID:           4678,
				CooldownSecs:      1530,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Beastlord")...),
	}
}

// GeneralTriggersPack returns the pre-built class-agnostic trigger pack with
// alerts for incoming tells, deaths, spell resists, and spell interrupts.
func GeneralTriggersPack() TriggerPack {
	return TriggerPack{
		PackName:    "General Triggers",
		Description: "Class-agnostic alerts: incoming tells, deaths, spell resists, and spell interrupts.",
		Triggers: []Trigger{
			{
				Name:     "Incoming Tell",
				Enabled:  true,
				Pattern:  `\w+ tells you,`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INCOMING TELL!", DurationSecs: 5, Color: "#44aaff"},
				},
				// Suppress tells from pets and NPC merchants/bankers/trainers
				// so only real player tells fire the alert. Bazaar trader
				// PCs are indistinguishable from regular players in the log;
				// add their character names here to silence them too.
				ExcludePatterns: []string{
					`\b[Mm]aster[.!]`,               // pet command responses (Attacking ... Master., By your command, master., Following you, Master.)
					`tells you, '[Tt]hat'll be `,    // NPC merchant: selling price
					`tells you, '[Ii]'ll give you `, // NPC merchant: buying offer
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
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "YOU DIED!", DurationSecs: 8, Color: "#ff0000"},
				},
			},
			{
				Name:     "Group Member Died",
				Enabled:  true,
				Pattern:  `\w+ has been slain by`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "GROUP DEATH!", DurationSecs: 5, Color: "#ff6600"},
				},
				// "X has been slain by Y" is logged for every death in range,
				// not just group members — so a charmed/normal pet killing an
				// NPC would otherwise fire GROUP DEATH. Player names are always
				// a single capitalized word, so anything else as the victim is
				// an NPC: exclude victims that start lowercase ("a poacher",
				// "an ulthork", "the guardian") or are multi-word ("Trooper
				// Coglee", "Verina Tomb"). Single-word named NPCs (e.g.
				// "Sontalak") are textually indistinguishable from a player and
				// can still leak — that needs NPC-awareness, not regex.
				ExcludePatterns: []string{
					`^[a-z]`,
					`^\S+ \S+ has been slain by`,
				},
			},
			{
				Name:     "Spell Resist",
				Enabled:  true,
				Pattern:  `Your target resisted the (.+) spell\.`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "RESISTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
			{
				Name:     "Spell Interrupt",
				Enabled:  true,
				Pattern:  `Your(?: (.+))? spell is interrupted\.`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INTERRUPTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
		},
	}
}

// CasterAlertsPack returns cast-failure alerts useful to every casting
// class. Community-submitted (Vortikai); patterns verified against live
// Quarm logs by the submitter.
func CasterAlertsPack() TriggerPack {
	return TriggerPack{
		PackName:    "Caster Alerts",
		Description: "Cast-failure alerts for any casting class: insufficient mana, must stand to cast, and target unaffected.",
		Triggers: []Trigger{
			{
				Name:     "Insufficient Mana",
				Enabled:  true,
				Pattern:  `^Insufficient Mana to cast this spell!$`,
				PackName: "Caster Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INSUFFICIENT MANA", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
			{
				Name:     "Stand to Cast",
				Enabled:  true,
				Pattern:  `^You must be standing to cast a spell\.$`,
				PackName: "Caster Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "STAND TO CAST", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
			{
				// Spell took hold but did nothing (e.g. debuff on an immune
				// target, stacking-blocked buff).
				Name:     "Target Unaffected",
				Enabled:  true,
				Pattern:  `^Your target looks unaffected\.$`,
				PackName: "Caster Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "TARGET UNAFFECTED", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
		},
	}
}

// CritAlertsPack returns crit announcement overlays with the damage/heal
// amount spliced in via the {1} capture token.
func CritAlertsPack() TriggerPack {
	return TriggerPack{
		PackName:    "Crit Alerts",
		Description: "Crit overlays with the amount shown: critical blast (caster), exceptional heal, and melee critical hits.",
		Triggers: []Trigger{
			{
				Name:     "Critical Blast",
				Enabled:  true,
				Pattern:  `^You deliver a critical blast! \((\d+)\)$`,
				PackName: "Crit Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CRITICAL BLAST ({1})", DurationSecs: 3, Color: "#ff5555"},
				},
			},
			{
				Name:     "Exceptional Heal",
				Enabled:  true,
				Pattern:  `^You perform an exceptional heal! \((\d+)\)$`,
				PackName: "Crit Alerts",
				Actions: []Action{
					// EQ healing-text blue.
					{Type: ActionOverlayText, Text: "EXCEPTIONAL HEAL ({1})", DurationSecs: 3, Color: "#4499ff"},
				},
			},
			{
				// Quarm logs melee crits in the PQ standalone form
				// "<Name> Scores a critical hit!(62)" — capital S, no space
				// before the parens (see logparser reCritHit). {C} expands to
				// the active character so other melees' crits stay quiet.
				Name:     "Critical Hit",
				Enabled:  true,
				Pattern:  `^{C} Scores a critical hit!\((\d+)\)$`,
				PackName: "Crit Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "CRIT ({1})", DurationSecs: 2, Color: "#ff5555"},
				},
			},
		},
	}
}

// SpellBreaksPack returns the shared crowd-control break alerts plus a
// catch-all "<spell> has worn off" overlay for everything that isn't CC.
// The CC breaks carry the same dedup keys the class packs ship, so
// installing this alongside a class pack never double-fires.
func SpellBreaksPack() TriggerPack {
	return TriggerPack{
		PackName:    "Spell Breaks",
		Description: "Charm/root/snare break alerts (shared with the class packs) plus a catch-all worn-off overlay for DoTs, debuffs, and buffs you cast.",
		Triggers: []Trigger{
			sharedCharmBreak("Spell Breaks"),
			sharedRootBreak("Spell Breaks"),
			sharedSnareBreak("Spell Breaks"),
			{
				// Go's regexp has no negative lookahead, so the CC spells (and
				// pet buffs, which have their own trigger below) are carved out
				// with ExcludePatterns instead: the broad pattern matches every
				// worn-off line, and any line a more specific alert already
				// covers is suppressed here.
				Name:     "Spell Worn Off",
				Enabled:  true,
				Pattern:  `^Your (.+) spell has worn off\.$`,
				PackName: "Spell Breaks",
				ExcludePatterns: []string{
					charmBreakPattern,
					rootBreakPattern,
					snareBreakPattern,
					mezWornOffPattern,
					petWornOffPattern,
				},
				Actions: []Action{
					{Type: ActionOverlayText, Text: "{1} has worn off", DurationSecs: 4, Color: "#cccccc"},
				},
			},
			petSpellWornOff("Spell Breaks"),
		},
	}
}

// petSpellWornOff returns the "Pet Spell Worn Off" trigger. Pet buffs wearing
// off ("Your pet's <Spell> spell has worn off.") get their own trigger so
// they're distinguishable from the player's own buffs and can be toggled/styled
// separately. Shared by SpellBreaksPack and the DefaultUpdate that adds it to
// already-installed Spell Breaks packs.
func petSpellWornOff(packName string) Trigger {
	return Trigger{
		Name:     "Pet Spell Worn Off",
		Enabled:  true,
		Pattern:  petWornOffPattern,
		PackName: packName,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "Pet: {1} has worn off", DurationSecs: 4, Color: "#999999"},
		},
	}
}

// GroupAlertsPack returns group-management overlays.
func GroupAlertsPack() TriggerPack {
	return TriggerPack{
		PackName:    "Group Alerts",
		Description: "Group invites and leadership changes.",
		Triggers: []Trigger{
			{
				Name:     "Group Invite",
				Enabled:  true,
				Pattern:  `^(\w+) invites you to join a group\.$`,
				PackName: "Group Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "{1} invites you to a group", DurationSecs: 6, Color: "#44aaff"},
				},
			},
			{
				Name:     "New Group Leader",
				Enabled:  true,
				Pattern:  `^(\w+) is now the leader of your group\.$`,
				PackName: "Group Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "{1} is now the group leader", DurationSecs: 5, Color: "#44aaff"},
				},
			},
		},
	}
}

// RaidAlertsPack returns raid-event overlays: Divine Intervention procs,
// mobs gating away, and assist calls in raid chat.
func RaidAlertsPack() TriggerPack {
	return TriggerPack{
		PackName:    "Raid Alerts",
		Description: "Divine Intervention procs, mobs casting gate, and assist calls in raid chat.",
		Triggers: []Trigger{
			{
				// The DI proc line — complementary to the Cleric pack's
				// Divine Intervention buff timer (which this same line clears).
				Name:     "Divine Intervention",
				Enabled:  true,
				Pattern:  `^(\w+) has been rescued by divine intervention!$`,
				PackName: "Raid Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "DIVINE INTERVENTION — {1}", DurationSecs: 8, Color: "#ffd700"},
					{Type: ActionTextToSpeech, Text: "Divine intervention on {1}", Volume: 1.0},
				},
			},
			{
				Name:     "Mob Gating",
				Enabled:  true,
				Pattern:  `^(.+) begins to cast the gate spell\.$`,
				PackName: "Raid Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "GATE: {1}", DurationSecs: 6, Color: "#ff8800"},
					{Type: ActionTextToSpeech, Text: "{1} is gating", Volume: 1.0},
				},
			},
			{
				// "Krazak tells the raid,  'ASSIST Vox >>>'" → "ASSIST Vox", and
				// the "kill" calling style "'< --- Kill Qua Zethon Xakra -->'" →
				// "ASSIST Qua Zethon Xakra". The keyword is case-insensitive and
				// word-bounded so "skill"/"killing"/"killed" don't false-fire;
				// the target capture runs from the first letter after the keyword
				// up to a trailing decoration character (- < > | ! ', covering
				// >>>, -->, <--) or the closing quote. Double-quoted for the
				// backtick in the mob-name class.
				Name:     "Raid Assist Call",
				Enabled:  true,
				Pattern:  "(?i)^(\\w+) tells the raid,\\s*'.*?\\b(?:assist|kill)\\b\\W*([A-Za-z][A-Za-z`' ]*?)(?:\\s*[-<>|!']|$)",
				PackName: "Raid Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "ASSIST {2}", DurationSecs: 5, Color: "#ff4444"},
				},
			},
		},
	}
}

// TrackingPack returns direction-arrow overlays for the tracking skill.
// Each bearing update flashes the target name with an arrow; the short
// 2-second duration keeps stale bearings from lingering between updates.
func TrackingPack() TriggerPack {
	bearing := func(name, suffix, arrow string) Trigger {
		return Trigger{
			Name:     name,
			Enabled:  true,
			Pattern:  `^(.+) is ` + suffix + `\.$`,
			PackName: "Tracking",
			Actions: []Action{
				{Type: ActionOverlayText, Text: "{1}: " + arrow, DurationSecs: 2, Color: "#ffffff"},
			},
		}
	}
	return TriggerPack{
		PackName:    "Tracking",
		Description: "Direction arrows for the tracking skill — shows your tracked target's bearing (↑ ↗ → …) on each update, plus a lost-target alert.",
		Triggers: []Trigger{
			bearing("Ahead", "straight ahead", "↑"),
			bearing("Ahead Left", "ahead and to the left", "↖"),
			bearing("Ahead Right", "ahead and to the right", "↗"),
			bearing("Left", "to the left", "←"),
			bearing("Right", "to the right", "→"),
			bearing("Behind", "behind you", "↓"),
			bearing("Behind Left", "behind and to the left", "↙"),
			bearing("Behind Right", "behind and to the right", "↘"),
			{
				Name:     "Lost Target",
				Enabled:  true,
				Pattern:  `^You have lost your target\.$`,
				PackName: "Tracking",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "Lost tracking target", DurationSecs: 5, Color: "#ffaa00"},
				},
			},
		},
	}
}

// MiscAlertsPack returns odds and ends that don't fit a class pack: feign
// death failure, stun state, and MGB announcements in the general channel.
func MiscAlertsPack() TriggerPack {
	return TriggerPack{
		PackName:    "Misc Alerts",
		Description: "Feign death failure, stun/unstun alerts, and MGB announcements in the general channel.",
		Triggers: []Trigger{
			{
				// {C} expands to the active character, so only YOUR failed FD
				// fires — other players (or your own success) stay quiet.
				// Complements the Monk/SK packs' success alert ("You feign
				// death.").
				Name:     "Feign Death Failed",
				Enabled:  true,
				Pattern:  `^{C} has fallen to the ground\.$`,
				PackName: "Misc Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "FEIGN DEATH FAILED", DurationSecs: 5, Color: "#ff0000"},
					{Type: ActionTextToSpeech, Text: "Feign death failed", Volume: 1.0},
				},
			},
			{
				Name:     "Stunned",
				Enabled:  true,
				Pattern:  `^You are stunned!$`,
				PackName: "Misc Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "STUNNED", DurationSecs: 3, Color: "#ff4444"},
				},
			},
			{
				Name:     "Unstunned",
				Enabled:  true,
				Pattern:  `^You are unstunned\.$`,
				PackName: "Misc Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "NO LONGER STUNNED", DurationSecs: 2, Color: "#44ff88"},
				},
			},
			{
				// General-channel MGB announcements, shown verbatim via {0}
				// (the whole matched line) — the message itself names the
				// spell and time, so no need to parse them out.
				//
				// Real announcements carry a time component ("MGB KEI at
				// 3:15", "mgb in 30 min nexus", "mgb at top of the hour"),
				// so the pattern requires a digit / hour / min / now / soon
				// near the mention. Plain chatter ("mgb when?", "any mgbs?",
				// "damn I missed MGB") stays quiet. RE2 has no lookahead, so
				// the time token is an alternation on either side of "mgb".
				Name:     "MGB Announcement",
				Enabled:  true,
				Pattern:  `(?i)^\w+ tells general:\d+, '.*(?:mgb.*(?:\d|hour|min|now|soon)|\d.*mgb).*'$`,
				PackName: "Misc Alerts",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "{0}", DurationSecs: 10, Color: "#44ddff"},
				},
			},
		},
	}
}

// AllPacks returns all built-in trigger packs with default audio alerts
// applied. Class packs run through applyDefaultTimerAlerts so every timer
// trigger gets a fading/expiring TTS without each pack having to spell it
// out per-spell. The class-agnostic packs have no timers and are returned
// as-is.
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
	generalPacks := []TriggerPack{
		GeneralTriggersPack(),
		CasterAlertsPack(),
		CritAlertsPack(),
		SpellBreaksPack(),
		GroupAlertsPack(),
		RaidAlertsPack(),
		TrackingPack(),
		MiscAlertsPack(),
	}
	out := make([]TriggerPack, 0, len(classPacks)+len(generalPacks))
	for _, p := range classPacks {
		out = append(out, applyBuffTargetCapture(applyDefaultTimerAlerts(p)))
	}
	return append(out, generalPacks...)
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
	// SetCooldownSecs, when > 0, sets the trigger's reuse-cooldown timer —
	// but only if it's currently 0 (unset), so we add a missing cooldown
	// without overwriting a value the user changed by hand. 0 = leave as-is.
	SetCooldownSecs int
	// InsertTrigger, when non-nil, adds a brand-new trigger to PackName for
	// users who already have that pack installed (a fresh InstallPack would
	// wipe their customizations, so it can't be used). Skipped when the pack
	// isn't installed or a trigger of the same name already exists, so it's
	// idempotent and never resurrects a pack the user removed. TriggerName is
	// ignored for insert updates.
	InsertTrigger *Trigger
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
			// Pack was renamed from "Group Awareness" to "General Triggers"
			// by MigrateGroupAwarenessToGeneralTriggers (see store.go) — the
			// rename runs before ApplyDefaultUpdates, so this targets the
			// new name. Existing installs that already have this key in
			// pack_default_updates skip it regardless.
			Key:         "GroupAwareness:IncomingTell:pet-merchant-excludes-v1",
			PackName:    "General Triggers",
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
		{
			// 2026-06-09: the monk Feign Death trigger gained a 9s reuse
			// cooldown (it previously fired only the overlay text alert).
			// Existing installs created the row with CooldownSecs=0; set it
			// to 9 — only when still unset — so the "Feign Death CD"
			// countdown shows without a destructive pack reinstall.
			Key:             "Monk:FeignDeath:cooldown-9s-v1",
			PackName:        "Monk",
			TriggerName:     "Feign Death",
			SetCooldownSecs: 9,
		},
		{
			// 2026-06-15: pet buffs wearing off ("Your pet's <Spell> spell has
			// worn off.") used to fire the generic "Spell Worn Off" overlay,
			// indistinguishable from the player's own buffs. Exclude pet lines
			// from that trigger for existing Spell Breaks installs…
			Key:         "SpellBreaks:SpellWornOff:exclude-pet-v1",
			PackName:    "Spell Breaks",
			TriggerName: "Spell Worn Off",
			AppendExcludePatterns: []string{
				petWornOffPattern,
			},
		},
		{
			// …and add the dedicated "Pet Spell Worn Off" trigger that now
			// catches those lines for existing installs.
			Key:           "SpellBreaks:PetSpellWornOff:add-v1",
			PackName:      "Spell Breaks",
			InsertTrigger: func() *Trigger { t := petSpellWornOff("Spell Breaks"); return &t }(),
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

// applyDefaultUpdate looks up the named trigger and applies any additive
// changes: appends missing AppendExcludePatterns and sets a missing
// SetCooldownSecs. Returns whether the trigger was actually mutated (false if
// the trigger doesn't exist, or every change was already present).
func applyDefaultUpdate(store *Store, u DefaultUpdate) (bool, error) {
	if u.PackName == "" {
		return false, nil
	}

	// Insert-a-new-trigger update: add the trigger to an already-installed pack.
	if u.InsertTrigger != nil {
		hasPack, err := store.packHasAnyTrigger(u.PackName)
		if err != nil {
			return false, err
		}
		if !hasPack {
			// Pack not installed — don't resurrect it. Marked applied by the
			// caller, but new installs get the trigger from InstallPack anyway.
			slog.Info("trigger: default insert skipped, pack not installed", "key", u.Key, "pack", u.PackName)
			return false, nil
		}
		existing, err := store.FindByPackAndName(u.PackName, u.InsertTrigger.Name)
		if err != nil {
			return false, err
		}
		if existing != nil {
			return false, nil // already present — idempotent
		}
		t := *u.InsertTrigger
		t.PackName = u.PackName
		id, err := NewID()
		if err != nil {
			return false, err
		}
		t.ID = id
		t.CreatedAt = time.Now().UTC()
		t.SourcePack = u.PackName
		if so, err := store.NextTriggerSortOrder(u.PackName); err == nil {
			t.SortOrder = so
		}
		if err := store.Insert(&t); err != nil {
			return false, err
		}
		slog.Info("trigger: default insert applied", "key", u.Key, "pack", u.PackName, "trigger", t.Name)
		return true, nil
	}

	if u.TriggerName == "" {
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

	changed := false

	// Append any missing exclude patterns.
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
	if added > 0 {
		changed = true
	}

	// Set a missing reuse cooldown, leaving a user-customized value alone.
	if u.SetCooldownSecs > 0 && t.CooldownSecs == 0 {
		t.CooldownSecs = u.SetCooldownSecs
		changed = true
	}

	if !changed {
		return false, nil
	}
	if err := store.Update(t); err != nil {
		return false, err
	}
	slog.Info("trigger: default update applied", "key", u.Key, "pack", u.PackName, "trigger", u.TriggerName, "added_excludes", added, "set_cooldown", u.SetCooldownSecs)
	return true, nil
}

// InstallPack replaces any existing triggers for pack.PackName with the
// pack's triggers, assigning fresh IDs and creation timestamps.
//
// Triggers carrying a non-empty DedupKey are skipped when another already-
// installed trigger (from any other pack) owns that key, so shared
// spells/disciplines (Root, Mez, Resistant Discipline, etc.) appear only
// once in the Triggers page regardless of how many class packs reference
// them. The "owning" pack is whichever installed first; UninstallPack
// promotes a replacement on removal.
func InstallPack(store *Store, pack TriggerPack) error {
	if err := store.DeleteByPack(pack.PackName); err != nil {
		return err
	}
	return insertPackTriggers(store, pack)
}

// insertPackTriggers writes a pack's triggers, honoring DedupKey collisions
// against already-installed triggers from other packs. Factored out so both
// InstallPack (first-time install) and the promote-on-uninstall path can
// share the skip logic.
func insertPackTriggers(store *Store, pack TriggerPack) error {
	now := time.Now().UTC()
	for i := range pack.Triggers {
		t := &pack.Triggers[i]
		if t.DedupKey != "" {
			existing, err := store.FindByDedupKey(t.DedupKey)
			if err != nil {
				return err
			}
			// Compare on SourcePack (install origin), not PackName, so a moved
			// shared trigger is still recognized as already owned.
			if existing != nil && existing.SourcePack != pack.PackName {
				slog.Info("trigger: skipped duplicate",
					"pack", pack.PackName,
					"trigger", t.Name,
					"dedup_key", t.DedupKey,
					"owned_by", existing.SourcePack)
				continue
			}
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		t.ID = id
		t.CreatedAt = now
		t.SourcePack = pack.PackName
		if err := store.Insert(t); err != nil {
			return err
		}
	}
	return nil
}

// UninstallPack removes every trigger for pack.PackName and then re-installs
// any DedupKey-owning triggers that other still-installed packs would
// provide. This keeps shared triggers (Root, Resistant Discipline, etc.)
// available when the originating pack is uninstalled, as long as some other
// pack ships the same dedup_key.
//
// installedPackNames is the set of pack names currently considered
// installed *excluding* the one being uninstalled. Pass the result of the
// caller's bookkeeping; this function does not infer installation state
// from the DB because a pack with all its triggers skipped at install time
// would otherwise look uninstalled.
func UninstallPack(store *Store, packName string, installedPackNames map[string]bool) error {
	if err := store.DeleteByPack(packName); err != nil {
		return err
	}
	for _, candidate := range AllPacks() {
		if candidate.PackName == packName {
			continue
		}
		if !installedPackNames[candidate.PackName] {
			continue
		}
		if err := promoteOrphanedTriggers(store, candidate); err != nil {
			return err
		}
	}
	return nil
}

// promoteOrphanedTriggers walks a still-installed pack's definition and
// re-inserts any triggers whose DedupKey is currently unclaimed in the DB.
// Idempotent — triggers already present (either under their own pack or as
// a previous promotion) are left alone.
func promoteOrphanedTriggers(store *Store, pack TriggerPack) error {
	now := time.Now().UTC()
	for i := range pack.Triggers {
		t := pack.Triggers[i]
		if t.DedupKey == "" {
			continue
		}
		existing, err := store.FindByDedupKey(t.DedupKey)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		id, err := NewID()
		if err != nil {
			return err
		}
		t.ID = id
		t.CreatedAt = now
		t.SourcePack = pack.PackName
		if err := store.Insert(&t); err != nil {
			return err
		}
		slog.Info("trigger: promoted on uninstall",
			"pack", pack.PackName,
			"trigger", t.Name,
			"dedup_key", t.DedupKey)
	}
	return nil
}

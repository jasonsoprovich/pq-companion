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
		Seconds:     float64(seconds),
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
//
// Resistant's CooldownSecs is a flat value across all classes/levels. Two
// independent user reports suggest it may actually decrease with caster
// level (e.g. a lower cooldown at 60 than at 51) rather than being fixed —
// see LIMITATIONS.md §11.3. Not enough data points yet to derive a formula,
// so this stays flat until more in-game samples come in.
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
			TimerDurationSecs: 60,
			SpellID:           4585,
			CooldownSecs:      3600,
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
			TimerDurationSecs: 11,
			SpellID:           4587,
			CooldownSecs:      3600,
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
// spell the Dire Charm AA casts. 2026-10-22: extended with the rest of the
// PoP (61–65) charm / root / snare spells — Beckon, Command of Druzzil,
// Command of Tunare, Call of the Banshee, Call of the Arch Mage; Greater
// Immobilize, Petrifying Earth, Earthen Roots, Savage Roots, Shackles of
// Tunare; Festering / Embracing Darkness. Undead Pact / Entrancing Lights /
// Elnerick's Entombment of Ice / Insidious Retrogression are Quarm-custom
// spells.
const (
	charmBreakPattern = "^Your (?:charm|Charm|Beguile|Cajoling Whispers|Allure|Boltran[`']s Agacerie|Dictate|Befriend Animal|Charm Animals|Beguile Plants|Beguile Animals|Allure of the Wild|Call of Karana|Tunare[`']s Request|Dominate Undead|Beguile Undead|Cajole Undead|Thrall of Bones|Enslave Death|Word of Terris|Beckon|Command of Druzzil|Command of Tunare|Call of the Banshee|Call of the Arch Mage|Undead Pact|Dominating Gaze|Solon[`']s Bewitching Bravura|Solon[`']s Song of the Sirens) spell has worn off\\.$"
	rootBreakPattern  = "^Your (?:Root|Instill|Fetter|Greater Fetter|Paralyzing Earth|Immobilize|Grasping Roots|Ensnaring Roots|Enveloping Roots|Engulfing Roots|Engorging Roots|Entrapping Roots|Hungry Earth|Greater Immobilize|Petrifying Earth|Earthen Roots|Savage Roots|Shackles of Tunare|Elnerick[`']s Entombment of Ice) spell has worn off\\.$"
	snareBreakPattern = "^Your (?:Snare|Ensnare|Tangling Weeds|Atol[`']s Spectral Shackles|Engulfing Darkness|Festering Darkness|Embracing Darkness|Dooming Darkness|Cascading Darkness|Clinging Darkness|Devouring Darkness|Bonds of Force|Bonds of Tunare|Insidious Retrogression|Largo[`']s Absonant Binding|Selo[`']s Consonant Chain|Selo[`']s Assonant Strane|Song of Midnight) spell has worn off\\.$"
	// mezWornOffPattern is only used as an ExcludePattern on the Spell Breaks
	// catch-all — mez break alerts stay class-specific (Enchanter, Bard,
	// Necromancer). Kept in sync with the union of those packs' Mez Broke
	// patterns so a line is suppressed here exactly when a class alert
	// covers it; lull-type spells (Wake of Tranquility, Lugubrious Lament)
	// are not mezzes and fall through to the generic worn-off overlay.
	mezWornOffPattern = "^Your (?:Mesmerize|Mesmerization|Enthrall|Entrance|Dazzle|Fascination|Entrancing Lights|Glamour of Kintaz|Rapture|Ancient: Eternal Rapture|Apathy|Sleep|Bliss|Word of Morell|Screaming Terror|Death[`']s Silence|Kelin[`']s Lucid Lullaby|Crission[`']s Pixie Strike|Sionachie[`']s Dreams|Song of Twilight|Dreams of Ayonae|Ancient: Lullaby of Shadow|Dreams of Thule|Dreams of Terris|Lullaby of Morell) spell has worn off\\.$"
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
		Description: "CC break + cast-failure alerts plus spell timers for the enchanter buff (VoG, Vallon's Quickening, KEI, Voice of Quellious, Tranquility, Gift of Brilliance, IS, GRM, Guard of Druzzil, Speed of the Shissar/Brood, Speed of Vallon), debuff (Tashanian, Howl of Tashan, Cripple, Asphyxiate, Strangle), root (Root, Fetter, Greater Fetter), mez (Mesmerize, Mesmerization, Dazzle, Enthrall, Entrance, Glamour of Kintaz, Rapture / Ancient: Eternal Rapture, Apathy, Sleep, Bliss), charm (Charm, Beguile, Cajoling Whispers, Allure, Dictate, Boltran's Agacerie, Beckon, Command of Druzzil), and pacify (Lull, Calm, Soothe, Pacify, Wake of Tranquility, Pacification) lines.",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			{
				// Mezzes only: Instill is a root (sharedRootBreak covers it —
				// listing it here double-fired MEZ BROKE on top of ROOT
				// BROKE) and Wake of Tranquility is a lull, not a mez.
				// Fascination and Entrancing Lights (Quarm-custom) are the
				// enchanter AE mezzes. Apathy / Sleep / Bliss / Word of Morell
				// are the PoP mezzes (61–64).
				Name:     "Mez Broke",
				Enabled:  true,
				Pattern:  `Your (?:Mesmerize|Mesmerization|Enthrall|Entrance|Dazzle|Fascination|Entrancing Lights|Glamour of Kintaz|Rapture|Ancient: Eternal Rapture|Apathy|Sleep|Bliss|Word of Morell) spell has worn off\.`,
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

			// ── PoP buffs (timers) ───────────────────────────────────────
			// Voice of Quellious shares KEI's land text and Guard of Druzzil
			// shares Group Resist Magic's, so both key on "You begin casting
			// <SpellName>." instead. The older land-text trigger still fires
			// on the shared line; the spell-landed pipeline absorbs that twin
			// into the correctly named timer (see spelltimer.sameLandOrphan).
			{
				Name:              "Voice of Quellious",
				Enabled:           true,
				Pattern:           `^You begin casting Voice of Quellious\.$`,
				WornOffPattern:    `^Your mind returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 4800,
				SpellID:           3360,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Tranquility",
				Enabled:           true,
				Pattern:           `^(?:Tranquility soothes your mind\.|[A-Z][a-zA-Z']{2,14} sighs in contentment\.)$`,
				WornOffPattern:    `^The tranquil breeze fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 4800,
				SpellID:           3350,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Guard of Druzzil",
				Enabled:           true,
				Pattern:           `^You begin casting Guard of Druzzil\.$`,
				WornOffPattern:    `^Your protection fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3000,
				SpellID:           3242,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				// quarm.db's cast_on_other has no trailing period; accept
				// either form in case the client adds one.
				Name:              "Speed of Vallon",
				Enabled:           true,
				Pattern:           `^(?:You move with the speed of Vallon\.|[A-Z][a-zA-Z']{2,14} moves with the speed of Vallon\.?)$`,
				WornOffPattern:    `^Your restless nature fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2520,
				SpellID:           3240,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Vallon's Quickening",
				Enabled:           true,
				Pattern:           `^(?:You feel drawn to battle\.|[A-Z][a-zA-Z']{2,14} is drawn toward the glory of battle\.)$`,
				WornOffPattern:    `^Your restless nature fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 2520,
				SpellID:           3178,
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
				// 780s = 130 ticks: formula 9 (level*2+10, base 140) at
				// level 60. The timer engine rescales to the caster's real
				// level, reaching the full 140 ticks at 65.
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
				// 420s = 70 ticks: formula 8 is min(level+10, base 75), so
				// it scales to the full 75 ticks only at 65. Pack values are
				// the level-60 formula result; the timer engine rescales any
				// formula-derived duration to the caster's real level.
				TimerDurationSecs: 420,
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
			{
				// Shares Tashanian's land text, so it keys on the cast line;
				// the Tashanian twin is absorbed by the spell-landed pipeline.
				Name:              "Howl of Tashan",
				Enabled:           true,
				Pattern:           `^You begin casting Howl of Tashan\.$`,
				WornOffPattern:    `^(?:The barking fades\.|Your target resisted the Howl of Tashan spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 780,
				SpellID:           3342,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Strangle",
				Enabled:           true,
				Pattern:           `^(?:Your lungs contract\.|` + npcNameClass + ` claws at their throat\.)$`,
				WornOffPattern:    `^You can breathe again\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           3345,
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
				TimerDurationSecs: 48,
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
			// PoP mezzes — each has unique land text, so like Enthrall they
			// match the land line directly.
			{
				Name:              "Apathy",
				Enabled:           true,
				Pattern:           `^(?:Your existence suddenly seems pointless\.|` + npcNameClass + ` stares off into the distance\.)$`,
				WornOffPattern:    `^(?:The apathy fades\.|Your Apathy spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           3341,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Sleep",
				Enabled:           true,
				Pattern:           `^(?:You fall asleep\.|` + npcNameClass + ` falls asleep\.)$`,
				WornOffPattern:    `^(?:You wake up, very disoriented\.|Your Sleep spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           3354,
				PackName:          "Enchanter",
				Actions:           []Action{},
			},
			{
				Name:              "Bliss",
				Enabled:           true,
				Pattern:           `^(?:You swoon, overcome by bliss\.|` + npcNameClass + `'s jaw falls as they begin to drool\.)$`,
				WornOffPattern:    `^(?:The bliss fades\.|Your Bliss spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 54,
				SpellID:           3358,
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
			// Charm/Beguile/Cajoling Whispers/Allure all share spells_new
			// buffduration=205 / buffdurationformula=10. With
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
					// what the log emits. Double-quoted for the backtick. 1705 is
					// the enchanter scroll (formula 8, like the PoP charms); 1706
					// is a non-class copy with the formula 10 duration.
					{Pattern: "^You begin casting (Boltran[`']s Agacerie)\\.$", Enabled: true, TimerDurationSecs: 420, SpellID: 1705},
					// PoP charms: formula 8 (min(level+10, base 75)) — 70 ticks
					// at 60, the full 75 at 65.
					{Pattern: `^You begin casting (Beckon)\.$`, Enabled: true, TimerDurationSecs: 420, SpellID: 3347},
					{Pattern: `^You begin casting (Command of Druzzil)\.$`, Enabled: true, TimerDurationSecs: 420, SpellID: 3355},
				},
				WornOffPattern:    "^Your (?:target resisted the )?(Cajoling Whispers|Boltran[`']s Agacerie|Beguile|Allure|Charm|Beckon|Command of Druzzil) spell(?: has worn off)?\\.$",
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
					{Pattern: `^You begin casting (Calm)\.$`, Enabled: true, TimerDurationSecs: 42, SpellID: 47},
					{Pattern: `^You begin casting (Soothe)\.$`, Enabled: true, TimerDurationSecs: 150, SpellID: 501},
					{Pattern: `^You begin casting (Pacify)\.$`, Enabled: true, TimerDurationSecs: 360, SpellID: 45},
					{Pattern: `^You begin casting (Wake of Tranquility)\.$`, Enabled: true, TimerDurationSecs: 42, SpellID: 1541},
					{Pattern: `^You begin casting (Pacification)\.$`, Enabled: true, TimerDurationSecs: 42, SpellID: 3197},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Wake of Tranquility|Pacification|Soothe|Pacify|Lull|Calm) spell(?: has worn off)?\.$`,
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
		Description: "Spell timers for Aegolism / Blessing of Aegolism / Ancient: Gift of Aegolism, Virtue / Hand of Virtue, Naltron's Mark, Symbol of Kazad / Kazad's Mark, Divine Intervention, Mark of Karn, Mark of Kings, Mark of the Righteous, and Divine Aura.",
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

			// ── PoP raid buffs (timers) ──────────────────────────────────
			// Virtue and Hand of Virtue share land and fade text, and both
			// Kazad symbols share Naltron's Mark's, so each keys on "You
			// begin casting <SpellName>." like the Aegolism line. The
			// Naltron's Mark land-text twin is absorbed by the spell-landed
			// pipeline (see spelltimer.sameLandOrphan).
			{
				Name:              "Virtue",
				Enabled:           true,
				Pattern:           `^You begin casting Virtue\.$`,
				WornOffPattern:    `^Your virtue fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           3467,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Hand of Virtue",
				Enabled:           true,
				Pattern:           `^You begin casting Hand of Virtue\.$`,
				WornOffPattern:    `^Your virtue fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9000,
				SpellID:           3479,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Symbol of Kazad",
				Enabled:           true,
				Pattern:           `^You begin casting Symbol of Kazad\.$`,
				WornOffPattern:    `^The mystic symbol fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3780,
				SpellID:           3466,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Kazad's Mark",
				Enabled:           true,
				Pattern:           "^You begin casting Kazad[`']s Mark\\.$",
				WornOffPattern:    `^The mystic symbol fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3780,
				SpellID:           3047,
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
				TimerDurationSecs: 600,
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
				TimerDurationSecs: 210,
				SpellID:           1548,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Mark of Kings",
				Enabled:           true,
				Pattern:           `^(?:Your skin gleams with the mark of kings\.|` + npcNameClass + `'s skin gleams with the mark of kings\.)$`,
				WornOffPattern:    `^The mark fades\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 240,
				SpellID:           3469,
				PackName:          "Cleric",
				Actions:           []Action{},
			},
			{
				Name:              "Mark of the Righteous",
				Enabled:           true,
				Pattern:           `^(?:Your skin gleams with bright red runes\.|` + npcNameClass + `'s skin gleams with bright red runes\.)$`,
				WornOffPattern:    `^The bright red runes fade\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 210,
				SpellID:           3477,
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
		Description: "Crowd-control break alerts (root, snare, charm) plus spell timers for Protection of the Glades, Protection / Blessing of the Nine, Blessing of Replenishment, Legacy of Thorn, Legacy of Bracken, Hand of Ro, Winged Death, Swarming Death, Ensnare, Entrapping Roots, Savage Roots, Command of Tunare, and the animal/plant charms (Befriend Animal, Charm Animals, Beguile Plants, Beguile Animals, Allure of the Wild, Call of Karana, Tunare's Request).",
		Triggers: []Trigger{
			// ── Crowd-control breaks ─────────────────────────────────────
			// Caster's "Your <SpellName> spell has worn off." line fires both
			// on damage-induced break and on natural expiry — either way the
			// druid needs to know the mob is loose.
			sharedRootBreak("Druid"),
			sharedSnareBreak("Druid"),
			sharedCharmBreak("Druid"),

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
			// PoP upgrades that share an older spell's land text key on
			// "You begin casting <SpellName>." instead; the older land-text
			// trigger's twin is absorbed by the spell-landed pipeline (see
			// spelltimer.sameLandOrphan).
			{
				Name:              "Protection of the Nine",
				Enabled:           true,
				Pattern:           `^You begin casting Protection of the Nine\.$`,
				WornOffPattern:    `^Your skin returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 6000,
				SpellID:           3234,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Blessing of the Nine",
				Enabled:           true,
				Pattern:           `^You begin casting Blessing of the Nine\.$`,
				WornOffPattern:    `^Your skin returns to normal\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 6000,
				SpellID:           3451,
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
			{
				Name:              "Legacy of Bracken",
				Enabled:           true,
				Pattern:           `^You begin casting Legacy of Bracken\.$`,
				WornOffPattern:    `^The bracken falls away\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           3295,
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
			{
				Name:              "Swarming Death",
				Enabled:           true,
				Pattern:           `^You begin casting Swarming Death\.$`,
				WornOffPattern:    `^(?:The swarm departs\.|Your target resisted the Swarming Death spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 36,
				SpellID:           3446,
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
				TimerDurationSecs: 780,
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
				TimerDurationSecs: 180,
				SpellID:           1608,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			{
				Name:              "Savage Roots",
				Enabled:           true,
				Pattern:           `^You begin casting Savage Roots\.$`,
				WornOffPattern:    `^(?:The roots fall from your feet\.|Your Savage Roots spell has worn off\.|Your target resisted the Savage Roots spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 270,
				SpellID:           3447,
				PackName:          "Druid",
				Actions:           []Action{},
			},

			// ── Charm (timer) ────────────────────────────────────────────
			// Command of Tunare has no cast_on_you and a generic "<mob>
			// blinks." cast_on_other, so it keys on the cast line like the
			// enchanter charms. Charm can break on any of the pet's ticks
			// before this runs out — the timer is the best case.
			{
				Name:              "Command of Tunare",
				Enabled:           true,
				Pattern:           `^You begin casting Command of Tunare\.$`,
				WornOffPattern:    `^Your (?:target resisted the Command of Tunare spell|charm spell has worn off|Command of Tunare spell has worn off)\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           3445,
				PackName:          "Druid",
				Actions:           []Action{},
			},
			// Pre-PoP animal/plant charms, merged like the enchanter charms
			// and keyed per spell by the captured name. All formula 10 (190
			// ticks at 60) except Tunare's Request (formula 3, 1800 ticks).
			{
				Name:    "Animal Charm",
				Enabled: true,
				Pattern: `^You begin casting (Call of Karana)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Befriend Animal)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 245},
					{Pattern: `^You begin casting (Charm Animals)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 260},
					{Pattern: `^You begin casting (Beguile Plants)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 753},
					{Pattern: `^You begin casting (Beguile Animals)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 141},
					{Pattern: `^You begin casting (Allure of the Wild)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 142},
					{Pattern: `^You begin casting (Tunare's Request)\.$`, Enabled: true, TimerDurationSecs: 10800, SpellID: 1556},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Call of Karana|Befriend Animal|Charm Animals|Beguile Plants|Beguile Animals|Allure of the Wild|Tunare's Request) spell(?: has worn off)?\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           1553,
				TimerKeyCapture:   "1",
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
		Description: "Spell timers for Turgur's Insects, Plague of Insects, Cloud of Grummus, Malo, Malosini, Malosinia, Malos, Torpor, Quiescence, Avatar, Ferine Avatar, Focus of Spirit, Focus of Soul, Focus of the Seventh, and Regrowth of Dar Khura.",
		Triggers: []Trigger{
			// ── Slows (timers) ──────────────────────────────────────────
			{
				Name:              "Turgur's Insects",
				Enabled:           true,
				Pattern:           `^(?:You feel drowsy\.|` + npcNameClass + ` yawns\.)$`,
				WornOffPattern:    `^You feel less drowsy\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 360,
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
				TimerDurationSecs: 192,
				SpellID:           2527,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			{
				Name:              "Cloud of Grummus",
				Enabled:           true,
				Pattern:           `^(?:A cloud of plague numbs your body\.|` + npcNameClass + `'s body is covered in a brown mist\.)$`,
				WornOffPattern:    `^The cloud disperses\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 192,
				SpellID:           3380,
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
				TimerDurationSecs: 780,
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
				TimerDurationSecs: 1140,
				SpellID:           1577,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			// PoP Malosinia / Malos share the same anchors and fade text.
			{
				Name:              "Malosinia",
				Enabled:           true,
				Pattern:           `^You begin casting Malosinia\.$`,
				WornOffPattern:    `^(?:Your vulnerability fades\.|Your target resisted the Malosinia spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           3387,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			{
				Name:              "Malos",
				Enabled:           true,
				Pattern:           `^You begin casting Malos\.$`,
				WornOffPattern:    `^(?:Your vulnerability fades\.|Your target resisted the Malos spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 780,
				SpellID:           3395,
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
			{
				Name:              "Quiescence",
				Enabled:           true,
				Pattern:           `^(?:You fall into a state of quiescence\.|[A-Z][a-zA-Z']{2,14} falls into a state of quiescence\.)$`,
				WornOffPattern:    `^Your state of quiescence ends\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 24,
				SpellID:           3398,
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
			{
				Name:              "Ferine Avatar",
				Enabled:           true,
				Pattern:           `^(?:Your body screams with the power of a ferine Avatar\.|[A-Z][a-zA-Z']{2,14} has been infused with the power of a ferine Avatar\.)$`,
				WornOffPattern:    `^The ferine Avatar departs\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 360,
				SpellID:           3399,
				CooldownSecs:      60,
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
			// Focus of Soul / Focus of the Seventh share Focus of Spirit's
			// land and fade text, so they key on the cast line; the Focus of
			// Spirit twin is absorbed by the spell-landed pipeline.
			{
				Name:              "Focus of Soul",
				Enabled:           true,
				Pattern:           `^You begin casting Focus of Soul\.$`,
				WornOffPattern:    `^Your focus fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 4800,
				SpellID:           3235,
				PackName:          "Shaman",
				Actions:           []Action{},
			},
			{
				Name:              "Focus of the Seventh",
				Enabled:           true,
				Pattern:           `^You begin casting Focus of the Seventh\.$`,
				WornOffPattern:    `^Your focus fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 4800,
				SpellID:           3397,
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
		Description: "Lay on Hands alert, root break alert, and spell timers for Immobilize, Greater Immobilize, Holyforge Discipline, Sanctification Discipline, plus the shared melee disciplines Resistant and Fearless.",
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
				TimerDurationSecs: 60,
				SpellID:           132,
				PackName:          "Paladin",
				Actions:           []Action{},
			},
			{
				// Shares Immobilize's land text, so it keys on the cast line; the
				// Immobilize twin is absorbed by the spell-landed pipeline.
				Name:              "Greater Immobilize",
				Enabled:           true,
				Pattern:           `^You begin casting Greater Immobilize\.$`,
				WornOffPattern:    `^(?:Your feet come free\.|Your Greater Immobilize spell has worn off\.|Your target resisted the Greater Immobilize spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           3195,
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
				CooldownSecs:      4320,
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
				CooldownSecs:      4320,
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
		Description: "Harm Touch and Feign Death alerts plus spell timers for Engulfing Darkness, Festering Darkness, Disease Cloud, Voice of Terris, Voice of Thule, Asystole, and Unholy Aura Discipline.",
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
			{
				Name:              "Festering Darkness",
				Enabled:           true,
				Pattern:           `^(?:You are engulfed by a festering darkness\.|` + npcNameClass + ` is engulfed by a festering darkness\.)$`,
				WornOffPattern:    `^(?:The festering darkness fades\.|Your Festering Darkness spell has worn off\.|Your target resisted the Festering Darkness spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           3400,
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
			{
				// Shares Voice of Terris's land and fade text, so it keys on the
				// cast line; the Voice of Terris twin is absorbed by the spell-landed
				// pipeline.
				Name:              "Voice of Thule",
				Enabled:           true,
				Pattern:           `^You begin casting Voice of Thule\.$`,
				WornOffPattern:    `^The voice of nightmares fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 600,
				SpellID:           3410,
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
				CooldownSecs:      4320,
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
				CooldownSecs:      900,
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
				CooldownSecs:      900,
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
				CooldownSecs:      1620,
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
				TimerDurationSecs: 180,
				SpellID:           4501,
				CooldownSecs:      1800,
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
				TimerDurationSecs: 9,
				SpellID:           4674,
				CooldownSecs:      3600,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Charge Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your focus becomes perfect\.|[A-Z][a-zA-Z']{2,14}'s focus becomes perfect\.)$`,
				WornOffPattern:    `^Your perfect focus fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 14,
				SpellID:           4672,
				CooldownSecs:      1800,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Mighty Strike Discipline",
				Enabled:           true,
				Pattern:           `^(?:You feel like a killing machine\.|[A-Z][a-zA-Z']{2,14} feels like a killing machine\.)$`,
				WornOffPattern:    `^Your killer instinct fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 10,
				SpellID:           4514,
				CooldownSecs:      3600,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
			{
				Name:              "Fellstrike Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons strike true\.|[A-Z][a-zA-Z']{2,14}'s weapons strike true\.)$`,
				WornOffPattern:    `^Your weapons lose their accuracy\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4675,
				CooldownSecs:      1800,
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
				TimerDurationSecs: 8,
				SpellID:           4670,
				CooldownSecs:      3600,
				PackName:          "Warrior",
				Actions:           []Action{},
			},
		}, meleeSharedDisciplines("Warrior")...),
	}
}

// MonkPack returns the pre-built monk trigger pack: a Feign Death skill
// overlay alert, the Mend skill's reuse timer, and timers for all 8 monk
// disciplines plus the two shared melee disciplines (Resistant, Fearless).
//
// The Feign Death entry overlaps with the Shadowknight pack — installing
// both packs will cause duplicate overlay fires on FD; users can disable
// one in the trigger editor if needed.
func MonkPack() TriggerPack {
	return TriggerPack{
		PackName:    "Monk",
		Class:       ClassPtr(ClassMonk),
		Description: "Feign Death alert, Mend reuse timer, plus spell timers for all 8 monk disciplines (Stonestance, Innerflame, Thunderkick, Ashenhand, Silentfist, Whirlwind, Voiddance, Hundred Fists) plus the shared melee disciplines Resistant and Fearless.",
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

			// ── Mend skill (overlay alert + reuse CD) ────────────────────
			// Fixed 6-minute (360s) reuse regardless of level, per user
			// confirmation. EQMacEmu's Handle_OP_Mend (client_packet.cpp)
			// starts the server-side reuse timer unconditionally on every
			// use — success, critical, fail, or worsen — but only the
			// success/critical log lines are confirmed against real Quarm
			// logs so far; a failed or worsened Mend won't restart this
			// countdown until those strings are verified too (matches the
			// "we'd risk false positives without verifying against real
			// Quarm logs" bar used elsewhere in this file).
			//
			// The "zoning resets Mend's cooldown by ~72s" report some users
			// pass around is a misreading of a client/server desync, not a
			// real reduction: zoning clears the client's Mend button so it
			// looks ready, but Handle_OP_Mend still checks the real
			// server-side timer, replies "Ability recovery time not yet
			// met." if it's early, and just re-arms a client-side timer to
			// reset the button once the real cooldown actually expires. The
			// underlying reuse time never changes.
			{
				Name:         "Mend",
				Enabled:      true,
				Pattern:      `^(?:You mend your wounds and heal some damage\.|You magically mend your wounds and heal considerable damage\.)$`,
				CooldownSecs: 360,
				PackName:     "Monk",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "MEND", DurationSecs: 4, Color: "#66cc66"},
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
				CooldownSecs:      360,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Innerflame Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your muscles bulge with the force of will\.|[A-Z][a-zA-Z']{2,14}'s muscles bulge with the force of will\.)$`,
				WornOffPattern:    `^Your strength of will fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4512,
				CooldownSecs:      1560,
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
				TimerDurationSecs: 8,
				SpellID:           4502,
				CooldownSecs:      3300,
				PackName:          "Monk",
				Actions:           []Action{},
			},
			{
				Name:              "Whirlwind Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your instincts take over as you turn aside every attack\.|[A-Z][a-zA-Z']{2,14}'s face becomes twisted with fury\.)$`,
				WornOffPattern:    `^Your fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9,
				SpellID:           4509,
				CooldownSecs:      3600,
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
				TimerDurationSecs: 15,
				SpellID:           4513,
				CooldownSecs:      1800,
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
				CooldownSecs:      240,
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
				CooldownSecs:      4320,
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
				CooldownSecs:      540,
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
				TimerDurationSecs: 12,
				SpellID:           4676,
				CooldownSecs:      1800,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Deadeye Discipline",
				Enabled:           true,
				Pattern:           `^(?:You feel unstoppable\.|[A-Z][a-zA-Z']{2,14} feels unstoppable\.)$`,
				WornOffPattern:    `^Your confidence fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 14,
				SpellID:           4505,
				CooldownSecs:      1800,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Counterattack Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons move with uncanny grace\.|[A-Z][a-zA-Z']{2,14}'s weapons move with uncanny grace\.)$`,
				WornOffPattern:    `^Your weapons lose their uncanny grace\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 9,
				SpellID:           4673,
				CooldownSecs:      3600,
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
				CooldownSecs:      1800,
				PackName:          "Rogue",
				Actions:           []Action{},
			},
			{
				Name:              "Kinesthetics Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your arms feel alive with mystic energy\.|[A-Z][a-zA-Z']{2,14}'s arms feel alive with mystic energy\.)$`,
				WornOffPattern:    `^The mystic energy fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 18,
				SpellID:           4517,
				CooldownSecs:      1800,
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
				TimerDurationSecs: 15,
				SpellID:           4677,
				CooldownSecs:      1800,
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
		Description: "Snare break alert plus spell timers for Trueshot Discipline, Weapon Shield Discipline, Call of Sky, Call of Fire, Nature's Rebuke, Cry of Thunder, Flame Lick, plus the shared melee disciplines Resistant and Fearless.",
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
				CooldownSecs:      4320,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
			{
				Name:              "Weapon Shield Discipline",
				Enabled:           true,
				Pattern:           `^(?:Your weapons begin to spin\.|[A-Z][a-zA-Z']{2,14}'s weapons begin to spin\.)$`,
				WornOffPattern:    `^Your weapons slow down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 18,
				SpellID:           4519,
				CooldownSecs:      4320,
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
			{
				Name:              "Nature's Rebuke",
				Enabled:           true,
				Pattern:           `^Nature's order fills your blade\.$`,
				WornOffPattern:    `^Nature's order leaves your weapons\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3600,
				SpellID:           3415,
				PackName:          "Ranger",
				Actions:           []Action{},
			},
			{
				Name:              "Cry of Thunder",
				Enabled:           true,
				Pattern:           `^The cry of thunder fills your weapons with power\.$`,
				WornOffPattern:    `^The cry of thunder leaves\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3600,
				SpellID:           3420,
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
		Description: "Crowd-control break alerts (mez, charm, snare) plus spell timers for Cantata of Replenishment, Warsong of Zek, Niv's Melody of Preservation, Psalm of Veeshan, Elemental Rhythms, Guardian Rhythms, Kelin's Lucid Lullaby / Lugubrious Lament, Dreams of Thule, Dreams of Terris, Lullaby of Morell, Solon's Bewitching Bravura, Call of the Banshee, Largo's Absonant Binding, and the Tiny Cloak of Darkest Night clicky (Shroud of Stealth).",
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
				Pattern:  "Your (?:Kelin[`']s Lucid Lullaby|Crission[`']s Pixie Strike|Sionachie[`']s Dreams|Song of Twilight|Dreams of Ayonae|Ancient: Lullaby of Shadow|Dreams of Thule|Dreams of Terris|Lullaby of Morell) spell has worn off\\.",
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
				// Dreams of Thule / Dreams of Terris share the same cast_on_other
				// ("<mob>'s head nods, as he begins to twitch."), so both key on the
				// song's "You begin singing <SpellName>." line.
				Name:              "Dreams of Thule",
				Enabled:           true,
				Pattern:           `^You begin singing Dreams of Thule\.$`,
				WornOffPattern:    `^(?:You wake up sweating\.|Your Dreams of Thule spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           3030,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Dreams of Terris",
				Enabled:           true,
				Pattern:           `^You begin singing Dreams of Terris\.$`,
				WornOffPattern:    `^(?:You wake up sweating\.|Your Dreams of Terris spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           3369,
				PackName:          "Bard",
				Actions:           []Action{},
			},
			{
				Name:              "Lullaby of Morell",
				Enabled:           true,
				Pattern:           `^(?:The Shadows of Morell force you to sleep\.|` + npcNameClass + ` blinks and falls to the ground\.)$`,
				WornOffPattern:    `^(?:You wake up in a blissful state\.|Your Lullaby of Morell spell has worn off\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 18,
				SpellID:           3376,
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
				// Shares Solon's Bewitching Bravura's "<mob>'s eyes glaze over."
				// cast_on_other, so it keys on the song's start line. Charm can break
				// on any of the pet's ticks before this runs out — it's the best case.
				Name:              "Call of the Banshee",
				Enabled:           true,
				Pattern:           `^You begin singing Call of the Banshee\.$`,
				WornOffPattern:    `^(?:The wailing voice fades\.|Your (?:charm|Call of the Banshee) spell has worn off\.|Your target resisted the Call of the Banshee spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           3371,
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
		Description: "Charm break alert plus spell timers for Burnout IV / Burnout V (pet haste), Shield of Lava / Flameshield of Ro / Maelstrom of Ro (damage shields), and Call of the Arch Mage (elemental charm).",
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
			{
				Name:              "Burnout V",
				Enabled:           true,
				Pattern:           `^You begin casting Burnout V\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 1500,
				SpellID:           3237,
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
			{
				Name:              "Flameshield of Ro",
				Enabled:           true,
				Pattern:           `^(?:You are enveloped by the flameshield of Ro\.|[A-Z][a-zA-Z']{2,14} is enveloped by the flameshield of Ro\.)$`,
				WornOffPattern:    `^The flameshield dies down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 420,
				SpellID:           3198,
				PackName:          "Magician",
				Actions:           []Action{},
			},
			{
				Name:              "Maelstrom of Ro",
				Enabled:           true,
				Pattern:           `^(?:You are enveloped by the maelstrom of Ro\.|[A-Z][a-zA-Z']{2,14} is enveloped by the maelstrom of Ro\.)$`,
				WornOffPattern:    `^The maelstrom dies down\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 420,
				SpellID:           3486,
				PackName:          "Magician",
				Actions:           []Action{},
			},

			// ── Charm (timer) ────────────────────────────────────────────
			{
				// Elemental charm. Keys on the cast line like the other class
				// charms; charm can break on any of the pet's ticks before this runs
				// out, so the timer is the best case.
				Name:              "Call of the Arch Mage",
				Enabled:           true,
				Pattern:           `^You begin casting Call of the Arch Mage\.$`,
				WornOffPattern:    `^Your (?:target resisted the Call of the Arch Mage spell|charm spell has worn off|Call of the Arch Mage spell has worn off)\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           3484,
				PackName:          "Magician",
				Actions:           []Action{},
			},
			sharedCharmBreak("Magician"),
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
		Description: "Mez/charm/snare break alerts plus spell timers for Arch Lich, Seduction of Saryrn, Splurt, Ignite Blood, Pyrocruor, Blood of Thule, Dark Plague, Embracing Darkness, Bond of Death, Saryrn's Kiss, Harmshield, and undead charm (Dominate Undead, Beguile Undead, Cajole Undead, Thrall of Bones, Enslave Death, Word of Terris).",
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
				Pattern:  "Your (?:Screaming Terror|Death[`']s Silence) spell has worn off\\.",
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
			{
				Name:              "Seduction of Saryrn",
				Enabled:           true,
				Pattern:           `^(?:Your soul is claimed in a cold caress\.|[A-Z][a-zA-Z']{2,14} radiates an aura of cold\.)$`,
				WornOffPattern:    `^The cold hands release your soul, for now\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 3600,
				SpellID:           3311,
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
				Name:              "Blood of Thule",
				Enabled:           true,
				Pattern:           `^(?:The blood of Thule pumps through your veins\.|` + npcNameClass + `'s veins turn a vile shade of green\.)$`,
				WornOffPattern:    `^Your veins return to normal\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 42,
				SpellID:           3303,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
			{
				Name:              "Dark Plague",
				Enabled:           true,
				Pattern:           `^(?:Black spots form on your skin\.|` + npcNameClass + `'s skin is covered in black spots\.)$`,
				WornOffPattern:    `^The plague has ended\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           3315,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},
			{
				Name:              "Embracing Darkness",
				Enabled:           true,
				Pattern:           `^(?:You are engulfed by an embracing darkness\.|` + npcNameClass + ` is engulfed in an embracing darkness\.)$`,
				WornOffPattern:    `^(?:The darkness fades\.|Your Embracing Darkness spell has worn off\.|Your target resisted the Embracing Darkness spell\.)$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 120,
				SpellID:           3309,
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
			{
				Name:              "Saryrn's Kiss",
				Enabled:           true,
				Pattern:           `^(?:You feel unseen lips touch your face\.|` + npcNameClass + ` screams in torment\.)$`,
				WornOffPattern:    `^Your torment ends, for now\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 60,
				SpellID:           3306,
				PackName:          "Necromancer",
				Actions:           []Action{},
			},

			// ── Undead charm (merged timer) ──────────────────────────────
			// Keys on the cast line like the enchanter charms; charm can break
			// on any of the pet's ticks before this runs out — it's the best
			// case. Every rank is formula 10 (190 ticks at 60) except Enslave
			// Death, whose base caps it at 5 ticks.
			{
				Name:    "Undead Charm",
				Enabled: true,
				Pattern: `^You begin casting (Word of Terris)\.$`,
				ExtraPatterns: []ExtraPattern{
					{Pattern: `^You begin casting (Thrall of Bones)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 1624},
					{Pattern: `^You begin casting (Dominate Undead)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 196},
					{Pattern: `^You begin casting (Beguile Undead)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 197},
					{Pattern: `^You begin casting (Cajole Undead)\.$`, Enabled: true, TimerDurationSecs: 1140, SpellID: 198},
					{Pattern: `^You begin casting (Enslave Death)\.$`, Enabled: true, TimerDurationSecs: 30, SpellID: 1629},
				},
				WornOffPattern:    `^Your (?:target resisted the )?(Word of Terris|Thrall of Bones|Dominate Undead|Beguile Undead|Cajole Undead|Enslave Death) spell(?: has worn off)?\.$`,
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 1140,
				SpellID:           3316,
				TimerKeyCapture:   "1",
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
		Description: "Spell timers for Spiritual Purity, Spiritual Dominion, Paragon of Spirit, Ferocity, Sha's Advantage, Sha's Revenge, both beastlord disciplines (Protective Spirit, Bestial Rage), plus the shared melee disciplines Resistant and Fearless.",
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
				TimerDurationSecs: 192,
				SpellID:           2942,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},
			{
				// Shares Sha's Advantage's land and fade text, so it keys on the
				// cast line; the Sha's Advantage twin is absorbed by the spell-landed
				// pipeline.
				Name:              "Sha's Revenge",
				Enabled:           true,
				Pattern:           "^You begin casting Sha[`']s Revenge\\.$",
				WornOffPattern:    "^(?:You regain your fighting edge\\.|Your target resisted the Sha[`']s Revenge spell\\.)$",
				TimerType:         TimerTypeDetrimental,
				TimerDurationSecs: 192,
				SpellID:           3462,
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
				CooldownSecs:      720,
				PackName:          "Beastlord",
				Actions:           []Action{},
			},
			{
				Name:              "Bestial Rage Discipline",
				Enabled:           true,
				Pattern:           `^(?:A bestial fury consumes you\.|[A-Z][a-zA-Z']{2,14} is consumed in a bestial fury\.)$`,
				WornOffPattern:    `^The bestial fury fades\.$`,
				TimerType:         TimerTypeBuff,
				TimerDurationSecs: 12,
				SpellID:           4678,
				CooldownSecs:      1800,
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
					`tells you, 'You are already trained to the limit`,
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
				// an NPC/pet: exclude victims that start lowercase ("a poacher",
				// "an ulthork", "the guardian") or are multi-word ("Trooper
				// Coglee", "Verina Tomb", "Eye of Zomm", a player's mount).
				// The multi-word check isn't anchored to exactly two words so
				// it also catches 3+ word victims like "Eye of Zomm". Single-
				// word named NPCs (e.g. "Sontalak") are textually
				// indistinguishable from a player and can still leak — that
				// needs NPC-awareness, not regex.
				ExcludePatterns: []string{
					`^[a-z]`,
					`^\S+ \S+.*has been slain by`,
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
	triggers := []Trigger{
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
	}
	triggers = append(triggers, raidSignatureSpellAlerts()...)
	return TriggerPack{
		PackName:    "Raid Alerts",
		Description: "Divine Intervention procs, mobs casting gate, assist calls, and signature-spell recast warnings for Ring War raid targets.",
		Triggers:    triggers,
	}
}

// raidSignatureSpellAlerts returns recast-warning triggers for a handful of
// high-impact signature spells cast by Kunark "Ring War" raid targets (Aten
// Ha Ra, Lord Inquisitor Seru, Vyzh`dra, Shei Vinitras, etc). The pattern
// only matches on the spell's own land text — which NPC cast it is
// irrelevant, so the same trigger covers every mob that has it in its spell
// set. Each pattern matches both branches of the land message ("cast on
// you" and "cast on other") so the timer restarts regardless of which raid
// member the spell actually lands on.
//
// TimerDurationSecs is the spell's own recast_time from spells_new — the
// best available proxy for how often a scripted boss re-casts it, since
// there's no other way to know the real cadence without the first cast
// landing in the log. TimerAlerts fires a TTS warning 5 seconds before the
// timer would hit zero (i.e. 5 seconds before the boss can cast it again),
// overriding the pack's default 10-second detrimental warning. The Actions
// overlay banner confirms each observed cast. Leaving TimerTargetCapture
// unset means the timer carries no bound NPC name, so it's swept by the
// spelltimer engine's orphan-cleanup on the next kill — "cleared when the
// boss dies" falls out of that for free without needing to identify which
// NPC was casting it.
//
// RefireCooldownSecs suppresses the duplicate lines a single AE cast
// produces (one broadcast line per raid member hit) from each restarting
// the timer and re-firing the banner/TTS — see the comment on the field
// literal for why.
func raidSignatureSpellAlerts() []Trigger {
	spell := func(name, landOnYou, landOnOtherSuffix string, spellID int, recastSecs float64) Trigger {
		return Trigger{
			Name:              name,
			Enabled:           true,
			Pattern:           `^(?:` + landOnYou + `|[A-Z][a-zA-Z']{2,14}` + landOnOtherSuffix + `)$`,
			PackName:          "Raid Alerts",
			TimerType:         TimerTypeDetrimental,
			TimerDurationSecs: recastSecs,
			SpellID:           spellID,
			BarColor:          "#ff3333",
			// These are PBAE-style effects that hit every nearby raid member
			// in the same instant, and EQ's cast_on_other line is broadcast
			// per-victim — so a single cast can produce several matching log
			// lines within the same second (one per person hit). Without a
			// refire lockout, each of those duplicate lines would restart
			// the timer and re-fire the overlay/TTS actions, stacking
			// several banners on screen for one actual cast.
			RefireCooldownSecs: 3,
			TimerAlerts: []TimerAlert{
				{
					ID:          "raid-sig-warn-5s",
					Seconds:     5,
					Type:        TimerAlertTypeTextToSpeech,
					TTSTemplate: "{spell} in 5 seconds",
					TTSVolume:   100,
				},
			},
			Actions: []Action{
				{Type: ActionOverlayText, Text: strings.ToUpper(name) + " CAST", DurationSecs: 4, Color: "#ff4444"},
			},
		}
	}
	return []Trigger{
		spell("Fling", `A massive force knocks you into the air\.`, ` is knocked into the air by a massive force\.`, 2167, 45),
		spell("Silence of the Shadows", `Your mind is assaulted by horrifying visions\.`, `'s mind is assaulted by horrifying visions\.`, 2164, 30),
		spell("Torturing Winds", `You are stricken by torturing winds\.`, ` is stricken by torturing winds\.`, 2061, 45),
		// The "landed on other" text ("'s flesh begins to liquefy.") is
		// byte-identical between Caustic Mist (Vyzh'dra the Cursed, 24s
		// recast) and Putrefy Flesh (Zlandicar, 30s recast) — EQ reused the
		// same broadcast string for both spells, so the trigger can't tell
		// which boss actually cast it from the log line alone. The name
		// reflects both so the alert isn't mislabeled on either fight.
		spell("Caustic Mist / Putrefy Flesh", `Your skin begins to rot\.`, `'s flesh begins to liquefy\.`, 2814, 24),
		spell("Touch of Vinitras", `Your soul fades into darkness\.`, `'s soul fades into darkness\.`, 2859, 120),
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

// slowTarget wraps npcNameClass in a named "target" capture group for a
// Slows-pack "landed on other" branch. TimerTargetCapture="target" then
// namespaces the resulting timer key by the captured mob name (see
// spelltimer.Engine.StartExternal), so slowing two differently-named mobs
// at once tracks two independent rows instead of one overwriting the
// other — the community pack this was built from set
// TimerTargetCapture="target" on every trigger but never actually wrapped
// a group named that in the pattern, so the capture silently did nothing
// and every cast of the same trigger collided into a single timer
// regardless of target. Two mobs sharing the exact same displayed name
// still collide — EQ's log carries no spawn id, a limitation documented in
// LIMITATIONS.md §1.3 that no pack can work around.
const slowTarget = `(?P<target>` + npcNameClass + `)`

// SlowsPack returns a raid-utility pack that tracks every detrimental
// "slow" effect a Seekers of Souls raid is likely to see land — caster
// self-inflicted slows (Beastlord, Shaman, Enchanter, Bard, Ranger,
// Necromancer, Rogue) plus a few NPC-cast slows with no owning class
// (Waves of the Deep Sea, Breath of the Sea, Sha's Vengeance, and the
// generic "Vas Ren" line) — so a slow landing on any raid member or NPC
// shows a countdown on the detrimental overlay.
//
// Ported from a NAG export originally assembled by Takkisini and Sandrian
// for the guild, converted to a PQC pack by Grimrose. Thanks to all three
// for the legwork — Takkisini did the heavy lifting on the trigger
// patterns, Sandrian tracked down the full list of slows to cover, and
// Grimrose did the NAG→PQC conversion and multiple rounds of in-game
// testing. Durations below are recomputed from spells_new via
// spelltimer.CalcDurationTicks at a level-60 caster (see duration.go) —
// one value from the original NAG export didn't match that formula
// (Rogue's Paralyzing Poison I, corrected here) and one of this pack's
// own earlier "corrections" didn't either (Ranger's Earthcall, since
// reverted — see its comment below).
//
// Every trigger's first-person ("You ...") branch was pulled after
// in-game testing (Grimrose, Sep 2026): these self-cast messages are the
// generic "you got slowed" text EQ also uses when an NPC lands a
// same-worded effect on a raid member, so keeping them fired the pack's
// TTS/timer for a player being slowed, not just for the class-cast debuff
// landing on a mob. Only the third-person "<mob> ..." branch — the one
// carrying the (?P<target>...) capture — should be matched.
func SlowsPack() TriggerPack {
	const packName = "Slows"
	slow := func(name, selfMsg, otherSuffix, wornOff string, spellID int, durationSecs float64) Trigger {
		other := slowTarget + otherSuffix
		pattern := "^(?:" + other + ")$"
		if selfMsg != "" {
			pattern = "^(?:" + selfMsg + "|" + other + ")$"
		}
		return Trigger{
			Name:               name,
			Enabled:            true,
			Pattern:            pattern,
			WornOffPattern:     wornOff,
			TimerType:          TimerTypeDetrimental,
			TimerDurationSecs:  durationSecs,
			SpellID:            spellID,
			TimerTargetCapture: "target",
			BarColor:           "#ff0000",
			PackName:           packName,
			// General (class-agnostic) packs aren't run through
			// applyDefaultTimerAlerts (see AllPacks), so the standard
			// 10-second "expiring" TTS is set by hand here, same alert a
			// class pack's detrimental timer would get for free.
			TimerAlerts: []TimerAlert{detrimentalExpiringAlert()},
			Actions: []Action{
				{Type: ActionTextToSpeech, Text: name, DurationSecs: 5, Volume: 1.0},
			},
		}
	}
	return TriggerPack{
		PackName: packName,
		Description: "Detrimental-overlay timers for every slow a raid is likely to " +
			"eat — Beastlord, Shaman, Enchanter, Bard, Ranger, Necromancer, and " +
			"Rogue self-cast slows plus a handful of NPC-cast ones. Built from a " +
			"NAG export originally assembled by Takkisini and Sandrian, converted " +
			"and tested in PQC by Grimrose — thanks to all three for putting this " +
			"pack together.",
		Triggers: []Trigger{
			slow("BST slow (50-65%)", ``, ` loses their fighting edge\.`,
				`^You regain your fighting edge\.$`, 2942, 192),
			slow("BST slow (30%)", ``, ` feels lethargic\.`,
				`^You are no longer lethargic\.$`, 2634, 192),
			// BST's pet-cast slow (Sha's Vengeance) — not a memorized spell
			// (classes1-15 are all 255 in spells_new); the "other" branch is
			// what a raid member sees the pet land it on the mob. spell_fades
			// has no trailing period in the DB.
			slow("BST slow (55%)", ``, ` is wracked by the vengeance of Sha\.`,
				`^The pain subsides$`, 2679, 120),
			slow("SHM slow (40%)", ``, `'s body is covered in a brown mist\.`,
				`^The cloud disperses\.$`, 3380, 192),
			// Turgur's Insects — the longest-duration slow in the pack
			// (formula 7, base 65 ticks; only formula in this pack whose
			// level-60 result doesn't hit the formula's own cap).
			slow("SHM slow (?/75%)", ``, ` yawns\.`,
				`^You feel less drowsy\.$`, 1588, 360),
			slow("SHM slow (25%)", ``,
				`'s motions slow as a plague of insects chews at their skin\.`,
				`^The plague of insects subsides\.$`, 2527, 192),
			// Forlorn Deeds — "X slows down." is shared verbatim by several
			// ENC slow ranks (Tepid Deeds, Shiftless Deeds, Languid Pace,
			// Slow, Forlorn Deeds) at different levels, so the exact rank
			// landed can't be told from the log line alone — hence the "?"
			// in the name, matching the original pack.
			slow("ENC slow (?/70%)", ``, ` slows down\.`,
				`^Your speed returns\.$`, 1712, 192),
			slow("WAVE slow (20%)", ``, ` is slowed by the  mist of the seas\.`,
				`^The mist melts away\.$`, 1978, 60),
			// Earthcall — this pack previously "corrected" this to 192s,
			// reasoning it belonged to the same formula-6/base-35 family as
			// Forlorn Deeds and the other 192s slows above. spells_new
			// actually has Earthcall at formula 6 / base 25 — a smaller
			// base than that family — so capToBase(level/2+2, 25) caps at
			// the base (25 ticks) at level 60, giving 150s, matching the
			// original NAG export's value. Reverted to 150.
			slow("RNG slow (50%)", ``,
				` is slowed by the embracing earth\.`,
				`^The call of earth recedes\.$`, 1928, 150),
			// Deafen has no spell_fades text in spells_new (it wears off
			// silently), so there's no worn-off pattern to give it — same as
			// the original export.
			slow("BRD slow (40%)", ``, ` has been deafened\.`,
				``, 1748, 60),
			slow("BRD slow (55%)", ``, `'s rhythm slows\.`,
				``, 3066, 60),
			// Necromancer's slow line comes in at least two named ranks
			// (Shackle of Bone, Shackle of Spirit) that share the same
			// "is hindered by a shackle of ___" phrasing and both fold to
			// 192s at level 60 — matched by name so a future third rank with
			// the same phrasing needs adding here explicitly rather than
			// falling through a bare wildcard. Neither rank has a self-cast
			// branch (not a memorized spell — pet/proc only) or spell_fades
			// text.
			slow("NEC slow (35/70%)", ``, ` is hindered by a shackle of (?:bone|spirit)\.`,
				``, 2544, 192),
			// spells_new has Paralyzing Poison I as formula 6 / base 7 — the
			// base-7 cap binds well before level 60, giving 42s.
			slow("ROG slow (50%)", ``, `'s muscles lock\.`,
				`^Your feet come free\.$`, 1833, 42),
			// Waves of the Deep Sea — an NPC-cast slow (not a memorized
			// spell), no spell_fades text.
			slow("COLD slow (10%)", ``, ` is crushed by a wall of water\.`,
				``, 1972, 150),
			// Generic "Vas Ren" raid-mob slow line — reported in Discord as
			// still showing up untracked. Three near-identical spells share
			// this exact text (Vas Ren Slow x2, Hinderance of the Vas Ren);
			// the log line can't tell them apart, so no single spell_id is
			// bound here and the duration uses the more common of the two
			// underlying formulas (192s; the other candidate is a flat
			// 300s). The worn-off line is shared by all three and is exact,
			// so the timer clears correctly regardless of which one landed.
			slow("Generic Slow (Vas Ren)", ``, ` has been slowed\.`,
				`^You are no longer slowed\.$`, 0, 192),
		},
	}
}

// AllPacks returns all built-in trigger packs with default audio alerts
// applied. Class packs run through applyDefaultTimerAlerts so every timer
// trigger gets a fading/expiring TTS without each pack having to spell it
// out per-spell. The class-agnostic packs are returned as-is — the few
// that carry timer triggers of their own (Raid Alerts, Slows) set their
// TimerAlerts by hand instead.
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
		RaidEventTimersPack(),
		TrackingPack(),
		MiscAlertsPack(),
		SlowsPack(),
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
	// SetRefireCooldownSecs, when > 0, sets the trigger's silent anti-spam
	// refire lockout — same "only if currently 0" semantics as
	// SetCooldownSecs. 0 = leave as-is.
	SetRefireCooldownSecs float64
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
		{
			// 2026-07-10: add the five Ring War signature-spell recast
			// warnings to existing Raid Alerts installs, one insert per
			// spell (InsertTrigger only adds one trigger per update).
			Key:           "RaidAlerts:SigSpell:Fling:add-v1",
			PackName:      "Raid Alerts",
			InsertTrigger: func() *Trigger { t := raidSignatureSpellAlerts()[0]; return &t }(),
		},
		{
			Key:           "RaidAlerts:SigSpell:SilenceOfTheShadows:add-v1",
			PackName:      "Raid Alerts",
			InsertTrigger: func() *Trigger { t := raidSignatureSpellAlerts()[1]; return &t }(),
		},
		{
			Key:           "RaidAlerts:SigSpell:TorturingWinds:add-v1",
			PackName:      "Raid Alerts",
			InsertTrigger: func() *Trigger { t := raidSignatureSpellAlerts()[2]; return &t }(),
		},
		{
			Key:           "RaidAlerts:SigSpell:CausticMist:add-v1",
			PackName:      "Raid Alerts",
			InsertTrigger: func() *Trigger { t := raidSignatureSpellAlerts()[3]; return &t }(),
		},
		{
			Key:           "RaidAlerts:SigSpell:TouchOfVinitras:add-v1",
			PackName:      "Raid Alerts",
			InsertTrigger: func() *Trigger { t := raidSignatureSpellAlerts()[4]; return &t }(),
		},
		{
			// 2026-07-10: the five signature-spell triggers above shipped
			// without a refire lockout, so a single AE cast (broadcast as
			// one land-message line per raid member hit) stacked several
			// duplicate banner/TTS firings. Retrofit the cooldown onto
			// rows the add-v1 updates already inserted.
			Key:                   "RaidAlerts:SigSpell:Fling:refire-cooldown-v1",
			PackName:              "Raid Alerts",
			TriggerName:           "Fling",
			SetRefireCooldownSecs: 3,
		},
		{
			Key:                   "RaidAlerts:SigSpell:SilenceOfTheShadows:refire-cooldown-v1",
			PackName:              "Raid Alerts",
			TriggerName:           "Silence of the Shadows",
			SetRefireCooldownSecs: 3,
		},
		{
			Key:                   "RaidAlerts:SigSpell:TorturingWinds:refire-cooldown-v1",
			PackName:              "Raid Alerts",
			TriggerName:           "Torturing Winds",
			SetRefireCooldownSecs: 3,
		},
		{
			Key:                   "RaidAlerts:SigSpell:CausticMist:refire-cooldown-v1",
			PackName:              "Raid Alerts",
			TriggerName:           "Caustic Mist",
			SetRefireCooldownSecs: 3,
		},
		{
			Key:                   "RaidAlerts:SigSpell:TouchOfVinitras:refire-cooldown-v1",
			PackName:              "Raid Alerts",
			TriggerName:           "Touch of Vinitras",
			SetRefireCooldownSecs: 3,
		},
		{
			// 2026-07-19: tradeskill trainers reply "You are already
			// trained to the limit of your abilities in <skill>." when
			// asked to train a maxed-out tradeskill — wasn't excluded, so
			// it falsely fired the Incoming Tell alert. Reported by
			// Vortikai on Discord.
			Key:         "GroupAwareness:IncomingTell:tradeskill-limit-exclude-v1",
			PackName:    "General Triggers",
			TriggerName: "Incoming Tell",
			AppendExcludePatterns: []string{
				`tells you, 'You are already trained to the limit`,
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
		t.PackKey = packKeyOf(&t)
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
		t.CooldownSecs = float64(u.SetCooldownSecs)
		changed = true
	}

	// Set a missing refire-cooldown lockout, leaving a user-customized value alone.
	if u.SetRefireCooldownSecs > 0 && t.RefireCooldownSecs == 0 {
		t.RefireCooldownSecs = u.SetRefireCooldownSecs
		changed = true
	}

	if !changed {
		return false, nil
	}
	if err := store.Update(t); err != nil {
		return false, err
	}
	slog.Info("trigger: default update applied", "key", u.Key, "pack", u.PackName, "trigger", u.TriggerName, "added_excludes", added, "set_cooldown", u.SetCooldownSecs, "set_refire_cooldown", u.SetRefireCooldownSecs)
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
	if err := store.DeletePackBaselines(pack.PackName); err != nil {
		return err
	}
	return insertPackTriggers(store, pack)
}

// insertPackTriggers writes a pack's triggers, honoring DedupKey collisions
// against already-installed triggers from other packs. Factored out so both
// InstallPack (first-time install) and the promote-on-uninstall path can
// share the skip logic.
//
// For built-in packs it also snapshots each inserted trigger's RAW shipped
// definition (from AllPacks, not the character-defaulted copy the caller may
// be installing) into pack_baselines, which is what the pack-update diff
// compares future builds against. User-imported packs have no shipped
// definition to diff against, so they get no baselines.
func insertPackTriggers(store *Store, pack TriggerPack) error {
	raw := rawPackDefs(pack.PackName)
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
		t.PackKey = packKeyOf(t)
		if err := store.Insert(t); err != nil {
			return err
		}
		if def := raw[t.PackKey]; def != nil {
			if err := store.UpsertPackBaseline(pack.PackName, def); err != nil {
				return err
			}
		}
	}
	return nil
}

// rawPackDefs returns the current build's definitions for a built-in pack,
// keyed by pack_key, or nil when packName isn't a built-in pack.
func rawPackDefs(packName string) map[string]*Trigger {
	for _, p := range AllPacks() {
		if p.PackName != packName {
			continue
		}
		m := make(map[string]*Trigger, len(p.Triggers))
		for i := range p.Triggers {
			m[packKeyOf(&p.Triggers[i])] = &p.Triggers[i]
		}
		return m
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
	if err := store.DeletePackBaselines(packName); err != nil {
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
		t.PackKey = packKeyOf(&t)
		if err := store.Insert(&t); err != nil {
			return err
		}
		if err := store.UpsertPackBaseline(pack.PackName, &pack.Triggers[i]); err != nil {
			return err
		}
		slog.Info("trigger: promoted on uninstall",
			"pack", pack.PackName,
			"trigger", t.Name,
			"dedup_key", t.DedupKey)
	}
	return nil
}

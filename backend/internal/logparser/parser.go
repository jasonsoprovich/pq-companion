package logparser

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tsLayout is the Go time layout for EQ log timestamps.
// EQ uses ctime()-style formatting: [Mon Jan _2 15:04:05 2006]
// _2 handles space-padded single-digit days (e.g., " 3") and two-digit days.
// EQ writes the player's local time with no timezone marker, so parsing must
// use ParseInLocation(time.Local) — bare time.Parse would assume UTC and put
// every event hours into the past for any non-UTC player, causing freshly
// created spell timers to be pruned as already-expired.
const tsLayout = "[Mon Jan _2 15:04:05 2006]"

// tsLen is the fixed character length of an EQ log timestamp.
// "[Mon Apr 13 06:00:00 2026]" = 26 chars, followed by a space.
const tsLen = 26

var (
	// Zone change: "You have entered The North Karana."
	reZone = regexp.MustCompile(`^You have entered (.+)\.$`)

	// Spell begin casting: "You begin casting Mesmerization."
	reSpellCast = regexp.MustCompile(`^You begin casting (.+)\.$`)

	// Spell interrupted: "Your spell is interrupted." or
	// "Your <SpellName> spell is interrupted."
	reSpellInterruptNamed   = regexp.MustCompile(`^Your (.+) spell is interrupted\.$`)
	reSpellInterruptGeneric = regexp.MustCompile(`^Your spell is interrupted\.$`)

	// Spell resist: "Your target resisted the Mesmerization spell."
	reSpellResist = regexp.MustCompile(`^Your target resisted the (.+) spell\.$`)

	// Spell fade: "Your Mesmerization spell has worn off."
	reSpellFade = regexp.MustCompile(`^Your (.+) spell has worn off\.$`)

	// Spell fade from target: "Tashanian effect fades from Soandso."
	reSpellFadeFrom = regexp.MustCompile(`^(.+) effect fades from (.+)\.$`)

	// Spell did not take hold — buff overwritten by a stronger version.
	// Two known variants:
	//   "Your spell did not take hold."
	//   "Your spell did not take hold on your target."
	// EQ does not include the spell name; consumers correlate with the most
	// recent cast.
	reSpellDidNotTakeHold = regexp.MustCompile(`^Your spell did not take hold(?: on your target)?\.$`)

	// Combat — player hits NPC:
	// "You slash a gnoll for 150 points of damage."
	reYouHit = regexp.MustCompile(`^You (\w+) (.+) for (\d+) points? of damage\.$`)

	// Combat — NPC hits player:
	// "A gnoll slashes you for 50 points of damage."
	// "A wolf bites YOU for 10 points of damage." (Project Quarm / EQMac
	// emit "YOU" all-caps for incoming hits.)
	// The verb is conjugated with an -s/-es suffix when an NPC hits you.
	reNPCHitYou = regexp.MustCompile(`^(.+?) (?:\w+) [Yy][Oo][Uu] for (\d+) points? of damage\.$`)

	// Combat — third-party hit: another player (or NPC) hits a target that is
	// not the player. The actor can be multi-word ("Enchanted Golem",
	// "Sambata Tribal Member", "an enchanted golem"), so we anchor on a
	// known attack verb instead of assuming a single-word actor.
	// Examples:
	//   "Playerone slashes a gnoll for 75 points of damage."
	//   "Enchanted Golem hits Tank for 50 points of damage."
	//   "an enchanted golem slashes Hakammer for 100 points of damage."
	// Checked after reYouHit and reNPCHitYou so those take priority.
	reThirdPartyHit = regexp.MustCompile(`^(.+?) (slashes|slices|crushes|pierces|bashes|punches|kicks|slams|bites|mauls|claws|gores|stings|jabs|gouges|smashes|hits|strikes|backstabs|throws|chops|stabs|rends|frenzies on) (.+?) for (\d+) points? of damage\.$`)

	// Combat — player misses NPC: "You try to slash a gnoll, but miss!"
	reYouMiss = regexp.MustCompile(`^You try to (\w+) (.+?), but miss!$`)

	// Combat — NPC misses player: "A gnoll tries to slash you, but misses!"
	// PQ/EQMac uses "YOU" all-caps for the player target.
	reNPCMissYou = regexp.MustCompile(`^(.+?) tries to \w+ [Yy][Oo][Uu], but misses?!$`)

	// Non-melee damage — player's spell hits target (EQ passive form seen in own log):
	// "a giant wasp drone was hit by non-melee for 4 points of damage."
	reTargetHitNonMelee = regexp.MustCompile(`^(.+?) was hit by non-melee for (\d+) points? of damage\.$`)

	// Non-melee damage — named actor hits named target (other players' / NPCs' spells):
	// "Takkisina hit a temple skirmisher for 18 points of non-melee damage."
	// "A Shissar Arch Arcanist hit Takkisina for 640 points of non-melee damage."
	reNonMeleeHit = regexp.MustCompile(`^(.+?) hit (.+) for (\d+) points? of non-melee damage\.$`)

	// /con output — EQ's consider system. The NPC name precedes a fixed set of
	// disposition phrases. Ordered longest-first so "warmly regards you" and
	// "kindly regards you" are tried before the shorter "regards you".
	// Examples:
	//   "a grimling cadaverist regards you as an ally."
	//   "a gnoll scowls at you, ready to attack -- what would you like your tombstone to say?"
	reConsider = regexp.MustCompile(`^(.+?) (?:scowls at you|glares at you|looks your way|looks upon you|judges you|warmly regards you|kindly regards you|regards you|considers you)`)

	// Combat — dodge/parry/riposte/block by player:
	// "You dodge a gnoll's attack!"
	// "You parry a gnoll's attack!"
	// "You riposte a gnoll's attack!"
	// "You block a gnoll's attack!"
	reYouDefend = regexp.MustCompile(`^You (dodge|parry|riposte|block) (.+?)(?:'s)? attack!$`)

	// Death: "You have been slain by a gnoll."
	reDeath = regexp.MustCompile(`^You have been slain by (.+)\.$`)

	// Death (no killer): "You died."
	reDiedSimple = regexp.MustCompile(`^You died\.$`)

	// Kill — player slays mob: "You have slain a gnoll!"
	reYouKill = regexp.MustCompile(`^You have slain (.+)!$`)

	// Kill — group member slays mob (active form, e.g. some clients):
	// "Playerone has slain a gnoll!"
	reSomeoneSlay = regexp.MustCompile(`^(\w+) has slain (.+)!$`)

	// Kill — passive form used by Project Quarm / EQMac and many EMU servers
	// for third-party kills the player witnesses:
	// "a lightcrawler has been slain by Ineka!"
	// "Zun Thall Xakra has been slain by Stonae!"
	// Target may be multi-word (NPC) or single-word (player); killer can be a
	// possessive pet name ("Gygr`s warder"), so the killer capture is also
	// loose.
	//
	// Note: the bare "X dies." form looks like a kill but is actually
	// Feign Death's cast_on_other text — left for the spell-landed pipeline
	// to classify (and it produces no timer because FD's duration is zero).
	reSlainByPassive = regexp.MustCompile(`^(.+) has been slain by (.+)!$`)

	// Heals — player heals a target:
	// "You healed Playerone for 150 hit points."
	// "You healed yourself for 150 hit points."
	reYouHeal = regexp.MustCompile(`^You healed (.+?) for (\d+) hit points?\.$`)

	// Heals — someone heals the player:
	// "Playerone healed you for 150 hit points."
	reHealedYou = regexp.MustCompile(`^(.+?) healed [Yy][Oo][Uu] for (\d+) hit points?\.$`)

	// Heals — third-party: another entity heals someone else.
	// "Playerone healed Playertwo for 150 hit points."
	// Checked after reYouHeal and reHealedYou so those take priority.
	reThirdPartyHeal = regexp.MustCompile(`^(\w+) healed (.+?) for (\d+) hit points?\.$`)

	// Pet owner binding — emitted by EQ when a charm spell takes hold:
	// "Kebartik says 'My leader is Kildrey.'"
	// Pet name allows possessive backtick (e.g. "Grimrose`s warder") and
	// other words; owner is a single player name.
	rePetOwner = regexp.MustCompile(`^(.+?) says '?My leader is (\w+)\.'?$`)

	// Illusion buff dropped — two distinct EQ messages, neither names the
	// race so we treat both as "all illusions on the active player ended":
	//   "Your illusion fades."
	//   "You forget Illusion: <Race>."
	reIllusionFadeNatural = regexp.MustCompile(`^Your illusion fades\.$`)
	reIllusionForget      = regexp.MustCompile(`^You forget Illusion: .+\.$`)
)

// ParseRawLine extracts the timestamp and message from any valid EQ log line
// without classifying it as a specific event type. Use this when you need to
// process every log line (e.g. custom trigger matching) regardless of whether
// the line matches a known event pattern.
// Returns (timestamp, message, true) on success, or zero values and false if
// the line does not start with a valid EQ timestamp.
func ParseRawLine(line string) (time.Time, string, bool) {
	if len(line) < tsLen+1 || line[0] != '[' {
		return time.Time{}, "", false
	}
	ts, err := time.ParseInLocation(tsLayout, line[:tsLen], time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return ts, line[tsLen+1:], true
}

// ParseLine parses a single raw line from an EQ log file into a LogEvent.
// Returns (event, true) on success, or (zero, false) if the line is not a
// recognised EQ log format or does not match any known event pattern.
func ParseLine(line string) (LogEvent, bool) {
	if len(line) < tsLen+1 || line[0] != '[' {
		return LogEvent{}, false
	}

	tsStr := line[:tsLen]
	ts, err := time.ParseInLocation(tsLayout, tsStr, time.Local)
	if err != nil {
		return LogEvent{}, false
	}

	message := line[tsLen+1:] // skip the trailing space after the timestamp

	ev, ok := classifyMessage(message)
	if !ok {
		return LogEvent{}, false
	}

	ev.Timestamp = ts
	ev.Message = message
	return ev, true
}

// classifyMessage matches a bare log message (timestamp stripped) to a known
// event type and returns a partially-filled LogEvent (no Timestamp/Message yet).
func classifyMessage(msg string) (LogEvent, bool) {
	// --- Zone change ---
	if m := reZone.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventZone,
			Data: ZoneData{ZoneName: m[1]},
		}, true
	}

	// --- Spell begin casting ---
	if m := reSpellCast.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventSpellCast,
			Data: SpellCastData{SpellName: m[1]},
		}, true
	}

	// --- Spell interrupted ---
	if m := reSpellInterruptNamed.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventSpellInterrupt,
			Data: SpellInterruptData{SpellName: m[1]},
		}, true
	}
	if reSpellInterruptGeneric.MatchString(msg) {
		return LogEvent{
			Type: EventSpellInterrupt,
			Data: SpellInterruptData{},
		}, true
	}

	// --- Spell resist ---
	if m := reSpellResist.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventSpellResist,
			Data: SpellResistData{SpellName: m[1]},
		}, true
	}

	// --- Spell fade ---
	if m := reSpellFade.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventSpellFade,
			Data: SpellFadeData{SpellName: m[1]},
		}, true
	}

	// --- Spell fade from target ---
	if m := reSpellFadeFrom.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventSpellFadeFrom,
			Data: SpellFadeFromData{SpellName: m[1], TargetName: m[2]},
		}, true
	}

	// --- Spell did not take hold ---
	if reSpellDidNotTakeHold.MatchString(msg) {
		return LogEvent{
			Type: EventSpellDidNotTakeHold,
			Data: SpellDidNotTakeHoldData{},
		}, true
	}

	// --- Illusion buff fade ---
	// Matched before generic combat / cast-index patterns so the very short
	// "Your illusion fades." line can't be misclassified as something else.
	if reIllusionFadeNatural.MatchString(msg) || reIllusionForget.MatchString(msg) {
		return LogEvent{
			Type: EventIllusionFade,
			Data: IllusionFadeData{},
		}, true
	}

	// --- Player hits NPC ---
	if m := reYouHit.FindStringSubmatch(msg); m != nil {
		// Guard: auxiliary verbs ("have", "are", "were", etc.) indicate passive
		// constructions like "You have been healed for X points of damage." that
		// are not combat hits. Only real attack verbs (slash, kick, bash…) pass.
		verb := strings.ToLower(m[1])
		if verb != "have" && verb != "are" && verb != "were" && verb != "been" && verb != "is" {
			dmg, _ := strconv.Atoi(m[3])
			return LogEvent{
				Type: EventCombatHit,
				Data: CombatHitData{
					Actor:  "You",
					Skill:  m[1],
					Target: m[2],
					Damage: dmg,
				},
			}, true
		}
	}

	// --- Player's spell hits target (passive non-melee form) ---
	if m := reTargetHitNonMelee.FindStringSubmatch(msg); m != nil {
		dmg, _ := strconv.Atoi(m[2])
		return LogEvent{
			Type: EventCombatHit,
			Data: CombatHitData{
				Actor:  "You",
				Skill:  "spell",
				Target: m[1],
				Damage: dmg,
			},
		}, true
	}

	// --- Named entity hits another with non-melee (spell damage) ---
	if m := reNonMeleeHit.FindStringSubmatch(msg); m != nil {
		actor := m[1]
		target := m[2]
		dmg, _ := strconv.Atoi(m[3])
		if strings.EqualFold(target, "you") {
			target = "You"
		}
		return LogEvent{
			Type: EventCombatHit,
			Data: CombatHitData{
				Actor:  actor,
				Skill:  "spell",
				Target: target,
				Damage: dmg,
			},
		}, true
	}

	// --- NPC hits player ---
	if m := reNPCHitYou.FindStringSubmatch(msg); m != nil {
		dmg, _ := strconv.Atoi(m[2])
		skill := extractVerb(msg, m[1])
		return LogEvent{
			Type: EventCombatHit,
			Data: CombatHitData{
				Actor:  m[1],
				Skill:  skill,
				Target: "You",
				Damage: dmg,
			},
		}, true
	}

	// --- Player misses NPC ---
	if m := reYouMiss.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventCombatMiss,
			Data: CombatMissData{
				Actor:    "You",
				Target:   m[2],
				MissType: "miss",
			},
		}, true
	}

	// --- NPC misses player ---
	if m := reNPCMissYou.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventCombatMiss,
			Data: CombatMissData{
				Actor:    m[1],
				Target:   "You",
				MissType: "miss",
			},
		}, true
	}

	// --- Player dodges/parries/ripostes/blocks ---
	if m := reYouDefend.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventCombatMiss,
			Data: CombatMissData{
				Actor:    m[2],
				Target:   "You",
				MissType: m[1],
			},
		}, true
	}

	// --- Third-party hit (other players hitting NPCs, etc.) ---
	// Checked after all player/NPC-specific patterns to avoid false matches.
	if m := reThirdPartyHit.FindStringSubmatch(msg); m != nil {
		// Guard: skip if actor is "You" (reYouHit already handled it), if the
		// target contains "you" (reNPCHitYou already handled it), or if the
		// actor is a bare article ("a", "an", "the") — this means the regex
		// captured only the first word of a multi-word NPC name.
		if m[1] != "You" && !strings.EqualFold(m[3], "you") && !isArticle(m[1]) {
			dmg, _ := strconv.Atoi(m[4])
			return LogEvent{
				Type: EventCombatHit,
				Data: CombatHitData{
					Actor:  m[1],
					Skill:  m[2],
					Target: m[3],
					Damage: dmg,
				},
			}, true
		}
	}

	// --- Player heals a target ---
	if m := reYouHeal.FindStringSubmatch(msg); m != nil {
		amt, _ := strconv.Atoi(m[2])
		target := m[1]
		if strings.EqualFold(target, "yourself") {
			target = "You"
		}
		return LogEvent{
			Type: EventHeal,
			Data: HealData{Actor: "You", Target: target, Amount: amt},
		}, true
	}

	// --- Someone heals the player ---
	if m := reHealedYou.FindStringSubmatch(msg); m != nil {
		amt, _ := strconv.Atoi(m[2])
		return LogEvent{
			Type: EventHeal,
			Data: HealData{Actor: m[1], Target: "You", Amount: amt},
		}, true
	}

	// --- Third-party heal ---
	if m := reThirdPartyHeal.FindStringSubmatch(msg); m != nil {
		if m[1] != "You" && !strings.EqualFold(m[2], "you") {
			amt, _ := strconv.Atoi(m[3])
			return LogEvent{
				Type: EventHeal,
				Data: HealData{Actor: m[1], Target: m[2], Amount: amt},
			}, true
		}
	}

	// --- Player slays mob ---
	if m := reYouKill.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventKill,
			Data: KillData{Killer: "You", Target: m[1]},
		}, true
	}

	// --- Group member slays mob (active form) ---
	if m := reSomeoneSlay.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventKill,
			Data: KillData{Killer: m[1], Target: m[2]},
		}, true
	}

	// --- Passive-form kill ("X has been slain by Y!") ---
	// Project Quarm / EQMac use this for every third-party kill the player
	// witnesses. Without this branch, raid-target deaths never produce
	// EventKill so the combat tracker's active fight is left alive until
	// the inactivity timeout — which is rare in practice because heals and
	// re-engages keep extending it.
	if m := reSlainByPassive.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventKill,
			Data: KillData{Killer: m[2], Target: m[1]},
		}, true
	}

	// --- Death ---
	if m := reDeath.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventDeath,
			Data: DeathData{SlainBy: m[1]},
		}, true
	}
	if reDiedSimple.MatchString(msg) {
		return LogEvent{
			Type: EventDeath,
			Data: DeathData{},
		}, true
	}

	// --- Pet owner binding ---
	// Matched before /con because both consume "<name> says ..." style lines.
	if m := rePetOwner.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventPetOwner,
			Data: PetOwnerData{Pet: m[1], Owner: m[2]},
		}, true
	}

	// --- /con result ---
	if m := reConsider.FindStringSubmatch(msg); m != nil {
		// NPC names never start with "You" — guard against player-action lines
		// (e.g. "You have entered …") that the regex could otherwise match if
		// they contain a disposition phrase elsewhere in the text.
		if !strings.HasPrefix(m[1], "You") {
			return LogEvent{
				Type: EventConsidered,
				Data: ConsideredData{TargetName: m[1]},
			}, true
		}
	}

	// --- Spell landed (cast_on_you / cast_on_other) ---
	// Tried last so structured event patterns take priority; this avoids
	// misclassifying combat/heal/zone lines whose phrasing might happen to
	// resemble a spell's flavor text.
	if idx := activeCastIndex.Load(); idx != nil {
		if cm := idx.Match(msg); cm != nil {
			return LogEvent{
				Type: EventSpellLanded,
				Data: spellLandedData(cm),
			}, true
		}
	}

	return LogEvent{}, false
}

// spellLandedData converts a CastMatch into the JSON payload emitted on the
// wire. Candidates is populated only for ambiguous matches so the typical
// (unique-text) case stays compact.
func spellLandedData(cm *CastMatch) SpellLandedData {
	d := SpellLandedData{
		SpellID:    cm.SpellID,
		SpellName:  cm.SpellName,
		TargetName: cm.TargetName,
	}
	switch cm.Kind {
	case MatchSelf:
		d.Kind = SpellLandedKindYou
	case MatchOther:
		d.Kind = SpellLandedKindOther
	}
	if cm.SpellID == 0 && len(cm.Candidates) > 1 {
		d.Candidates = make([]SpellLandedCandidate, 0, len(cm.Candidates))
		for _, c := range cm.Candidates {
			d.Candidates = append(d.Candidates, SpellLandedCandidate{
				SpellID:   c.SpellID,
				SpellName: c.SpellName,
			})
		}
	}
	return d
}

// extractVerb attempts to pull the conjugated attack verb out of an NPC hit
// message. The actor name is known, so the verb immediately follows it.
// e.g. "A gnoll slashes you for 50 points of damage." → "slashes"
// Returns an empty string if the verb cannot be extracted.
func extractVerb(msg, actor string) string {
	rest := strings.TrimPrefix(msg, actor+" ")
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// isArticle reports whether word is a bare English article (a, an, the).
// Used to prevent the third-party hit regex from misparsing multi-word NPC
// names where only the leading article is captured as the actor.
func isArticle(word string) bool {
	switch strings.ToLower(word) {
	case "a", "an", "the":
		return true
	}
	return false
}

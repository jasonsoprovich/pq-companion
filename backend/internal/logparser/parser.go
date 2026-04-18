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

	// Combat — player hits NPC:
	// "You slash a gnoll for 150 points of damage."
	reYouHit = regexp.MustCompile(`^You (\w+) (.+) for (\d+) points? of damage\.$`)

	// Combat — NPC hits player:
	// "A gnoll slashes you for 50 points of damage."
	// The verb is conjugated with an -s/-es suffix when an NPC hits you.
	reNPCHitYou = regexp.MustCompile(`^(.+?) (?:\w+) [Yy]ou for (\d+) points? of damage\.$`)

	// Combat — third-party hit: another player (or NPC) hits a target that is
	// not the player. EQ player names are always single words (no spaces).
	// "Playerone slashes a gnoll for 75 points of damage."
	// Checked after reYouHit and reNPCHitYou so those take priority.
	reThirdPartyHit = regexp.MustCompile(`^(\w+) (\w+) (.+?) for (\d+) points? of damage\.$`)

	// Combat — player misses NPC: "You try to slash a gnoll, but miss!"
	reYouMiss = regexp.MustCompile(`^You try to (\w+) (.+?), but miss!$`)

	// Combat — NPC misses player: "A gnoll tries to slash you, but misses!"
	reNPCMissYou = regexp.MustCompile(`^(.+?) tries to \w+ you, but misses?!$`)

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

	// Kill — group member slays mob: "Playerone has slain a gnoll!"
	reSomeoneSlay = regexp.MustCompile(`^(\w+) has slain (.+)!$`)

	// Heals — player heals a target:
	// "You healed Playerone for 150 hit points."
	// "You healed yourself for 150 hit points."
	reYouHeal = regexp.MustCompile(`^You healed (.+?) for (\d+) hit points?\.$`)

	// Heals — someone heals the player:
	// "Playerone healed you for 150 hit points."
	reHealedYou = regexp.MustCompile(`^(.+?) healed [Yy]ou for (\d+) hit points?\.$`)

	// Heals — third-party: another entity heals someone else.
	// "Playerone healed Playertwo for 150 hit points."
	// Checked after reYouHeal and reHealedYou so those take priority.
	reThirdPartyHeal = regexp.MustCompile(`^(\w+) healed (.+?) for (\d+) hit points?\.$`)
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
	ts, err := time.Parse(tsLayout, line[:tsLen])
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
	ts, err := time.Parse(tsLayout, tsStr)
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

	// --- Player hits NPC ---
	if m := reYouHit.FindStringSubmatch(msg); m != nil {
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

	// --- Group member slays mob ---
	if m := reSomeoneSlay.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventKill,
			Data: KillData{Killer: m[1], Target: m[2]},
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

	return LogEvent{}, false
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

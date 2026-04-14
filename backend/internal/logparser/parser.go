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
	reSpellInterruptNamed = regexp.MustCompile(`^Your (.+) spell is interrupted\.$`)
	reSpellInterruptGeneric = regexp.MustCompile(`^Your spell is interrupted\.$`)

	// Spell resist: "Your target resisted the Mesmerization spell."
	reSpellResist = regexp.MustCompile(`^Your target resisted the (.+) spell\.$`)

	// Spell fade: "Your Mesmerization spell has worn off."
	reSpellFade = regexp.MustCompile(`^Your (.+) spell has worn off\.$`)

	// Combat — player hits NPC:
	// "You slash a gnoll for 150 points of damage."
	reYouHit = regexp.MustCompile(`^You (\w+) (.+) for (\d+) points? of damage\.$`)

	// Combat — NPC hits player:
	// "A gnoll slashes you for 50 points of damage."
	// The verb is conjugated with an -s/-es suffix when an NPC hits you.
	reNPCHitYou = regexp.MustCompile(`^(.+?) (?:\w+) [Yy]ou for (\d+) points? of damage\.$`)

	// Combat — player misses NPC: "You try to slash a gnoll, but miss!"
	reYouMiss = regexp.MustCompile(`^You try to (\w+) (.+?), but miss!$`)

	// Combat — NPC misses player: "A gnoll tries to slash you, but misses!"
	reNPCMissYou = regexp.MustCompile(`^(.+?) tries to \w+ you, but misses?!$`)

	// Combat — dodge/parry/riposte/block by player:
	// "You dodge a gnoll's attack!"
	// "You parry a gnoll's attack!"
	// "You riposte a gnoll's attack!"
	// "You block a gnoll's attack!"
	reYouDefend = regexp.MustCompile(`^You (dodge|parry|riposte|block) (.+?)(?:'s)? attack!$`)

	// Death: "You have been slain by a gnoll."
	reDeath = regexp.MustCompile(`^You have been slain by (.+)\.$`)
)

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

	// --- Death ---
	if m := reDeath.FindStringSubmatch(msg); m != nil {
		return LogEvent{
			Type: EventDeath,
			Data: DeathData{SlainBy: m[1]},
		}, true
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

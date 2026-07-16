// Package lockout tracks per-character raid lockouts from two log sources and
// persists them as a per-character snapshot in user.db. Each entry records,
// for a raid target / legacy item, the absolute instant the player's lockout
// expires — derived once from the log line's timestamp plus the relative
// duration the game printed. Storing the absolute expiry (rather than a
// countdown) is what lets the UI show a live, continuously-counting-down
// timer even after the game and app have been closed for a while, and flip a
// row to "available" once the instant passes.
//
// The primary source is the per-target kill notice, printed the instant a
// lockout is applied with no player action required:
//
//	You have incurred a lockout for Diabo Xi Xin Thall that expires in 6 Days and 18 Hours.
//
// The secondary source is the `/sll` ("show loot lockouts") command, which the
// player can run in-game for a full, second-precision resync; its snapshot
// overwrites anything the kill notices inserted. `/sll` output has no command
// echo and no footer line, so the consumer detects the block the same way the
// keyring tracker detects /keys: a burst of recognisable lines committed on
// the first unrelated line or after a short idle. The block does carry
// section headers, which anchor parsing:
//
//	=== Current Loot Lockouts ===
//	== King Tranix: Available
//	== Lord Nagafen: Expires in 5 Hours, 50 Minutes, and 55 Seconds
//	...
//	=== Current Legacy Item Lockouts ===
//	== Shining Metallic Robes: Available
package lockout

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Section discriminates the two lists `/sll` prints. The value is normalised
// from the header text so a future additional section still parses (anything
// containing "Legacy" is legacy; everything else is loot).
type Section string

const (
	SectionLoot   Section = "loot"
	SectionLegacy Section = "legacy"
)

var (
	// Section header: "=== Current Loot Lockouts ===" / "=== Current Legacy
	// Item Lockouts ===". The inner label is captured but we only use it to
	// pick the normalised Section.
	reHeader = regexp.MustCompile(`^=== Current (.+?) ===$`)

	// One lockout row: "== <Name>: Available" or
	// "== <Name>: Expires in <duration>". The name is non-greedy up to the
	// first ": " so multi-word, backticked, and lowercase names all work; it
	// is TrimSpace'd by the caller to absorb the stray "Name :" spacing seen
	// in some rows (e.g. "== Shei Vinitras : Available").
	reRow = regexp.MustCompile(`^== (.+?): (Available|Expires in (.+))$`)

	// Individual duration units within "5 Days, 13 Hours, 25 Minutes, and 7
	// Seconds". Units are omitted when zero and appear in singular or plural
	// form, so each is matched independently rather than positionally.
	reDays    = regexp.MustCompile(`(\d+)\s+Days?`)
	reHours   = regexp.MustCompile(`(\d+)\s+Hours?`)
	reMinutes = regexp.MustCompile(`(\d+)\s+Minutes?`)
	reSeconds = regexp.MustCompile(`(\d+)\s+Seconds?`)

	// Per-target lockout notice printed the instant a lockout is incurred
	// (typically on a raid boss kill), independent of `/sll`:
	// "You have incurred a lockout for <Name> that expires in <duration>."
	reIncurred = regexp.MustCompile(`^You have incurred a lockout for (.+?) that expires in (.+?)\.?$`)
)

// IsHeader reports whether msg is an `/sll` section header and, if so, returns
// the normalised section it begins.
func IsHeader(msg string) (Section, bool) {
	m := reHeader.FindStringSubmatch(msg)
	if m == nil {
		return "", false
	}
	if strings.Contains(m[1], "Legacy") {
		return SectionLegacy, true
	}
	return SectionLoot, true
}

// Row is one parsed lockout line, before the active character / observed time
// are stamped on it.
type Row struct {
	// TargetName is the boss / legacy-item name with surrounding space trimmed.
	TargetName string
	// Available is true for "Available" rows (no active lockout).
	Available bool
	// Remaining is the parsed time until the lockout expires. Zero when
	// Available is true.
	Remaining time.Duration
}

// ParseRow parses a single "== ..." lockout row. Returns ok=false for any line
// that is not a recognised lockout row (headers, chat, etc.).
func ParseRow(msg string) (Row, bool) {
	m := reRow.FindStringSubmatch(msg)
	if m == nil {
		return Row{}, false
	}
	name := strings.TrimSpace(m[1])
	if name == "" {
		return Row{}, false
	}
	// m[2] is either "Available" or "Expires in <duration>"; m[3] is the
	// duration text (empty for the Available branch).
	if m[3] == "" {
		return Row{TargetName: name, Available: true}, true
	}
	d, ok := parseDuration(m[3])
	if !ok {
		return Row{}, false
	}
	return Row{TargetName: name, Remaining: d}, true
}

// ParseIncurred parses a "You have incurred a lockout for <Name> that expires
// in <duration>." notice — printed the instant a lockout is applied (e.g. on a
// raid boss kill), separately from `/sll`. Returns ok=false for any
// non-matching line.
func ParseIncurred(msg string) (name string, remaining time.Duration, ok bool) {
	m := reIncurred.FindStringSubmatch(msg)
	if m == nil {
		return "", 0, false
	}
	name = strings.TrimSpace(m[1])
	if name == "" {
		return "", 0, false
	}
	d, found := parseDuration(m[2])
	if !found {
		return "", 0, false
	}
	return name, d, true
}

// parseDuration converts an `/sll` remaining-time phrase such as
// "5 Days, 13 Hours, 25 Minutes, and 7 Seconds" (with any units omitted) into
// a time.Duration. Returns ok=false only when no unit at all is recognised, so
// a malformed row doesn't masquerade as an instantly-expiring lockout.
func parseDuration(s string) (time.Duration, bool) {
	var total time.Duration
	found := false
	if m := reDays.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total += time.Duration(n) * 24 * time.Hour
		found = true
	}
	if m := reHours.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total += time.Duration(n) * time.Hour
		found = true
	}
	if m := reMinutes.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total += time.Duration(n) * time.Minute
		found = true
	}
	if m := reSeconds.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		total += time.Duration(n) * time.Second
		found = true
	}
	return total, found
}

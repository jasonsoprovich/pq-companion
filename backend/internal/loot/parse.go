// Package loot persists item loot lines from the log ("--Name has looted a
// X.--" / "--You have looted a X.--") into a searchable feed. It mirrors the
// players/chat tracker shape: a raw-line consumer for live capture, a
// dedup-safe backfill handler, and a store with filtered queries.
package loot

import (
	"regexp"
	"strings"
)

// reOther matches another player's loot: "--Soandso has looted a Item Name.--".
// The article (a/an/the) is optional and stripped; on Project Quarm it's
// always "a " in practice, even before a vowel.
var reOther = regexp.MustCompile(`^--(\w+) has looted (?:an? |the )?(.+)\.--$`)

// reSelf matches the local character's loot: "--You have looted a Item.--".
var reSelf = regexp.MustCompile(`^--You have looted (?:an? |the )?(.+)\.--$`)

// Parsed is one matched loot line. Player is the looter's name (already
// capitalized); for self-loot Player is empty and Self is true so the caller
// can attribute it to the active character.
type Parsed struct {
	Player string
	Item   string
	Self   bool
}

// CapitalizeName normalizes an EQ player name to canonical casing.
func CapitalizeName(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

// ParseLoot classifies a bare log message (timestamp stripped) as a loot line.
func ParseLoot(msg string) (Parsed, bool) {
	if m := reSelf.FindStringSubmatch(msg); m != nil {
		return Parsed{Self: true, Item: m[1]}, true
	}
	if m := reOther.FindStringSubmatch(msg); m != nil {
		return Parsed{Player: CapitalizeName(m[1]), Item: m[2]}, true
	}
	return Parsed{}, false
}

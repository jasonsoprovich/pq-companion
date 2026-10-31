package popflag

import (
	"strings"
	"time"
)

// Hail-grant detection: the always-up PoP hail NPCs annotated with
// GrantLines all set their qglobal in the same Lua shape —
//
//	e.other:Message(0, "<NPC line>")
//	eq.set_global("<qglobal>", "<value>", 5, "F")
//	e.other:Message(15, "You have received a character flag!")
//
// so a grant is recognised only as an exact PAIR of consecutive log lines:
// one of a step's GrantLines immediately followed by the server's
// character-flag line. The flag line is a speakerless system message only the
// granted player receives, so a player who happens to share an NPC's name
// can't spoof the pair with a /tell, and an NPC's broadcast Say (Miak) seen
// while a groupmate does the step never matches on its own. Each GrantLine
// prints only in qglobal states at or above the step's own value, so a repeat
// hail (which re-prints both lines) can never mark anything that isn't done.
//
// NOT every progression step prints the flag line, which is why grant lines
// are opt-in per step rather than inferred. Audited against SecretsOTheP/quests
// (572bc06): the HoH trials set hohtrials with flavor text only, the Grummus
// projection sets grummus (CoD access) silently, the Mithaniel projection sets
// mmarr_book silently, and a projection hailed without its preflag prints
// "You have received a new checklist flag!" instead — none of those are
// matched. Steps without a flag line simply aren't auto-detected here.
//
// Verified against a live Quarm log (2026-10): the pair always lands as two
// consecutive lines with the same timestamp, in both server wordings below.

// flagGrantLines are the server's character-flag confirmations. Elder
// Poxbourne's Lua uses the shorter wording; every other NPC the longer one.
var flagGrantLines = map[string]bool{
	"You have received a character flag!": true,
	"You receive a character flag!":       true,
}

// IsFlagGrantLine reports whether msg is the server's character-flag line.
func IsFlagGrantLine(msg string) bool {
	return flagGrantLines[strings.TrimSpace(msg)]
}

// MatchGrantLine returns the ID of the flag whose GrantLines match msg — the
// NPC half of a grant pair — and whether one did. The match is a prefix match
// after collapsing whitespace runs, since the Lua text carries double spaces
// after sentences that a log viewer or copy-paste may not preserve.
func MatchGrantLine(msg string) (string, bool) {
	load()
	norm := collapseSpaces(msg)
	if norm == "" {
		return "", false
	}
	for _, f := range flags {
		for _, gl := range f.GrantLines {
			if strings.HasPrefix(norm, collapseSpaces(gl)) {
				return f.ID, true
			}
		}
	}
	return "", false
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// pendingGrant is the NPC half of a grant pair, waiting for the next line.
type pendingGrant struct {
	flagID    string
	character string
	at        time.Time
}

// grantPairWindow bounds how far apart the two halves of a pair may be. Both
// are printed by the same Lua call so they share a timestamp in practice; the
// one-second slack covers a second boundary falling between them.
const grantPairWindow = time.Second

// HailReminder is one hail the character can do right now in a zone.
type HailReminder struct {
	FlagID string `json:"flag_id"`
	NPC    string `json:"npc"`
	Label  string `json:"label"`
}

// PendingHails returns the always-up hail steps whose NPC stands in zone
// (long name, as in "You have entered Plane of Tranquility.") that the
// resolved character can do now: not done, not locked behind an unmet
// prereq, not a superseded any-of member, and not optional. Dataset order.
func PendingHails(r Resolved, zone string) []HailReminder {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return nil
	}
	var out []HailReminder
	for _, f := range r.Flags {
		if f.HailZone == "" || !strings.EqualFold(f.HailZone, zone) {
			continue
		}
		if f.Done || f.Locked || f.Superseded || f.Optional {
			continue
		}
		out = append(out, HailReminder{FlagID: f.ID, NPC: f.HailNPC, Label: f.Label})
	}
	return out
}

package popflag

import (
	"fmt"
	"strings"
	"time"
)

// Quintessence of Elements and Plane of Time grants (Project Quarm patch of
// 2026-11-23, SecretsOTheP/quests lua_modules/quintessence_progression.lua @
// 05a3ec0).
//
// Hailing Grand Librarian Maelin or Chronographer Muon, or clicking the
// Plane of Innovation Time machine, while carrying the Quintessence sets every
// earlier planar progression qglobal at once and, when anything changed,
// prints one system line to that player only:
//
//	e:Message(15, "The Quintessence of Elements completes your earlier planar progression. Activate the machine ...")
//
// It's speakerless (a /tell or say carries "X tells you," in front, so it
// can't spoof the prefix match) and isn't followed by a character-flag line,
// so it's matched on its own rather than as a grant pair.
//
// The Time machine then grants Time access with the pair in the reverse
// order from the NPC hails, character-flag line FIRST:
//
//	e.self:Message(15, "You have received a character flag!")
//	e.self:Message(257, "The ages begin to tear through your body.  You wake to find yourself in another time.")

// quintessenceLine is the Quintessence completion line's leading sentence.
const quintessenceLine = "The Quintessence of Elements completes your earlier planar progression."

// timeGrantLine is the Time machine line that follows its character-flag line.
const timeGrantLine = "The ages begin to tear through your body."

// quintessenceQglobals mirrors the module's FLAGS table: the qglobal values
// the Quintessence sets.
var quintessenceQglobals = map[string]string{
	"mavuin": "3", "fuirstel": "5", "thelin": "4",
	"poi_door": "1", "zeks": "7", "aerindar": "2",
	"tylis": "2", "hohtrials": "111", "cipher": "1",
	"grummus": "1", "sol_room": "11111", "zebuxoruk": "2",
	"pofire": "2", "bertox_key": "1", "earthb_key": "1",
}

// quintessenceKills are the elemental god kills behind Time access. They have
// no qglobal, but the Quintessence is crafted from their drops and stands in
// for them, so they're marked with the rest of the earlier progression.
var quintessenceKills = []string{
	"poair_xegony", "powater_coirnav", "poearth_rathe", "pofire_fennin",
}

// IsQuintessenceLine reports whether msg is the Quintessence completion line.
func IsQuintessenceLine(msg string) bool {
	return strings.HasPrefix(collapseSpaces(msg), quintessenceLine)
}

// IsTimeGrantLine reports whether msg is the Time machine's line that follows
// its character-flag line.
func IsTimeGrantLine(msg string) bool {
	return strings.HasPrefix(collapseSpaces(msg), timeGrantLine)
}

// ApplyQuintessence records the Quintessence completing a character's
// earlier planar progression, built only from the store's own grant paths so
// it shares their precedence and bookkeeping: each scalar qglobal the
// Quintessence sets is folded in through ApplyHailGrant on the dataset flag
// it completes (raising the stored snapshot's value, never lowering it), and
// the bitmask steps it fills (HoH trials, Sol Ro minis) plus the elemental
// god kills are recorded with SetAuto. Existing rows are left untouched.
// Returns whether any new row was written.
func (s *Store) ApplyQuintessence(character string, observedAt time.Time) (bool, error) {
	if character == "" {
		return false, fmt.Errorf("character required")
	}
	grants, autos := quintessenceSteps()
	changed := false
	for _, id := range grants {
		c, err := s.ApplyHailGrant(character, id, observedAt)
		if err != nil {
			return changed, err
		}
		changed = changed || c
	}
	for _, id := range autos {
		c, err := s.SetAuto(character, id)
		if err != nil {
			return changed, err
		}
		changed = changed || c
	}
	return changed, nil
}

// quintessenceSteps maps quintessenceQglobals onto the dataset: grants are
// the flags whose scalar qglobal value it sets (one per qglobal, for
// ApplyHailGrant), autos the bitmask steps whose bit it sets plus
// quintessenceKills (for SetAuto). Dataset order.
func quintessenceSteps() (grants, autos []string) {
	load()
	for _, f := range flags {
		v, ok := quintessenceQglobals[f.Qglobal]
		switch {
		case !ok:
		case f.BitPosition > 0:
			if len(v) >= f.BitPosition && v[f.BitPosition-1] == '1' {
				autos = append(autos, f.ID)
			}
		case f.QglobalValue == v:
			grants = append(grants, f.ID)
		}
	}
	return grants, append(autos, quintessenceKills...)
}

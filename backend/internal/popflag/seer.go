package popflag

import (
	"strconv"
	"strings"
)

// ParseSeer reconstructs a character's PoP qglobal map from the Seer Mal
// Nae`Shi "guided meditation" output (sit, say "guided meditation"). The Seer
// prints one deterministic line per qglobal state; this matches those lines as
// substrings and rebuilds the underlying qglobal values.
//
// Ported verbatim from the TAKP Seer Lua (testdata/pop-flags/seer_script.txt
// GuidedMeditation). NOTE: the real Quarm text may differ slightly once PoP is
// live; the phrase substrings here are the post-release reconciliation point
// (see docs/pop-flagging-plan.md open item #1).
//
// Reconstruction rules that aren't a plain 1:1 line→value:
//   - counters print only their CURRENT value's line → take the matched value
//   - hohtrials / sol_room are bitmasks → one line per set bit
//   - zeks is two parallel branches (Vallon=3, Tallon=4) that merge at 5 → both
//     "pack of notes" lines present means 5
//   - cipher deletes mmarr+saryrn and zebuxoruk deletes mmarr_book+karana
//     server-side, so once the replacement is set the originals' lines never
//     print; that any-of is resolved later in DeriveCompletion via SatisfiedBy.
func ParseSeer(text string) map[string]string {
	lc := strings.ToLower(text)
	has := func(s string) bool { return strings.Contains(lc, strings.ToLower(s)) }
	q := map[string]string{}

	// Single-value flags.
	if has("Your soul has formed a bond with the Plane of Time") {
		q["time"] = "1"
	}
	if has("The Cipher of the Divine Language appears on your arms") {
		q["cipher"] = "1"
	}
	if has("Now that Grummus has been destroyed, the entrance to the Crypt") {
		q["grummus"] = "1"
	}
	if has("You remember Nitram's words") {
		q["poi_door"] = "1"
	}
	if has("Saryrn been destroyed") {
		q["saryrn"] = "1"
	}
	if has("Mithaniel has been bested") {
		q["mmarr"] = "1"
	}
	if has("The information obtained from Mithaniel is written in a language") {
		q["mmarr_book"] = "1"
	}

	// zebuxoruk (1 = elemental access, 2 = PoTime prep). Replaces mmarr_book+karana.
	if has("Learning of Zebuxoruk's fate") {
		q["zebuxoruk"] = "2"
	} else if has("The History translated for you reveals the fate of Zebuxoruk") {
		q["zebuxoruk"] = "1"
	}

	// karana 1/2/4 (value 3 shares the value-2 line; 2 is the meaningful node).
	if has("The information obtained from Karana is written in a language") {
		q["karana"] = "4"
	} else if has("obtained the Talisman of Thunderous Foyer from Askr") {
		q["karana"] = "2"
	} else if has("You have shown your prowess in battle to Askr") {
		q["karana"] = "1"
	}

	// mavuin 1/2/3.
	if has("Mavuin is grateful to you") {
		q["mavuin"] = "3"
	} else if has("the Tribunal has agreed to reconsider Mavuin") {
		q["mavuin"] = "2"
	} else if has("The evidence of Mavuin is the only thing") {
		q["mavuin"] = "1"
	}

	// thelin 1/2/3/4.
	if has("Saved from a world of eternal nightmares, Thelin") {
		q["thelin"] = "4"
	} else if has("Terris Thule's grasp over Thelin has been released") {
		q["thelin"] = "3"
	} else if has("Thelin has completed his pact with Terris Thule") {
		q["thelin"] = "2"
	} else if has("Thelin being tormented by the imagery of Terris Thule") {
		q["thelin"] = "1"
	}

	// fuirstel 1/2/3/4/5.
	if has("Saved from certain doom, Milyk and Adler") {
		q["fuirstel"] = "5"
	} else if has("Bertoxxulous has been slain, the curse from Milyk") {
		q["fuirstel"] = "4"
	} else if has("Milyk has been saved from certain death") {
		q["fuirstel"] = "3"
	} else if has("Grummus has been destroyed, about his corpse you found a small ward") {
		q["fuirstel"] = "2"
	} else if has("Alder Fuirstel wishes you to obtain the Ward") {
		q["fuirstel"] = "1"
	}

	// tylis 1/2.
	if has("Tylis has been removed from his agony") {
		q["tylis"] = "2"
	} else if has("Tylis is being tortured by Saryrn") {
		q["tylis"] = "1"
	}

	// pofire 1/2.
	if has("The true route to the Plane of Fire is now clear") {
		q["pofire"] = "2"
	} else if has("The portal into the Plane of Fire has been altered") {
		q["pofire"] = "1"
	}

	// aerindar (only value 2 has Seer text).
	if has("You have bested Aerin`Dar and proven yourself honorable") {
		q["aerindar"] = "2"
	}

	// zeks — parallel Vallon(3)/Tallon(4) branches merging at 5, then 6/7.
	vallon := has("pack of notes from Vallon")
	tallon := has("pack of notes from Tallon")
	switch {
	case has("parchments of Rallos"):
		q["zeks"] = "7"
	case has("The words of Maelin echo in your mind"):
		q["zeks"] = "6"
	case vallon && tallon:
		q["zeks"] = "5"
	case tallon:
		q["zeks"] = "4"
	case vallon:
		q["zeks"] = "3"
	case has("Giwin would like you to find him in Drunder"):
		q["zeks"] = "2"
	}

	// hohtrials bitmask (3 wide). "completed all" implies all three bits.
	if has("You have completed all of Honor's Trials") {
		q["hohtrials"] = "111"
	} else {
		b := []byte("000")
		if has("beaten Rydda`Dar in the first of Honor's Trials") {
			b[0] = '1'
		}
		if has("saved the villagers in the second of Honor's Trials") {
			b[1] = '1'
		}
		if has("defeated the nomads in the third of Honor's Trials") {
			b[2] = '1'
		}
		if string(b) != "000" {
			q["hohtrials"] = string(b)
		}
	}

	// sol_room bitmask (5 wide). Lines only print while pofire == 1.
	sb := []byte("00000")
	if has("Xuzl's arcane wisdom pulses in your mind") {
		sb[0] = '1'
	}
	if has("Arlyxir's wealth of knowledge flows through your mind") {
		sb[1] = '1'
	}
	if has("The power of Dresolik surges through you") {
		sb[2] = '1'
	}
	if has("Rizlona's song slips through your thoughts") {
		sb[3] = '1'
	}
	if has("Jiva's strength fills your body") {
		sb[4] = '1'
	}
	if string(sb) != "00000" {
		q["sol_room"] = string(sb)
	}

	return q
}

// MatchSeerLine reports whether a single log line is a Seer guided-meditation
// line — used by the live-log consumer to decide what to buffer.
func MatchSeerLine(line string) bool {
	return len(ParseSeer(line)) > 0
}

// DeriveCompletion returns the IDs of every dataset flag a reconstructed
// qglobal map marks complete. This is where the dataset's completion semantics
// are finally evaluated: counter thresholds, bitmask sub-steps, the cipher/
// zebuxoruk replacement any-of (SatisfiedBy), and the zeks Vallon/Tallon
// parallel-branch special case.
func DeriveCompletion(q map[string]string) []string {
	load()
	out := []string{}
	for _, f := range flags {
		if flagDone(f, q) {
			out = append(out, f.ID)
		}
	}
	return out
}

func flagDone(f PoPFlag, q map[string]string) bool {
	// zeks Vallon/Tallon merge at 5: at zeks>=5 both are done even though only
	// the higher branch's value is stored, and at 6/7 the per-branch lines no
	// longer print at all.
	switch f.ID {
	case "potac_vallon":
		z := atoi(q["zeks"])
		return z == 3 || z >= 5
	case "potac_tallon":
		z := atoi(q["zeks"])
		return z == 4 || z >= 5
	}

	if qglobalSatisfies(f.Qglobal, f.QglobalValue, f.Counter, f.BitPosition, q) {
		return true
	}
	// Replacement any-of: a node backed by a deleted qglobal is satisfied once
	// the replacement is present (cipher>=1, zebuxoruk>=1). Numeric >=.
	for _, c := range f.SatisfiedBy {
		if v, ok := q[c.Qglobal]; ok && atoi(v) >= atoi(c.Value) {
			return true
		}
	}
	return false
}

func qglobalSatisfies(qg, val string, counter bool, bit int, q map[string]string) bool {
	if qg == "" {
		return false
	}
	cur, ok := q[qg]
	if !ok {
		return false
	}
	if bit > 0 {
		return len(cur) >= bit && cur[bit-1] == '1'
	}
	if val == "" {
		val = "1"
	}
	if counter {
		return atoi(cur) >= atoi(val)
	}
	return cur == val || atoi(cur) >= atoi(val)
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

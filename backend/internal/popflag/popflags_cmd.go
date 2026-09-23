package popflag

import (
	"strconv"
	"strings"
)

// This file parses the player-facing '#popflags' command added in
// EQMacEmu/EQMacEmu PR #382 (zone/gm_commands/popflags.cpp, commit cec9e73a,
// 2026-09-06 — unchanged since). '#popflags [overview|1-5]' prints a
// deterministic report built directly from the character's PoP qglobals, the
// same underlying state the Seer Mal Nae`Shi "guided meditation" (seer.go)
// reconstructs from narrative text. Unlike the Seer reading (one big burst
// that covers every qglobal at once), a '#popflags' report only covers the
// section the player ran — 'overview' gives coarse per-tier status, while
// '#popflags 1'..'5' give one tier's full detail. Repeated syncs across
// different sections progressively fill in the character's stored snapshot
// (see Store.ApplyPopFlagsReport).
//
// Every literal string below is copied verbatim from popflags.cpp so a wording
// change upstream fails loudly (the line simply stops matching) rather than
// silently mis-parsing.

// PopFlagsSection identifies which '#popflags' report a block came from.
type PopFlagsSection string

const (
	PopFlagsOverview PopFlagsSection = "overview"
	PopFlagsTier1    PopFlagsSection = "tier1"
	PopFlagsTier2    PopFlagsSection = "tier2"
	PopFlagsTier3    PopFlagsSection = "tier3"
	PopFlagsTier4    PopFlagsSection = "tier4"
	PopFlagsTime     PopFlagsSection = "time"
)

var popFlagsHeaders = map[string]PopFlagsSection{
	"=== Planes of Power Progression ===": PopFlagsOverview,
	"=== Tier 1 Progression ===":          PopFlagsTier1,
	"=== Tier 2 Progression ===":          PopFlagsTier2,
	"=== Tier 3 Progression ===":          PopFlagsTier3,
	"=== Tier 4 Progression ===":          PopFlagsTier4,
	"=== Plane of Time ===":               PopFlagsTime,
}

// PopFlagsReport is everything ParsePopFlagsReport recovers from one
// '#popflags' block: qglobals it read an exact value (or confirmed absence)
// for, qglobals it can only confirm a floor for (a stage array position that
// implies every earlier stage also happened), and any pending checklist
// ('cl_*') flags the report named.
type PopFlagsReport struct {
	Section PopFlagsSection
	Exact   map[string]string // qglobal -> value ("" means confirmed absent)
	AtLeast map[string]int    // qglobal -> confirmed floor (numeric >=)
	Pending map[string]bool   // cl_* checklist flag names the report named
}

func newPopFlagsReport() PopFlagsReport {
	return PopFlagsReport{
		Exact:   map[string]string{},
		AtLeast: map[string]int{},
		Pending: map[string]bool{},
	}
}

// setAtLeast records a floor, keeping the highest seen if called twice for the
// same qglobal within one block (harmless — a block should never contradict
// itself, but this keeps the merge conservative just in case).
func (r PopFlagsReport) setAtLeast(qglobal string, floor int) {
	if cur, ok := r.AtLeast[qglobal]; !ok || floor > cur {
		r.AtLeast[qglobal] = floor
	}
}

// popFlagsPendingDesc maps the description text '#popflags' prints in a
// "Pending memory: X" line back to the 'cl_*' qglobal name (popflags.cpp's
// PopFlagsPrintPending calls). cl_mmarr and cl_mmarr_book are never printed
// this way upstream (PopFlagsHasPendingMemories checks them, but no call site
// names them individually) — matches this file's Section header comment.
var popFlagsPendingDesc = map[string]string{
	"Grummus":             "cl_grummus",
	"Thelin's hedge maze": "cl_maze",
	"Manaetic Behemoth":   "cl_behemoth",
	"Aerin`Dar":           "cl_aerindar",
	"Terris Thule":        "cl_terris",
	"Bertoxxulous":        "cl_bertox",
	"Keeper of Sorrows":   "cl_keeper",
	"Saryrn":              "cl_saryrn",
	"Vallon Zek":          "cl_vallon",
	"Tallon Zek":          "cl_tallon",
	"Rallos Zek":          "cl_rallos",
	"Karana":              "cl_karana",
	"Solusek Ro":          "cl_solusek",
}

// Stage-array text, copied verbatim from popflags.cpp. Index N-1 is the value
// PopFlagsPrintStage prints when the qglobal's exact current value is N (so
// unlike the Seer reconstruction, which sometimes can only infer a floor,
// matching one of these lines gives the qglobal's EXACT current value).
var (
	popFlagsMavuinStages = []string{
		"The evidence needed to save Mavuin has been requested",
		"The Tribunal has agreed to hear Mavuin's case",
		"Mavuin's case is complete",
	}
	popFlagsFuirstelStages = []string{
		"Obtain the Ward for Milyk",
		"The Ward was recovered",
		"Grummus was defeated",
		"Crypt of Decay access was granted",
		"Fuirstel progression complete",
	}
	popFlagsThelinStages = []string{
		"Help Thelin escape the hedge maze",
		"Defeat Terris Thule",
		"Terris Thule was defeated",
		"Thelin was released from Terris Thule",
		"Nightmare progression complete",
	}
	popFlagsAerindarStages = []string{
		"Aerin`Dar defeated; the meaning of Justice remains",
		"Halls of Honor access unlocked",
	}
	popFlagsTylisStages = []string{
		"Rescue Tylis from the Plane of Torment",
		"Tylis progression complete",
	}
	popFlagsZekStages = []string{
		"Initial Zek progression recorded",
		"Meet Giwin in Drunder",
		"Vallon Zek's information obtained",
		"Tallon Zek's information obtained",
		"Both Zek information sets obtained",
		"Defeat Rallos Zek",
		"Zek progression complete",
	}
	// Tier 2's and Tier 3's Karana arrays are distinct C arrays upstream, but
	// their non-"Complete" entries carry the same relative meaning and the two
	// print sites use different labels ("Askr and Karana progression" vs
	// "Agnarr and Karana progression"), so there's no ambiguity parsing them.
	popFlagsKaranaTier2Stages = []string{
		"Prove yourself to Askr",
		"Complete Mavuin's case, then use the Storms shrine to enter Bastion of Thunder",
		"Bastion of Thunder progression recorded",
		"Karana's information obtained",
	}
	popFlagsKaranaTier3Stages = []string{
		"Return to Askr",
		"Complete Mavuin's case, then use the Storms shrine to enter Bastion of Thunder",
		"Karana progression continues",
		"Karana's information obtained",
	}
)

// popFlagsStageLabels binds a printed label to its stage array and the
// qglobal it backs, for the generic stage-line matcher.
type popFlagsStageLabel struct {
	label   string
	qglobal string
	stages  []string
}

var popFlagsStageLabels = []popFlagsStageLabel{
	{"Mavuin's case", "mavuin", popFlagsMavuinStages},
	{"Fuirstel progression", "fuirstel", popFlagsFuirstelStages},
	{"Thelin progression", "thelin", popFlagsThelinStages},
	{"Aerin`Dar progression", "aerindar", popFlagsAerindarStages},
	{"Tylis progression", "tylis", popFlagsTylisStages},
	{"Giwin and Zek progression", "zeks", popFlagsZekStages},
}

// popFlagsKaranaLabels handles the two Karana print sites, which additionally
// have a "Complete; combined into Zebuxoruk lore" variant printed instead of
// the stage line once zebuxoruk is set (server text, not in the stage array).
var popFlagsKaranaLabels = []struct {
	label  string
	stages []string
}{
	{"Askr and Karana progression", popFlagsKaranaTier2Stages},
	{"Agnarr and Karana progression", popFlagsKaranaTier3Stages},
}

// popFlagsBoolLines are literal "Label: Value" lines with no numeric stage —
// each maps straight to an Exact assignment ("" = confirmed absent) or an
// AtLeast floor on a different qglobal (the Unlocked/Complete side of a few of
// these actually reports on a SEPARATE qglobal than the label's own name —
// e.g. an "Unlocked" access line can mean a companion flag was granted).
type popFlagsBoolEffect func(r PopFlagsReport)

func exactEffect(qglobal, value string) popFlagsBoolEffect {
	return func(r PopFlagsReport) { r.Exact[qglobal] = value }
}
func atLeastEffect(qglobal string, floor int) popFlagsBoolEffect {
	return func(r PopFlagsReport) { r.setAtLeast(qglobal, floor) }
}
func noEffect(PopFlagsReport) {}

var popFlagsLiteralLines = map[string]popFlagsBoolEffect{
	"Seventh Hammer access: Unlocked": exactEffect("seventh", "1"),
	"Seventh Hammer access: Locked":   exactEffect("seventh", ""),

	"Crypt of Decay access: Unlocked": exactEffect("grummus", "1"),
	"Crypt of Decay access: Locked":   exactEffect("grummus", ""),

	"Factory door access: Unlocked": exactEffect("poi_door", "1"),
	"Factory door access: Locked":   exactEffect("poi_door", ""),

	"Giwin and Manaetic Behemoth progression: Progress recorded": atLeastEffect("zeks", 1),
	"Giwin and Manaetic Behemoth progression: Not started":       exactEffect("zeks", ""),

	"Lower Crypt access: Unlocked": exactEffect("bertox_key", "1"),
	"Lower Crypt access: Locked":   exactEffect("bertox_key", ""),

	"Saryrn cipher half: Combined into Cipher": atLeastEffect("cipher", 1),
	"Saryrn cipher half: Complete":             exactEffect("saryrn", "1"),
	"Saryrn cipher half: Incomplete":           exactEffect("saryrn", ""),

	"Mithaniel Marr cipher half: Combined into Cipher": atLeastEffect("cipher", 1),
	"Mithaniel Marr cipher half: Complete":             exactEffect("mmarr", "1"),
	"Mithaniel Marr cipher half: Incomplete":           exactEffect("mmarr", ""),

	"Cipher information: Received": atLeastEffect("cipher", 1),
	"Cipher information: Missing":  noEffect,

	"Zebuxoruk lore: Received": atLeastEffect("zebuxoruk", 1),
	"Zebuxoruk lore: Missing":  noEffect,

	"Combined Zek information: Received": atLeastEffect("zeks", 5),
	"Combined Zek information: Missing":  noEffect,

	"Final elemental information: Received": atLeastEffect("zebuxoruk", 2),
	"Final elemental information: Missing":  noEffect,

	"Plane of Fire progression: Unlocked":    atLeastEffect("pofire", 2),
	"Plane of Fire progression: In progress": atLeastEffect("pofire", 1),
	"Plane of Fire progression: Not started": exactEffect("pofire", ""),

	"Plane of Earth B access: Unlocked": exactEffect("earthb_key", "1"),
	"Plane of Earth B access: Locked":   exactEffect("earthb_key", ""),

	"Plane of Time access: Unlocked": exactEffect("time", "1"),
	"Plane of Time access: Locked":   exactEffect("time", ""),

	"Air, Earth, and Water access: Unlocked": atLeastEffect("zebuxoruk", 2),
	"Air, Earth, and Water access: Locked":   noEffect,

	// Distinct print site from "Plane of Fire progression" above (Tier 4's
	// access summary vs Tier 3's Tower-of-Solusek-Ro detail), same meaning.
	"Plane of Fire access: Unlocked": atLeastEffect("pofire", 2),
	"Plane of Fire access: Locked":   noEffect,

	"Halls of Honor trials: None completed": exactEffect("hohtrials", "000"),
	"Tower wing flags: None completed":      exactEffect("sol_room", "00000"),

	// Overview per-tier status. "In progress" is intentionally absent — the
	// server's own tri-state can't be resolved to any specific qglobal floor
	// or absence, so it contributes nothing.
	"Tier 1: Complete": func(r PopFlagsReport) {
		r.setAtLeast("mavuin", 3)
		r.setAtLeast("fuirstel", 5)
		r.setAtLeast("thelin", 4)
		r.setAtLeast("poi_door", 1)
		r.setAtLeast("zeks", 2)
	},
	"Tier 1: Not started": func(r PopFlagsReport) {
		for _, q := range []string{"mavuin", "seventh", "fuirstel", "grummus", "thelin", "poi_door"} {
			r.Exact[q] = ""
		}
	},
	"Tier 2: Complete": func(r PopFlagsReport) {
		// karana>=2||zebuxoruk and saryrn||cipher are OR-conditions in the
		// server's own PopFlagsTier2Complete — deliberately not asserted here.
		r.setAtLeast("aerindar", 2)
		r.Exact["bertox_key"] = "1"
		r.setAtLeast("tylis", 2)
	},
	"Tier 2: Not started": func(r PopFlagsReport) {
		for _, q := range []string{"aerindar", "karana", "bertox_key", "tylis", "saryrn"} {
			r.Exact[q] = ""
		}
	},
	"Tier 3: Complete": func(r PopFlagsReport) {
		r.Exact["hohtrials"] = "111"
		r.Exact["sol_room"] = "11111"
		r.setAtLeast("cipher", 1)
		r.setAtLeast("zebuxoruk", 2)
		r.setAtLeast("zeks", 7)
		r.setAtLeast("pofire", 2)
	},
	"Tier 3: Not started": func(r PopFlagsReport) {
		for _, q := range []string{"hohtrials", "mmarr", "mmarr_book", "cipher", "zebuxoruk", "zeks", "sol_room", "pofire"} {
			r.Exact[q] = ""
		}
	},
	"Tier 4: Complete":                    exactEffect("time", "1"),
	"Tier 4: Not started":                 func(r PopFlagsReport) { r.Exact["earthb_key"] = ""; r.Exact["time"] = "" },
	"Tier 5 - Plane of Time: Complete":    exactEffect("time", "1"),
	"Tier 5 - Plane of Time: Not started": exactEffect("time", ""),
}

// popFlagsBitLine handles one hohtrials/sol_room per-bit line.
type popFlagsBitLine struct {
	label   string
	qglobal string
	pos     int // 1-based, matching PoPFlag.BitPosition
}

var popFlagsBitLines = []popFlagsBitLine{
	{"Rydda`Dar trial", "hohtrials", 1},
	{"Village trial", "hohtrials", 2},
	{"Nomad trial", "hohtrials", 3},
	{"Xuzl", "sol_room", 1},
	{"Arlyxir", "sol_room", 2},
	{"Dresolik", "sol_room", 3},
	{"Rizlona", "sol_room", 4},
	{"Jiva", "sol_room", 5},
}

const popFlagsBitmaskWidth = 5 // widest of the two (sol_room); hohtrials only uses the first 3.

// popFlagsBitState accumulates a bitmask across a block's lines. A bit is only
// known ('k'=true) once its own line (or a "None completed" line) appears.
type popFlagsBitState struct {
	known [popFlagsBitmaskWidth]bool
	set   [popFlagsBitmaskWidth]bool
}

func (b *popFlagsBitState) apply(pos int, done bool) {
	b.known[pos-1] = true
	b.set[pos-1] = done
}

func (b *popFlagsBitState) exactValue(width int) (string, bool) {
	any := false
	for i := 0; i < width; i++ {
		if b.known[i] {
			any = true
		}
	}
	if !any {
		return "", false
	}
	bits := make([]byte, width)
	for i := 0; i < width; i++ {
		if b.set[i] {
			bits[i] = '1'
		} else {
			bits[i] = '0'
		}
	}
	return string(bits), true
}

// ParsePopFlagsReport parses one buffered '#popflags' block (as recovered by
// the live Consumer or ScanLogForPopFlags) into a PopFlagsReport. lines should
// be the block's message text only (no timestamps), in the order printed. The
// header line's own text picks Section; if no known header is present the
// zero Section is returned with whatever the remaining lines still parse to
// (a defensive fallback — in practice the header always leads).
func ParsePopFlagsReport(lines []string) PopFlagsReport {
	r := newPopFlagsReport()
	hoh := &popFlagsBitState{}
	sol := &popFlagsBitState{}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if section, ok := popFlagsHeaders[line]; ok {
			r.Section = section
			continue
		}
		if effect, ok := popFlagsLiteralLines[line]; ok {
			effect(r)
			continue
		}
		if desc, ok := matchPending(line); ok {
			r.Pending[desc] = true
			continue
		}
		if matchBitLine(line, hoh, sol) {
			continue
		}
		if matchStageLine(r, line) {
			continue
		}
		// Zone sub-headers ("--- X ---"), "Tier complete: ..." lore, the seer
		// notices, and the trailing "Details: ..." / "Valid sections: ..." /
		// "Use #timelockout ..." lines carry no state — silently skipped.
	}

	if v, ok := hoh.exactValue(3); ok {
		r.Exact["hohtrials"] = v
	}
	if v, ok := sol.exactValue(5); ok {
		r.Exact["sol_room"] = v
	}
	return r
}

func matchPending(line string) (string, bool) {
	const prefix = "Pending memory: "
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	desc := line[len(prefix):]
	name, ok := popFlagsPendingDesc[desc]
	return name, ok
}

func matchBitLine(line string, hoh, sol *popFlagsBitState) bool {
	for _, b := range popFlagsBitLines {
		if rest, ok := strings.CutPrefix(line, b.label+": "); ok {
			var done bool
			switch rest {
			case "Complete":
				done = true
			case "Incomplete":
				done = false
			default:
				return false
			}
			switch b.qglobal {
			case "hohtrials":
				hoh.apply(b.pos, done)
			case "sol_room":
				sol.apply(b.pos, done)
			}
			return true
		}
	}
	return false
}

func matchStageLine(r PopFlagsReport, line string) bool {
	for _, sl := range popFlagsStageLabels {
		if rest, ok := strings.CutPrefix(line, sl.label+": "); ok {
			applyStageRest(r, sl.qglobal, sl.stages, rest)
			return true
		}
	}
	for _, kl := range popFlagsKaranaLabels {
		if rest, ok := strings.CutPrefix(line, kl.label+": "); ok {
			if rest == "Complete; combined into Zebuxoruk lore" {
				r.setAtLeast("zebuxoruk", 1)
				return true
			}
			applyStageRest(r, "karana", kl.stages, rest)
			return true
		}
	}
	return false
}

// applyStageRest resolves one stage line's value text against its array: an
// exact match at index i means the qglobal's CURRENT value is i+1; "Not
// started" means confirmed absent; anything else (the "Progress recorded"
// defensive fallback for an out-of-range value) is recognized but contributes
// no state.
func applyStageRest(r PopFlagsReport, qglobal string, stages []string, rest string) {
	if rest == "Not started" {
		r.Exact[qglobal] = ""
		return
	}
	for i, s := range stages {
		if rest == s {
			r.Exact[qglobal] = strconv.Itoa(i + 1)
			return
		}
	}
}

// zoneHeaderPrefix/Suffix recognize a "--- Zone Name ---" sub-header line.
const (
	zoneHeaderPrefix = "--- "
	zoneHeaderSuffix = " ---"
)

func isZoneHeaderLine(line string) bool {
	return strings.HasPrefix(line, zoneHeaderPrefix) && strings.HasSuffix(line, zoneHeaderSuffix) && len(line) > len(zoneHeaderPrefix)+len(zoneHeaderSuffix)
}

const popFlagsTierCompletePrefix = "Tier complete: "

// popFlagsNoticeLines are the fixed non-header, non-value lines the command
// can print (Seer-unlock hints, the trailer, and the Maelin hint) — recognized
// purely so the live Consumer doesn't treat them as ending the block.
var popFlagsNoticeLines = map[string]bool{
	"Details: #popflags 1, 2, 3, 4, or 5 (tier1-tier5 also work).":                        true,
	"A checklist memory is ready to be unlocked.":                                         true,
	"Sit near Seer Mal Nae`Shi and say 'unlock memories', then check #popflags again.":    true,
	"Pending checklist memories exist, but their prerequisite steps are incomplete.":      true,
	"Complete the unfinished progression shown above, then return to Seer Mal Nae`Shi.":   true,
	"If one of these is missing, hail Maelin and ask about new lore and new information.": true,
	"Use #timelockout <1-6> to list a phase's encounters.":                                true,
}

// MatchPopFlagsLine reports whether a single log line is part of a '#popflags'
// report — used by the live-log consumer to decide what to buffer. It must
// recognize every line the command can print (including ones that carry no
// parseable state, like zone sub-headers and tier-completion lore), or the
// consumer would flush mid-block and split one reading into several.
func MatchPopFlagsLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	if _, ok := popFlagsHeaders[line]; ok {
		return true
	}
	if isZoneHeaderLine(line) {
		return true
	}
	if strings.HasPrefix(line, popFlagsTierCompletePrefix) {
		return true
	}
	if popFlagsNoticeLines[line] {
		return true
	}
	if _, ok := popFlagsLiteralLines[line]; ok {
		return true
	}
	if _, ok := matchPending(line); ok {
		return true
	}
	// Bit and stage lines: reuse the parse path against throwaway state so
	// recognition never drifts from what ParsePopFlagsReport actually accepts.
	throwaway := newPopFlagsReport()
	if matchBitLine(line, &popFlagsBitState{}, &popFlagsBitState{}) {
		return true
	}
	if matchStageLine(throwaway, line) {
		return true
	}
	return false
}

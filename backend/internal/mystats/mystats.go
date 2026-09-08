// Package mystats parses the multi-line output of Zeal's `/mystats` command
// out of the EQ log and turns it into a stored, comparable stat snapshot.
//
// `/mystats` (Zeal, flagged "Beta" upstream) prints a block to chat that, with
// `/log on`, lands in eqlog_<Char>_pq.proj.txt as ordinary timestamped lines:
//
//	---- Misc stats ----
//	Movement speed: 0%
//	Movement modifier: +55%
//	---- Defensive stats ----
//	AC (display): 1783 = (Mit: 1012  + Avoidance: 499) * 1000/847
//	Mitigation: 497 (softcap: 451)
//	Avoidance: 558 (with AAs)
//	---- Melee Primary: Goldenrod ----
//	Offense: 395 (Skill 225 + Stat 90 + SpellAtk 10 + ItemAtk 70 + Class 0)
//	To Hit: 457
//	Display ATK: 1145 = (offense + to hit) * 1000 / 744
//	Dmg = BonusDmg + BaseDmg * MitFactor * DmgMultiplier
//	Dmg = 30 + 40 * (0.1 to 2.0x) * (1 to 2.64, ave = 1.62)
//	Dmg = 34.00 to 242.99, ave = 95.19
//	DPS = 10.62 to 75.93, ave = 29.75
//	DPS = 20.00 to 142.94, ave = 56.00 (81% haste)
//
// The command's numbers are Zeal's own client-side estimate (disciplines,
// dual-wield and double-attack are not yet modelled upstream) and the exact
// line text can drift between Zeal releases — the parser is best-effort and
// the consumer stores the raw block alongside the parse so a future parser
// revision can re-derive the structured form.
package mystats

import (
	"regexp"
	"strconv"
	"strings"
)

// MeleeBlock is one "---- Melee <slot>: <weapon> ----" section. The Display*
// and haste fields are zero when `/mystats` omitted them (Display ATK prints
// for the primary hand only; the haste line prints only when haste > 100%).
type MeleeBlock struct {
	Slot   string `json:"slot"`   // "Primary" | "Secondary"
	Weapon string `json:"weapon"` // "HandToHand" when unarmed

	Offense         int `json:"offense"`
	OffenseSkill    int `json:"offense_skill"`
	OffenseStat     int `json:"offense_stat"`
	OffenseSpellAtk int `json:"offense_spell_atk"`
	OffenseItemAtk  int `json:"offense_item_atk"`
	OffenseClass    int `json:"offense_class"`

	ToHit      int `json:"to_hit"`
	DisplayATK int `json:"display_atk,omitempty"`

	BonusDmg   int     `json:"bonus_dmg"`
	BaseDmg    int     `json:"base_dmg"`
	DmgMultMax float64 `json:"dmg_mult_max"`
	DmgMultAvg float64 `json:"dmg_mult_avg"`

	DmgMin float64 `json:"dmg_min"`
	DmgMax float64 `json:"dmg_max"`
	DmgAvg float64 `json:"dmg_avg"`

	DPSMin float64 `json:"dps_min"`
	DPSMax float64 `json:"dps_max"`
	DPSAvg float64 `json:"dps_avg"`

	HastePct    int     `json:"haste_pct,omitempty"`
	DPSHasteMin float64 `json:"dps_haste_min,omitempty"`
	DPSHasteMax float64 `json:"dps_haste_max,omitempty"`
	DPSHasteAvg float64 `json:"dps_haste_avg,omitempty"`
}

// Snapshot is a parsed `/mystats` block.
type Snapshot struct {
	MovementSpeedPct    int `json:"movement_speed_pct"`
	MovementModifierPct int `json:"movement_modifier_pct"`

	// AC line: "AC (display): D = (Mit: M  + Avoidance: A) * 1000/847".
	ACDisplay  int `json:"ac_display"`
	ACRawMit   int `json:"ac_raw_mit"`
	ACRawAvoid int `json:"ac_raw_avoid"`

	// Standalone lines (post-softcap mitigation, avoidance including AAs).
	Mitigation        int    `json:"mitigation"`
	MitigationCap     int    `json:"mitigation_cap"`
	MitigationCapKind string `json:"mitigation_cap_kind"` // "softcap" | "hardcap"
	Avoidance         int    `json:"avoidance"`

	Melee []MeleeBlock `json:"melee"`
}

var (
	reMoveSpeed  = regexp.MustCompile(`^Movement speed: (-?\d+)%`)
	reMoveMod    = regexp.MustCompile(`^Movement modifier: ([+-]?\d+)%`)
	reACDisplay  = regexp.MustCompile(`^AC \(display\): (\d+) = \(Mit: (\d+)\s*\+ Avoidance: (\d+)\)`)
	reMitigation = regexp.MustCompile(`^Mitigation: (\d+) \((softcap|hardcap): (\d+)\)`)
	reAvoidance  = regexp.MustCompile(`^Avoidance: (\d+)`)

	reMeleeHdr  = regexp.MustCompile(`^---- Melee (Primary|Secondary): (.+) ----$`)
	reOffense   = regexp.MustCompile(`^Offense: (\d+)`)
	reOffBreak  = regexp.MustCompile(`Skill (\d+) \+ Stat (\d+) \+ SpellAtk (\d+) \+ ItemAtk (\d+) \+ Class (\d+)`)
	reToHit     = regexp.MustCompile(`^To Hit: (\d+)`)
	reDisplayAT = regexp.MustCompile(`^Display ATK: (\d+)`)
	// Formula line: "Dmg = 30 + 40 * (0.1 to 2.0x) * (1 to 2.64, ave = 1.62)".
	reDmgFormula = regexp.MustCompile(`^Dmg = (\d+) \+ (\d+) \* \([\d.]+ to [\d.]+x\) \* \(1 to ([\d.]+), ave = ([\d.]+)\)`)
	// Result line: "Dmg = 34.00 to 242.99, ave = 95.19" (no " + ", no "x").
	reDmgResult = regexp.MustCompile(`^Dmg = ([\d.]+) to ([\d.]+), ave = ([\d.]+)$`)
	reDPS       = regexp.MustCompile(`^DPS = ([\d.]+) to ([\d.]+), ave = ([\d.]+)$`)
	reDPSHaste  = regexp.MustCompile(`^DPS = ([\d.]+) to ([\d.]+), ave = ([\d.]+) \((\d+)% haste\)`)
)

// StartMarker is the first line of a no-args `/mystats` block. The consumer
// keys block detection off it; `/mystats info` and `/mystats affects` start
// with different text and so never open a snapshot.
const StartMarker = "---- Misc stats ----"

// IsBlockLine reports whether msg could belong to a `/mystats` output block —
// used by the consumer to decide whether to keep buffering. Deliberately loose:
// a stray match only risks a slightly longer buffer, never a bad parse (Parse
// ignores lines it doesn't recognise).
func IsBlockLine(msg string) bool {
	switch {
	case strings.HasPrefix(msg, "---- "):
		return true
	case strings.HasPrefix(msg, "Movement speed:"),
		strings.HasPrefix(msg, "Movement modifier:"),
		strings.HasPrefix(msg, "AC (display):"),
		strings.HasPrefix(msg, "Mitigation:"),
		strings.HasPrefix(msg, "Avoidance:"),
		strings.HasPrefix(msg, "Offense:"),
		strings.HasPrefix(msg, "To Hit:"),
		strings.HasPrefix(msg, "Display ATK:"),
		strings.HasPrefix(msg, "Dmg = "),
		strings.HasPrefix(msg, "DPS = "),
		strings.HasPrefix(msg, "Anti-twink:"),
		strings.HasPrefix(msg, "Can not use weapon"):
		return true
	}
	return false
}

// Parse turns the lines of a `/mystats` block (message text only, no log
// timestamp) into a Snapshot. ok is false when nothing recognisable was found
// — the caller should not store an empty snapshot.
func Parse(lines []string) (Snapshot, bool) {
	var s Snapshot
	var cur *MeleeBlock
	got := false

	flush := func() {
		if cur != nil {
			s.Melee = append(s.Melee, *cur)
			cur = nil
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if m := reMeleeHdr.FindStringSubmatch(line); m != nil {
			flush()
			cur = &MeleeBlock{Slot: m[1], Weapon: strings.TrimSpace(m[2])}
			got = true
			continue
		}

		if cur != nil {
			if parseMeleeLine(cur, line) {
				got = true
				continue
			}
			// A non-melee line (e.g. a section header we don't model, or the
			// end of the block) closes the current weapon block. Fall through
			// so the line still gets a shot at the snapshot-level matchers.
			flush()
		}

		switch {
		case reMoveSpeed.MatchString(line):
			s.MovementSpeedPct = atoi(reMoveSpeed.FindStringSubmatch(line)[1])
			got = true
		case reMoveMod.MatchString(line):
			s.MovementModifierPct = atoi(reMoveMod.FindStringSubmatch(line)[1])
			got = true
		case reACDisplay.MatchString(line):
			m := reACDisplay.FindStringSubmatch(line)
			s.ACDisplay, s.ACRawMit, s.ACRawAvoid = atoi(m[1]), atoi(m[2]), atoi(m[3])
			got = true
		case reMitigation.MatchString(line):
			m := reMitigation.FindStringSubmatch(line)
			s.Mitigation, s.MitigationCapKind, s.MitigationCap = atoi(m[1]), m[2], atoi(m[3])
			got = true
		case strings.HasPrefix(line, "Avoidance: ") && reAvoidance.MatchString(line):
			s.Avoidance = atoi(reAvoidance.FindStringSubmatch(line)[1])
			got = true
		}
	}
	flush()

	return s, got
}

// parseMeleeLine applies the per-weapon matchers to line, returning true if it
// consumed the line.
func parseMeleeLine(b *MeleeBlock, line string) bool {
	switch {
	case reOffense.MatchString(line):
		b.Offense = atoi(reOffense.FindStringSubmatch(line)[1])
		if bk := reOffBreak.FindStringSubmatch(line); bk != nil {
			b.OffenseSkill = atoi(bk[1])
			b.OffenseStat = atoi(bk[2])
			b.OffenseSpellAtk = atoi(bk[3])
			b.OffenseItemAtk = atoi(bk[4])
			b.OffenseClass = atoi(bk[5])
		}
		return true
	case reToHit.MatchString(line):
		b.ToHit = atoi(reToHit.FindStringSubmatch(line)[1])
		return true
	case reDisplayAT.MatchString(line):
		b.DisplayATK = atoi(reDisplayAT.FindStringSubmatch(line)[1])
		return true
	case line == "Dmg = BonusDmg + BaseDmg * MitFactor * DmgMultiplier":
		return true // constant legend line
	case reDmgFormula.MatchString(line):
		m := reDmgFormula.FindStringSubmatch(line)
		b.BonusDmg, b.BaseDmg = atoi(m[1]), atoi(m[2])
		b.DmgMultMax, b.DmgMultAvg = atof(m[3]), atof(m[4])
		return true
	case reDmgResult.MatchString(line):
		m := reDmgResult.FindStringSubmatch(line)
		b.DmgMin, b.DmgMax, b.DmgAvg = atof(m[1]), atof(m[2]), atof(m[3])
		return true
	case reDPSHaste.MatchString(line):
		m := reDPSHaste.FindStringSubmatch(line)
		b.DPSHasteMin, b.DPSHasteMax, b.DPSHasteAvg = atof(m[1]), atof(m[2]), atof(m[3])
		b.HastePct = atoi(m[4])
		return true
	case reDPS.MatchString(line):
		m := reDPS.FindStringSubmatch(line)
		b.DPSMin, b.DPSMax, b.DPSAvg = atof(m[1]), atof(m[2]), atof(m[3])
		return true
	case strings.HasPrefix(line, "Anti-twink:"),
		strings.HasPrefix(line, "Can not use weapon"):
		return true // recognised, nothing to store
	}
	return false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(s, "+")))
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

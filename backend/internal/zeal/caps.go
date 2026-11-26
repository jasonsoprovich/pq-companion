package zeal

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Zeal settings checklist (Settings › Zeal, setup wizard): each Zeal feature
// PQ Companion benefits from, the in-game command that turns it on, and
// whether it's on for this install.
//
// Some entries come from Zeal pull requests that may never be accepted. Those
// have MinVersion "" and are only listed for a user once PQC has seen the
// feature working on the pipe from that user's exact Zeal build. A zeal.ini
// key alone never counts as support: it survives downgrading Zeal. When a
// pending PR ships in a release, set its MinVersion; if it's declined, delete
// the entry (and the UI hints that mention it).
//
// Status 2026-11-26: spell actions (#281), quiet consider (#282) and roster
// appearance (#283) shipped in Zeal 1.5.2. Target spawn location (#284) was
// declined for now; its entry stays (it only shows once proven on the pipe)
// in case it's reopened.

// CapID names one checklist entry.
type CapID string

const (
	CapPipeVerbose      CapID = "pipe_verbose"
	CapExportOnCamp     CapID = "export_on_camp"
	CapPopFlagsOnCamp   CapID = "popflags_on_camp"
	CapPipeSpells       CapID = "pipe_spells"
	CapPipeConsider     CapID = "pipe_consider"
	CapRosterAppearance CapID = "roster_appearance"
	CapTargetSpawnLoc   CapID = "target_spawn_loc"
)

// Capability is one checklist entry.
type Capability struct {
	ID      CapID
	Label   string
	Benefit string // what PQC does with it
	// Command is what to type in game, or how to turn it on when Zeal has no
	// command for it. "" for features with no toggle.
	Command string
	// INIKey is the [Zeal] key in zeal.ini holding the setting, with Zeal's
	// default when the key is missing. "" for features with no setting.
	INIKey     string
	INIDefault string
	// DependsOn names the entry whose setting turns this one on (roster
	// appearance rides on PipeVerbose).
	DependsOn CapID
	// MinVersion is the first Zeal release with the feature; "" = not in any
	// release yet (a pending pull request).
	MinVersion string
	// Proof is true when PQC can see the feature working on the pipe. Export
	// settings have none: an export file can be old or written by hand.
	Proof bool
	// WaitingHint explains why an enabled setting hasn't shown data yet.
	WaitingHint string
	Recommended bool
}

// Capabilities is the checklist, in display order.
var Capabilities = []Capability{
	{
		ID:          CapExportOnCamp,
		Label:       "Export character data on camp",
		Benefit:     "Inventory, spellbook, spell sets and Quarmy stats update every time you camp.",
		Command:     "Zeal options window: tick Export on camp (no slash command)",
		INIKey:      "ExportOnCamp",
		INIDefault:  "FALSE",
		MinVersion:  "1.4.0",
		WaitingHint: "Updates on your next camp.",
		Recommended: true,
	},
	{
		ID:          CapPopFlagsOnCamp,
		Label:       "Include PoP flags in the camp export",
		Benefit:     "Keeps the PoP flag tracker current without typing #popflags.",
		Command:     "/outputfile popflagsoncamp",
		INIKey:      "PopFlagsOnCamp",
		INIDefault:  "FALSE",
		MinVersion:  "1.5.0",
		WaitingHint: "Updates on your next camp (with Export on camp on).",
		Recommended: true,
	},
	{
		ID:          CapPipeVerbose,
		Label:       "Verbose pipe",
		Benefit:     "Adds group and raid members' HP, level and class to the live data.",
		Command:     "/pipeverbose on",
		INIKey:      "PipeVerbose",
		INIDefault:  "FALSE",
		MinVersion:  "1.4.0",
		Proof:       true,
		WaitingHint: "Shows once you're in a group or raid.",
		Recommended: true,
	},
	{
		ID:          CapPipeSpells,
		Label:       "Spell actions",
		Benefit:     "Ties each spell timer to the exact mob it landed on and who cast it.",
		Command:     "/pipespells all",
		INIKey:      "PipeSpells",
		INIDefault:  "0",
		MinVersion:  "1.5.2",
		Proof:       true,
		WaitingHint: "Shows the next time a spell lands near you.",
		Recommended: true,
	},
	{
		ID:          CapPipeConsider,
		Label:       "Quiet consider",
		Benefit:     "NPC overlay KOS border and faction readings on every target, without con messages in chat.",
		Command:     "/pipeconsider on",
		INIKey:      "PipeConsider",
		INIDefault:  "FALSE",
		MinVersion:  "1.5.2",
		Proof:       true,
		WaitingHint: "Shows when you target an NPC.",
		Recommended: true,
	},
	{
		ID:          CapRosterAppearance,
		Label:       "Group and raid appearance",
		Benefit:     "Groupmates and raiders show their real race and gear on 3D maps.",
		Command:     "/pipeverbose on",
		DependsOn:   CapPipeVerbose,
		MinVersion:  "1.5.2",
		Proof:       true,
		WaitingHint: "Shows once you're in a group or raid.",
	},
	{
		ID:      CapTargetSpawnLoc,
		Label:   "Target spawn location",
		Benefit: "Picks the right one of several same-named NPCs from where it spawned.",
		Proof:   true,
	},
}

// CapByID returns the entry for id.
func CapByID(id CapID) (Capability, bool) {
	for _, c := range Capabilities {
		if c.ID == id {
			return c, true
		}
	}
	return Capability{}, false
}

// ZealINI is the [Zeal] section of zeal.ini. Readable is false when the file
// couldn't be opened, which is different from a key that's simply missing
// (Zeal only writes a setting once it has been changed).
type ZealINI struct {
	Readable bool
	values   map[string]string // lowercased key → raw value
}

// Get returns the raw value of key and whether it was present.
func (z ZealINI) Get(key string) (string, bool) {
	v, ok := z.values[strings.ToLower(key)]
	return v, ok
}

// ReadZealINI reads the [Zeal] section of <eqPath>/zeal.ini. Per-character
// sections ([Osui]) are skipped; every key the checklist uses is global.
func ReadZealINI(eqPath string) ZealINI {
	if strings.TrimSpace(eqPath) == "" {
		return ZealINI{}
	}
	f, err := os.Open(filepath.Join(eqPath, ZealINIFilename))
	if err != nil {
		return ZealINI{}
	}
	defer f.Close()
	out := ZealINI{Readable: true, values: map[string]string{}}
	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "zeal" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		if _, dup := out.values[key]; !dup {
			out.values[key] = strings.TrimSpace(line[eq+1:])
		}
	}
	if sc.Err() != nil {
		return ZealINI{}
	}
	return out
}

// Checklist row states.
const (
	CapWorking = "working" // seen working on the pipe this session
	CapOn      = "on"      // zeal.ini says on, no data seen yet
	CapOff     = "off"
	CapUnknown = "unknown"  // zeal.ini unreadable
	CapNotSeen = "not_seen" // no setting to read and not seen yet
)

// CapRow is one checklist row as the API returns it.
type CapRow struct {
	ID          CapID  `json:"id"`
	Label       string `json:"label"`
	Benefit     string `json:"benefit"`
	Command     string `json:"command,omitempty"`
	State       string `json:"state"`
	Value       string `json:"value,omitempty"` // e.g. "detrimental" for PipeSpells
	Hint        string `json:"hint,omitempty"`
	Recommended bool   `json:"recommended"`
	// Pending marks a feature not in any Zeal release yet (the user is on a
	// test build that has it).
	Pending bool `json:"pending"`
}

// CapEvidence is what's known about one install when building its rows.
type CapEvidence struct {
	Version string // installed Zeal version, "" if unknown
	INI     ZealINI
	// Live: seen working on the pipe this session, from this install's game.
	Live map[CapID]bool
	// Persisted: seen before on this install's exact Zeal build.
	Persisted map[CapID]bool
}

// BuildCapRows returns the rows for the features this install supports.
func BuildCapRows(ev CapEvidence) []CapRow {
	var rows []CapRow
	for _, c := range Capabilities {
		released := c.MinVersion != "" && ev.Version != "" && CompareVersions(ev.Version, c.MinVersion) >= 0
		if !released && !ev.Live[c.ID] && !ev.Persisted[c.ID] {
			continue
		}
		row := CapRow{
			ID: c.ID, Label: c.Label, Benefit: c.Benefit, Command: c.Command,
			Recommended: c.Recommended, Pending: c.MinVersion == "",
		}
		row.State, row.Value = capState(c, ev)
		switch row.State {
		case CapOn:
			row.Hint = c.WaitingHint
		case CapOff:
			if c.DependsOn != "" {
				if dep, ok := CapByID(c.DependsOn); ok {
					row.Hint = "Turns on with " + dep.Label + "."
				}
			}
		case CapUnknown:
			row.Hint = "Couldn't read zeal.ini in your EverQuest folder."
		case CapNotSeen:
			row.Hint = "Always on in Zeal builds that have it; shows when you target an NPC."
		}
		rows = append(rows, row)
	}
	return rows
}

// capState derives a row's state. Live proof beats zeal.ini, which can be
// missing or out of date.
func capState(c Capability, ev CapEvidence) (state, value string) {
	if c.Proof && ev.Live[c.ID] {
		_, value = settingValue(c, ev.INI)
		return CapWorking, value
	}
	key := c
	if c.DependsOn != "" {
		dep, ok := CapByID(c.DependsOn)
		if !ok {
			return CapNotSeen, ""
		}
		key = dep
	}
	if key.INIKey == "" {
		return CapNotSeen, ""
	}
	if !ev.INI.Readable {
		return CapUnknown, ""
	}
	on, value := settingValue(key, ev.INI)
	if on {
		return CapOn, value
	}
	return CapOff, value
}

// settingValue reads c's zeal.ini setting (or its default) as on/off plus a
// display value for multi-valued settings.
func settingValue(c Capability, ini ZealINI) (on bool, value string) {
	if c.INIKey == "" {
		return false, ""
	}
	raw, ok := ini.Get(c.INIKey)
	if !ok {
		raw = c.INIDefault
	}
	if c.ID == CapPipeSpells {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return false, ""
		}
		switch n {
		case 1:
			return true, "detrimental"
		case 2:
			return true, "all"
		default:
			return false, "off"
		}
	}
	return isTruthyINI(raw), ""
}

package progress

import (
	"sort"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/skills"
)

// tradeskillSkillIDs is the EQMac skill_id set (see internal/skills.go's
// skillNameToID) covering the classic tradeskill list: Make Poison,
// Tinkering, Research, Alchemy, Baking, Tailoring, Blacksmithing, Fletching,
// Brewing, Jewelry Making, Pottery. Used to split EventSkillUp-derived
// KindSkill journal rows into "skill ups" vs "tradeskill ups" for the recap.
// Fishing (55) and Foraging (27) are deliberately excluded — they're EQ's
// "trade" skills but not tradeskills in the crafting sense the user asked
// about.
var tradeskillSkillIDs = map[int]bool{
	56: true, 57: true, 58: true, 59: true, 60: true,
	61: true, 63: true, 64: true, 65: true, 68: true, 69: true,
}

// DayBucket is one day's activity count, for the recap's trend strip.
type DayBucket struct {
	Date  string `json:"date"` // YYYY-MM-DD, character's local time
	Count int    `json:"count"`
}

// CharacterRecap summarizes one character's progression over [WindowStart,
// WindowEnd].
type CharacterRecap struct {
	Character   string    `json:"character"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`

	// Journal-derived (backfillable — populated even for windows before this
	// feature existed, as far back as the character's log file reaches).
	StartLevel    int `json:"start_level,omitempty"`
	EndLevel      int `json:"end_level,omitempty"`
	LevelsGained  int `json:"levels_gained"`
	AAsGained     int `json:"aas_gained"`
	SpellsScribed int `json:"spells_scribed"`
	SkillUps      int `json:"skill_ups"`
	TradeskillUps int `json:"tradeskill_ups"`
	ActiveDays    int `json:"active_days"`

	// Snapshot-derived (forward-only — HasSnapshotData is false, and the
	// coin/total fields are zero, until the Recorder has captured at least
	// one point at or before WindowStart AND one at or before WindowEnd).
	HasSnapshotData bool  `json:"has_snapshot_data"`
	CoinDelta       int64 `json:"coin_delta"`
	CurrentCopper   int64 `json:"current_copper"` // absolute total (on person + bank) as of WindowEnd

	DailyActivity []DayBucket `json:"daily_activity"`
}

// BuildRecap aggregates one character's journal events (already filtered to
// [since, now] and to this character, oldest first), the distinct calendar
// days the character's log showed any activity in that window (loginDays,
// used for ActiveDays — see character_active_days), plus optional start/end
// totals snapshots into a CharacterRecap. startSnap/endSnap may be nil.
func BuildRecap(character string, events []Event, loginDays []string, startSnap, endSnap *Snapshot, since, now time.Time) CharacterRecap {
	r := CharacterRecap{
		Character:   character,
		WindowStart: since,
		WindowEnd:   now,
	}

	dayCounts := map[string]int{}

	for _, ev := range events {
		day := ev.At.Local().Format("2006-01-02")
		dayCounts[day]++

		switch ev.Kind {
		case KindLevel:
			// The first level event in the window establishes the "before"
			// level: its resulting Value minus its own Delta, so a window
			// that starts mid-gain still reports the level the character
			// had entering the window.
			if r.StartLevel == 0 {
				r.StartLevel = ev.Value - ev.Delta
			}
			r.EndLevel = ev.Value
			r.LevelsGained += ev.Delta
		case KindAA:
			r.AAsGained++
		case KindSpell:
			r.SpellsScribed++
		case KindSkill:
			id, _ := skills.SkillID(ev.Detail)
			if tradeskillSkillIDs[id] {
				r.TradeskillUps++
			} else {
				r.SkillUps++
			}
		}
	}
	r.ActiveDays = len(loginDays)

	dates := make([]string, 0, len(dayCounts))
	for d := range dayCounts {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		r.DailyActivity = append(r.DailyActivity, DayBucket{Date: d, Count: dayCounts[d]})
	}

	if startSnap != nil && endSnap != nil {
		r.HasSnapshotData = true
		r.CoinDelta = endSnap.Copper - startSnap.Copper
		r.CurrentCopper = endSnap.Copper
		if r.EndLevel == 0 {
			r.EndLevel = endSnap.Level
		}
		if r.StartLevel == 0 {
			r.StartLevel = startSnap.Level
		}
	}

	return r
}

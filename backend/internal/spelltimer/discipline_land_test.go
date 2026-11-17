package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// A rogue's Duelist (reported missing since 0.33): the Rogue pack starts a
// 12s timer and a reuse timer on "Your muscles quiver with power.", then the
// spell-landed pipeline resolves the same line. The land used to absorb the
// reuse timer as a same-line twin, and replace the 12s with spells_new's
// 30s, which is wrong for disciplines.
func TestDisciplineLandKeepsPackDurationAndReuseTimer(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Stonae", 65 }
	e := NewEngine(ws.NewHub(), database, charCtx, func() string { return scopeCastByMe }, nil, nil, nil, nil, nil)
	now := time.Now()
	e.StartExternal("Duelist Discipline", "buff", 12, 0, now, nil, 4676, "", "", false, "", false)
	e.StartExternal("Duelist Discipline CD", "buff", 1476, 0, now, nil, 0, "", "", false, "", false)
	e.onSpellLanded(now, logparser.SpellLandedData{
		Kind: logparser.SpellLandedKindYou, SpellName: "Duelist Discipline", SpellID: 4676,
	})

	var disc, cd *ActiveTimer
	for _, tm := range e.timers {
		switch tm.SpellName {
		case "Duelist Discipline":
			disc = tm
		case "Duelist Discipline CD":
			cd = tm
		}
	}
	if cd == nil {
		t.Error("the reuse timer was absorbed by the land")
	}
	if disc == nil {
		t.Fatal("no Duelist timer")
	}
	if disc.DurationSeconds != 12 {
		t.Errorf("Duelist lasts %.0fs, want the pack's 12s", disc.DurationSeconds)
	}
}

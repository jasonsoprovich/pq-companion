package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Casting Howl of Tashan with the Enchanter pack loaded leaves two trigger
// twins by the time the land line arrives: the Howl trigger's cast-line row
// and the broad Tashanian trigger's row on the shared "glances nervously
// about." text. The spell-landed pipeline must absorb both, leaving one Howl
// row. Repeated because the absorb loop walks a map: a fix that only absorbs
// the first twin passes or fails by iteration order.
func TestOnSpellLanded_AbsorbsCastAndLandTwins(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	msgs, err := database.LoadCastMessages()
	if err != nil {
		t.Fatalf("load cast messages: %v", err)
	}
	idx := make([]logparser.CastMessage, 0, len(msgs))
	for _, m := range msgs {
		idx = append(idx, logparser.CastMessage{
			SpellID:     m.SpellID,
			SpellName:   m.SpellName,
			CastOnYou:   m.CastOnYou,
			CastOnOther: m.CastOnOther,
		})
	}
	logparser.SetCastIndex(logparser.NewCastIndex(idx))
	t.Cleanup(func() { logparser.SetCastIndex(nil) })

	for i := 0; i < 20; i++ {
		e := NewEngine(ws.NewHub(), database,
			func() (string, string, int) { return "/eq", "Osui", -1 },
			func() string { return scopeAnyone },
			func() (bool, int) { return true, enchanterClassLevelsIdx },
			nil, nil, nil, nil)
		castAt := time.Now().Truncate(time.Second)
		landAt := castAt.Add(time.Second)

		e.Handle(logparser.LogEvent{
			Type:      logparser.EventSpellCast,
			Timestamp: castAt,
			Data:      logparser.SpellCastData{SpellName: "Howl of Tashan"},
		})
		e.StartExternal("Howl of Tashan", "debuff", 780, 0, castAt, nil, 3342, "a gnoll", "", false, "", false)
		e.StartExternal("Tashanian", "debuff", 780, 0, landAt, nil, 1702, "a gnoll", "", false, "", false)
		ev, ok := logparser.ParseLine("[" + landAt.Format("Mon Jan 02 15:04:05 2006") + "] A gnoll glances nervously about.")
		if !ok {
			t.Fatal("land line did not parse")
		}
		e.Handle(ev)

		if len(e.timers) != 1 {
			keys := make([]string, 0, len(e.timers))
			for k := range e.timers {
				keys = append(keys, k)
			}
			t.Fatalf("iteration %d: want one Howl of Tashan row, got %v", i, keys)
		}
		for _, tm := range e.timers {
			if tm.SpellName != "Howl of Tashan" {
				t.Fatalf("iteration %d: surviving row is %q", i, tm.SpellName)
			}
		}
	}
}

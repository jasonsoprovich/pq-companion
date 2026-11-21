package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// A pack trigger keyed on "You begin casting <spell>." starts its timer at
// the cast line, so by the land line it is a whole cast time old — 6s for
// Swarming Death, 2.5s plus pushback for Savage Roots. The land must still
// absorb that twin; a flat 3s grace window left two rows for one cast (the
// Druid pack report: "a grallok seeker" next to "A grallok seeker").
func TestOnSpellLanded_AbsorbsSlowCastLineTwin(t *testing.T) {
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

	cases := []struct {
		spell    string
		id       int
		dur      float64
		castSecs int
		landText string
		landTrig string // broad pack trigger also firing on the land text
		landID   int
	}{
		{"Swarming Death", 3446, 36, 6, "is engulfed by a swarm of deadly insects.", "Winged Death", 1601},
		{"Savage Roots", 3447, 270, 4, "is entrapped by roots.", "Entrapping Roots", 1608},
	}
	for _, c := range cases {
		t.Run(c.spell, func(t *testing.T) {
			e := NewEngine(ws.NewHub(), database,
				func() (string, string, int) { return "/eq", "Osui", -1 },
				func() string { return scopeAnyone },
				func() (bool, int) { return true, -1 },
				nil, nil, nil, nil)
			landAt := time.Now().Truncate(time.Second)
			castAt := landAt.Add(-time.Duration(c.castSecs) * time.Second)

			e.Handle(logparser.LogEvent{
				Type:      logparser.EventSpellCast,
				Timestamp: castAt,
				Data:      logparser.SpellCastData{SpellName: c.spell},
			})
			// Cast-line trigger: target from the combat-target fallback.
			e.StartExternal(c.spell, "debuff", c.dur, 0, castAt, nil, c.id, "a grallok seeker", "", false, "", false)
			// Land-text trigger for the older rank sharing the text.
			e.StartExternal(c.landTrig, "debuff", 54, 0, landAt, nil, c.landID, "A grallok seeker", "", false, "", false)
			ev, ok := logparser.ParseLine("[" + landAt.Format("Mon Jan 02 15:04:05 2006") + "] A grallok seeker " + c.landText)
			if !ok {
				t.Fatal("land line did not parse")
			}
			e.Handle(ev)

			if len(e.timers) != 1 {
				keys := make([]string, 0, len(e.timers))
				for k := range e.timers {
					keys = append(keys, k)
				}
				t.Fatalf("want one %s row, got %v", c.spell, keys)
			}
			for _, tm := range e.timers {
				if tm.SpellName != c.spell {
					t.Fatalf("surviving row is %q", tm.SpellName)
				}
			}
		})
	}
}

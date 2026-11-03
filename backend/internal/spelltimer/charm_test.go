package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func charmEngine(t *testing.T) *Engine {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", 13 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeCastByMe }, nil, nil, nil, nil, nil)
	e.SetLevelProvider(func() int { return 60 })
	return e
}

func castCharm(e *Engine, spell string) uint64 {
	e.Handle(logparser.LogEvent{Type: logparser.EventSpellCast, Timestamp: time.Now(),
		Data: logparser.SpellCastData{SpellName: spell}})
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pendingCharm == nil {
		return 0
	}
	return e.pendingCharm.seq
}

func charmTimers(e *Engine) []*ActiveTimer {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []*ActiveTimer
	for _, t := range e.timers {
		if t.IsCharm {
			out = append(out, t)
		}
	}
	return out
}

// A charm logs no land line for its caster, so the engine starts the timer
// itself once the cast has gone through unopposed — no trigger pack needed.
func TestNativeCharmTimer(t *testing.T) {
	t.Run("lands", func(t *testing.T) {
		e := charmEngine(t)
		e.HandlePipeTarget("Rygan Anisher")
		seq := castCharm(e, "Boltran`s Agacerie") // backtick, as the log writes it
		if seq == 0 {
			t.Fatal("charm cast was not armed")
		}
		e.confirmCharm(seq)
		got := charmTimers(e)
		if len(got) != 1 {
			t.Fatalf("want one charm timer, got %d", len(got))
		}
		if got[0].TargetName != "Rygan Anisher" || got[0].DurationSeconds <= 0 {
			t.Errorf("got target %q duration %v", got[0].TargetName, got[0].DurationSeconds)
		}
	})

	t.Run("resisted", func(t *testing.T) {
		e := charmEngine(t)
		seq := castCharm(e, "Allure")
		e.Handle(logparser.LogEvent{Type: logparser.EventSpellResist, Timestamp: time.Now(),
			Data: logparser.SpellResistData{SpellName: "Allure"}})
		e.confirmCharm(seq)
		if n := len(charmTimers(e)); n != 0 {
			t.Errorf("resisted charm started %d timers", n)
		}
	})

	t.Run("non-charm cast is ignored", func(t *testing.T) {
		e := charmEngine(t)
		seq := castCharm(e, "Allure")
		castCharm(e, "Tashanian") // the next spell must not cancel a landed charm
		e.confirmCharm(seq)
		if n := len(charmTimers(e)); n != 1 {
			t.Errorf("want one charm timer, got %d", n)
		}
	})

	t.Run("pack timer wins", func(t *testing.T) {
		e := charmEngine(t)
		seq := castCharm(e, "Allure")
		e.StartExternal("Allure", "debuff", 1140, 0, time.Now(), nil, 184, "", "", false, "", false)
		e.confirmCharm(seq)
		if n := len(charmTimers(e)); n != 1 {
			t.Errorf("want only the pack's charm timer, got %d", n)
		}
	})

	t.Run("live pet feed with no pet", func(t *testing.T) {
		e := charmEngine(t)
		e.SetPipePetID(nil)
		seq := castCharm(e, "Allure")
		e.mu.Lock()
		e.pendingCharm.petDeadline = time.Now().Add(-time.Second)
		e.mu.Unlock()
		e.confirmCharm(seq)
		if n := len(charmTimers(e)); n != 0 {
			t.Errorf("charm with no pet started %d timers", n)
		}
	})

	t.Run("live pet feed with the new pet", func(t *testing.T) {
		e := charmEngine(t)
		seq := castCharm(e, "Allure")
		id := 4321
		e.SetPipePetID(&id)
		e.confirmCharm(seq)
		if n := len(charmTimers(e)); n != 1 {
			t.Errorf("want one charm timer, got %d", n)
		}
	})
}

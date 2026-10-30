package trigger_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func encounterOwners(t *testing.T) (*trigger.Store, *trigger.Engine, *spelltimer.Engine) {
	t.Helper()
	s, err := trigger.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := ws.NewHub()
	timers := spelltimer.NewEngine(h, nil, nil, nil, nil, nil, nil, nil, nil)
	e := trigger.NewEngine(s, h, timers, func() string { return "Tester" })
	return s, e, timers
}

// The JSON decoder deliberately allows this regression to compile on the old
// owner. Its RED must be a duplicate action/deadline, not a missing new symbol.
func TestEncounterIfIdleBehavior(t *testing.T) {
	s, e, timers := encounterOwners(t)
	var tr trigger.Trigger
	if err := json.Unmarshal([]byte(`{"id":"attempt","name":"Encounter","enabled":true,"pattern":"^START$","worn_off_pattern":"^STOP$","timer_type":"custom","timer_duration_secs":5880,"timer_if_idle":true,"timer_clear_on_context_change":true,"actions":[{"type":"text_to_speech","text":"start"}]}`), &tr); err != nil {
		t.Fatal(err)
	}
	if err := s.Insert(&tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	now := time.Now()
	e.Handle(now, "START")
	e.Handle(now.Add(5*time.Second), "START")
	rows := timers.GetState().Timers
	if len(rows) != 1 || !rows[0].ExpiresAt.Equal(now.Add(5880*time.Second)) {
		t.Fatalf("duplicate moved first deadline: %+v", rows)
	}
	if len(e.GetHistory()) != 1 {
		t.Fatalf("duplicate initial actions fired: %d", len(e.GetHistory()))
	}
	e.Handle(now.Add(6*time.Second), "STOP")
	if len(timers.GetState().Timers) != 0 {
		t.Fatal("stop did not retire row")
	}
	e.Handle(now.Add(7*time.Second), "START")
	if len(e.GetHistory()) != 2 {
		t.Fatal("stop did not immediately rearm")
	}
}

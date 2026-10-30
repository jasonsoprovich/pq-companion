package api

import (
	"bytes"
	"encoding/json"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestEncounterShippingRouterPersistence(t *testing.T) {
	dir := t.TempDir()
	mgr, err := config.LoadFrom(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := trigger.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hub := ws.NewHub()
	timers := spelltimer.NewEngine(hub, nil, nil, nil, nil, nil, nil, nil, nil)
	engine := trigger.NewEngine(store, hub, timers, nil)
	router := NewRouter(nil, hub, mgr, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, timers, nil, store, engine, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, 0)
	request := func(method, path string, body any, status int) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(data)))
		if rec.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	body := map[string]any{"name": "Encounter", "enabled": true, "pattern": "^START$", "worn_off_pattern": "^STOP$", "source": "log", "timer_type": "custom", "timer_duration_secs": 5880, "timer_if_idle": true, "timer_clear_on_context_change": true}
	data := request(http.MethodPost, "/api/triggers/", body, 201)
	var saved trigger.Trigger
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.TimerIfIdle || !saved.TimerClearOnContextChange {
		t.Fatal("create response lost flags")
	}
	// Full toggle/move clients omit these optional fields, not false them.
	body["enabled"] = false
	body["pack_name"] = "Moved"
	delete(body, "timer_if_idle")
	delete(body, "timer_clear_on_context_change")
	request(http.MethodPut, "/api/triggers/"+saved.ID, body, 200)
	got, err := store.Get(saved.ID)
	if err != nil || !got.TimerIfIdle || !got.TimerClearOnContextChange {
		t.Fatal("omitted flags lost on toggle/move", got, err)
	}
	body["enabled"] = true
	request(http.MethodPut, "/api/triggers/"+saved.ID, body, 200)
	engine.Handle(time.Now(), "START")
	engine.Handle(time.Now(), "START")
	if len(engine.GetHistory()) != 1 {
		t.Fatal("persisted router options did not reach actual owners")
	}
	packBytes := request(http.MethodGet, "/api/triggers/export", nil, 200)
	var pack trigger.TriggerPack
	if err = json.Unmarshal(packBytes, &pack); err != nil {
		t.Fatal(err)
	}
	if len(pack.Triggers) != 1 || !pack.Triggers[0].TimerIfIdle || !pack.Triggers[0].TimerClearOnContextChange {
		t.Fatal("export lost flags")
	}
	pack.PackName = "Copied Encounter"
	pack.Triggers[0].ID = ""
	pack.Triggers[0].PackName = ""
	pack.Triggers[0].CategoryID = ""
	request(http.MethodPost, "/api/triggers/import", pack, 200)
	rows, err := store.List()
	if err != nil || len(rows) != 2 {
		t.Fatal("import did not copy", rows, err)
	}
	for _, row := range rows {
		if !row.TimerIfIdle || !row.TimerClearOnContextChange {
			t.Fatal("copy/import lost flags")
		}
	}
	data = request(http.MethodPost, "/api/triggers/test", trigger.TestRequest{Trigger: &saved, Lines: "START", FireEffects: false}, 200)
	var report trigger.TestReport
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Fired != 0 || !report.Lines[0].Matches[0].Timer.TimerIfIdle || !report.Lines[0].Matches[0].Timer.TimerClearOnContextChange {
		t.Fatal("readonly API draft policy incorrect", string(data))
	}
	for _, mode := range []map[string]any{{"source": "pipe", "pipe_condition": map[string]string{"kind": "pipe_command", "text": "go"}, "timer_type": "custom"}, {"source": "log", "timer_type": "buff"}, {"source": "log", "timer_type": "custom", "timer_stack": true}, {"source": "log", "timer_type": "custom", "timer_repeat": true}} {
		for key, value := range mode {
			body[key] = value
		}
		body["timer_if_idle"] = true
		body["timer_clear_on_context_change"] = true
		request(http.MethodPut, "/api/triggers/"+saved.ID, body, 200)
		got, err = store.Get(saved.ID)
		if err != nil || got.TimerIfIdle || got.TimerClearOnContextChange {
			t.Fatal("ineligible flags were not normalized", got, err)
		}
		delete(body, "timer_stack")
		delete(body, "timer_repeat")
		delete(body, "pipe_condition")
	}
}

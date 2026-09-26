package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestTriggerHandler builds a triggerHandler backed by a temp store,
// wired the same way router.go wires it (minus the parts unrelated to the
// Trigger Tester endpoints under test).
func newTestTriggerHandler(t *testing.T) *triggerHandler {
	t.Helper()
	store, err := trigger.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	hub := ws.NewHub()
	engine := trigger.NewEngine(store, hub, nil, nil)
	h := &triggerHandler{store: store, engine: engine, hub: hub}
	h.tester = trigger.NewTester(engine, func(trigger.TestLineResult) {}, func([]string) {})
	return h
}

func TestTriggerHandler_Test_InstantRun(t *testing.T) {
	h := newTestTriggerHandler(t)
	tr := &trigger.Trigger{
		ID: "h1", Name: "Rampage", Enabled: true, Pattern: `RAMPAGE`,
		Actions: []trigger.Action{{Type: trigger.ActionOverlayText, Text: "RAMP"}},
	}
	if err := h.store.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	body := `{"lines":"A gnoll goes on a RAMPAGE!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/triggers/test", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.test(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var report trigger.TestReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("expected 1 match, got %+v", report)
	}

	// A test run must never write to the real trigger history.
	if len(h.engine.GetHistory()) != 0 {
		t.Errorf("expected no history entries from a test run")
	}
}

func TestTriggerHandler_Test_EmptyLinesRejected(t *testing.T) {
	h := newTestTriggerHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/triggers/test", strings.NewReader(`{"lines":""}`))
	w := httptest.NewRecorder()
	h.test(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty lines, got %d", w.Code)
	}
}

func TestTriggerHandler_Test_RealtimeStartStopStatus(t *testing.T) {
	h := newTestTriggerHandler(t)
	tr := &trigger.Trigger{
		ID: "h2", Name: "Slow", Enabled: true, Pattern: `is slowed\.`,
		Actions: []trigger.Action{{Type: trigger.ActionOverlayText, Text: "slowed"}},
	}
	if err := h.store.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// A long-ish paced blob so the session is still running when we check
	// its status right after starting.
	lines := "[Thu Jun 26 12:00:00 2026] A gnoll is slowed.\n" +
		"[Thu Jun 26 12:00:05 2026] A gnoll is slowed.\n"
	req := httptest.NewRequest(http.MethodPost, "/api/triggers/test", strings.NewReader(
		`{"lines":`+jsonString(lines)+`,"realtime":true}`))
	w := httptest.NewRecorder()
	h.test(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	statusW := httptest.NewRecorder()
	h.testStatus(statusW, httptest.NewRequest(http.MethodGet, "/api/triggers/test/status", nil))
	var status map[string]string
	if err := json.Unmarshal(statusW.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status["state"] != string(trigger.TestPlaybackPlaying) {
		t.Fatalf("expected playing immediately after start, got %+v", status)
	}

	// Starting a second session while one is active must be rejected.
	req2 := httptest.NewRequest(http.MethodPost, "/api/triggers/test", strings.NewReader(`{"lines":"x","realtime":true}`))
	w2 := httptest.NewRecorder()
	h.test(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a concurrent realtime session, got %d", w2.Code)
	}

	stopW := httptest.NewRecorder()
	h.testStop(stopW, httptest.NewRequest(http.MethodPost, "/api/triggers/test/stop", nil))
	if stopW.Code != http.StatusNoContent {
		t.Fatalf("expected 204 from stop, got %d", stopW.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	for h.tester.Status() != trigger.TestPlaybackIdle && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := h.tester.Status(); got != trigger.TestPlaybackIdle {
		t.Fatalf("expected idle after stop, got %q", got)
	}
}

// jsonString marshals s as a JSON string literal, for building request
// bodies inline without fighting Go's raw-string escaping.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

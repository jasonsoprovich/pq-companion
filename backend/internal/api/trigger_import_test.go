package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func TestTriggerImportCommitRollbackAndRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	store, err := trigger.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hub := ws.NewHub()
	h := &triggerHandler{store: store, hub: hub, engine: trigger.NewEngine(store, hub, nil, nil)}
	data, err := os.ReadFile(filepath.Join("..", "trigger", "testdata", "import", "GINA_Generic.gtp"))
	if err != nil {
		t.Fatal(err)
	}
	preview := httptest.NewRecorder()
	h.importPreview(preview, httptest.NewRequest(http.MethodPost, "/api/triggers/import/preview?filename=GINA_Generic.gtp", bytes.NewReader(data)))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body.String())
	}
	var parsed trigger.ImportPreview
	if err := json.Unmarshal(preview.Body.Bytes(), &parsed); err != nil || len(parsed.Triggers) < 2 {
		t.Fatalf("preview decode: %v", err)
	}
	payload, err := json.Marshal(importCommitRequest{Category: "Imported/Retry", Triggers: []trigger.Trigger{parsed.Triggers[0].Trigger, parsed.Triggers[1].Trigger}})
	if err != nil {
		t.Fatal(err)
	}
	// Inject a genuine SQLite insert error on the second row, not an invented
	// HTTP response. Only this test-owned database receives the failure trigger.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER fail_second_import BEFORE INSERT ON triggers
		WHEN (SELECT COUNT(*) FROM triggers) >= 1
		BEGIN SELECT RAISE(ABORT, 'injected second-row failure'); END`); err != nil {
		t.Fatal(err)
	}
	commit := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.importCommit(w, httptest.NewRequest(http.MethodPost, "/api/triggers/import/commit", bytes.NewReader(payload)))
		return w
	}
	failed := commit()
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("expected second-row failure: %d %s", failed.Code, failed.Body.String())
	}
	rows, err := store.List()
	if err != nil || len(rows) != 0 {
		t.Fatalf("partial import survived: %d rows, %v", len(rows), err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_second_import`); err != nil {
		t.Fatal(err)
	}
	retried := commit()
	if retried.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", retried.Code, retried.Body.String())
	}
	var receipt struct {
		Imported int `json:"imported"`
	}
	if err := json.Unmarshal(retried.Body.Bytes(), &receipt); err != nil || receipt.Imported != 2 {
		t.Fatalf("retry receipt: %s, %v", retried.Body.String(), err)
	}
	rows, err = store.List()
	if err != nil || len(rows) != 2 {
		t.Fatalf("retry duplicated rows: %d, %v", len(rows), err)
	}
	seen := make(map[string]bool)
	for _, row := range rows {
		if seen[row.Name] || row.CategoryID == "" || row.PackName != "Retry" {
			t.Fatalf("bad imported row: %+v", row)
		}
		seen[row.Name] = true
	}
	for _, selected := range parsed.Triggers[:2] {
		if !seen[selected.Trigger.Name] {
			t.Fatalf("missing selected trigger %q", selected.Trigger.Name)
		}
	}
}

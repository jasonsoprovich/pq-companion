package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
)

// newTestRouter wires a minimal chi router around a combatHandler so the
// route patterns (which carry the {id} URL param) are exercised end-to-end.
// Each test gets a fresh store so list/delete ordering is deterministic.
func newTestRouter(t *testing.T) (*chi.Mux, *combat.HistoryStore) {
	t.Helper()
	store, err := combat.OpenHistoryStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenHistoryStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	h := &combatHandler{historyStore: store}
	r := chi.NewRouter()
	r.Route("/api/combat/history", func(r chi.Router) {
		r.Get("/", h.historyList)
		r.Delete("/", h.historyClear)
		r.Get("/{id}", h.historyGet)
		r.Delete("/{id}", h.historyDelete)
	})
	return r, store
}

func seedFight(t *testing.T, s *combat.HistoryStore, npc string, start time.Time) int64 {
	t.Helper()
	id, err := s.SaveFight(combat.FightSummary{
		StartTime:     start,
		EndTime:       start.Add(5 * time.Second),
		Duration:      5,
		PrimaryTarget: npc,
		TotalDamage:   100,
		YouDamage:     60,
	}, "TestZone", "Tester")
	if err != nil {
		t.Fatalf("SaveFight: %v", err)
	}
	return id
}

func TestHistoryList_FiltersAndPaginates(t *testing.T) {
	r, store := newTestRouter(t)
	now := time.Now().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		seedFight(t, store, "a gnoll", now.Add(time.Duration(i)*time.Minute))
	}
	seedFight(t, store, "Aten Ha Ra", now.Add(10*time.Minute))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history?npc=gnoll&limit=2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Fights []combat.StoredFight `json:"fights"`
		Total  int64                `json:"total"`
		Limit  int                  `json:"limit"`
		Offset int                  `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("Total = %d, want 3", resp.Total)
	}
	if len(resp.Fights) != 2 {
		t.Errorf("Fights len = %d, want 2 (limited)", len(resp.Fights))
	}
	for _, f := range resp.Fights {
		if f.NPCName != "a gnoll" {
			t.Errorf("filter leaked: got NPC %q", f.NPCName)
		}
	}
}

func TestHistoryGet_FoundAndMissing(t *testing.T) {
	r, store := newTestRouter(t)
	id := seedFight(t, store, "a gnoll", time.Now())

	// Found
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history/"+strconv.FormatInt(id, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("found status = %d, want 200", rec.Code)
	}
	var got combat.StoredFight
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}

	// Missing → 404, not 500
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history/99999", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", rec.Code)
	}

	// Bad id → 400
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history/notanumber", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad-id status = %d, want 400", rec.Code)
	}
}

func TestHistoryDelete_AndClearAll(t *testing.T) {
	r, store := newTestRouter(t)
	id1 := seedFight(t, store, "a gnoll", time.Now())
	seedFight(t, store, "a wolf", time.Now())

	// DELETE /api/combat/history/{id} → 204
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/combat/history/"+strconv.FormatInt(id1, 10), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete one status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	// DELETE /api/combat/history → 200 with {removed:N}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/combat/history", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("clear-all status = %d, want 200", rec.Code)
	}
	var resp map[string]int64
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["removed"] != 1 {
		t.Errorf("removed = %d, want 1 (one fight remained after first delete)", resp["removed"])
	}
}

func TestHistoryEndpoints_DisabledWhenStoreNil(t *testing.T) {
	h := &combatHandler{historyStore: nil}
	r := chi.NewRouter()
	r.Get("/api/combat/history", h.historyList)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when historyStore is nil", rec.Code)
	}
}

func TestHistoryList_BadStartParam(t *testing.T) {
	r, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/combat/history?start=notatime", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed start", rec.Code)
	}
}

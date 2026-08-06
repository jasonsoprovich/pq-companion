package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/progress"
)

func newTestProgressRouter(t *testing.T) (*chi.Mux, *progress.Store, *character.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "user.db")

	store, err := progress.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("progress.OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	charStore, err := character.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("character.OpenStore: %v", err)
	}
	t.Cleanup(func() { charStore.Close() })

	h := &progressHandler{store: store, charStore: charStore}
	r := chi.NewRouter()
	r.Route("/api/progress", func(r chi.Router) {
		r.Get("/recap", h.recap)
		r.Get("/events", h.events)
	})
	return r, store, charStore
}

func TestProgressRecap_SingleCharacter(t *testing.T) {
	r, store, charStore := newTestProgressRouter(t)
	if _, err := charStore.Create("Osui", -1, -1, 1); err != nil {
		t.Fatalf("Create character: %v", err)
	}

	now := time.Now()
	if _, err := store.AppendEvent(progress.Event{
		Character: "Osui", At: now.Add(-time.Hour), Kind: progress.KindLevel, Value: 55, Delta: 1,
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/progress/recap?character=Osui&days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var rec progress.CharacterRecap
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if rec.Character != "Osui" || rec.LevelsGained != 1 {
		t.Errorf("recap = %+v, want Character=Osui LevelsGained=1", rec)
	}
}

func TestProgressRecap_AllCharacters_SortedByActivity(t *testing.T) {
	r, store, charStore := newTestProgressRouter(t)
	if _, err := charStore.Create("Quiet", -1, -1, 1); err != nil {
		t.Fatalf("Create character: %v", err)
	}
	if _, err := charStore.Create("Busy", -1, -1, 1); err != nil {
		t.Fatalf("Create character: %v", err)
	}

	now := time.Now()
	if _, err := store.AppendEvent(progress.Event{
		Character: "Busy", At: now.Add(-time.Hour), Kind: progress.KindLevel, Value: 55, Delta: 2,
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	// "Quiet" has no events at all in the window.

	req := httptest.NewRequest(http.MethodGet, "/api/progress/recap?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var recaps []progress.CharacterRecap
	if err := json.Unmarshal(w.Body.Bytes(), &recaps); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(recaps) != 2 {
		t.Fatalf("got %d recaps, want 2", len(recaps))
	}
	if recaps[0].Character != "Busy" {
		t.Errorf("most-active recap = %q, want Busy", recaps[0].Character)
	}
}

func TestProgressEvents_RequiresCharacter(t *testing.T) {
	r, _, _ := newTestProgressRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/progress/events?days=30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

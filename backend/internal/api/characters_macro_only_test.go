package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

func TestCharactersHandler_MacroOnly(t *testing.T) {
	eqDir := t.TempDir()
	writeIni := func(name string) {
		p := filepath.Join(eqDir, name+"_pq.proj.ini")
		if err := os.WriteFile(p, []byte("[Socials]\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Three on-disk macro files: one is a real (imported) character, one is a
	// plain disk-only mule, one is a disk-only mule the user has hidden.
	writeIni("Realguy")
	writeIni("Muleone")
	writeIni("Hiddenmule")
	// A config-file config the client also writes with the same suffix — must
	// be ignored by the scan, so it must never appear here either.
	if err := os.WriteFile(filepath.Join(eqDir, "UI_Realguy_pq.proj.ini"), []byte("[x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := character.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.Create("Realguy", 0, 1, 60); err != nil {
		t.Fatalf("create Realguy: %v", err)
	}
	if err := store.SetHidden("Hiddenmule", true); err != nil {
		t.Fatalf("hide Hiddenmule: %v", err)
	}

	mgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := mgr.Get()
	cfg.EQPath = eqDir
	if err := mgr.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}

	watcher := zeal.NewWatcher(mgr, ws.NewHub(), store)
	h := &charactersHandler{store: store, mgr: mgr, watcher: watcher}

	r := chi.NewRouter()
	r.Get("/api/characters/macro-only", h.macroOnly)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/characters/macro-only", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}

	var got []macroOnlyCharacter
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}

	byName := map[string]bool{}
	for _, e := range got {
		byName[e.Name] = e.Hidden
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries (%v), want 2 (Muleone, Hiddenmule)", len(got), got)
	}
	if _, ok := byName["Realguy"]; ok {
		t.Error("Realguy has a character record — must not be in macro-only")
	}
	if h, ok := byName["Muleone"]; !ok || h {
		t.Errorf("Muleone: present=%v hidden=%v, want present and not hidden", ok, h)
	}
	if h, ok := byName["Hiddenmule"]; !ok || !h {
		t.Errorf("Hiddenmule: present=%v hidden=%v, want present and hidden", ok, h)
	}
}

func TestCharactersHandler_MacroOnly_NilWatcher(t *testing.T) {
	h := &charactersHandler{}
	r := chi.NewRouter()
	r.Get("/x", h.macroOnly)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("nil watcher: status=%d body=%q, want 200 []", rec.Code, rec.Body.String())
	}
}

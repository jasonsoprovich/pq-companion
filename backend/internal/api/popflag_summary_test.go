package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
)

func newPopflagSummaryRouter(t *testing.T) (*chi.Mux, *popflag.Store, *character.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "user.db")
	ps, err := popflag.OpenStore(path)
	if err != nil {
		t.Fatalf("popflag.OpenStore: %v", err)
	}
	t.Cleanup(func() { ps.Close() })
	cs, err := character.OpenStore(path)
	if err != nil {
		t.Fatalf("character.OpenStore: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	h := &popflagHandler{store: ps, charStore: cs}
	r := chi.NewRouter()
	r.Get("/api/popflags/summary", h.summary)
	r.Get("/api/popflags/export", h.export)
	return r, ps, cs
}

func TestPopflagSummaryProfilesAndHidden(t *testing.T) {
	r, ps, cs := newPopflagSummaryRouter(t)

	// Alpha: stored character with a flag. Bravo: stored character, no flags.
	// Charlie: flag rows only (a deleted profile). Delta: hidden.
	for _, n := range []string{"Alpha", "Bravo", "Delta"} {
		if _, err := cs.Create(n, 1, 1, 60); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	if err := cs.SetHidden("Delta", true); err != nil {
		t.Fatalf("SetHidden: %v", err)
	}
	for _, n := range []string{"Alpha", "Charlie", "Delta"} {
		if err := ps.SetManual(n, "poj_preflag", true); err != nil {
			t.Fatalf("SetManual %s: %v", n, err)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/popflags/summary", nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp popflagSummaryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var got []string
	done := map[string]int{}
	for _, c := range resp.Characters {
		got = append(got, c.Name)
		done[c.Name] = c.Done
	}
	want := []string{"Alpha", "Bravo"}
	if len(got) != len(want) {
		t.Fatalf("characters = %v, want %v (deleted Charlie and hidden Delta "+
			"must be dropped)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("characters = %v, want %v", got, want)
		}
	}
	if done["Alpha"] != 1 || done["Bravo"] != 0 {
		t.Errorf("done tallies = %v, want Alpha=1 Bravo=0", done)
	}
	if len(resp.Zones) == 0 {
		t.Error("expected zone columns")
	}

	// Re-adding Charlie's profile brings the kept flag rows back.
	if _, err := cs.Create("Charlie", 1, 1, 60); err != nil {
		t.Fatalf("Create Charlie: %v", err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/popflags/summary", nil))
	resp = popflagSummaryResponse{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var restored bool
	for _, c := range resp.Characters {
		if c.Name == "Charlie" && c.Done == 1 {
			restored = true
		}
	}
	if !restored {
		t.Errorf("re-added Charlie missing or lost progress: %+v", resp.Characters)
	}
}

func TestPopflagExportEnvelopeAndFilter(t *testing.T) {
	r, ps, _ := newPopflagSummaryRouter(t)
	for _, n := range []string{"Alpha", "Charlie"} {
		if err := ps.SetManual(n, "poj_preflag", true); err != nil {
			t.Fatalf("SetManual: %v", err)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/popflags/export?characters=Charlie", nil))
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out popflagExport
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Kind != popflagExportKind || out.Version != popflagExportVersion {
		t.Errorf("envelope = %q v%d", out.Kind, out.Version)
	}
	if len(out.Characters) != 1 || out.Characters[0].Name != "Charlie" {
		t.Fatalf("characters = %+v, want only Charlie", out.Characters)
	}
	var found bool
	for _, f := range out.Characters[0].Flags {
		if f.ID == "poj_preflag" && f.Done {
			found = true
		}
	}
	if !found {
		t.Error("poj_preflag not exported as done")
	}
}

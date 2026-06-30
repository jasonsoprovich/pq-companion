package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// popflagHandler serves the curated PoP flag dataset plus per-character
// progress. The store may be nil when user.db failed to open; dataset reads
// still work, but per-character reads/writes respond 503. mgr resolves the EQ
// log directory for the "scan log" path (nil-safe).
type popflagHandler struct {
	store *popflag.Store
	hub   *ws.Hub
	mgr   *config.Manager
}

// WSEventPopflagSnapshot is broadcast after a Seer reading (paste-in or
// live-log) commits, so open PoP Flags views refresh in place.
const WSEventPopflagSnapshot = "popflag.snapshot"

// GET /api/popflags/dataset
// Returns the embedded curated dataset (the frontend's source of truth).
func (h *popflagHandler) dataset(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"flags": popflag.Flags()})
}

// GET /api/popflags/{character}
// Returns the resolved per-flag status + progress for one character.
func (h *popflagHandler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	if character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	states, err := h.store.Get(character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, popflag.Resolve(states))
}

type popflagSetRequest struct {
	Done bool `json:"done"`
}

// POST /api/popflags/{character}/{flagID}
// Records a manual toggle (done=true confirms, done=false retracts).
func (h *popflagHandler) setManual(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	flagID := strings.TrimSpace(chi.URLParam(r, "flagID"))
	if character == "" || flagID == "" {
		writeError(w, http.StatusBadRequest, "character and flagID required")
		return
	}
	var req popflagSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetManual(character, flagID, req.Done); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	states, err := h.store.Get(character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, popflag.Resolve(states))
}

type seerRequest struct {
	Text string `json:"text"`
}

// seerDetected is one flag a Seer reading marks complete, plus how it relates
// to the character's current state (so the preview can show "new" vs "have").
type seerDetected struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Zone          string `json:"zone"`
	Tier          int    `json:"tier"`
	AlreadyDone   bool   `json:"already_done"`
	ManualBlocked bool   `json:"manual_blocked"` // a manual retraction will keep this not-done
}

type seerPreviewResponse struct {
	Qglobals map[string]string `json:"qglobals"`
	Detected []seerDetected    `json:"detected"`
	NewCount int               `json:"new_count"`
}

// POST /api/popflags/{character}/seer/preview
// Parses pasted Seer guided-meditation text and reports which flags it detects,
// without writing anything.
func (h *popflagHandler) seerPreview(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	if character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	var req seerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	preview, err := h.buildSeerPreview(character, popflag.ParseSeer(req.Text))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// buildSeerPreview turns a reconstructed qglobal map into the preview response:
// which flags it marks complete, and how each relates to the character's
// current state (new / already-have / manual-blocked). Shared by the paste-in
// preview and the scan-log path.
func (h *popflagHandler) buildSeerPreview(character string, q map[string]string) (seerPreviewResponse, error) {
	derived := popflag.DeriveCompletion(q)
	states, err := h.store.Get(character)
	if err != nil {
		return seerPreviewResponse{}, err
	}
	cur := make(map[string]popflag.State, len(states))
	for _, s := range states {
		cur[s.FlagID] = s
	}
	resp := seerPreviewResponse{Qglobals: q, Detected: []seerDetected{}}
	for _, id := range derived {
		f, ok := popflag.ByID(id)
		if !ok {
			continue
		}
		st, has := cur[id]
		d := seerDetected{ID: id, Label: f.Label, Zone: f.Zone, Tier: f.Tier}
		d.AlreadyDone = has && st.Done
		d.ManualBlocked = has && st.Source == popflag.SourceManual && !st.Done
		if !d.AlreadyDone && !d.ManualBlocked {
			resp.NewCount++
		}
		resp.Detected = append(resp.Detected, d)
	}
	return resp, nil
}

// seerScanResponse is the scan-log result: the same preview the paste path
// returns, plus the raw recovered text to feed back to seer/commit. Found is
// false when the character's log holds no Seer reading.
type seerScanResponse struct {
	Found    bool              `json:"found"`
	Text     string            `json:"text,omitempty"`
	Qglobals map[string]string `json:"qglobals,omitempty"`
	Detected []seerDetected    `json:"detected,omitempty"`
	NewCount int               `json:"new_count,omitempty"`
}

// POST /api/popflags/{character}/seer/scan
// Scans the character's EQ log for the most recent Seer guided-meditation
// reading and returns the same preview as the paste path plus the raw text to
// commit. Responds found=false (200) when the EQ folder isn't configured, the
// log doesn't exist, or it holds no reading — the UI falls back to paste.
func (h *popflagHandler) seerScan(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	if character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	eqPath := ""
	if h.mgr != nil {
		eqPath = h.mgr.Get().EQPath
	}
	if eqPath == "" {
		writeJSON(w, http.StatusOK, seerScanResponse{Found: false})
		return
	}
	logPath := filepath.Join(eqPath, "eqlog_"+character+"_pq.proj.txt")
	burst, found, err := popflag.ScanLogForSeer(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, seerScanResponse{Found: false})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, seerScanResponse{Found: false})
		return
	}
	preview, err := h.buildSeerPreview(character, popflag.ParseSeer(burst.Text))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, seerScanResponse{
		Found:    true,
		Text:     burst.Text,
		Qglobals: preview.Qglobals,
		Detected: preview.Detected,
		NewCount: preview.NewCount,
	})
}

// POST /api/popflags/{character}/seer/commit
// Applies a Seer reading (seer-sourced rows, manual rows preserved) and returns
// the refreshed resolved state.
func (h *popflagHandler) seerCommit(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	if character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	var req seerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := popflag.ParseSeer(req.Text)
	if _, err := h.store.ApplySeer(character, q, req.Text, time.Now()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.hub != nil {
		h.hub.Broadcast(ws.Event{Type: WSEventPopflagSnapshot, Data: map[string]any{"character": character}})
	}
	states, err := h.store.Get(character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, popflag.Resolve(states))
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/customoverlay"
)

// customOverlaysHandler serves /api/custom-overlays: user-built overlays
// driven by regex rules (see internal/customoverlay).
type customOverlaysHandler struct {
	engine *customoverlay.Engine
}

func (h *customOverlaysHandler) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.State())
}

func decodeOverlayDef(w http.ResponseWriter, r *http.Request) (customoverlay.OverlayDef, bool) {
	var def customoverlay.OverlayDef
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return def, false
	}
	return def, true
}

func (h *customOverlaysHandler) create(w http.ResponseWriter, r *http.Request) {
	def, ok := decodeOverlayDef(w, r)
	if !ok {
		return
	}
	o, err := h.engine.Create(def)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *customOverlaysHandler) update(w http.ResponseWriter, r *http.Request) {
	def, ok := decodeOverlayDef(w, r)
	if !ok {
		return
	}
	def.ID = chi.URLParam(r, "id")
	o, err := h.engine.Update(def)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *customOverlaysHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.Delete(chi.URLParam(r, "id")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// customOverlayActionRequest is the body of POST /api/custom-overlays/{id}/action.
type customOverlayActionRequest struct {
	Action  string `json:"action"` // reset | reset_widget | remove_row | enable | move
	Widget  string `json:"widget,omitempty"`
	Key     string `json:"key,omitempty"`
	Enabled bool   `json:"enabled,omitempty"`
	Value   int    `json:"value,omitempty"` // move: -1 up, 1 down
}

func (h *customOverlaysHandler) action(w http.ResponseWriter, r *http.Request) {
	var req customOverlayActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	id := chi.URLParam(r, "id")
	var err error
	switch req.Action {
	case "reset":
		err = h.engine.Reset(id)
	case "reset_widget":
		err = h.engine.ResetWidget(id, req.Widget)
	case "remove_row":
		err = h.engine.RemoveRow(id, req.Widget, req.Key)
	case "enable":
		err = h.engine.SetEnabled(id, req.Enabled)
	case "move":
		err = h.engine.Move(id, req.Value)
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.engine.State())
}

// customOverlayTestRequest is the body of POST /api/custom-overlays/test.
type customOverlayTestRequest struct {
	Def         customoverlay.OverlayDef `json:"def"`
	Lines       []string                 `json:"lines"`
	Character   string                   `json:"character,omitempty"`
	FromCurrent bool                     `json:"from_current,omitempty"`
}

// maxTestLines caps one test run (a pasted chunk of log).
const maxTestLines = 2000

// test dry-runs pasted log lines against an unsaved definition for the
// editor's test box. Validation problems come back in the result's error
// field (200) so the editor can show them inline; a draft that validates
// also comes back normalized in def.
func (h *customOverlaysHandler) test(w http.ResponseWriter, r *http.Request) {
	var req customOverlayTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Lines) > maxTestLines {
		req.Lines = req.Lines[:maxTestLines]
	}
	writeJSON(w, http.StatusOK, h.engine.TestLines(req.Def, req.Lines, req.Character, req.FromCurrent))
}

// templates lists the built-in overlay templates for "+ New custom overlay".
func (h *customOverlaysHandler) templates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": customoverlay.Templates()})
}

func (h *customOverlaysHandler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, customoverlay.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chatwindow"
)

// chatWindowsHandler serves /api/chat-windows: user-made overlay windows that
// each show a filtered slice of chat and log lines (see internal/chatwindow).
type chatWindowsHandler struct {
	engine *chatwindow.Engine
}

func (h *chatWindowsHandler) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.State())
}

func decodeChatWindowDef(w http.ResponseWriter, r *http.Request) (chatwindow.WindowDef, bool) {
	var def chatwindow.WindowDef
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return def, false
	}
	return def, true
}

func (h *chatWindowsHandler) create(w http.ResponseWriter, r *http.Request) {
	def, ok := decodeChatWindowDef(w, r)
	if !ok {
		return
	}
	v, err := h.engine.Create(def)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *chatWindowsHandler) update(w http.ResponseWriter, r *http.Request) {
	def, ok := decodeChatWindowDef(w, r)
	if !ok {
		return
	}
	def.ID = chi.URLParam(r, "id")
	v, err := h.engine.Update(def)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *chatWindowsHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.Delete(chi.URLParam(r, "id")); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chatWindowActionRequest is the body of POST /api/chat-windows/{id}/action.
type chatWindowActionRequest struct {
	Action     string                 `json:"action"` // enable | move | clear | appearance
	Enabled    bool                   `json:"enabled,omitempty"`
	Value      int                    `json:"value,omitempty"` // move: -1 up, 1 down
	Appearance *chatwindow.Appearance `json:"appearance,omitempty"`
}

func (h *chatWindowsHandler) action(w http.ResponseWriter, r *http.Request) {
	var req chatWindowActionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	id := chi.URLParam(r, "id")
	var err error
	switch req.Action {
	case "enable":
		err = h.engine.SetEnabled(id, req.Enabled)
	case "move":
		err = h.engine.Move(id, req.Value)
	case "clear":
		err = h.engine.Clear(id)
	case "appearance":
		if req.Appearance == nil {
			writeError(w, http.StatusBadRequest, "appearance is required")
			return
		}
		err = h.engine.SetAppearance(id, *req.Appearance)
	default:
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dismiss removes one pin: POST /api/chat-windows/{id}/pins/{seq}/dismiss.
func (h *chatWindowsHandler) dismiss(w http.ResponseWriter, r *http.Request) {
	seq, err := strconv.ParseInt(chi.URLParam(r, "seq"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid seq")
		return
	}
	if err := h.engine.Dismiss(chi.URLParam(r, "id"), seq); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// chatWindowSuggestRequest is the body of POST /api/chat-windows/{id}/suggest.
type chatWindowSuggestRequest struct {
	Seq  int64  `json:"seq"`
	Mode string `json:"mode"` // hide | keep
}

// suggest builds a "Hide/Keep lines like this" rule from a stored line.
func (h *chatWindowsHandler) suggest(w http.ResponseWriter, r *http.Request) {
	var req chatWindowSuggestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	s, err := h.engine.Suggest(chi.URLParam(r, "id"), req.Seq, req.Mode)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// chatWindowTestRequest is the body of POST /api/chat-windows/test.
type chatWindowTestRequest struct {
	Def       chatwindow.WindowDef `json:"def"`
	Lines     []string             `json:"lines"`
	Character string               `json:"character,omitempty"`
}

// test dry-runs pasted log lines against an unsaved definition. Validation
// problems come back in the result's error field (200) so the editor can
// show them inline.
func (h *chatWindowsHandler) test(w http.ResponseWriter, r *http.Request) {
	var req chatWindowTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Lines) > maxTestLines {
		req.Lines = req.Lines[:maxTestLines]
	}
	writeJSON(w, http.StatusOK, h.engine.TestLines(req.Def, req.Lines, req.Character))
}

// export returns a share file: GET /api/chat-windows/export?id=a&id=b, or
// every window when no id is given.
func (h *chatWindowsHandler) export(w http.ResponseWriter, r *http.Request) {
	sh, err := h.engine.Export(r.URL.Query()["id"])
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sh)
}

// chatWindowImportRequest is the body of POST /api/chat-windows/import.
type chatWindowImportRequest struct {
	Text    string `json:"text"`
	Preview bool   `json:"preview,omitempty"`
}

// importShare reads a share file. With preview it only returns the validated
// definitions; otherwise it adds them as new windows.
func (h *chatWindowsHandler) importShare(w http.ResponseWriter, r *http.Request) {
	var req chatWindowImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Preview {
		defs, err := chatwindow.ParseShare([]byte(req.Text))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"windows": defs})
		return
	}
	defs, err := h.engine.Import([]byte(req.Text))
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"windows": defs})
}

func (h *chatWindowsHandler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, chatwindow.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

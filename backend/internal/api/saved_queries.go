package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/savedquery"
)

// savedQueryHandler exposes CRUD + import/export for the user's saved SQL
// queries. Every endpoint is gated by DeveloperMode, matching the rest of
// the sandbox surface — these queries are useless without the sandbox
// itself, so there's no value in exposing them when dev mode is off.
type savedQueryHandler struct {
	store  *savedquery.Store
	cfgMgr *config.Manager
}

func (h *savedQueryHandler) guarded(w http.ResponseWriter) bool {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "saved queries not initialized")
		return false
	}
	if !h.cfgMgr.Get().Preferences.DeveloperMode {
		writeError(w, http.StatusForbidden, "developer mode is disabled")
		return false
	}
	return true
}

type savedQueryPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SQL         string `json:"sql"`
}

func (h *savedQueryHandler) list(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	queries, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queries": queries})
}

func (h *savedQueryHandler) create(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	var body savedQueryPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	q := savedquery.SavedQuery{
		Name:        body.Name,
		Description: body.Description,
		SQL:         body.SQL,
	}
	if err := h.store.Create(&q); err != nil {
		if errors.Is(err, savedquery.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (h *savedQueryHandler) update(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	var body savedQueryPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	updated, err := h.store.Update(id, savedquery.SavedQuery{
		Name:        body.Name,
		Description: body.Description,
		SQL:         body.SQL,
	})
	if err != nil {
		switch {
		case errors.Is(err, savedquery.ErrNotFound):
			writeError(w, http.StatusNotFound, "not found")
		case errors.Is(err, savedquery.ErrInvalid):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *savedQueryHandler) delete(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.store.Delete(id); err != nil {
		if errors.Is(err, savedquery.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// exportPack returns every saved query as a Pack JSON document. The
// renderer downloads the response body as a file; serving JSON inline
// keeps the surface simple and means HeidiSQL-style "save as" pickers
// stay on the client side where they belong.
func (h *savedQueryHandler) exportPack(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	pack, err := h.store.ExportPack()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Attachment hint lets the renderer's anchor-download flow name the
	// file sensibly when the user clicks Export.
	w.Header().Set("Content-Disposition", `attachment; filename="pq-companion-queries.json"`)
	writeJSON(w, http.StatusOK, pack)
}

func (h *savedQueryHandler) importPack(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	var pack savedquery.Pack
	if err := json.NewDecoder(r.Body).Decode(&pack); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	inserted, err := h.store.ImportPack(pack)
	if err != nil {
		// Bad pack kind / version is a user error; surface as 400.
		msg := err.Error()
		if strings.Contains(msg, "unrecognized pack kind") || strings.Contains(msg, "pack version") {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "inserted": inserted})
}

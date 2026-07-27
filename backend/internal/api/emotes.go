package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/emote"
)

// emotesHandler exposes the developer-mode spell emote customizer: editing
// the client-visible chat emotes in spells_en.txt, backed by structured
// overrides in user.db so they survive a server patch replacing the file.
type emotesHandler struct {
	service *emote.Service
}

// unavailable responds 503 and reports true when the emote store failed to
// open at startup, in which case service is nil and every handler is a no-op.
func (h *emotesHandler) unavailable(w http.ResponseWriter) bool {
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "spell emote editor unavailable")
		return true
	}
	return false
}

// GET /api/emotes/status
func (h *emotesHandler) status(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	st, err := h.service.Status()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// GET /api/emotes/overrides
func (h *emotesHandler) list(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	rows, err := h.service.ListCustomized()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// GET /api/emotes/diff
func (h *emotesHandler) diff(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	diffs, err := h.service.Diff()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diffs)
}

func emoteSpellID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	return id, err == nil
}

// GET /api/emotes/spell/{id}
func (h *emotesHandler) get(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	id, ok := emoteSpellID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	se, err := h.service.GetSpellEmote(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// PUT /api/emotes/spell/{id}
// Body: any subset of {you_cast, other_casts, cast_on_you, cast_on_other,
// spell_fades}. Only the supplied fields are set as overrides; omitted
// fields keep their current override state.
func (h *emotesHandler) put(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	id, ok := emoteSpellID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var patch emote.ColumnsPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if patch.Empty() {
		writeError(w, http.StatusBadRequest, "no columns provided")
		return
	}
	if err := h.service.SetOverride(id, patch); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	se, err := h.service.GetSpellEmote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// DELETE /api/emotes/spell/{id}
// Reverts the spell to its default emotes.
func (h *emotesHandler) revert(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	id, ok := emoteSpellID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.service.RevertOverride(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	se, err := h.service.GetSpellEmote(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// POST /api/emotes/restore-defaults
func (h *emotesHandler) restoreDefaults(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	if err := h.service.RestoreDefaults(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/emotes/reapply
// Re-applies every stored override onto whatever is currently live (the
// server's patched content), used by the "Re-apply now" prompt after an
// external change is detected.
func (h *emotesHandler) reapply(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	if err := h.service.ReapplyAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/emotes/ignore-external-change
// Adopts the patched file as the new pristine default without re-applying
// overrides — used by the "Ignore" choice on the patch-detected banner.
func (h *emotesHandler) ignoreExternalChange(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	if err := h.service.IgnoreExternalChange(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/emotes/pending-import
// Spells whose emotes were already customized in spells_en.txt before this
// feature was ever used (detected against quarm.db's canonical text on
// first bootstrap), still awaiting the user's decision to import them as
// tracked, patch-surviving overrides.
func (h *emotesHandler) pendingImport(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	pending, err := h.service.PendingImports()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pending)
}

// POST /api/emotes/import-existing
// Body: { "spell_ids": [123, 456] } — omitted or empty imports every
// pending entry. Adopts the selected pending imports as tracked overrides;
// the live file's bytes don't change (they already had this text).
func (h *emotesHandler) importExisting(w http.ResponseWriter, r *http.Request) {
	if h.unavailable(w) {
		return
	}
	var body struct {
		SpellIDs []int `json:"spell_ids"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	imported, err := h.service.ImportExisting(body.SpellIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": imported})
}

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/factiontracker"
)

// factionSearchLimit caps how many factions a single picker query returns —
// generous for a type-to-filter list, small enough to stay a cheap query
// against the ~2100-row faction_list table.
const factionSearchLimit = 50

// factionsHandler handles the faction picker, per-character faction
// wishlist, and the Faction Tracker's read/reset endpoints. Tallies persist
// in user.db across restarts and character switches — "reset" here means
// the user explicitly discarding a character's tracked history, not a
// per-session boundary.
type factionsHandler struct {
	store  *character.Store
	db     *db.DB
	engine *factiontracker.Engine

	// reloadTracked re-derives the tracked faction set (and persisted
	// tallies) for the currently active character and pushes it into
	// engine.SetTracked. Called after any wishlist mutation so a live
	// session picks up the change without requiring an app restart or
	// character reselect. May be nil in tests.
	reloadTracked func()
}

func (h *factionsHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	factions, err := h.db.SearchFactions(q, factionSearchLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"factions": factions})
}

func (h *factionsHandler) session(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.State())
}

func (h *factionsHandler) resetSession(w http.ResponseWriter, _ *http.Request) {
	h.engine.Reset()
	writeJSON(w, http.StatusOK, h.engine.State())
}

func (h *factionsHandler) listWishlist(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	if _, ok, err := h.store.Get(charID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	entries, err := h.store.ListFactionWishlist(charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

type factionWishlistAddRequest struct {
	FactionID int `json:"faction_id"`
}

func (h *factionsHandler) addWishlist(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	if _, ok, err := h.store.Get(charID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var req factionWishlistAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FactionID <= 0 {
		writeError(w, http.StatusBadRequest, "faction_id is required")
		return
	}
	faction, err := h.db.GetFactionByID(req.FactionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "faction not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to look up faction")
		}
		return
	}
	entry, err := h.store.AddFactionWishlistEntry(charID, faction.ID, faction.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.reloadTracked != nil {
		h.reloadTracked()
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *factionsHandler) deleteWishlist(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	factionID, err := strconv.Atoi(chi.URLParam(r, "factionID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid faction id")
		return
	}
	if err := h.store.DeleteFactionWishlistEntry(charID, factionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Drop the persisted tally too — untracking a faction discards its
	// history, so re-adding it later starts fresh rather than resurrecting
	// old counts.
	if err := h.store.DeleteFactionTally(charID, factionID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.reloadTracked != nil {
		h.reloadTracked()
	}
	w.WriteHeader(http.StatusNoContent)
}

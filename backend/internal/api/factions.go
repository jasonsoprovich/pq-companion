package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/factiontracker"
)

// factionSearchLimit caps how many factions a single picker query returns —
// generous for a type-to-filter list, small enough to stay a cheap query
// against the ~2100-row faction_list table.
const factionSearchLimit = 50

// factionsHandler handles the faction picker, per-character faction pins
// (the wishlist), and the Faction Tracker's read/reset endpoints. The
// tracker records every faction the active character has ever killed toward
// or /con'd — not just pinned ones — persisted in user.db across restarts
// and character switches. Pinning is purely a display favorite; it never
// gates what gets recorded. "Reset" means the user explicitly discarding a
// character's tracked history, not a per-session boundary.
type factionsHandler struct {
	store  *character.Store
	db     *db.DB
	engine *factiontracker.Engine
}

// factionSearchResult pairs a faction_list row with the requested
// character's persisted tally for it, if any — lets the picker show
// "already have data" for a faction before the user pins it, even one
// they've never starred. Tally is shaped identically to the /session
// endpoint's payload (not the raw storage row) so the frontend has one Tally
// type to render regardless of which endpoint it came from.
type factionSearchResult struct {
	db.Faction
	Tally *factiontracker.Tally `json:"tally,omitempty"`
}

func tallyFromRow(row character.FactionTallyRow) factiontracker.Tally {
	t := factiontracker.Tally{
		FactionID:           row.FactionID,
		FactionName:         row.FactionName,
		Better:              row.Better,
		Worse:               row.Worse,
		EstimatedNet:        row.EstimatedNet,
		Unresolved:          row.Unresolved,
		LastBucket:          row.LastBucket,
		LastConsiderSuspect: row.LastConsiderSuspect,
	}
	if row.LastConsideredAt > 0 {
		ts := time.Unix(row.LastConsideredAt, 0)
		t.LastConsideredAt = &ts
	}
	return t
}

func (h *factionsHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	factions, err := h.db.SearchFactions(q, factionSearchLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	charIDStr := r.URL.Query().Get("character_id")
	if charIDStr == "" {
		writeJSON(w, http.StatusOK, map[string]any{"factions": factions})
		return
	}
	charID, err := strconv.Atoi(charIDStr)
	if err != nil || charID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid character_id")
		return
	}
	tallyRows, err := h.store.ListFactionTallies(charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byFactionID := make(map[int]character.FactionTallyRow, len(tallyRows))
	for _, row := range tallyRows {
		byFactionID[row.FactionID] = row
	}
	out := make([]factionSearchResult, len(factions))
	for i, f := range factions {
		res := factionSearchResult{Faction: f}
		if row, ok := byFactionID[f.ID]; ok {
			t := tallyFromRow(row)
			res.Tally = &t
		}
		out[i] = res
	}
	writeJSON(w, http.StatusOK, map[string]any{"factions": out})
}

// session returns the Faction Tracker state. With no character_id, it
// returns the live engine state (whichever character is currently active).
// With an explicit character_id, it instead reads that character's
// persisted tallies directly from storage — the only way to see a
// non-active character's tracked history, since the engine only ever
// watches one character's log at a time. That history won't update again
// until the character becomes active.
func (h *factionsHandler) session(w http.ResponseWriter, r *http.Request) {
	charIDStr := r.URL.Query().Get("character_id")
	if charIDStr == "" {
		writeJSON(w, http.StatusOK, h.engine.State())
		return
	}
	charID, err := strconv.Atoi(charIDStr)
	if err != nil || charID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid character_id")
		return
	}
	rows, err := h.store.ListFactionTallies(charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tallies := make([]factiontracker.Tally, len(rows))
	for i, row := range rows {
		tallies[i] = tallyFromRow(row)
	}
	writeJSON(w, http.StatusOK, factiontracker.State{Tallies: tallies})
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
	writeJSON(w, http.StatusCreated, entry)
}

// deleteWishlist unpins a faction. This is a display-favorite change only —
// the persisted tally (better/worse/estimated net/last /con reading) is left
// untouched, exactly like unstarring a player in the Player Tracker doesn't
// erase their sighting history. Re-pinning later picks the same data back up.
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
	w.WriteHeader(http.StatusNoContent)
}

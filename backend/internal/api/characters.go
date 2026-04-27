package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type charactersHandler struct {
	store  *character.Store
	mgr    *config.Manager
	db     *db.DB
	tailer *logparser.Tailer
}

type charactersListResponse struct {
	Characters []character.Character `json:"characters"`
	Active     string                `json:"active"`
	Manual     bool                  `json:"manual"`
}

// list returns all stored characters and the currently active selection.
// Active is the manually-configured character when set; otherwise the
// auto-detected character (most-recently-modified EQ log file).
func (h *charactersHandler) list(w http.ResponseWriter, r *http.Request) {
	chars, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chars == nil {
		chars = []character.Character{}
	}
	cfg := h.mgr.Get()
	manual := cfg.Character != ""
	active := cfg.Character
	if !manual && h.tailer != nil {
		active = h.tailer.ActiveCharacter()
	}
	resp := charactersListResponse{
		Characters: chars,
		Manual:     manual,
		Active:     active,
	}
	writeJSON(w, http.StatusOK, resp)
}

// discover returns character names found in EQ log files that are not yet stored.
func (h *charactersHandler) discover(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.Get()
	discovered := logparser.DiscoverCharacters(cfg.EQPath)

	stored, err := h.store.Names()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var untracked []string
	for _, d := range discovered {
		if _, exists := stored[strings.ToLower(d.Name)]; !exists {
			// Check case-insensitively
			found := false
			for k := range stored {
				if strings.EqualFold(k, d.Name) {
					found = true
					break
				}
			}
			if !found {
				untracked = append(untracked, d.Name)
			}
		}
	}
	if untracked == nil {
		untracked = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"names": untracked})
}

type characterRequest struct {
	Name  string `json:"name"`
	Class int    `json:"class"`
	Race  int    `json:"race"`
	Level int    `json:"level"`
}

// create adds a new character profile.
func (h *charactersHandler) create(w http.ResponseWriter, r *http.Request) {
	var req characterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Level < 1 {
		req.Level = 1
	}
	c, err := h.store.Create(req.Name, req.Class, req.Race, req.Level)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create character: %s", err))
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// update replaces name/class/race/level for an existing character.
func (h *charactersHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req characterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Level < 1 {
		req.Level = 1
	}
	if err := h.store.Update(id, req.Name, req.Class, req.Race, req.Level); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, character.Character{ID: id, Name: req.Name, Class: req.Class, Race: req.Race, Level: req.Level})
}

// del removes a character profile.
func (h *charactersHandler) del(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// aas returns the AA abilities for a character: both the trained list (with
// names resolved from quarm.db) and the full catalog of class-eligible AAs so
// the UI can render every ability and dim untrained ones.
//
// AA IDs throughout this endpoint are altadv_vars.eqmacid values (the EQ
// client AA index used by the Zeal "AAIndex" export).
func (h *charactersHandler) aas(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	trained, err := h.store.ListAAs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trained == nil {
		trained = []character.AAEntry{}
	}

	var available []db.AAInfo
	if h.db != nil {
		// EQ class indices in our character store run 0-14 (zero-indexed); the
		// altadv_vars `classes` bitmask uses bit N for class N (1-indexed). Map
		// from our 0-indexed class to the bitmask's 1-indexed class id.
		eqClass := char.Class + 1

		available, err = h.db.ListAvailableAAs(eqClass)
		if err == nil {
			eligible := make(map[int]bool, len(available))
			ids := make([]int, len(available))
			for i, a := range available {
				ids[i] = a.AAID
				eligible[a.AAID] = true
			}
			// Drop trained entries that aren't eligible for this class. Zeal's
			// AAIndex export can contain cross-class AAs (e.g. Fleet of Foot
			// for a Wizard) that the character can't actually use; including
			// them makes the tab badge disagree with the points-spent total.
			filtered := trained[:0]
			for _, t := range trained {
				if eligible[t.AAID] {
					filtered = append(filtered, t)
				}
			}
			trained = filtered
			names, _ := h.db.LookupAANames(ids)
			for i := range trained {
				trained[i].Name = names[trained[i].AAID]
			}
		}
	}
	if available == nil {
		available = []db.AAInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"trained":   trained,
		"available": available,
	})
}

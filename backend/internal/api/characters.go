package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type charactersHandler struct {
	mgr *config.Manager
}

// charactersResponse is the payload returned by GET /api/characters.
type charactersResponse struct {
	// Characters is the set of characters discovered from eqlog_*_pq.proj.txt
	// files in the configured EQ directory, sorted by most-recently-modified.
	Characters []logparser.DiscoveredCharacter `json:"characters"`
	// Active is the name of the currently active character — either the
	// manually configured Character, or the auto-detected fallback.
	Active string `json:"active"`
	// Manual is true when config.Character is set explicitly, false when
	// the active character is determined automatically from the newest log.
	Manual bool `json:"manual"`
}

// list returns the discovered characters and the active selection.
func (h *charactersHandler) list(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.Get()
	chars := logparser.DiscoverCharacters(cfg.EQPath)

	resp := charactersResponse{
		Characters: chars,
		Manual:     cfg.Character != "",
	}
	if cfg.Character != "" {
		resp.Active = cfg.Character
	} else if len(chars) > 0 {
		resp.Active = chars[0].Name
	}
	writeJSON(w, http.StatusOK, resp)
}

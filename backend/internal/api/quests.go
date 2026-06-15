package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type questsHandler struct{ db *db.DB }

// search handles GET /api/quests?q=<query>&limit=&offset= — browses the
// quest-script-derived quest list, matching NPC, zone, or related item names.
func (h *questsHandler) search(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	if limit > 200 {
		limit = 200
	}
	result := h.db.SearchQuests(r.URL.Query().Get("q"), limit, queryInt(r, "offset", 0))
	writeJSON(w, http.StatusOK, result)
}

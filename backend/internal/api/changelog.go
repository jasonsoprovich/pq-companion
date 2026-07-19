package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/changelog"
)

type changelogHandler struct {
	entries []changelog.Entry
}

// GET /api/changelog
// Returns every parsed CHANGELOG.md entry, newest first.
func (h *changelogHandler) list(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(struct {
		Entries []changelog.Entry `json:"entries"`
	}{Entries: h.entries})
}

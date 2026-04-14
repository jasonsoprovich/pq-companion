package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type logHandler struct {
	tailer *logparser.Tailer
}

// status handles GET /api/log/status — returns the current tailer state.
func (h *logHandler) status(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(h.tailer.Status())
}

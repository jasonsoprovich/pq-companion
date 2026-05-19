package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
)

// enumsHandler serves the canonical raw-code → label catalog so the
// frontend can render labels without maintaining its own duplicate
// copies. The payload is static for the lifetime of the process; clients
// fetch once on startup.
type enumsHandler struct{}

func (h *enumsHandler) get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, enums.Snapshot())
}

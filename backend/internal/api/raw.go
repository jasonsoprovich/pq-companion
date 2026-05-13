package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type rawHandler struct{ db *db.DB }

// rawHandlerFor returns a chi handler that fetches a raw row from the given
// table by its primary id column.
func (h *rawHandler) rowFromTable(table, idCol string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		row, err := h.db.GetRawRow(table, idCol, id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "row not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

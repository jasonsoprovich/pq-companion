package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/lockout"
)

// lockoutsHandler serves the per-character loot/legacy lockout tracker driven
// by parsing the in-game /sll command from the log file.
type lockoutsHandler struct {
	store *lockout.Store
	// db resolves lockout target names to game-database entities so the UI can
	// link each row. Loot rows are raid-boss NPCs; legacy rows are items.
	db *db.DB
}

// lockoutEntryDTO is a lockout entry enriched with a best-effort link target.
// ResolvedKind is "npc" or "item" (empty when the name couldn't be resolved);
// ResolvedID is the matching database id. Both are omitted when unresolved so
// the frontend falls back to plain text.
type lockoutEntryDTO struct {
	lockout.Entry
	ResolvedKind string `json:"resolved_kind,omitempty"`
	ResolvedID   int    `json:"resolved_id,omitempty"`
}

// resolveEntry attaches a link target to a lockout entry, best-effort. Loot
// lockouts name raid-boss NPCs; legacy lockouts name items. Unresolvable names
// (instanced bosses, renamed targets, data gaps) are returned link-less.
func (h *lockoutsHandler) resolveEntry(e lockout.Entry) lockoutEntryDTO {
	dto := lockoutEntryDTO{Entry: e}
	if h.db == nil || e.TargetName == "" {
		return dto
	}
	if e.Section == lockout.SectionLegacy {
		if id, ok := h.db.GetItemIDByName(e.TargetName); ok {
			dto.ResolvedKind, dto.ResolvedID = "item", id
		}
		return dto
	}
	if id, ok := h.db.GetNPCIDByName(e.TargetName); ok {
		dto.ResolvedKind, dto.ResolvedID = "npc", id
	}
	return dto
}

// listCharacters handles GET /api/lockouts/characters and returns the names of
// every character that has at least one captured lockout snapshot.
func (h *lockoutsHandler) listCharacters(w http.ResponseWriter, _ *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "lockout store unavailable")
		return
	}
	names, err := h.store.Characters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"characters": names})
}

// getCharacter handles GET /api/lockouts/characters/{name} and returns the
// character's lockout entries (both sections) in snapshot order.
func (h *lockoutsHandler) getCharacter(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "lockout store unavailable")
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	entries, err := h.store.ListByCharacter(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resolved := make([]lockoutEntryDTO, len(entries))
	for i, e := range entries {
		resolved[i] = h.resolveEntry(e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"character": name, "entries": resolved})
}

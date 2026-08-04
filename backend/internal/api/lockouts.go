package api

import (
	"net/http"
	"strings"

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

// zoneLockoutBossDTO is one raid-target boss in a zone, paired with every
// character's known lockout status for it.
type zoneLockoutBossDTO struct {
	NPCID      int                       `json:"npc_id,omitempty"`
	TargetName string                    `json:"target_name"`
	Characters []zoneLockoutCharacterDTO `json:"characters"`
}

// zoneLockoutCharacterDTO is one character's captured lockout status for a
// boss. Omitted entirely from a boss's Characters slice when that character
// has never had this target captured (distinct from ExpiresAt = 0, which
// means it was explicitly observed "Available").
type zoneLockoutCharacterDTO struct {
	Character  string `json:"character"`
	ExpiresAt  int64  `json:"expires_at"`
	ObservedAt int64  `json:"observed_at"`
}

// getZoneLockouts handles GET /api/lockouts/zone/{shortName} and returns
// every raid-target boss known to spawn in the zone, each with every
// character's lockout status for it — the data backing the Zones tab's
// Lockouts sub-tab. Scoped to loot lockouts only: legacy item lockouts have
// no zone to key off of.
func (h *lockoutsHandler) getZoneLockouts(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "lockout store unavailable")
		return
	}
	if h.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	shortName := chi.URLParam(r, "shortName")
	if shortName == "" {
		writeError(w, http.StatusBadRequest, "zone short name required")
		return
	}
	targets, err := h.db.GetRaidTargetsByZone(shortName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	names := make([]string, len(targets))
	for i, t := range targets {
		names[i] = t.Name
	}
	entries, err := h.store.ListBySectionAndTargets(lockout.SectionLoot, names)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Keyed lower-cased: target_name is player-typed game output and npc_types
	// names are data-entry text, so casing isn't guaranteed to match exactly
	// even though the SQL lookup was already case-insensitive.
	byName := make(map[string][]zoneLockoutCharacterDTO, len(names))
	for _, e := range entries {
		key := strings.ToLower(e.TargetName)
		byName[key] = append(byName[key], zoneLockoutCharacterDTO{
			Character:  e.Character,
			ExpiresAt:  e.ExpiresAt,
			ObservedAt: e.ObservedAt,
		})
	}
	bosses := make([]zoneLockoutBossDTO, len(targets))
	for i, t := range targets {
		chars := byName[strings.ToLower(t.Name)]
		if chars == nil {
			chars = []zoneLockoutCharacterDTO{}
		}
		bosses[i] = zoneLockoutBossDTO{
			NPCID:      t.NPCID,
			TargetName: t.Name,
			Characters: chars,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"zone": shortName, "bosses": bosses})
}

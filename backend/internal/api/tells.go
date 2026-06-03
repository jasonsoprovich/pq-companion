package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tells"
)

type tellsHandler struct {
	store  *tells.Store
	mgr    *config.Manager
	tailer *logparser.Tailer
}

// activeCharacter resolves the character to scope tell queries to: an explicit
// ?character= wins, else the in-game detected character, else the configured
// override. Empty means "all characters".
func (h *tellsHandler) activeCharacter(r *http.Request) string {
	if c := r.URL.Query().Get("character"); c != "" {
		return c
	}
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			return c
		}
	}
	return h.mgr.Get().Character
}

// detectedCharacter resolves the in-game / configured active character without
// honoring the ?character= override (used to highlight the default tab).
func (h *tellsHandler) detectedCharacter() string {
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			return c
		}
	}
	return h.mgr.Get().Character
}

// characters handles GET /api/tells/characters — the set of characters to show
// as tabs: every character with an EQ log file plus any that already have
// stored tells, case-folded and sorted. Each has its own log file and so its
// own separate conversations.
func (h *tellsHandler) characters(w http.ResponseWriter, r *http.Request) {
	byLower := map[string]string{}
	for _, d := range logparser.DiscoverCharacters(h.mgr.Get().EQPath) {
		byLower[strings.ToLower(d.Name)] = d.Name
	}
	withTells, err := h.store.Characters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, c := range withTells {
		if _, ok := byLower[strings.ToLower(c)]; !ok {
			byLower[strings.ToLower(c)] = c
		}
	}
	names := make([]string, 0, len(byLower))
	for _, n := range byLower {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"characters": names,
		"active":     h.detectedCharacter(),
	})
}

// list handles GET /api/tells — per-peer conversation summaries.
func (h *tellsHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 500)
	if limit > 2000 {
		limit = 2000
	}
	f := tells.ConversationFilters{
		Character:    h.activeCharacter(r),
		PeerContains: r.URL.Query().Get("search"),
		SortDesc:     r.URL.Query().Get("sort") != "asc",
		Limit:        limit,
		Offset:       queryInt(r, "offset", 0),
	}
	out, err := h.store.Conversations(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []tells.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// thread handles GET /api/tells/{peer} — the full message history with a peer.
func (h *tellsHandler) thread(w http.ResponseWriter, r *http.Request) {
	peer := chi.URLParam(r, "peer")
	if peer == "" {
		writeError(w, http.StatusBadRequest, "peer required")
		return
	}
	sortDesc := r.URL.Query().Get("sort") == "desc"
	rows, err := h.store.Messages(h.activeCharacter(r), peer, sortDesc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []tells.Tell{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": rows})
}

// delete handles DELETE /api/tells/{peer}.
func (h *tellsHandler) delete(w http.ResponseWriter, r *http.Request) {
	peer := chi.URLParam(r, "peer")
	if peer == "" {
		writeError(w, http.StatusBadRequest, "peer required")
		return
	}
	if err := h.store.DeletePeer(h.activeCharacter(r), peer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// clear handles POST /api/tells/clear — wipes the active character's tells (or
// all of them when no character is in scope).
func (h *tellsHandler) clear(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.Clear(h.activeCharacter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

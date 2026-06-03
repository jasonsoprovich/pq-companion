package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type chatHandler struct {
	store  *chat.Store
	mgr    *config.Manager
	tailer *logparser.Tailer
}

// scopeCharacter resolves the character to scope queries to: explicit
// ?character= wins, else the in-game character, else the configured override.
func (h *chatHandler) scopeCharacter(r *http.Request) string {
	if c := r.URL.Query().Get("character"); c != "" {
		return c
	}
	return h.detectedCharacter()
}

func (h *chatHandler) detectedCharacter() string {
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			return c
		}
	}
	return h.mgr.Get().Character
}

// channels handles GET /api/chat/channels — the character tabs plus the
// channels that have messages (tell is always offered first as the default).
func (h *chatHandler) channels(w http.ResponseWriter, r *http.Request) {
	// Characters: every char with a log file plus any with stored chat.
	byLower := map[string]string{}
	for _, d := range logparser.DiscoverCharacters(h.mgr.Get().EQPath) {
		byLower[strings.ToLower(d.Name)] = d.Name
	}
	if withChat, err := h.store.Characters(); err == nil {
		for _, c := range withChat {
			if _, ok := byLower[strings.ToLower(c)]; !ok {
				byLower[strings.ToLower(c)] = c
			}
		}
	}
	chars := make([]string, 0, len(byLower))
	for _, n := range byLower {
		chars = append(chars, n)
	}
	sort.Slice(chars, func(i, j int) bool { return strings.ToLower(chars[i]) < strings.ToLower(chars[j]) })

	channels, err := h.store.Channels(h.scopeCharacter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"channels":   channels,
		"characters": chars,
		"active":     h.detectedCharacter(),
	})
}

// conversations handles GET /api/chat/conversations — per-peer tell summaries.
func (h *chatHandler) conversations(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 1000)
	if limit > 5000 {
		limit = 5000
	}
	f := chat.ConversationFilters{
		Character:    h.scopeCharacter(r),
		PeerContains: r.URL.Query().Get("search"),
		From:         int64(queryInt(r, "from", 0)),
		To:           int64(queryInt(r, "to", 0)),
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
		out = []chat.Conversation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

// thread handles GET /api/chat/thread/{peer} — full tell history with a peer.
func (h *chatHandler) thread(w http.ResponseWriter, r *http.Request) {
	peer := chi.URLParam(r, "peer")
	if peer == "" {
		writeError(w, http.StatusBadRequest, "peer required")
		return
	}
	sortDesc := r.URL.Query().Get("sort") == "desc"
	rows, err := h.store.Thread(h.scopeCharacter(r), peer, sortDesc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []chat.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": rows})
}

// feed handles GET /api/chat/feed — a flat message list for one channel.
func (h *chatHandler) feed(w http.ResponseWriter, r *http.Request) {
	channel := strings.ToLower(r.URL.Query().Get("channel"))
	if channel == "" {
		writeError(w, http.StatusBadRequest, "channel required")
		return
	}
	limit := queryInt(r, "limit", 1000)
	if limit > 5000 {
		limit = 5000
	}
	f := chat.FeedFilters{
		Character: h.scopeCharacter(r),
		Channel:   channel,
		Search:    r.URL.Query().Get("search"),
		From:      int64(queryInt(r, "from", 0)),
		To:        int64(queryInt(r, "to", 0)),
		SortDesc:  r.URL.Query().Get("sort") != "asc",
		Limit:     limit,
		Offset:    queryInt(r, "offset", 0),
	}
	rows, err := h.store.Feed(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []chat.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": rows})
}

// deletePeer handles DELETE /api/chat/peer/{peer} — removes a tell conversation.
func (h *chatHandler) deletePeer(w http.ResponseWriter, r *http.Request) {
	peer := chi.URLParam(r, "peer")
	if peer == "" {
		writeError(w, http.StatusBadRequest, "peer required")
		return
	}
	if err := h.store.DeletePeer(h.scopeCharacter(r), peer); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// clear handles POST /api/chat/clear?channel= — wipes the active character's
// chat, optionally limited to one channel.
func (h *chatHandler) clear(w http.ResponseWriter, r *http.Request) {
	channel := strings.ToLower(r.URL.Query().Get("channel"))
	n, err := h.store.Clear(h.scopeCharacter(r), channel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

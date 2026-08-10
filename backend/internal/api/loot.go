package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/loot"
)

type lootHandler struct {
	store  *loot.Store
	mgr    *config.Manager
	tailer *logparser.Tailer
	db     *db.DB
}

// lootRow is a loot.Entry enriched with the item's type (resolved by name
// against the game DB, not persisted — see db.ItemTypesByNames), so the
// Character Info recap's loot listing can offer a "type" filter without a
// schema change to loot_events.
type lootRow struct {
	loot.Entry
	ItemType int `json:"item_type"` // -1 when the item name has no match in the game DB
}

func (h *lootHandler) scopeCharacter(r *http.Request) string {
	if c := r.URL.Query().Get("character"); c != "" {
		return c
	}
	return h.detectedCharacter()
}

func (h *lootHandler) detectedCharacter() string {
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			return c
		}
	}
	return h.mgr.Get().Character
}

// meta handles GET /api/loot/meta — character tabs plus the distinct players
// and zones present (for the filter dropdowns).
func (h *lootHandler) meta(w http.ResponseWriter, r *http.Request) {
	byLower := map[string]string{}
	for _, d := range logparser.DiscoverCharacters(h.mgr.Get().EQPath) {
		byLower[strings.ToLower(d.Name)] = d.Name
	}
	if withLoot, err := h.store.Characters(); err == nil {
		for _, c := range withLoot {
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

	scope := h.scopeCharacter(r)
	players, err := h.store.Players(scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	zones, err := h.store.Zones(scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"characters": chars,
		"players":    players,
		"zones":      zones,
		"active":     h.detectedCharacter(),
	})
}

// list handles GET /api/loot — filtered loot feed.
func (h *lootHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 2000)
	if limit > 10000 {
		limit = 10000
	}
	f := loot.Filters{
		Character: h.scopeCharacter(r),
		Search:    r.URL.Query().Get("search"),
		Player:    r.URL.Query().Get("player"),
		Zone:      r.URL.Query().Get("zone"),
		From:      int64(queryInt(r, "from", 0)),
		To:        int64(queryInt(r, "to", 0)),
		SortDesc:  r.URL.Query().Get("sort") != "asc",
		Limit:     limit,
		Offset:    queryInt(r, "offset", 0),
	}
	rows, err := h.store.List(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"loot": h.withItemTypes(rows)})
}

// withItemTypes resolves each row's item name to its game-DB itemtype
// (best-effort; unmatched names get -1, the enum's "None" sentinel) and
// returns the enriched rows.
func (h *lootHandler) withItemTypes(rows []loot.Entry) []lootRow {
	out := make([]lootRow, len(rows))
	if h.db == nil {
		for i, r := range rows {
			out[i] = lootRow{Entry: r, ItemType: -1}
		}
		return out
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		key := strings.ToLower(r.Item)
		if !seen[key] {
			seen[key] = true
			names = append(names, r.Item)
		}
	}
	types, err := h.db.ItemTypesByNames(names)
	if err != nil {
		types = map[string]int{}
	}
	for i, r := range rows {
		t, ok := types[strings.ToLower(r.Item)]
		if !ok {
			t = -1
		}
		out[i] = lootRow{Entry: r, ItemType: t}
	}
	return out
}

// clear handles POST /api/loot/clear — wipes the active character's loot.
func (h *lootHandler) clear(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.Clear(h.scopeCharacter(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}

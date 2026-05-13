package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type zealHandler struct {
	watcher *zeal.Watcher
	cfgMgr  *config.Manager
	db      *db.DB
}

// enrichEntries fills in the Icon field on each entry by looking up
// items.icon for all referenced IDs in a single query. Errors are logged
// implicitly (entries are returned without icons) — icons are decorative
// and shouldn't fail the inventory request.
func (h *zealHandler) enrichEntries(entries []zeal.InventoryEntry) {
	if len(entries) == 0 || h.db == nil {
		return
	}
	ids := make([]int, 0, len(entries))
	for _, e := range entries {
		if e.ID > 0 {
			ids = append(ids, e.ID)
		}
	}
	icons, err := h.db.ItemIcons(ids)
	if err != nil {
		return
	}
	for i := range entries {
		if icon, ok := icons[entries[i].ID]; ok {
			entries[i].Icon = icon
		}
	}
}

// GET /api/zeal/inventory
// Returns the most recently parsed Zeal inventory export.
// If no export file has been found yet (character or eq_path not configured,
// or file not yet written) returns {"inventory": null}.
func (h *zealHandler) inventory(w http.ResponseWriter, r *http.Request) {
	inv := h.watcher.Inventory()
	if inv != nil {
		h.enrichEntries(inv.Entries)
	}
	json.NewEncoder(w).Encode(struct {
		Inventory *zeal.Inventory `json:"inventory"`
	}{Inventory: inv})
}

// GET /api/zeal/spells
// Returns the most recently parsed Zeal spellbook export. With no query, returns
// the active character's cached spellbook. With ?character=Name, parses that
// character's <Name>-Spellbook.txt directly. Returns {"spellbook": null} if the
// requested file doesn't exist yet.
func (h *zealHandler) spellbook(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Spellbook *zeal.Spellbook `json:"spellbook"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		resp.Spellbook = h.watcher.Spellbook()
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	sb, err := zeal.ParseSpellbook(zeal.SpellbookPath(cfg.EQPath, name), name)
	if err != nil {
		// Missing file is not an error from the caller's perspective.
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Spellbook = sb
	json.NewEncoder(w).Encode(resp)
}

// GET /api/zeal/all-inventories
// Scans the configured EQ directory for all Zeal inventory exports and returns
// per-character inventories plus a deduplicated SharedBank.
func (h *zealHandler) allInventories(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllInventories()
	if err != nil {
		http.Error(w, `{"error":"failed to scan inventories"}`, http.StatusInternalServerError)
		return
	}
	for _, c := range resp.Characters {
		if c != nil {
			h.enrichEntries(c.Entries)
		}
	}
	h.enrichEntries(resp.SharedBank)
	json.NewEncoder(w).Encode(resp)
}

// GET /api/zeal/quarmy
// Returns the most recently parsed quarmy export (stats, inventory, AAs).
// With no query returns the active character's cached data; with
// ?character=Name parses that character's <Name>-Quarmy.txt directly.
// Returns {"quarmy": null} if the file doesn't exist yet.
func (h *zealHandler) quarmy(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Quarmy interface{} `json:"quarmy"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		q := h.watcher.Quarmy()
		if q != nil {
			h.enrichEntries(q.Inventory)
		}
		resp.Quarmy = q
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	q, err := zeal.ParseQuarmy(zeal.QuarmyPath(cfg.EQPath, name), name)
	if err != nil {
		json.NewEncoder(w).Encode(resp)
		return
	}
	h.enrichEntries(q.Inventory)
	resp.Quarmy = q
	json.NewEncoder(w).Encode(resp)
}

// GET /api/zeal/spellsets
// Returns Zeal-exported spellsets. With no query, returns the active character's
// cached spellsets (or null if none). With ?character=Name, parses that character's
// <Name>_spellsets.ini directly.
func (h *zealHandler) spellsets(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Spellsets *zeal.SpellsetFile `json:"spellsets"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		resp.Spellsets = h.watcher.Spellsets()
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	sf, err := zeal.ParseSpellsets(zeal.SpellsetPath(cfg.EQPath, name), name)
	if err != nil {
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Spellsets = sf
	json.NewEncoder(w).Encode(resp)
}

// PUT /api/zeal/spellsets
// Persists a SpellsetFile back to <eq_path>/<Character>_spellsets.ini.
// Body: {"character": "Name", "spellsets": [{"name": "...", "spell_ids": [...]}, ...]}
// Returns the reparsed file on success.
func (h *zealHandler) updateSpellsets(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character string          `json:"character"`
		Spellsets []zeal.Spellset `json:"spellsets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.Character == "" {
		http.Error(w, `{"error":"character is required"}`, http.StatusBadRequest)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		http.Error(w, `{"error":"EQ path not configured"}`, http.StatusBadRequest)
		return
	}

	// Validate slot counts up-front so a malformed request can't truncate the file.
	for i, s := range body.Spellsets {
		if len(s.SpellIDs) != zeal.SpellsetSlotCount {
			http.Error(w, fmt.Sprintf(`{"error":"spellset %d (%q) must have %d slots"}`, i, s.Name, zeal.SpellsetSlotCount), http.StatusBadRequest)
			return
		}
	}

	path := zeal.SpellsetPath(cfg.EQPath, body.Character)
	sf := &zeal.SpellsetFile{
		Character: body.Character,
		Spellsets: body.Spellsets,
	}
	if err := zeal.WriteSpellsets(path, sf); err != nil {
		http.Error(w, `{"error":"failed to write spellsets"}`, http.StatusInternalServerError)
		return
	}

	// Reparse so the response reflects the on-disk file (including its new mod time).
	reloaded, err := zeal.ParseSpellsets(path, body.Character)
	if err != nil {
		http.Error(w, `{"error":"wrote file but failed to reparse"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(struct {
		Spellsets *zeal.SpellsetFile `json:"spellsets"`
	}{Spellsets: reloaded})
}

// GET /api/zeal/spellsets/all
// Scans the configured EQ directory for every <CharName>_spellsets.ini and
// returns one parsed file per character.
func (h *zealHandler) allSpellsets(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllSpellsets()
	if err != nil {
		http.Error(w, `{"error":"failed to scan spellsets"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

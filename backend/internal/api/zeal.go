package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

var spellsetFilenameRe = regexp.MustCompile(`(?i)^(.+?)_spellsets\.ini$`)

type zealHandler struct {
	watcher *zeal.Watcher
	cfgMgr  *config.Manager
	db      *db.DB
	pipe    *zealpipe.Supervisor
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

	// Validate slot counts and names up-front so a malformed request can't
	// truncate or corrupt the file.
	seenNames := make(map[string]bool, len(body.Spellsets))
	for i, s := range body.Spellsets {
		if len(s.SpellIDs) != zeal.SpellsetSlotCount {
			http.Error(w, fmt.Sprintf(`{"error":"spellset %d (%q) must have %d slots"}`, i, s.Name, zeal.SpellsetSlotCount), http.StatusBadRequest)
			return
		}
		if s.Name == "" {
			http.Error(w, fmt.Sprintf(`{"error":"spellset %d has empty name"}`, i), http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(s.Name, "[]\r\n") {
			http.Error(w, fmt.Sprintf(`{"error":"spellset %d (%q) contains illegal characters"}`, i, s.Name), http.StatusBadRequest)
			return
		}
		if seenNames[s.Name] {
			http.Error(w, fmt.Sprintf(`{"error":"duplicate spellset name %q"}`, s.Name), http.StatusBadRequest)
			return
		}
		seenNames[s.Name] = true
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

// POST /api/zeal/spellsets/parse-file
// Parses an arbitrary spellsets .ini file path (typically chosen via the
// Electron file dialog when importing another player's spellsets) without
// reading it into the EQ-directory cache. The character name is inferred
// from the filename when possible.
// Body: {"path": "..."}
func (h *zealHandler) parseSpellsetsFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		http.Error(w, `{"error":"path is required"}`, http.StatusBadRequest)
		return
	}
	if !strings.EqualFold(filepath.Ext(body.Path), ".ini") {
		http.Error(w, `{"error":"file must have .ini extension"}`, http.StatusBadRequest)
		return
	}

	character := ""
	if m := spellsetFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	sf, err := zeal.ParseSpellsets(body.Path, character)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(struct {
		Spellsets *zeal.SpellsetFile `json:"spellsets"`
	}{Spellsets: sf})
}

// GET /api/zeal/pipe-status
// Reports the runtime connection state of the Zeal named-pipe supervisor.
// Used by the Settings UI to show whether we're actively receiving live game
// state from Zeal, distinct from /detect which only reports filesystem
// presence of Zeal.asi.
func (h *zealHandler) pipeStatus(w http.ResponseWriter, r *http.Request) {
	if h.pipe == nil {
		writeJSON(w, http.StatusOK, zealpipe.Status{State: zealpipe.StateUnsupported})
		return
	}
	writeJSON(w, http.StatusOK, h.pipe.Status())
}

// GET /api/zeal/detect
// Probes an EverQuest folder for the Zeal mod (Zeal.asi next to eqgame.exe).
// Defaults to the configured EQ path; an explicit ?path= override lets the
// onboarding wizard check before the user has saved their config. Runtime
// pipe connectivity is a separate check handled by the zealpipe supervisor.
func (h *zealHandler) detect(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = h.cfgMgr.Get().EQPath
	}
	writeJSON(w, http.StatusOK, zeal.DetectInstall(path))
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

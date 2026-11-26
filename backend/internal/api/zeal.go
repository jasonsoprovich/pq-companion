package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqconfig"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

var spellsetFilenameRe = regexp.MustCompile(`(?i)^(.+?)_spellsets\.ini$`)
var bandolierFilenameRe = regexp.MustCompile(`(?i)^(.+?)_bandolier\.ini$`)

// Underscore-free name only: EQ character names have no underscore, so this
// skips the client's UI_/BZR_ prefixed _pq.proj.ini config files.
var macroFilenameRe = regexp.MustCompile(`(?i)^([^_]+)_pq\.proj\.ini$`)

type zealHandler struct {
	watcher   *zeal.Watcher
	cfgMgr    *config.Manager
	db        *db.DB
	pipe      *zealpipe.Supervisor
	latest    *zeal.LatestFetcher
	charStore *character.Store
	// backupMgr snapshots the EQ config files before a write to eqclient.ini.
	backupMgr *backup.Manager
}

// enrichEntries fills in the Icon, MaxCharges and Value fields on each entry by looking
// up the items DB for all referenced IDs in batch queries. Errors are swallowed
// (entries are returned without the enrichment) — these fields are decorative /
// supplementary and shouldn't fail the inventory request. MaxCharges is set only
// for rechargeable click items, which flags them for the Rechargeable Items view.
// isSafeCharacterName rejects a character name that could escape the EQ
// directory when composed into a file path (e.g. `..\..\foo`). Every zeal file
// path is filepath.Join(eqPath, "<character>_something") with the name coming
// from the request, so an unsanitized name is a path-traversal write/read.
// Real EQ names are plain letters, so this never rejects a legitimate one.
func isSafeCharacterName(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	for _, r := range name {
		if r < 0x20 { // control chars, incl. NUL
			return false
		}
	}
	return true
}

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
	if icons, err := h.db.ItemIcons(ids); err == nil {
		for i := range entries {
			if icon, ok := icons[entries[i].ID]; ok {
				entries[i].Icon = icon
			}
		}
	}
	if charges, err := h.db.RechargeableMaxCharges(ids); err == nil {
		for i := range entries {
			if max, ok := charges[entries[i].ID]; ok {
				entries[i].MaxCharges = max
			}
		}
	}
	if prices, err := h.db.ItemPrices(ids); err == nil {
		for i := range entries {
			p, ok := prices[entries[i].ID]
			if !ok {
				continue
			}
			qty := 1
			if p.Stackable && entries[i].Count > 1 {
				qty = entries[i].Count
			}
			entries[i].Price = p.Price
			entries[i].Value = p.Price * qty
		}
	}
}

// GET /api/zeal/inventory
// Returns the most recently parsed Zeal inventory export. With no query,
// returns the active character's cached inventory. With ?character=Name,
// parses that character's <Name>-Inventory.txt directly (same pattern as
// spellbook/quarmy/spellsets below). Returns {"inventory": null} if no
// export file has been found yet.
func (h *zealHandler) inventory(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Inventory *zeal.Inventory `json:"inventory"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		inv := h.watcher.Inventory()
		if inv != nil {
			// Inventory() returns the watcher's shared cached pointer. Enrich a
			// copy so we don't mutate Entries in place — concurrent GETs would
			// otherwise write the same slice elements unsynchronized (trips -race).
			cp := *inv
			cp.Entries = append([]zeal.InventoryEntry(nil), inv.Entries...)
			h.enrichEntries(cp.Entries)
			inv = &cp
		}
		resp.Inventory = inv
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	path := zeal.FindInventoryFile(cfg.EQPath, name)
	if path == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	inv, err := zeal.ParseInventory(path, name)
	if err != nil {
		json.NewEncoder(w).Encode(resp)
		return
	}
	h.enrichEntries(inv.Entries)
	resp.Inventory = inv
	json.NewEncoder(w).Encode(resp)
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
	path := zeal.FindSpellbookFile(cfg.EQPath, name)
	if path == "" {
		// No export in either format — not an error to the caller.
		json.NewEncoder(w).Encode(resp)
		return
	}
	sb, err := zeal.ParseSpellbook(path, name)
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
		writeError(w, http.StatusInternalServerError, "failed to scan inventories")
		return
	}
	for _, c := range resp.Characters {
		if c != nil {
			h.enrichEntries(c.Entries)
		}
	}
	h.enrichEntries(resp.SharedBank)
	if eqPath := h.cfgMgr.Get().EQPath; eqPath != "" {
		for _, c := range resp.Characters {
			if c != nil {
				c.CHA = h.estimateCHA(eqPath, c.Character)
			}
		}
	}
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
			// Enrich a copy — Quarmy() hands back the shared cached pointer
			// (same race as the inventory handler above).
			cp := *q
			cp.Inventory = append([]zeal.InventoryEntry(nil), q.Inventory...)
			h.enrichEntries(cp.Inventory)
			q = &cp
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
	path := zeal.FindQuarmyFile(cfg.EQPath, name)
	if path == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	q, err := zeal.ParseQuarmy(path, name)
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
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(body.Character) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}

	// Validate slot counts and names up-front so a malformed request can't
	// truncate or corrupt the file.
	seenNames := make(map[string]bool, len(body.Spellsets))
	for i, s := range body.Spellsets {
		if len(s.SpellIDs) != zeal.SpellsetSlotCount {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("spellset %d (%q) must have %d slots", i, s.Name, zeal.SpellsetSlotCount))
			return
		}
		if s.Name == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("spellset %d has empty name", i))
			return
		}
		if strings.ContainsAny(s.Name, "[]\r\n") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("spellset %d (%q) contains illegal characters", i, s.Name))
			return
		}
		if seenNames[s.Name] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("duplicate spellset name %q", s.Name))
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
		writeError(w, http.StatusInternalServerError, "failed to write spellsets")
		return
	}

	// Reparse so the response reflects the on-disk file (including its new mod time).
	reloaded, err := zeal.ParseSpellsets(path, body.Character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wrote file but failed to reparse")
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
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.EqualFold(filepath.Ext(body.Path), ".ini") {
		writeError(w, http.StatusBadRequest, "file must have .ini extension")
		return
	}

	character := ""
	if m := spellsetFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	sf, err := zeal.ParseSpellsets(body.Path, character)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse: %s", err.Error()))
		return
	}
	json.NewEncoder(w).Encode(struct {
		Spellsets *zeal.SpellsetFile `json:"spellsets"`
	}{Spellsets: sf})
}

// GET /api/zeal/bandolier
// Returns Zeal-exported bandolier sets. With no query, returns the active
// character's cached sets (or null if none). With ?character=Name, parses that
// character's <Name>_bandolier.ini directly.
func (h *zealHandler) bandolier(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Bandolier *zeal.BandolierFile `json:"bandolier"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		resp.Bandolier = h.watcher.Bandolier()
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	bf, err := zeal.ParseBandolier(zeal.BandolierPath(cfg.EQPath, name), name)
	if err != nil {
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Bandolier = bf
	json.NewEncoder(w).Encode(resp)
}

// PUT /api/zeal/bandolier
// Persists a BandolierFile back to <eq_path>/<Character>_bandolier.ini.
// Body: {"character": "Name", "sets": [{"name": "...", "item_ids": [...]}, ...]}
//
// Every non-zero item ID is validated against the character's current inventory
// AND against the requested slot's worn-slot bit. This is the anti-crash guard:
// a saved set can never reference an item the character doesn't have or that
// can't physically go in that slot. Returns the reparsed file on success.
func (h *zealHandler) updateBandolier(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character string              `json:"character"`
		Sets      []zeal.BandolierSet `json:"sets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(body.Character) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}

	owned := h.ownedItemIDs(cfg.EQPath, body.Character)

	// Validate slot counts, names, and item ownership/fit up-front so a malformed
	// request can't corrupt the file or save an unloadable set.
	seenNames := make(map[string]bool, len(body.Sets))
	for i, s := range body.Sets {
		if len(s.ItemIDs) != zeal.BandolierSlotCount {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("set %d (%q) must have %d slots", i, s.Name, zeal.BandolierSlotCount))
			return
		}
		if s.Name == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("set %d has empty name", i))
			return
		}
		if len([]rune(s.Name)) > 32 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("set %q name exceeds 32 characters", s.Name))
			return
		}
		if strings.ContainsAny(s.Name, "[]\r\n") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("set %d (%q) contains illegal characters", i, s.Name))
			return
		}
		if seenNames[s.Name] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("duplicate set name %q", s.Name))
			return
		}
		seenNames[s.Name] = true

		for slot, id := range s.ItemIDs {
			if id == 0 {
				continue // empty slot is always valid
			}
			if !owned[id] {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("set %q: item %d is not in %s's inventory", s.Name, id, body.Character))
				return
			}
			fits, err := h.itemFitsSlot(id, slot)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to validate item slot")
				return
			}
			if !fits {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("set %q: item %d cannot go in the %s slot", s.Name, id, bandolierSlotName(slot)))
				return
			}
		}
	}

	path := zeal.BandolierPath(cfg.EQPath, body.Character)
	bf := &zeal.BandolierFile{
		Character: body.Character,
		Sets:      body.Sets,
	}
	if err := zeal.WriteBandolier(path, bf); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write bandolier")
		return
	}

	reloaded, err := zeal.ParseBandolier(path, body.Character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wrote file but failed to reparse")
		return
	}
	json.NewEncoder(w).Encode(struct {
		Bandolier *zeal.BandolierFile `json:"bandolier"`
	}{Bandolier: reloaded})
}

// GET /api/zeal/bandolier/all
// Scans the configured EQ directory for every <CharName>_bandolier.ini and
// returns one parsed file per character.
func (h *zealHandler) allBandoliers(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllBandoliers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan bandoliers")
		return
	}
	json.NewEncoder(w).Encode(resp)
}

// GET /api/zeal/bandolier/slot-items?character=Name&slot=0..3&q=search
// Returns the items the character owns that fit the given bandolier slot,
// optionally name-filtered. This powers the slot picker and enforces the
// ownership guard server-side — the UI can only offer items the character has.
func (h *zealHandler) bandolierSlotItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("character")
	if name == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(name) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	slot, err := strconv.Atoi(q.Get("slot"))
	if err != nil || slot < 0 || slot >= zeal.BandolierSlotCount {
		writeError(w, http.StatusBadRequest, "slot must be 0..3")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeJSON(w, http.StatusOK, struct {
			Items []db.BandolierItem `json:"items"`
		}{Items: []db.BandolierItem{}})
		return
	}

	owned := h.ownedItemIDs(cfg.EQPath, name)
	ids := make([]int, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}

	// Build the equip guardrail from the character's class/race/level, which the
	// client supplies from the persona it already loaded. Each is optional: an
	// absent or out-of-range value leaves that check off (see BandolierSlotFilter).
	var filter db.BandolierSlotFilter
	if lvl, err := strconv.Atoi(q.Get("level")); err == nil && lvl > 0 {
		filter.Level = lvl
	}
	if cls, err := strconv.Atoi(q.Get("class")); err == nil && cls >= 0 && cls <= 14 {
		filter.ClassBit = 1 << cls
	}
	if race, err := strconv.Atoi(q.Get("race")); err == nil {
		filter.RaceBit = enums.RaceBitForCharRace(race)
	}

	items, err := h.db.BandolierSlotItems(slot, ids, q.Get("q"), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query slot items")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []db.BandolierItem `json:"items"`
	}{Items: items})
}

// GET /api/zeal/bandolier/bag?character=Name
// Returns the character's preferred bandolier storage bag from zeal.ini's
// [Zeal_<Char>] section. Found is false when unset (never written), which the
// UI distinguishes from an explicit 0 (disabled).
func (h *zealHandler) bandolierBag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("character")
	if name == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(name) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	cfg := h.cfgMgr.Get()
	status := eqconfig.BandolierBagStatus{}
	if cfg.EQPath != "" {
		status = eqconfig.ReadBandolierBagSlot(cfg.EQPath, name)
	}
	writeJSON(w, http.StatusOK, status)
}

// PUT /api/zeal/bandolier/bag  {"character":"Name","slot":0..8}
// Writes the character's preferred storage bag (0=disabled) into zeal.ini's
// [Zeal_<Char>] section, leaving every other section untouched.
func (h *zealHandler) updateBandolierBag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character string `json:"character"`
		Slot      int    `json:"slot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(body.Character) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	if body.Slot < 0 || body.Slot > 8 {
		writeError(w, http.StatusBadRequest, "slot must be 0..8")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}
	if err := eqconfig.SetBandolierBagSlot(cfg.EQPath, body.Character, body.Slot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write bag slot")
		return
	}
	writeJSON(w, http.StatusOK, eqconfig.ReadBandolierBagSlot(cfg.EQPath, body.Character))
}

// POST /api/zeal/bandolier/parse-file
// Parses an arbitrary bandolier .ini file path (typically chosen via the
// Electron file dialog when importing another player's sets). The character
// name is inferred from the filename when possible.
// Body: {"path": "..."}
func (h *zealHandler) parseBandolierFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.EqualFold(filepath.Ext(body.Path), ".ini") {
		writeError(w, http.StatusBadRequest, "file must have .ini extension")
		return
	}

	character := ""
	if m := bandolierFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	bf, err := zeal.ParseBandolier(body.Path, character)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse: %s", err.Error()))
		return
	}
	json.NewEncoder(w).Encode(struct {
		Bandolier *zeal.BandolierFile `json:"bandolier"`
	}{Bandolier: bf})
}

// ownedItemIDs resolves the set of item IDs the named character currently owns,
// from their Zeal inventory export (falling back to the Quarmy export's
// inventory section). Includes equipped, bagged, and bank items. Returns an
// empty set when no export is available.
func (h *zealHandler) ownedItemIDs(eqPath, character string) map[int]bool {
	owned := map[int]bool{}
	add := func(entries []zeal.InventoryEntry) {
		for _, e := range entries {
			if e.ID > 0 {
				owned[e.ID] = true
			}
		}
	}
	if path := zeal.FindInventoryFile(eqPath, character); path != "" {
		if inv, err := zeal.ParseInventory(path, character); err == nil {
			add(inv.Entries)
		}
	}
	if len(owned) == 0 {
		if path := zeal.FindQuarmyFile(eqPath, character); path != "" {
			if q, err := zeal.ParseQuarmy(path, character); err == nil {
				add(q.Inventory)
			}
		}
	}
	return owned
}

// itemFitsSlot reports whether item id can be equipped in the given bandolier
// slot index, by intersecting the item's worn-slot bitmask with the slot bit.
func (h *zealHandler) itemFitsSlot(id, slot int) (bool, error) {
	if slot < 0 || slot >= len(db.BandolierSlotBits) {
		return false, nil
	}
	item, err := h.db.GetItem(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil // owned but not a known DB item — treat as no-fit
		}
		return false, err
	}
	if item == nil {
		return false, nil
	}
	return item.Slots&db.BandolierSlotBits[slot] != 0, nil
}

// bandolierSlotName returns the human-readable name for a bandolier slot index.
func bandolierSlotName(slot int) string {
	switch slot {
	case zeal.BandolierPrimary:
		return "Primary"
	case zeal.BandolierSecondary:
		return "Secondary"
	case zeal.BandolierRange:
		return "Range"
	case zeal.BandolierAmmo:
		return "Ammo"
	}
	return "unknown"
}

// GET /api/zeal/macros
// Returns in-game social macros. With no query, returns the active character's
// cached macros (or null). With ?character=Name, parses that character's
// <Name>_pq.proj.ini [Socials] section directly.
func (h *zealHandler) macros(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Macros *zeal.MacroFile `json:"macros"`
	}{}
	name := r.URL.Query().Get("character")
	if name == "" {
		resp.Macros = h.watcher.Macros()
		json.NewEncoder(w).Encode(resp)
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		json.NewEncoder(w).Encode(resp)
		return
	}
	mf, err := zeal.ParseMacros(zeal.MacroPath(cfg.EQPath, name), name)
	if err != nil {
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Macros = mf
	json.NewEncoder(w).Encode(resp)
}

// PUT /api/zeal/macros
// Surgically rewrites the [Socials] section of <Character>_pq.proj.ini.
// Body: {"character": "Name", "buttons": [{"page","button","name","color","lines"}, ...],
//
//	"base_modified_at": "<exported_at from the GET, optional>"}
//
// The file must already exist (it's the live client config — we never fabricate
// it). Validation rejects out-of-range pages/buttons, wrong line counts, and CR/
// LF in names/lines (which would corrupt the INI). When base_modified_at is
// sent and the file's mtime no longer matches, the write is refused with 409 so
// changes made on disk since the editor loaded (e.g. by the game client) are
// never clobbered. Returns the reparsed macros.
func (h *zealHandler) updateMacros(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character      string             `json:"character"`
		Buttons        []zeal.MacroButton `json:"buttons"`
		BaseModifiedAt *time.Time         `json:"base_modified_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Character == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	if !isSafeCharacterName(body.Character) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}

	for _, b := range body.Buttons {
		if b.Page < 1 || b.Page > zeal.MacroPageCount {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("page %d out of range (1..%d)", b.Page, zeal.MacroPageCount))
			return
		}
		if b.Button < 1 || b.Button > zeal.MacroButtonsPerPage {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("button %d out of range (1..%d)", b.Button, zeal.MacroButtonsPerPage))
			return
		}
		if len(b.Lines) != zeal.MacroLineCount {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("button %d/%d must have %d lines", b.Page, b.Button, zeal.MacroLineCount))
			return
		}
		if strings.ContainsAny(b.Name, "\r\n") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("button %d/%d name contains a line break", b.Page, b.Button))
			return
		}
		for _, l := range b.Lines {
			if strings.ContainsAny(l, "\r\n") {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("button %d/%d has a command line with a line break", b.Page, b.Button))
				return
			}
		}
	}

	path := zeal.MacroPath(cfg.EQPath, body.Character)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("no _pq.proj.ini for %s yet — log in once so the client creates it", body.Character))
		} else {
			// A permission error / unreadable path is a real failure, not "the
			// file hasn't been created yet".
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("stat config file: %s", err))
		}
		return
	}
	if body.BaseModifiedAt != nil && !info.ModTime().Equal(*body.BaseModifiedAt) {
		writeError(w, http.StatusConflict, fmt.Sprintf("%s's config file changed on disk since it was loaded — Refresh to pick up the latest macros, then reapply your edits", body.Character))
		return
	}

	mf := &zeal.MacroFile{Character: body.Character, Buttons: body.Buttons}
	if err := zeal.WriteMacros(path, mf); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to write macros")
		return
	}

	reloaded, err := zeal.ParseMacros(path, body.Character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wrote file but failed to reparse")
		return
	}
	json.NewEncoder(w).Encode(struct {
		Macros *zeal.MacroFile `json:"macros"`
	}{Macros: reloaded})
}

// GET /api/zeal/keybinds/page10?character=Name
// Returns the character's Zeal Page 10 social keybinds from eqclient.ini, read
// from [KeyMaps_<Name>] or [KeyMaps] per Zeal's PerCharKeybinds.
func (h *zealHandler) page10Keybinds(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("character")
	if !isSafeCharacterName(name) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}
	binds, err := eqconfig.ReadPage10Binds(cfg.EQPath, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(binds)
}

// PUT /api/zeal/keybinds/page10
// Body: {"character": "Name", "social": [{"slot","primary","alternate"}, ...],
//
//	"base_modified_at": "<modified_at from the GET>"}
//
// Writes only the given Page 10 social binds, after an automatic config backup.
// 409 when eqclient.ini changed since the GET — the client rewrites it on exit,
// and that must never be silently overwritten. Returns the re-read binds.
func (h *zealHandler) updatePage10Keybinds(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Character      string             `json:"character"`
		Social         []eqconfig.KeyBind `json:"social"`
		BaseModifiedAt *time.Time         `json:"base_modified_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !isSafeCharacterName(body.Character) {
		writeError(w, http.StatusBadRequest, "invalid character name")
		return
	}
	for _, kb := range body.Social {
		if kb.Slot < 1 || kb.Slot > eqconfig.Page10SocialSlots {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("slot %d out of range", kb.Slot))
			return
		}
		for _, v := range []string{kb.Primary, kb.Alternate} {
			if !eqconfig.ValidBindValue(v) {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("slot %d: unsupported key %q", kb.Slot, v))
				return
			}
		}
	}
	cfg := h.cfgMgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path not configured")
		return
	}
	var base time.Time
	if body.BaseModifiedAt != nil {
		base = *body.BaseModifiedAt
	}
	// Checked here as well as in SetPage10Binds so a stale editor gets its 409
	// before a backup is taken for a write that will not happen.
	if cur, err := eqconfig.ReadPage10Binds(cfg.EQPath, body.Character); err == nil &&
		cur.Found && !base.IsZero() && !cur.ModifiedAt.Equal(base) {
		writeError(w, http.StatusConflict, "eqclient.ini changed on disk since it was loaded — Refresh, then reapply your keybind edits")
		return
	}
	if h.backupMgr != nil {
		if _, err := h.backupMgr.Create("Before changing Page 10 keybinds", "Auto-backup before writing eqclient.ini"); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup before write failed: %s", err))
			return
		}
	}
	if err := eqconfig.SetPage10Binds(cfg.EQPath, body.Character, body.Social, base); err != nil {
		if errors.Is(err, eqconfig.ErrChangedOnDisk) {
			writeError(w, http.StatusConflict, "eqclient.ini changed on disk since it was loaded — Refresh, then reapply your keybind edits")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	binds, err := eqconfig.ReadPage10Binds(cfg.EQPath, body.Character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "wrote file but failed to re-read it")
		return
	}
	json.NewEncoder(w).Encode(binds)
}

// GET /api/zeal/macros/all
// Scans the configured EQ directory for every <CharName>_pq.proj.ini and returns
// the parsed [Socials] macros per character.
func (h *zealHandler) allMacros(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllMacros()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan macros")
		return
	}
	// Drop macro files for characters the user has hidden from the ribbon,
	// unless ?include_hidden=1. Keyed by name so a mule that was never imported
	// into the characters table is still hidden here.
	if h.charStore != nil && r.URL.Query().Get("include_hidden") != "1" {
		if hidden, hErr := h.charStore.HiddenNames(); hErr == nil && len(hidden) > 0 {
			kept := resp.Characters[:0]
			for _, mf := range resp.Characters {
				if _, isHidden := hidden[strings.ToLower(mf.Character)]; !isHidden {
					kept = append(kept, mf)
				}
			}
			resp.Characters = kept
		}
	}
	json.NewEncoder(w).Encode(resp)
}

// POST /api/zeal/macros/parse-file
// Parses an arbitrary _pq.proj.ini file path (chosen via the Electron file
// dialog when importing another character's macros). Read-only.
// Body: {"path": "..."}
func (h *zealHandler) parseMacrosFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !strings.EqualFold(filepath.Ext(body.Path), ".ini") {
		writeError(w, http.StatusBadRequest, "file must have .ini extension")
		return
	}

	character := ""
	if m := macroFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	mf, err := zeal.ParseMacros(body.Path, character)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse: %s", err.Error()))
		return
	}
	json.NewEncoder(w).Encode(struct {
		Macros *zeal.MacroFile `json:"macros"`
	}{Macros: mf})
}

// GET /api/zeal/text-colors
// Returns the social color palette: the client's built-in defaults (0–19)
// overridden per-slot by eqclient.ini [TextColors], used to render best-effort
// swatches for social-macro color indices.
func (h *zealHandler) textColors(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfgMgr.Get()
	resp := struct {
		Configured bool              `json:"configured"`
		Colors     []zeal.MacroColor `json:"colors"`
	}{Configured: cfg.EQPath != "", Colors: []zeal.MacroColor{}}
	if cfg.EQPath == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	colors, err := zeal.MacroColorPalette(cfg.EQPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read eqclient.ini colors")
		return
	}
	if colors != nil {
		resp.Colors = colors
	}
	writeJSON(w, http.StatusOK, resp)
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

// zealSettingsResponse is the Zeal settings checklist (Settings › Zeal and
// the setup wizard).
type zealSettingsResponse struct {
	EQPath        string        `json:"eq_path"`
	Installed     bool          `json:"installed"`
	Version       string        `json:"version,omitempty"`
	INIReadable   bool          `json:"ini_readable"`
	PipeConnected bool          `json:"pipe_connected"`
	// PipeMatches is true when the connected game runs from EQPath, so live
	// pipe data counts for this install.
	PipeMatches  bool          `json:"pipe_matches"`
	Capabilities []zeal.CapRow `json:"capabilities"`
}

// GET /api/zeal/settings
// The Zeal features PQ Companion benefits from that this install's Zeal
// supports, each with whether it's on (zeal.ini) and seen working (pipe).
// ?path= checks an unsaved folder, like /detect, for the onboarding wizard.
func (h *zealHandler) settings(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = h.cfgMgr.Get().EQPath
	}
	install := zeal.DetectInstall(r.Context(), path, nil)
	resp := zealSettingsResponse{EQPath: path, Installed: install.Installed, Version: install.Version}
	if h.pipe != nil {
		resp.PipeConnected = h.pipe.Status().State == zealpipe.StateConnected
	}
	if !install.Installed {
		resp.Capabilities = []zeal.CapRow{}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	ev := zeal.CapEvidence{Version: install.Version, INI: zeal.ReadZealINI(path)}
	if t := h.watcher.CapTracker(); t != nil {
		ev.Live, ev.Persisted, resp.PipeMatches = t.Evidence(path)
	}
	resp.INIReadable = ev.INI.Readable
	resp.Capabilities = zeal.BuildCapRows(ev)
	if resp.Capabilities == nil {
		resp.Capabilities = []zeal.CapRow{}
	}
	writeJSON(w, http.StatusOK, resp)
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
	writeJSON(w, http.StatusOK, zeal.DetectInstall(r.Context(), path, h.latest))
}

// GET /api/zeal/spellsets/all
// Scans the configured EQ directory for every <CharName>_spellsets.ini and
// returns one parsed file per character.
func (h *zealHandler) allSpellsets(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllSpellsets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan spellsets")
		return
	}
	json.NewEncoder(w).Encode(resp)
}

// estimateCHA returns a character's Charisma for the merchant sell-price
// estimate: Quarmy base CHA + equipped gear + trained AAs, clamped to the
// level cap — the same layers as the Character Info "Equipped" column. Buffs
// are left out (they come and go). 0 when there's no character record.
func (h *zealHandler) estimateCHA(eqPath, name string) int {
	if h.charStore == nil || h.db == nil {
		return 0
	}
	char, ok, err := h.charStore.GetByName(name)
	if err != nil || !ok || char.BaseCHA <= 0 {
		return 0
	}
	var aa db.AABonuses
	if trained, err := h.charStore.ListAAs(char.ID); err == nil {
		conv := make([]db.TrainedAA, 0, len(trained))
		for _, t := range trained {
			conv = append(conv, db.TrainedAA{AAID: t.AAID, Rank: t.Rank})
		}
		aa, _ = h.db.AAStatBonuses(conv)
	}
	ch := &charactersHandler{store: h.charStore, mgr: h.cfgMgr, db: h.db}
	gear, _ := ch.sumEquipment(eqPath, char.Name)
	raw := char.BaseCHA + gear.CHA + aa.CHA
	return eqstat.CapAttribute(raw, char.Level, aa.StatCap.CHA)
}

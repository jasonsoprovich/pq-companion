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

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

var spellsetFilenameRe = regexp.MustCompile(`(?i)^(.+?)_spellsets\.ini$`)
var bandolierFilenameRe = regexp.MustCompile(`(?i)^(.+?)_bandolier\.ini$`)
var macroFilenameRe = regexp.MustCompile(`(?i)^(.+?)_pq\.proj\.ini$`)

type zealHandler struct {
	watcher *zeal.Watcher
	cfgMgr  *config.Manager
	db      *db.DB
	pipe    *zealpipe.Supervisor
	latest  *zeal.LatestFetcher
}

// enrichEntries fills in the Icon and MaxCharges fields on each entry by looking
// up the items DB for all referenced IDs in batch queries. Errors are swallowed
// (entries are returned without the enrichment) — these fields are decorative /
// supplementary and shouldn't fail the inventory request. MaxCharges is set only
// for rechargeable click items, which flags them for the Rechargeable Items view.
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

	owned := h.ownedItemIDs(cfg.EQPath, body.Character)

	// Validate slot counts, names, and item ownership/fit up-front so a malformed
	// request can't corrupt the file or save an unloadable set.
	seenNames := make(map[string]bool, len(body.Sets))
	for i, s := range body.Sets {
		if len(s.ItemIDs) != zeal.BandolierSlotCount {
			http.Error(w, fmt.Sprintf(`{"error":"set %d (%q) must have %d slots"}`, i, s.Name, zeal.BandolierSlotCount), http.StatusBadRequest)
			return
		}
		if s.Name == "" {
			http.Error(w, fmt.Sprintf(`{"error":"set %d has empty name"}`, i), http.StatusBadRequest)
			return
		}
		if len([]rune(s.Name)) > 32 {
			http.Error(w, fmt.Sprintf(`{"error":"set %q name exceeds 32 characters"}`, s.Name), http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(s.Name, "[]\r\n") {
			http.Error(w, fmt.Sprintf(`{"error":"set %d (%q) contains illegal characters"}`, i, s.Name), http.StatusBadRequest)
			return
		}
		if seenNames[s.Name] {
			http.Error(w, fmt.Sprintf(`{"error":"duplicate set name %q"}`, s.Name), http.StatusBadRequest)
			return
		}
		seenNames[s.Name] = true

		for slot, id := range s.ItemIDs {
			if id == 0 {
				continue // empty slot is always valid
			}
			if !owned[id] {
				http.Error(w, fmt.Sprintf(`{"error":"set %q: item %d is not in %s's inventory"}`, s.Name, id, body.Character), http.StatusBadRequest)
				return
			}
			fits, err := h.itemFitsSlot(id, slot)
			if err != nil {
				http.Error(w, `{"error":"failed to validate item slot"}`, http.StatusInternalServerError)
				return
			}
			if !fits {
				http.Error(w, fmt.Sprintf(`{"error":"set %q: item %d cannot go in the %s slot"}`, s.Name, id, bandolierSlotName(slot)), http.StatusBadRequest)
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
		http.Error(w, `{"error":"failed to write bandolier"}`, http.StatusInternalServerError)
		return
	}

	reloaded, err := zeal.ParseBandolier(path, body.Character)
	if err != nil {
		http.Error(w, `{"error":"wrote file but failed to reparse"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"failed to scan bandoliers"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"character is required"}`, http.StatusBadRequest)
		return
	}
	slot, err := strconv.Atoi(q.Get("slot"))
	if err != nil || slot < 0 || slot >= zeal.BandolierSlotCount {
		http.Error(w, `{"error":"slot must be 0..3"}`, http.StatusBadRequest)
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

	items, err := h.db.BandolierSlotItems(slot, ids, q.Get("q"))
	if err != nil {
		http.Error(w, `{"error":"failed to query slot items"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Items []db.BandolierItem `json:"items"`
	}{Items: items})
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
	if m := bandolierFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	bf, err := zeal.ParseBandolier(body.Path, character)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse: %s"}`, err.Error()), http.StatusBadRequest)
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

	for _, b := range body.Buttons {
		if b.Page < 1 || b.Page > zeal.MacroPageCount {
			http.Error(w, fmt.Sprintf(`{"error":"page %d out of range (1..%d)"}`, b.Page, zeal.MacroPageCount), http.StatusBadRequest)
			return
		}
		if b.Button < 1 || b.Button > zeal.MacroButtonsPerPage {
			http.Error(w, fmt.Sprintf(`{"error":"button %d out of range (1..%d)"}`, b.Button, zeal.MacroButtonsPerPage), http.StatusBadRequest)
			return
		}
		if len(b.Lines) != zeal.MacroLineCount {
			http.Error(w, fmt.Sprintf(`{"error":"button %d/%d must have %d lines"}`, b.Page, b.Button, zeal.MacroLineCount), http.StatusBadRequest)
			return
		}
		if strings.ContainsAny(b.Name, "\r\n") {
			http.Error(w, fmt.Sprintf(`{"error":"button %d/%d name contains a line break"}`, b.Page, b.Button), http.StatusBadRequest)
			return
		}
		for _, l := range b.Lines {
			if strings.ContainsAny(l, "\r\n") {
				http.Error(w, fmt.Sprintf(`{"error":"button %d/%d has a command line with a line break"}`, b.Page, b.Button), http.StatusBadRequest)
				return
			}
		}
	}

	path := zeal.MacroPath(cfg.EQPath, body.Character)
	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"no _pq.proj.ini for %s yet — log in once so the client creates it"}`, body.Character), http.StatusBadRequest)
		return
	}
	if body.BaseModifiedAt != nil && !info.ModTime().Equal(*body.BaseModifiedAt) {
		http.Error(w, fmt.Sprintf(`{"error":"%s's config file changed on disk since it was loaded — Refresh to pick up the latest macros, then reapply your edits"}`, body.Character), http.StatusConflict)
		return
	}

	mf := &zeal.MacroFile{Character: body.Character, Buttons: body.Buttons}
	if err := zeal.WriteMacros(path, mf); err != nil {
		http.Error(w, `{"error":"failed to write macros"}`, http.StatusInternalServerError)
		return
	}

	reloaded, err := zeal.ParseMacros(path, body.Character)
	if err != nil {
		http.Error(w, `{"error":"wrote file but failed to reparse"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(struct {
		Macros *zeal.MacroFile `json:"macros"`
	}{Macros: reloaded})
}

// GET /api/zeal/macros/all
// Scans the configured EQ directory for every <CharName>_pq.proj.ini and returns
// the parsed [Socials] macros per character.
func (h *zealHandler) allMacros(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllMacros()
	if err != nil {
		http.Error(w, `{"error":"failed to scan macros"}`, http.StatusInternalServerError)
		return
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
	if m := macroFilenameRe.FindStringSubmatch(filepath.Base(body.Path)); m != nil {
		character = m[1]
	}

	mf, err := zeal.ParseMacros(body.Path, character)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to parse: %s"}`, err.Error()), http.StatusBadRequest)
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
		http.Error(w, `{"error":"failed to read eqclient.ini colors"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"failed to scan spellsets"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/era"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type charactersHandler struct {
	store   *character.Store
	mgr     *config.Manager
	db      *db.DB
	watcher *zeal.Watcher
}

type charactersListResponse struct {
	Characters []character.Character `json:"characters"`
	Active     string                `json:"active"`
	Manual     bool                  `json:"manual"`
	// Detected is the name auto-detection would currently pick (the most
	// recently modified eqlog file). Reported even in manual mode so the
	// switcher UI can show what "Auto" would resolve to without first
	// clearing the manual override.
	Detected string `json:"detected"`
}

// list returns all stored characters and the currently active selection.
// Active is the manually-configured character when set; otherwise the
// auto-detected character (most-recently-modified EQ log file).
func (h *charactersHandler) list(w http.ResponseWriter, r *http.Request) {
	// Re-sync persona/stats/AAs from each character's Quarmy export before
	// reading the store, so the page reflects what's actually on disk rather
	// than whatever the active-character watcher last persisted.
	if h.watcher != nil {
		h.watcher.RefreshAllPersonas()
	}
	chars, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if chars == nil {
		chars = []character.Character{}
	}
	cfg := h.mgr.Get()
	manual := cfg.Character != ""
	detected := logparser.ResolveActiveCharacter(cfg.EQPath)
	active := cfg.Character
	if !manual {
		active = detected
	}
	resp := charactersListResponse{
		Characters: chars,
		Manual:     manual,
		Active:     active,
		Detected:   detected,
	}
	writeJSON(w, http.StatusOK, resp)
}

// discover returns character names found in EQ log files that are not yet stored.
func (h *charactersHandler) discover(w http.ResponseWriter, r *http.Request) {
	cfg := h.mgr.Get()
	discovered := logparser.DiscoverCharacters(cfg.EQPath)

	stored, err := h.store.Names()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var untracked []string
	for _, d := range discovered {
		if _, exists := stored[strings.ToLower(d.Name)]; !exists {
			// Check case-insensitively
			found := false
			for k := range stored {
				if strings.EqualFold(k, d.Name) {
					found = true
					break
				}
			}
			if !found {
				untracked = append(untracked, d.Name)
			}
		}
	}
	if untracked == nil {
		untracked = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"names": untracked})
}

type characterRequest struct {
	Name  string `json:"name"`
	Class int    `json:"class"`
	Race  int    `json:"race"`
	Level int    `json:"level"`
}

// create adds a new character profile.
func (h *charactersHandler) create(w http.ResponseWriter, r *http.Request) {
	var req characterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Level < 1 {
		req.Level = 1
	}
	c, err := h.store.Create(req.Name, req.Class, req.Race, req.Level)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create character: %s", err))
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// del removes a character profile.
func (h *charactersHandler) del(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// aas returns the AA abilities for a character: both the trained list (with
// names resolved from quarm.db) and the full catalog of class-eligible AAs so
// the UI can render every ability and dim untrained ones.
//
// AA IDs throughout this endpoint are altadv_vars.eqmacid values (the EQ
// client AA index used by the Zeal "AAIndex" export).
func (h *charactersHandler) aas(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	trained, err := h.store.ListAAs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if trained == nil {
		trained = []character.AAEntry{}
	}

	var available []db.AAInfo
	if h.db != nil {
		// EQ class indices in our character store run 0-14 (zero-indexed); the
		// altadv_vars `classes` bitmask uses bit N for class N (1-indexed). Map
		// from our 0-indexed class to the bitmask's 1-indexed class id.
		eqClass := char.Class + 1

		available, err = h.db.ListAvailableAAs(eqClass)
		if err == nil {
			eligible := make(map[int]bool, len(available))
			ids := make([]int, len(available))
			for i, a := range available {
				ids[i] = a.AAID
				eligible[a.AAID] = true
			}
			// Drop trained entries that aren't eligible for this class. Zeal's
			// AAIndex export can contain cross-class AAs (e.g. Fleet of Foot
			// for a Wizard) that the character can't actually use; including
			// them makes the tab badge disagree with the points-spent total.
			filtered := trained[:0]
			for _, t := range trained {
				if eligible[t.AAID] {
					filtered = append(filtered, t)
				}
			}
			trained = filtered
			names, _ := h.db.LookupAANames(ids)
			for i := range trained {
				trained[i].Name = names[trained[i].AAID]
			}
		}
	}
	if available == nil {
		available = []db.AAInfo{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"trained":   trained,
		"available": available,
	})
}

// raidBuffs returns the saved raid-buff preset for the character as an
// ordered list of spell IDs. An empty list means the user hasn't customized
// theirs yet — the frontend falls back to its default preset in that case.
func (h *charactersHandler) raidBuffs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok, err := h.store.Get(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	ids, err := h.store.ListRaidBuffs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []int{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"spell_ids": ids})
}

// updateRaidBuffs atomically replaces the saved raid-buff preset for the
// character. Body shape: { "spell_ids": [int, ...] }. Enforces the in-game
// 13-slot cap via character.MaxRaidBuffSlots.
func (h *charactersHandler) updateRaidBuffs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok, err := h.store.Get(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var body struct {
		SpellIDs []int `json:"spell_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.SpellIDs) > character.MaxRaidBuffSlots {
		writeError(w, http.StatusBadRequest, "too many raid buffs (max 13)")
		return
	}
	if err := h.store.ReplaceRaidBuffs(id, body.SpellIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spell_ids": body.SpellIDs})
}

// spellModifiers returns the focus contributions (item focuses + AA focuses)
// available to the character from their most recent Quarmy export.
//
// Optional `spell_id` query param: when set, the response also includes a
// per-spell Resolution showing which contributors apply to that spell after
// EQEmu's filter and stacking rules. Use this to sanity-check the math
// (e.g. Aegolism on Osui should resolve to +65% duration).
func (h *charactersHandler) spellModifiers(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}

	res, err := buffmod.Compute(cfg.EQPath, char.Name, h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("compute modifiers: %s", err))
		return
	}

	resp := map[string]interface{}{
		"character":    res.Character,
		"contributors": res.Contributors,
	}

	if sidStr := r.URL.Query().Get("spell_id"); sidStr != "" {
		spellID, err := strconv.Atoi(sidStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid spell_id")
			return
		}
		sp, err := h.db.GetSpell(spellID)
		if err != nil || sp == nil {
			writeError(w, http.StatusNotFound, "spell not found")
			return
		}
		spellType := buffmod.SpellTypeBeneficial
		if !isBeneficialSpell(h.db, spellID) {
			spellType = buffmod.SpellTypeDetrimental
		}
		// Per-class spell level: SPA 134/139 limits compare against the level
		// the caster's OWN class learns the spell, not the lowest class level
		// (multi-class spells like Celerity: ENC 39 vs SHM 56).
		spellLevel := buffmod.SpellLevelForClass(sp.ClassLevels, char.Class)
		// character_aas table defaults level=1, so anything ≤ 1 likely means
		// "not set yet" rather than literally a level-1 character. Treat that
		// as "cast at the spell's effective level" so the duration formula
		// produces a useful number for sanity-checking modifiers.
		casterLevel := char.Level
		if casterLevel <= 1 {
			casterLevel = spellLevel
		}
		if casterLevel < 1 {
			casterLevel = era.MaxLevel(h.mgr.Get().Preferences.PoPEnabled)
		}
		baseTicks := spelltimer.CalcDurationTicks(sp.BuffDurationFormula, sp.BuffDuration, casterLevel)
		resolution := buffmod.Resolve(
			sp.ID, sp.Name, spellLevel, casterLevel,
			baseTicks*6, // ticks → seconds
			spellType, sp.EffectIDs[:],
			res.Contributors,
			char.Class,
			sp.ClassLevels,
		)
		resp["resolution"] = resolution
	}

	writeJSON(w, http.StatusOK, resp)
}

// equipSlots is the set of inventory locations we treat as worn equipment for
// purposes of summing item stats. Bag/bank contents are ignored.
var equipSlots = map[string]bool{
	"Charm": true, "Ear": true, "Head": true, "Face": true, "Neck": true,
	"Shoulders": true, "Arms": true, "Back": true, "Wrist": true,
	"Range": true, "Hands": true, "Primary": true, "Secondary": true,
	"Fingers": true, "Chest": true, "Legs": true, "Feet": true, "Waist": true,
	"PowerSource": true, "Ammo": true,
}

// statBlock is one column of the Stats panel: a complete set of derived
// vitals, attributes, resists, and worn-bonus stats from a single source
// (base, equipment, or buffs).
//
// Worn-bonus fields (Attack, Haste, Regen, ManaRegen, FT, DmgShield) are
// populated by walking each equipped item's worneffect spell and parsing
// its SPA effect slots — see parseWornEffect.
type statBlock struct {
	HP         int `json:"hp"`
	Mana       int `json:"mana"`
	AC         int `json:"ac"`
	STR        int `json:"str"`
	STA        int `json:"sta"`
	AGI        int `json:"agi"`
	DEX        int `json:"dex"`
	WIS        int `json:"wis"`
	INT        int `json:"int"`
	CHA        int `json:"cha"`
	PR         int `json:"pr"`
	MR         int `json:"mr"`
	DR         int `json:"dr"`
	FR         int `json:"fr"`
	CR         int `json:"cr"`
	Attack     int `json:"attack"`
	Haste      int `json:"haste"`
	SpellHaste int `json:"spell_haste"`
	Regen      int `json:"regen"`
	ManaRegen  int `json:"mana_regen"`
	FT         int `json:"ft"`
	DmgShield  int `json:"dmg_shield"`
	// ATKRating is the EQ inventory-window Attack rating (offense + to-hit),
	// distinct from Attack above (the raw worn/AA/buff SE_ATK bonus that feeds
	// into it). Zero on the base layer until skills are known.
	ATKRating int `json:"atk_rating"`
	// Breakdown splits the regen-type and attack-bonus stats by source so the
	// UI can show where each total comes from (see issue #128).
	Breakdown statBreakdown `json:"breakdown"`
}

// sourceSplit attributes a single stat's total to its three contributing
// sources. The components sum to the corresponding statBlock field.
type sourceSplit struct {
	Item int `json:"item"`
	AA   int `json:"aa"`
	Buff int `json:"buff"`
}

// statBreakdown carries the per-source split for the stats the Stats panel
// exposes a hover breakdown on. FT has no AA or buff source (worn focus only),
// so its AA/Buff components are always zero.
type statBreakdown struct {
	ManaRegen  sourceSplit `json:"mana_regen"`
	Regen      sourceSplit `json:"regen"`
	FT         sourceSplit `json:"ft"`
	Attack     sourceSplit `json:"attack"`
	Haste      sourceSplit `json:"haste"`
	SpellHaste sourceSplit `json:"spell_haste"`
	DmgShield  sourceSplit `json:"dmg_shield"`
}

// equippedStatsResponse is the per-source breakdown the Stats panel renders.
// Total = Base + (Equipment if mode != base) + (Buff sum if mode == buffed,
// computed on the frontend). Level and Class are echoed back so the frontend
// can apply EQ's stat-derived HP/Mana bonus on top, using whichever STA/INT/
// WIS the active mode produces.
type equippedStatsResponse struct {
	Character string    `json:"character"`
	Level     int       `json:"level"`
	Class     int       `json:"class"`
	Base      statBlock `json:"base"`
	Equipment statBlock `json:"equipment"`
}

// defaultBaseResist is EQ's blank-slate per-resist starting value before any
// race or class adjustments. Hardcoded in EQEmu source rather than in the DB,
// so we mirror it here.
const defaultBaseResist = 25

// itemFTCap is the per-character cap on item-sourced "Flowing Thought"
// (worn mana regen) contributions, matching EQEmu's worn-bonus cap. Buff-
// sourced mana regen (e.g. Clarity, KEI) is not capped against this.
const itemFTCap = 15

// EQEmu SPA codes referenced when parsing a worneffect spell.
const (
	spaHitpoints   = 0 // base/tick = HP regen on a worn buff
	spaAC          = 1
	spaATK         = 2
	spaSTR         = 4
	spaDEX         = 5
	spaAGI         = 6
	spaSTA         = 7
	spaINT         = 8
	spaWIS         = 9
	spaCHA         = 10
	spaMeleeHaste  = 11 // base = haste% + 100
	spaMana        = 15 // base/tick = mana regen on a worn buff
	spaResistFire  = 46
	spaResistCold  = 47
	spaPoisonRes   = 48
	spaDiseaseRes  = 49
	spaMagicRes    = 50
	spaDmgShield   = 59
	spaMaxHP       = 69
	spaManaPool    = 97
	spaMeleeHaste2 = 119
)

// parseWornEffect walks the 12 effect slots of a worneffect spell and adds
// every recognized SPA contribution into `out`. SPA 11 (Melee Haste) is
// returned separately so the caller can take the max across all items rather
// than summing — worn haste doesn't stack.
//
// wornLevel is the item's wornlevel column, used to apply the spell's effect
// formula for haste (formula 102 = base + level). Stat SPAs (STR/AC/HP/etc.)
// ignore wornLevel because items use them as static values.
func parseWornEffect(s *db.Spell, wornLevel int, out *statBlock) (haste int) {
	for i := 0; i < 12; i++ {
		spa := s.EffectIDs[i]
		base := s.EffectBaseValues[i]
		if spa == 254 || spa == 255 || spa == 0 && base == 0 {
			continue
		}
		switch spa {
		case spaAC:
			out.AC += base
		case spaATK:
			out.Attack += base
		case spaSTR:
			out.STR += base
		case spaSTA:
			out.STA += base
		case spaAGI:
			out.AGI += base
		case spaDEX:
			out.DEX += base
		case spaWIS:
			out.WIS += base
		case spaINT:
			out.INT += base
		case spaCHA:
			out.CHA += base
		case spaHitpoints:
			// On a worn buff (long buff_duration), positive base = HP regen
			// per tick. Negative base on a worn effect is rare and skipped.
			if base > 0 {
				out.Regen += base
			}
		case spaMana:
			if base > 0 {
				// All worn mana-regen items are exposed as "Flowing Thought
				// N" by the spell name; that's the FT bucket. Anything else
				// rolls into generic ManaRegen.
				if strings.HasPrefix(s.Name, "Flowing Thought") {
					out.FT += base
				} else {
					out.ManaRegen += base
				}
			}
		case spaMeleeHaste, spaMeleeHaste2:
			// Apply the spell's effect formula to wornLevel (formula 102 =
			// linear scaling for spell 998 "Haste"; formula 100 = static for
			// the rare fixed-value haste templates), then convert from
			// EQEmu's "100 + percent" encoding. Reported separately so the
			// caller can max-stack — worn haste doesn't stack.
			v := db.ComputeEffectValue(s.EffectFormulas[i], base, s.EffectMaxValues[i], wornLevel)
			if v > 100 {
				h := v - 100
				if h > haste {
					haste = h
				}
			}
		case spaResistFire:
			out.FR += base
		case spaResistCold:
			out.CR += base
		case spaPoisonRes:
			out.PR += base
		case spaDiseaseRes:
			out.DR += base
		case spaMagicRes:
			out.MR += base
		case spaDmgShield:
			// SPA 59 base on items is conventionally negative (dmg dealt to
			// attacker is "negative" hp). Show the magnitude.
			if base < 0 {
				out.DmgShield += -base
			} else {
				out.DmgShield += base
			}
		case spaMaxHP:
			out.HP += base
		case spaManaPool:
			out.Mana += base
		}
	}
	return haste
}

// equippedStats returns the character's "base" stats (level/class HP+Mana
// from base_data, race resists, attribs from quarmy) and the summed
// contribution from currently equipped items.
func (h *charactersHandler) equippedStats(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}

	resp := equippedStatsResponse{
		Character: char.Name,
		Level:     char.Level,
		Class:     char.Class,
	}

	// ── Base block ──
	// Quarmy attribs (already populated on the character row from the most
	// recent export) become the seven attribute columns. base_data gives us
	// the level/class HP and Mana floor; the frontend layers a stat-derived
	// bonus on top using whichever STA/INT/WIS the active mode produces.
	resp.Base = statBlock{
		STR: char.BaseSTR, STA: char.BaseSTA, AGI: char.BaseAGI,
		DEX: char.BaseDEX, WIS: char.BaseWIS, INT: char.BaseINT, CHA: char.BaseCHA,
		PR: defaultBaseResist, MR: defaultBaseResist, DR: defaultBaseResist,
		FR: defaultBaseResist, CR: defaultBaseResist,
	}
	if char.Class >= 0 && char.Level > 0 {
		// character.Class is 0-indexed (0=WAR); base_data uses 1-indexed.
		bd, err := h.db.GetBaseData(char.Level, char.Class+1)
		if err == nil {
			resp.Base.HP = int(bd.HP)
			resp.Base.Mana = int(bd.Mana)
		}
	}

	// ── Equipment block ──
	resp.Equipment, _ = h.sumEquipment(cfg.EQPath, char.Name)

	// Spell haste (SPA 127) summary across equipped item focuses + trained
	// AAs, hard-capped at SpellHasteCapPercent (50%). Distinct from melee
	// haste above. Best-effort: a missing Quarmy export just leaves it at 0.
	if mods, err := buffmod.Compute(cfg.EQPath, char.Name, h.db); err == nil && mods != nil {
		resp.Equipment.SpellHaste = buffmod.SpellHasteSummary(mods.Contributors)
	}

	writeJSON(w, http.StatusOK, resp)
}

// sumEquipment returns the raw summed contribution of the character's currently
// worn items: attributes, item AC, resists, the flat HP/Mana pools, and the
// parsed worn-effect bonuses (attack, regen, FT, damage shield). bestHaste is
// the single highest worn melee-haste value (worn haste doesn't stack). A
// missing or unreadable Quarmy export yields a zero block.
func (h *charactersHandler) sumEquipment(eqPath, charName string) (block statBlock, bestHaste int) {
	path := zeal.FindQuarmyFile(eqPath, charName)
	if path == "" {
		return block, 0
	}
	q, err := zeal.ParseQuarmy(path, charName)
	if err != nil || q == nil {
		return block, 0
	}
	for _, entry := range q.Inventory {
		if !equipSlots[entry.Location] || entry.ID <= 0 {
			continue
		}
		item, err := h.db.GetItem(entry.ID)
		if err != nil || item == nil {
			continue
		}
		block.HP += item.HP
		block.Mana += item.Mana
		block.AC += item.AC
		block.STR += item.Strength
		block.STA += item.Stamina
		block.AGI += item.Agility
		block.DEX += item.Dexterity
		block.WIS += item.Wisdom
		block.INT += item.Intelligence
		block.CHA += item.Charisma
		block.PR += item.PoisonResist
		block.MR += item.MagicResist
		block.DR += item.DiseaseResist
		block.FR += item.FireResist
		block.CR += item.ColdResist

		if item.WornEffect > 0 {
			worn, err := h.db.GetSpell(item.WornEffect)
			if err == nil && worn != nil {
				if hv := parseWornEffect(worn, item.WornLevel, &block); hv > bestHaste {
					bestHaste = hv
				}
			}
		}
	}
	block.Haste = bestHaste
	// Item Flowing Thought is capped at 15 per EQ's worn-mana-regen rule.
	if block.FT > itemFTCap {
		block.FT = itemFTCap
	}
	return block, bestHaste
}

// overhasteSpellIDs are the only sources of v3 "overhaste" on Project Quarm —
// the lone tier that stacks past the level-based melee-haste cap.
var overhasteSpellIDs = map[int]bool{
	2610: true, // Warsong of the Vah Shir
}

// resolvedBuff pairs a buff spell's id (needed to classify overhaste) with its
// parsed stat delta.
type resolvedBuff struct {
	id    int
	delta db.BuffStatDelta
}

// resolveBuffs loads each unique buff spell and computes its stat delta. Unknown
// or duplicate spell ids are skipped.
func (h *charactersHandler) resolveBuffs(ids []int) []resolvedBuff {
	seen := make(map[int]bool, len(ids))
	out := make([]resolvedBuff, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		sp, err := h.db.GetSpell(id)
		if err != nil || sp == nil {
			continue
		}
		out = append(out, resolvedBuff{id: id, delta: db.ComputeBuffStatDelta(sp)})
	}
	return out
}

// derivedStatsRequest is the body of the derived-stats endpoint: the buff spell
// ids active in the "+Buffs" preset and in the "Live Buffs" view. The backend
// computes all four display layers so the UI can switch modes instantly.
type derivedStatsRequest struct {
	PresetBuffIDs []int `json:"preset_buff_ids"`
	LiveBuffIDs   []int `json:"live_buff_ids"`
}

// derivedStatsResponse carries one fully-derived statBlock per display mode.
type derivedStatsResponse struct {
	Character string    `json:"character"`
	Level     int       `json:"level"`
	Class     int       `json:"class"`
	Base      statBlock `json:"base"`
	Equipped  statBlock `json:"equipped"`
	Buffed    statBlock `json:"buffed"`
	Live      statBlock `json:"live"`
}

// derivedStats computes the character's HP, Mana, AC, attributes, resists, and
// worn-bonus stats for every display layer, using Project Quarm's real
// server-side formulas (see internal/eqstat). Unlike the older additive
// equipped-stats endpoint, vitals are *derived* from each layer's total
// attributes — so a buff that raises STA correctly compounds into HP, etc.
func (h *charactersHandler) derivedStats(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}

	var req derivedStatsRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body → all-equipped only
	}

	// Shared inputs computed once.
	var aa db.AABonuses
	if trained, err := h.store.ListAAs(id); err == nil {
		conv := make([]db.TrainedAA, 0, len(trained))
		for _, t := range trained {
			conv = append(conv, db.TrainedAA{AAID: t.AAID, Rank: t.Rank})
		}
		aa, _ = h.db.AAStatBonuses(conv)
	}
	spellHasteItem, spellHasteAA := 0, 0
	if mods, err := buffmod.Compute(cfg.EQPath, char.Name, h.db); err == nil && mods != nil {
		spellHasteItem, spellHasteAA = buffmod.SpellHasteSources(mods.Contributors)
	}
	spellHaste := spellHasteSplit{item: spellHasteItem, aa: spellHasteAA}
	// Skill caps the ATK rating and displayed AC assume — a max-level main is
	// virtually always at cap and the export carries no live skill values.
	classIdx := char.Class + 1
	defenseSkill, _ := h.db.DefenseSkillCap(classIdx, char.Level)
	offenseSkill, _ := h.db.OffenseSkillCap(classIdx, char.Level)
	weaponSkill, _ := h.db.BestWeaponSkillCap(classIdx, char.Level)
	skills := skillCaps{defense: defenseSkill, offense: offenseSkill, weapon: weaponSkill}
	itemBlock, itemHaste := h.sumEquipment(cfg.EQPath, char.Name)

	presetBuffs := h.resolveBuffs(req.PresetBuffIDs)
	liveBuffs := h.resolveBuffs(req.LiveBuffIDs)

	empty := statBlock{}
	resp := derivedStatsResponse{
		Character: char.Name,
		Level:     char.Level,
		Class:     char.Class,
		Base:      h.deriveBlock(char, aa, spellHasteSplit{}, skills, empty, 0, nil),
		Equipped:  h.deriveBlock(char, aa, spellHaste, skills, itemBlock, itemHaste, nil),
		Buffed:    h.deriveBlock(char, aa, spellHaste, skills, itemBlock, itemHaste, presetBuffs),
		Live:      h.deriveBlock(char, aa, spellHaste, skills, itemBlock, itemHaste, liveBuffs),
	}
	writeJSON(w, http.StatusOK, resp)
}

// skillCaps bundles the assumed-at-cap skill values a derived stat layer needs.
type skillCaps struct {
	defense int
	offense int
	weapon  int
}

// spellHasteSplit carries the worn-focus and AA components of spell haste (SPA
// 127) into a derived layer so the panel can show an Equip / Buffs / AA split.
// Buffs add on top inside deriveBlock; the combined total is capped there.
type spellHasteSplit struct {
	item int
	aa   int
}

// deriveBlock combines one display layer's inputs (base attributes + always-on
// AA bonuses + optional equipment + optional buffs) and derives the vitals from
// the resulting totals using the Quarm formulas. item is the raw worn-item
// contribution (zero for the base layer); buffs is the active buff set (nil for
// base/equipped).
func (h *charactersHandler) deriveBlock(
	char character.Character,
	aa db.AABonuses,
	spellHasteSrc spellHasteSplit, skills skillCaps,
	item statBlock, itemHaste int,
	buffs []resolvedBuff,
) statBlock {
	level, class, race := char.Level, char.Class, char.Race

	// Additive attribute contributions beyond base: AA grants + items + buffs.
	addSTR := aa.STR + item.STR
	addSTA := aa.STA + item.STA
	addAGI := aa.AGI + item.AGI
	addDEX := aa.DEX + item.DEX
	addWIS := aa.WIS + item.WIS
	addINT := aa.INT + item.INT
	addCHA := aa.CHA + item.CHA

	// Additive resists: AA + item + buffs (racial/class base added in eqstat).
	addRes := eqstat.Resists{
		MR: aa.MR + item.MR, CR: aa.CR + item.CR, FR: aa.FR + item.FR,
		DR: aa.DR + item.DR, PR: aa.PR + item.PR,
	}

	itemHP := item.HP     // inside the AA HP-percent
	buffHP := 0           // added after the HP-percent
	flatMana := item.Mana // item + buff mana pool (linear)
	itemAC := item.AC
	spellAC := 0
	attack := item.Attack + aa.Attack
	regen := item.Regen + aa.HPRegen
	manaRegen := item.ManaRegen + aa.ManaRegen
	ft := item.FT
	dmgShield := item.DmgShield
	spellHaste := spellHasteSrc.item + spellHasteSrc.aa

	// Buff-only sums for the source breakdown (issue #128) — kept alongside the
	// running totals so the UI can split a stat into Equip / Buffs / AA.
	buffAttack, buffRegen, buffManaRegen := 0, 0, 0
	buffDmgShield, buffSpellHaste := 0, 0

	buffHasteV2 := 0 // highest non-overhaste buff haste
	overhasteV3 := 0 // summed overhaste
	for _, b := range buffs {
		d := b.delta
		addSTR += d.STR
		addSTA += d.STA
		addAGI += d.AGI
		addDEX += d.DEX
		addWIS += d.WIS
		addINT += d.INT
		addCHA += d.CHA
		addRes.MR += d.MR
		addRes.CR += d.CR
		addRes.FR += d.FR
		addRes.DR += d.DR
		addRes.PR += d.PR
		buffHP += d.HP
		flatMana += d.Mana
		spellAC += d.AC
		attack += d.Attack
		regen += d.Regen
		manaRegen += d.ManaRegen
		dmgShield += d.DmgShield
		spellHaste += d.SpellHaste
		buffAttack += d.Attack
		buffRegen += d.Regen
		buffManaRegen += d.ManaRegen
		buffDmgShield += d.DmgShield
		buffSpellHaste += d.SpellHaste
		if d.Haste > 0 {
			if overhasteSpellIDs[b.id] {
				overhasteV3 += d.Haste
			} else if d.Haste > buffHasteV2 {
				buffHasteV2 = d.Haste
			}
		}
	}

	// Total attributes = base + additive, each capped at the level's stat cap.
	totSTR := eqstat.CapAttribute(char.BaseSTR+addSTR, level, 0)
	totSTA := eqstat.CapAttribute(char.BaseSTA+addSTA, level, 0)
	totAGI := eqstat.CapAttribute(char.BaseAGI+addAGI, level, 0)
	totDEX := eqstat.CapAttribute(char.BaseDEX+addDEX, level, 0)
	totWIS := eqstat.CapAttribute(char.BaseWIS+addWIS, level, 0)
	totINT := eqstat.CapAttribute(char.BaseINT+addINT, level, 0)
	totCHA := eqstat.CapAttribute(char.BaseCHA+addCHA, level, 0)

	res := eqstat.Resistance(class, level, race, addRes, eqstat.Resists{})

	if spellHaste > buffmod.SpellHasteCapPercent {
		spellHaste = buffmod.SpellHasteCapPercent
	}
	haste := itemHaste + buffHasteV2
	if cap := eqstat.MeleeHasteCap(level); haste > cap {
		haste = cap
	}
	haste += overhasteV3

	return statBlock{
		HP:         eqstat.MaxHP(class, level, totSTA, itemHP, buffHP, 0, aa.HPPct),
		Mana:       eqstat.MaxMana(class, level, totWIS, totINT, flatMana),
		AC:         eqstat.DisplayedAC(class, level, race, itemAC, spellAC, totAGI, skills.defense, 0),
		STR:        totSTR,
		STA:        totSTA,
		AGI:        totAGI,
		DEX:        totDEX,
		WIS:        totWIS,
		INT:        totINT,
		CHA:        totCHA,
		MR:         res.MR,
		CR:         res.CR,
		FR:         res.FR,
		DR:         res.DR,
		PR:         res.PR,
		Attack:     attack,
		Haste:      haste,
		SpellHaste: spellHaste,
		Regen:      regen,
		ManaRegen:  manaRegen,
		FT:         ft,
		DmgShield:  dmgShield,
		ATKRating:  eqstat.DisplayedATK(class, level, totSTR, attack, skills.offense, skills.weapon),
		Breakdown: statBreakdown{
			// attack = item.Attack + aa.Attack + Σ buff; regen/manaRegen
			// similarly start from item + AA before the buff loop adds in.
			Attack:    sourceSplit{Item: item.Attack, AA: aa.Attack, Buff: buffAttack},
			Regen:     sourceSplit{Item: item.Regen, AA: aa.HPRegen, Buff: buffRegen},
			ManaRegen: sourceSplit{Item: item.ManaRegen, AA: aa.ManaRegen, Buff: buffManaRegen},
			// FT is worn-focus only — no AA or buff source on Quarm.
			FT: sourceSplit{Item: item.FT},
			// Melee haste has no AA source in this model; Equip is the worn-item
			// haste, Buffs the highest non-overhaste buff plus summed overhaste.
			// Spell haste splits worn focus / AA / buff (SPA 127). Damage Shield
			// is item + buff only. These show raw source contributions, so when a
			// cap applies (melee/level cap or the 50% spell-haste cap) the parts
			// can sum higher than the capped total shown on the row.
			Haste:      sourceSplit{Item: itemHaste, Buff: buffHasteV2 + overhasteV3},
			SpellHaste: sourceSplit{Item: spellHasteSrc.item, AA: spellHasteSrc.aa, Buff: buffSpellHaste},
			DmgShield:  sourceSplit{Item: item.DmgShield, Buff: buffDmgShield},
		},
	}
}

// isBeneficialSpell pulls spells_new.goodEffect (1 = beneficial). On any
// error, defaults to beneficial — the caller's filter check already handles
// the "no SPA 138 limit" case correctly for either bucket.
func isBeneficialSpell(d *db.DB, spellID int) bool {
	var good int
	err := d.QueryRow(`SELECT goodEffect FROM spells_new WHERE id = ?`, spellID).Scan(&good)
	if err != nil {
		return true
	}
	return good == 1
}

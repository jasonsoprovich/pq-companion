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
		spellLevel := buffmod.SpellLevel(sp.ClassLevels)
		// character_aas table defaults level=1, so anything ≤ 1 likely means
		// "not set yet" rather than literally a level-1 character. Treat that
		// as "cast at the spell's effective level" so the duration formula
		// produces a useful number for sanity-checking modifiers.
		casterLevel := char.Level
		if casterLevel <= 1 {
			casterLevel = spellLevel
		}
		if casterLevel < 1 {
			casterLevel = 60
		}
		baseTicks := spelltimer.CalcDurationTicks(sp.BuffDurationFormula, sp.BuffDuration, casterLevel)
		resolution := buffmod.Resolve(
			sp.ID, sp.Name, spellLevel, casterLevel,
			baseTicks*6, // ticks → seconds
			spellType, sp.EffectIDs[:],
			res.Contributors,
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
	HP        int `json:"hp"`
	Mana      int `json:"mana"`
	AC        int `json:"ac"`
	STR       int `json:"str"`
	STA       int `json:"sta"`
	AGI       int `json:"agi"`
	DEX       int `json:"dex"`
	WIS       int `json:"wis"`
	INT       int `json:"int"`
	CHA       int `json:"cha"`
	PR        int `json:"pr"`
	MR        int `json:"mr"`
	DR        int `json:"dr"`
	FR        int `json:"fr"`
	CR        int `json:"cr"`
	Attack    int `json:"attack"`
	Haste     int `json:"haste"`
	Regen     int `json:"regen"`
	ManaRegen int `json:"mana_regen"`
	FT        int `json:"ft"`
	DmgShield int `json:"dmg_shield"`
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
	spaHitpoints   = 0  // base/tick = HP regen on a worn buff
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
func parseWornEffect(s *db.Spell, out *statBlock) (haste int) {
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
			// Convert from EQEmu's "100 + percent" encoding back to a raw
			// percent. Reported separately so the caller can max-stack.
			if base > 100 {
				h := base - 100
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
	q, err := zeal.ParseQuarmy(zeal.QuarmyPath(cfg.EQPath, char.Name), char.Name)
	if err == nil && q != nil {
		// Worn haste doesn't stack — only the highest-haste item applies.
		// Track the running max separately and assign at the end.
		bestHaste := 0
		for _, entry := range q.Inventory {
			if !equipSlots[entry.Location] || entry.ID <= 0 {
				continue
			}
			item, err := h.db.GetItem(entry.ID)
			if err != nil || item == nil {
				continue
			}
			resp.Equipment.HP += item.HP
			resp.Equipment.Mana += item.Mana
			resp.Equipment.AC += item.AC
			resp.Equipment.STR += item.Strength
			resp.Equipment.STA += item.Stamina
			resp.Equipment.AGI += item.Agility
			resp.Equipment.DEX += item.Dexterity
			resp.Equipment.WIS += item.Wisdom
			resp.Equipment.INT += item.Intelligence
			resp.Equipment.CHA += item.Charisma
			resp.Equipment.PR += item.PoisonResist
			resp.Equipment.MR += item.MagicResist
			resp.Equipment.DR += item.DiseaseResist
			resp.Equipment.FR += item.FireResist
			resp.Equipment.CR += item.ColdResist

			if item.WornEffect > 0 {
				worn, err := h.db.GetSpell(item.WornEffect)
				if err == nil && worn != nil {
					if h := parseWornEffect(worn, &resp.Equipment); h > bestHaste {
						bestHaste = h
					}
				}
			}
		}
		resp.Equipment.Haste = bestHaste

		// Item Flowing Thought is capped at 15 per EQ's worn-mana-regen rule.
		if resp.Equipment.FT > itemFTCap {
			resp.Equipment.FT = itemFTCap
		}
	}

	writeJSON(w, http.StatusOK, resp)
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

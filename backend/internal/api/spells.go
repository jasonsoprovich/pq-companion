package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/shoproute"
)

// maxStatDeltaIDs caps a single batch request — covers a full active-buff
// list (13 slots) plus headroom for raid-buff preset queries.
const maxStatDeltaIDs = 200

// maxShoppingIDs caps a shopping-route request. A class's full missing-spell
// list is at most a couple hundred entries; this leaves comfortable headroom.
const maxShoppingIDs = 500

// GET /api/spells/class/{classIndex}
// Returns all spells castable by the given class (0=Warrior … 14=Beastlord),
// ordered by that class's required level. Supports ?limit= and ?offset= for
// pagination; limit defaults to 500 and is capped at 1000.
func (h *spellsHandler) byClass(w http.ResponseWriter, r *http.Request) {
	classIndex, err := strconv.Atoi(chi.URLParam(r, "classIndex"))
	if err != nil || classIndex < 0 || classIndex > 14 {
		writeError(w, http.StatusBadRequest, "invalid class index: must be 0–14")
		return
	}
	limit := queryInt(r, "limit", 500)
	if limit > 1000 {
		limit = 1000
	}
	offset := queryInt(r, "offset", 0)
	result, err := h.db.GetSpellsByClass(classIndex, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type spellsHandler struct{ db *db.DB }

func (h *spellsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	spell, err := h.db.GetSpell(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spell not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, spell)
}

func (h *spellsHandler) crossRefs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	refs, err := h.db.GetSpellCrossRefs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

// spellStatDeltaEntry is one row of the /api/spells/stat-deltas response —
// the spell's name and icon plus its computed buff stat contribution. Name
// and icon are bundled so the raid-buff / live-buff UIs don't need a second
// round-trip to render labels.
type spellStatDeltaEntry struct {
	Name  string           `json:"name"`
	Icon  int              `json:"icon"`
	Delta db.BuffStatDelta `json:"delta"`
}

// POST /api/spells/stat-deltas
// Body: { "ids": [123, 456, ...] }
// Returns: { "123": { name, icon, delta }, ... }
//
// IDs that don't resolve to a spell are silently omitted from the response.
// Used by the character stats page to compute aggregate buff contributions
// from active or preset buff lists.
func (h *spellsHandler) statDeltas(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.IDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]spellStatDeltaEntry{})
		return
	}
	if len(body.IDs) > maxStatDeltaIDs {
		writeError(w, http.StatusBadRequest, "too many ids")
		return
	}
	out := make(map[string]spellStatDeltaEntry, len(body.IDs))
	for _, id := range body.IDs {
		sp, err := h.db.GetSpell(id)
		if err != nil || sp == nil {
			continue
		}
		icon := sp.NewIcon
		if icon == 0 {
			icon = sp.Icon
		}
		out[strconv.Itoa(id)] = spellStatDeltaEntry{
			Name:  sp.Name,
			Icon:  icon,
			Delta: db.ComputeBuffStatDelta(sp),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// shoppingSpell is a spell id+name pair used throughout the route response.
type shoppingSpell struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// shoppingVendor is a vendor at a stop and the spells (from the list) it sells.
type shoppingVendor struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	SpellIDs []int   `json:"spell_ids"`
}

// shoppingStop is one zone in the itinerary.
type shoppingStop struct {
	ZoneShort string           `json:"zone_short"`
	ZoneName  string           `json:"zone_name"`
	Reason    string           `json:"reason"` // "anchor" or "greedy"
	Spells    []shoppingSpell  `json:"spells"`
	Vendors   []shoppingVendor `json:"vendors"`
}

// shoppingRoute is the full POST /api/spells/shopping-route response.
type shoppingRoute struct {
	Stops       []shoppingStop  `json:"stops"`
	Unavailable []shoppingSpell `json:"unavailable"` // no vendor sells these
	TotalZones  int             `json:"total_zones"`
	TotalSpells int             `json:"total_spells"` // spells successfully routed
}

// POST /api/spells/shopping-route
// Body: { "spell_ids": [123, 456, ...] }
//
// Resolves each spell to the zones where its scroll is sold, runs the greedy
// set-cover optimizer (internal/shoproute), and returns an ordered itinerary
// of zones to visit plus the spells/vendors at each, and any spells no vendor
// carries. Used by the spell-checklist "plan shopping route" action.
func (h *spellsHandler) shoppingRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpellIDs []int `json:"spell_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.SpellIDs) == 0 {
		writeJSON(w, http.StatusOK, shoppingRoute{})
		return
	}
	if len(body.SpellIDs) > maxShoppingIDs {
		writeError(w, http.StatusBadRequest, "too many spell ids")
		return
	}

	opts, err := h.db.GetSpellVendorOptions(body.SpellIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Index the options for assembly. zonesPerSpell feeds the solver; the rest
	// are lookups for enriching the result.
	zonesPerSpell := make(map[int]map[string]bool)
	spellName := make(map[int]string)
	zoneName := make(map[string]string)
	// byZone[zoneShort] -> list of (spell, vendor) options in that zone.
	byZone := make(map[string][]db.SpellVendorOption)
	for _, o := range opts {
		if zonesPerSpell[o.SpellID] == nil {
			zonesPerSpell[o.SpellID] = make(map[string]bool)
		}
		zonesPerSpell[o.SpellID][o.ZoneShort] = true
		spellName[o.SpellID] = o.SpellName
		zoneName[o.ZoneShort] = o.ZoneName
		byZone[o.ZoneShort] = append(byZone[o.ZoneShort], o)
	}

	// Build the solver input over the originally requested spells so that
	// spells with no vendor option surface as unavailable.
	input := make([]shoproute.SpellAvail, 0, len(body.SpellIDs))
	seen := make(map[int]bool, len(body.SpellIDs))
	for _, id := range body.SpellIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		input = append(input, shoproute.SpellAvail{
			SpellID: id, Zones: zonesPerSpell[id],
		})
	}

	plan := shoproute.Solve(input)

	resp := shoppingRoute{
		Stops:       make([]shoppingStop, 0, len(plan.Stops)),
		Unavailable: []shoppingSpell{},
		TotalZones:  len(plan.Stops),
	}
	for _, st := range plan.Stops {
		covered := make(map[int]bool, len(st.SpellIDs))
		spells := make([]shoppingSpell, 0, len(st.SpellIDs))
		for _, id := range st.SpellIDs {
			covered[id] = true
			spells = append(spells, shoppingSpell{ID: id, Name: spellName[id]})
		}
		resp.TotalSpells += len(spells)

		// Collect the vendors in this zone that carry a covered spell, with
		// the covered spell ids each one sells.
		vendorIdx := make(map[int]*shoppingVendor)
		var vendorOrder []int
		for _, o := range byZone[st.Zone] {
			if !covered[o.SpellID] {
				continue
			}
			v := vendorIdx[o.VendorID]
			if v == nil {
				v = &shoppingVendor{
					ID: o.VendorID, Name: o.VendorName, X: o.X, Y: o.Y,
				}
				vendorIdx[o.VendorID] = v
				vendorOrder = append(vendorOrder, o.VendorID)
			}
			v.SpellIDs = append(v.SpellIDs, o.SpellID)
		}
		vendors := make([]shoppingVendor, 0, len(vendorOrder))
		for _, vid := range vendorOrder {
			v := vendorIdx[vid]
			sort.Ints(v.SpellIDs)
			vendors = append(vendors, *v)
		}
		sort.Slice(vendors, func(i, j int) bool { return vendors[i].Name < vendors[j].Name })

		resp.Stops = append(resp.Stops, shoppingStop{
			ZoneShort: st.Zone,
			ZoneName:  zoneName[st.Zone],
			Reason:    string(st.Reason),
			Spells:    spells,
			Vendors:   vendors,
		})
	}

	// Name the unavailable spells (they have no vendor row, so look them up).
	for _, id := range plan.Uncovered {
		name := ""
		if sp, err := h.db.GetSpell(id); err == nil && sp != nil {
			name = sp.Name
		}
		resp.Unavailable = append(resp.Unavailable, shoppingSpell{ID: id, Name: name})
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *spellsHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit > 1000 {
		limit = 1000
	}
	classIndex := queryInt(r, "class", -1)
	minLevel := queryInt(r, "minLevel", 0)
	maxLevel := queryInt(r, "maxLevel", 0)
	goodEffectOnly := r.URL.Query().Get("goodEffect") == "1"
	result, err := h.db.SearchSpells(q, classIndex, minLevel, maxLevel, limit, offset, goodEffectOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

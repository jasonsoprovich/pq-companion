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
	Reason    string           `json:"reason"`    // "anchor" or "greedy"
	Alignment string           `json:"alignment"` // "good", "neutral", or "evil"
	Spells    []shoppingSpell  `json:"spells"`
	Vendors   []shoppingVendor `json:"vendors"`
}

// shoppingRoute is the full POST /api/spells/shopping-route response.
type shoppingRoute struct {
	Stops               []shoppingStop  `json:"stops"`
	Unavailable         []shoppingSpell `json:"unavailable"`           // no vendor sells these anywhere
	ExcludedByAlignment []shoppingSpell `json:"excluded_by_alignment"` // only sold in filtered-out towns
	ExcludedByExpansion []shoppingSpell `json:"excluded_by_expansion"` // only sold in a not-yet-released zone (Plane of Knowledge)
	TotalZones          int             `json:"total_zones"`
	TotalSpells         int             `json:"total_spells"` // spells successfully routed
}

// POST /api/spells/shopping-route
//
//	Body: {
//	  "spell_ids": [123, 456, ...],
//	  "exclude_alignments": ["evil"],   // optional: drop good/neutral/evil towns
//	  "start_zone": "poknowledge"        // optional: order stops from here
//	}
//
// Resolves each spell to the zones where its scroll is sold, optionally filters
// out towns by alignment, runs the greedy set-cover optimizer
// (internal/shoproute), optionally orders the stops from a starting zone, and
// returns the itinerary plus the spells/vendors at each. Spells no vendor
// carries are reported in unavailable; spells whose only vendors were filtered
// out by the alignment choice are reported separately in excluded_by_alignment.
// Used by the spell-checklist "plan shopping route" action.
func (h *spellsHandler) shoppingRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpellIDs          []int    `json:"spell_ids"`
		ExcludeAlignments []string `json:"exclude_alignments"`
		StartZone         string   `json:"start_zone"`
		// IncludePoK opts Plane of Knowledge back in as a source. It's false by
		// default because the Planes of Power book hub isn't on this server's
		// timeline yet, so routing players there would be wrong.
		IncludePoK bool `json:"include_pok"`
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

	excludedAlignment := make(map[string]bool, len(body.ExcludeAlignments))
	for _, a := range body.ExcludeAlignments {
		excludedAlignment[a] = true
	}

	opts, err := h.db.GetSpellVendorOptions(body.SpellIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Zones dropped as sources because their expansion isn't live on this
	// server yet. Plane of Knowledge (the Planes of Power book hub) sells a huge
	// slice of the spell list, so leaving it in would route everyone there;
	// it's opt-in via include_pok.
	excludedZone := map[string]bool{}
	if !body.IncludePoK {
		excludedZone["poknowledge"] = true
	}

	// Index the options for assembly. allowedZones feeds the solver (after the
	// alignment and expansion filters); the rest are lookups for enriching the
	// result. anyVendorZone and alignmentOK record availability at each filter
	// stage so an uncovered spell can be bucketed: no vendor at all, only in a
	// filtered-out town, or only in a not-yet-released zone.
	allowedZones := make(map[int]map[string]bool)
	anyVendorZone := make(map[int]bool)
	alignmentOK := make(map[int]bool)
	spellName := make(map[int]string)
	zoneName := make(map[string]string)
	// byZone[zoneShort] -> list of (spell, vendor) options in that zone.
	byZone := make(map[string][]db.SpellVendorOption)
	for _, o := range opts {
		spellName[o.SpellID] = o.SpellName
		anyVendorZone[o.SpellID] = true
		if excludedAlignment[shoproute.Alignment(o.ZoneShort)] {
			continue
		}
		alignmentOK[o.SpellID] = true
		if excludedZone[o.ZoneShort] {
			continue
		}
		if allowedZones[o.SpellID] == nil {
			allowedZones[o.SpellID] = make(map[string]bool)
		}
		allowedZones[o.SpellID][o.ZoneShort] = true
		zoneName[o.ZoneShort] = o.ZoneName
		byZone[o.ZoneShort] = append(byZone[o.ZoneShort], o)
	}

	// Build the solver input over the originally requested spells (deduped).
	input := make([]shoproute.SpellAvail, 0, len(body.SpellIDs))
	seen := make(map[int]bool, len(body.SpellIDs))
	for _, id := range body.SpellIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		input = append(input, shoproute.SpellAvail{
			SpellID: id, Zones: allowedZones[id],
		})
	}

	// With a start zone, fetch the connectivity graph once and derive hop
	// distances so the solver prefers nearer sources (and so we can order the
	// stops afterwards). Excluded zones are pruned from the graph too, so the
	// route never paths through, say, Plane of Knowledge while it's disabled.
	var dist map[string]int
	var adj map[string][]string
	if body.StartZone != "" {
		adj, err = h.db.GetZoneAdjacency()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(excludedZone) > 0 {
			adj = pruneAdjacency(adj, excludedZone)
		}
		dist = shoproute.Distances(body.StartZone, adj)
	}

	plan := shoproute.Solve(input, dist)

	// Order the stops from the starting zone, if one was given.
	if body.StartZone != "" && len(plan.Stops) > 1 {
		plan.Stops = shoproute.Order(plan.Stops, body.StartZone, adj)
	}

	resp := shoppingRoute{
		Stops:               make([]shoppingStop, 0, len(plan.Stops)),
		Unavailable:         []shoppingSpell{},
		ExcludedByAlignment: []shoppingSpell{},
		ExcludedByExpansion: []shoppingSpell{},
		TotalZones:          len(plan.Stops),
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
			Alignment: shoproute.Alignment(st.Zone),
			Spells:    spells,
			Vendors:   vendors,
		})
	}

	// Split uncovered spells into why-we-couldn't-route-them buckets, in order
	// of precedence: no vendor anywhere; only sold in a filtered-out town; or
	// (alignment was fine) only sold in a disabled zone like Plane of Knowledge.
	// Names come from the vendor rows when available, else a direct lookup.
	for _, id := range plan.Uncovered {
		name := spellName[id]
		if name == "" {
			if sp, err := h.db.GetSpell(id); err == nil && sp != nil {
				name = sp.Name
			}
		}
		entry := shoppingSpell{ID: id, Name: name}
		switch {
		case !anyVendorZone[id]:
			resp.Unavailable = append(resp.Unavailable, entry)
		case !alignmentOK[id]:
			resp.ExcludedByAlignment = append(resp.ExcludedByAlignment, entry)
		default:
			resp.ExcludedByExpansion = append(resp.ExcludedByExpansion, entry)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// pruneAdjacency returns a copy of the zone-connectivity graph with the dropped
// zones removed as both nodes and neighbours, so distance and ordering never
// traverse a disabled zone (e.g. Plane of Knowledge while it's off).
func pruneAdjacency(adj map[string][]string, drop map[string]bool) map[string][]string {
	out := make(map[string][]string, len(adj))
	for z, neighbors := range adj {
		if drop[z] {
			continue
		}
		kept := make([]string, 0, len(neighbors))
		for _, n := range neighbors {
			if !drop[n] {
				kept = append(kept, n)
			}
		}
		out[z] = kept
	}
	return out
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

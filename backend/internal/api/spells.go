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

// teleportHub is the zone Druid/Wizard teleport destinations are linked to in
// the travel graph. Most Quarm players bind in the Nexus and can readily catch
// a port there, so portable zones count as one hop from it.
const teleportHub = "nexus"

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
// shoppingZone is a candidate source town offered for exclusion: a zone that
// sells at least one selected spell (after alignment/expansion filtering).
type shoppingZone struct {
	ZoneShort  string `json:"zone_short"`
	ZoneName   string `json:"zone_name"`
	Alignment  string `json:"alignment"`
	SpellCount int    `json:"spell_count"` // selected spells this town can supply
}

type shoppingRoute struct {
	Stops               []shoppingStop  `json:"stops"`
	Unavailable         []shoppingSpell `json:"unavailable"`           // no vendor sells these anywhere
	ExcludedByAlignment []shoppingSpell `json:"excluded_by_alignment"` // only sold in filtered-out towns
	ExcludedByExpansion []shoppingSpell `json:"excluded_by_expansion"` // only sold in a not-yet-released zone (Plane of Knowledge)
	ExcludedByZone      []shoppingSpell `json:"excluded_by_zone"`      // only sold in towns the player excluded
	CandidateZones      []shoppingZone  `json:"candidate_zones"`       // every source town, for the exclusion picker
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
		// ExcludeZones are zone short_names the player never wants routed
		// through (faction, preference). Their spells re-route to the next-best
		// town; a spell sold *only* in excluded towns is reported separately.
		ExcludeZones []string `json:"exclude_zones"`
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

	// Plane of Knowledge is dropped as a source because its expansion isn't live
	// on this server yet (the PoP book hub sells a huge slice of the spell list,
	// so leaving it in would route everyone there); it's opt-in via include_pok.
	pokExcluded := !body.IncludePoK

	// Zones the player chose to skip. Their spells re-route elsewhere.
	userExcluded := make(map[string]bool, len(body.ExcludeZones))
	for _, z := range body.ExcludeZones {
		userExcluded[z] = true
	}

	// Index the options for assembly. allowedZones feeds the solver (after every
	// filter); the rest are lookups for enriching the result. The per-stage
	// availability flags let an uncovered spell be bucketed precisely: no vendor
	// at all, only in a wrong-alignment town, only in a disabled zone (PoK), or
	// only in a town the player excluded.
	allowedZones := make(map[int]map[string]bool)
	anyVendorZone := make(map[int]bool)
	alignmentOK := make(map[int]bool)
	expansionOK := make(map[int]bool)
	spellName := make(map[int]string)
	zoneName := make(map[string]string)
	// byZone[zoneShort] -> list of (spell, vendor) options in that zone.
	byZone := make(map[string][]db.SpellVendorOption)
	// candidateSpells[zoneShort] -> set of selected spells that zone could sell,
	// before the player's own exclusions — the universe offered for exclusion.
	candidateSpells := make(map[string]map[int]bool)
	for _, o := range opts {
		spellName[o.SpellID] = o.SpellName
		anyVendorZone[o.SpellID] = true
		if excludedAlignment[shoproute.Alignment(o.ZoneShort)] {
			continue
		}
		alignmentOK[o.SpellID] = true
		if pokExcluded && o.ZoneShort == "poknowledge" {
			continue
		}
		expansionOK[o.SpellID] = true
		zoneName[o.ZoneShort] = o.ZoneName
		if candidateSpells[o.ZoneShort] == nil {
			candidateSpells[o.ZoneShort] = make(map[int]bool)
		}
		candidateSpells[o.ZoneShort][o.SpellID] = true
		if userExcluded[o.ZoneShort] {
			continue
		}
		if allowedZones[o.SpellID] == nil {
			allowedZones[o.SpellID] = make(map[string]bool)
		}
		allowedZones[o.SpellID][o.ZoneShort] = true
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
	// stops afterwards). We layer Druid/Wizard teleport destinations onto the
	// graph as cheap edges from the Nexus (the bind/port hub), then prune
	// excluded zones so the route never paths through, say, Plane of Knowledge
	// while it's disabled.
	var dist map[string]int
	var adj map[string][]string
	if body.StartZone != "" {
		adj, err = h.db.GetZoneAdjacency()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Teleport links are additive; if the lookup fails, fall back to plain
		// zone-line travel rather than failing the whole route.
		if dests, derr := h.db.GetTeleportDestinations(); derr == nil {
			adj = shoproute.LinkHub(adj, teleportHub, dests)
		}
		// Don't path through zones we won't shop in (disabled PoK, or towns the
		// player excluded).
		graphExclude := make(map[string]bool, len(userExcluded)+1)
		for z := range userExcluded {
			graphExclude[z] = true
		}
		if pokExcluded {
			graphExclude["poknowledge"] = true
		}
		if len(graphExclude) > 0 {
			adj = pruneAdjacency(adj, graphExclude)
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
		ExcludedByZone:      []shoppingSpell{},
		CandidateZones:      []shoppingZone{},
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
	// of precedence: no vendor anywhere; only in a wrong-alignment town; only in
	// a disabled zone (PoK); otherwise only in a town the player excluded.
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
		case !expansionOK[id]:
			resp.ExcludedByExpansion = append(resp.ExcludedByExpansion, entry)
		default:
			resp.ExcludedByZone = append(resp.ExcludedByZone, entry)
		}
	}

	// Every source town that could supply a selected spell (post alignment/
	// expansion, pre player-exclusion) — the universe the UI offers for the
	// "skip these towns" picker. Sorted by name for a stable list.
	for short, spells := range candidateSpells {
		resp.CandidateZones = append(resp.CandidateZones, shoppingZone{
			ZoneShort:  short,
			ZoneName:   zoneName[short],
			Alignment:  shoproute.Alignment(short),
			SpellCount: len(spells),
		})
	}
	sort.Slice(resp.CandidateZones, func(i, j int) bool {
		a, b := resp.CandidateZones[i], resp.CandidateZones[j]
		if a.ZoneName != b.ZoneName {
			return a.ZoneName < b.ZoneName
		}
		return a.ZoneShort < b.ZoneShort
	})

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

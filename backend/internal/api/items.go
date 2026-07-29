package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/shoproute"
)

// maxItemShoppingIDs caps an item shopping-route request. A recipe's
// component list tops out well under this (the largest known combines are a
// few dozen lines); the headroom matches maxShoppingIDs in spells.go.
const maxItemShoppingIDs = 500

type itemsHandler struct {
	db     *db.DB
	cfgMgr *config.Manager
}

func (h *itemsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.db.GetItem(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *itemsHandler) sources(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sources, err := h.db.GetItemSources(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *itemsHandler) quests(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	quests, err := h.db.GetItemQuests(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quests)
}

// shoppingItem is an item id+name pair used throughout the route response.
type shoppingItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// itemShoppingVendor is a vendor at a stop and the items (from the list) it
// sells.
type itemShoppingVendor struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	ItemIDs []int   `json:"item_ids"`
}

// itemShoppingStop is one zone in the itinerary.
type itemShoppingStop struct {
	ZoneShort string               `json:"zone_short"`
	ZoneName  string               `json:"zone_name"`
	Reason    string               `json:"reason"` // "anchor" or "greedy"
	Alignment string               `json:"alignment"`
	Items     []shoppingItem       `json:"items"`
	Vendors   []itemShoppingVendor `json:"vendors"`
}

// itemShoppingZone is a candidate source town offered for exclusion: a zone
// that sells at least one selected item (after alignment/expansion filtering).
type itemShoppingZone struct {
	ZoneShort string `json:"zone_short"`
	ZoneName  string `json:"zone_name"`
	Alignment string `json:"alignment"`
	ItemCount int    `json:"item_count"`
}

// itemShoppingRoute is the full POST /api/items/shopping-route response.
type itemShoppingRoute struct {
	Stops               []itemShoppingStop `json:"stops"`
	Unavailable         []shoppingItem     `json:"unavailable"`           // no vendor sells these anywhere
	ExcludedByAlignment []shoppingItem     `json:"excluded_by_alignment"` // only sold in filtered-out towns
	ExcludedByExpansion []shoppingItem     `json:"excluded_by_expansion"` // only sold in a not-yet-released zone (Plane of Knowledge)
	ExcludedByZone      []shoppingItem     `json:"excluded_by_zone"`      // only sold in towns the player excluded
	CandidateZones      []itemShoppingZone `json:"candidate_zones"`       // every source town, for the exclusion picker
	TotalZones          int                `json:"total_zones"`
	TotalItems          int                `json:"total_items"` // items successfully routed
}

// POST /api/items/shopping-route
//
//	Body: {
//	  "item_ids": [123, 456, ...],
//	  "exclude_alignments": ["evil"],
//	  "start_zone": "shadowhaven",
//	  "include_pok": false,
//	  "exclude_zones": []
//	}
//
// Same solver and filtering rules as spellsHandler.shoppingRoute (see that
// doc comment) applied to items instead of spell scrolls — item ids resolve
// to vendor/zone pairs via GetItemVendorOptions instead of the scroll-effect
// join. Used by the recipe view's "find vendors for these components" action.
//
// Plane of Knowledge stays era-gated exactly like the spell route: it's only
// offered as a source when the player has opted in (include_pok) or the PoP
// preference flag is on. A recipe's components genuinely aren't purchasable
// there on a pre-PoP server, so pinning it as an always-on source regardless
// of era would route players to a vendor that doesn't exist yet on their
// timeline.
func (h *itemsHandler) shoppingRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemIDs           []int    `json:"item_ids"`
		ExcludeAlignments []string `json:"exclude_alignments"`
		StartZone         string   `json:"start_zone"`
		IncludePoK        bool     `json:"include_pok"`
		ExcludeZones      []string `json:"exclude_zones"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.ItemIDs) == 0 {
		writeJSON(w, http.StatusOK, itemShoppingRoute{})
		return
	}
	if len(body.ItemIDs) > maxItemShoppingIDs {
		writeError(w, http.StatusBadRequest, "too many item ids")
		return
	}

	excludedAlignment := make(map[string]bool, len(body.ExcludeAlignments))
	for _, a := range body.ExcludeAlignments {
		excludedAlignment[a] = true
	}

	opts, err := h.db.GetItemVendorOptions(body.ItemIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pokExcluded := !body.IncludePoK && !h.cfgMgr.Get().Preferences.PoPEnabled

	userExcluded := make(map[string]bool, len(body.ExcludeZones))
	for _, z := range body.ExcludeZones {
		userExcluded[z] = true
	}

	allowedZones := make(map[int]map[string]bool)
	anyVendorZone := make(map[int]bool)
	alignmentOK := make(map[int]bool)
	expansionOK := make(map[int]bool)
	itemName := make(map[int]string)
	zoneName := make(map[string]string)
	byZone := make(map[string][]db.ItemVendorOption)
	candidateItems := make(map[string]map[int]bool)
	for _, o := range opts {
		itemName[o.ItemID] = o.ItemName
		anyVendorZone[o.ItemID] = true
		if excludedAlignment[shoproute.Alignment(o.ZoneShort)] {
			continue
		}
		alignmentOK[o.ItemID] = true
		if pokExcluded && o.ZoneShort == "poknowledge" {
			continue
		}
		expansionOK[o.ItemID] = true
		zoneName[o.ZoneShort] = o.ZoneName
		if candidateItems[o.ZoneShort] == nil {
			candidateItems[o.ZoneShort] = make(map[int]bool)
		}
		candidateItems[o.ZoneShort][o.ItemID] = true
		if userExcluded[o.ZoneShort] {
			continue
		}
		if allowedZones[o.ItemID] == nil {
			allowedZones[o.ItemID] = make(map[string]bool)
		}
		allowedZones[o.ItemID][o.ZoneShort] = true
		byZone[o.ZoneShort] = append(byZone[o.ZoneShort], o)
	}

	// shoproute.SpellAvail is generic over any purchasable id — it's named for
	// its original use (spell scrolls) but the solver only ever looks at the
	// id and its zone set, so it's reused as-is here rather than duplicated
	// under a new name.
	input := make([]shoproute.SpellAvail, 0, len(body.ItemIDs))
	seen := make(map[int]bool, len(body.ItemIDs))
	for _, id := range body.ItemIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		input = append(input, shoproute.SpellAvail{
			SpellID: id, Zones: allowedZones[id],
		})
	}

	var dist map[string]int
	var adj map[string][]string
	if body.StartZone != "" {
		adj, err = h.db.GetZoneAdjacency()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if dests, derr := h.db.GetTeleportDestinations(); derr == nil {
			adj = shoproute.LinkHub(adj, teleportHub, dests)
		}
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

	if body.StartZone != "" && len(plan.Stops) > 1 {
		plan.Stops = shoproute.Order(plan.Stops, body.StartZone, adj)
	}

	resp := itemShoppingRoute{
		Stops:               make([]itemShoppingStop, 0, len(plan.Stops)),
		Unavailable:         []shoppingItem{},
		ExcludedByAlignment: []shoppingItem{},
		ExcludedByExpansion: []shoppingItem{},
		ExcludedByZone:      []shoppingItem{},
		CandidateZones:      []itemShoppingZone{},
		TotalZones:          len(plan.Stops),
	}
	for _, st := range plan.Stops {
		covered := make(map[int]bool, len(st.SpellIDs))
		items := make([]shoppingItem, 0, len(st.SpellIDs))
		for _, id := range st.SpellIDs {
			covered[id] = true
			items = append(items, shoppingItem{ID: id, Name: itemName[id]})
		}
		resp.TotalItems += len(items)

		vendorIdx := make(map[int]*itemShoppingVendor)
		var vendorOrder []int
		for _, o := range byZone[st.Zone] {
			if !covered[o.ItemID] {
				continue
			}
			v := vendorIdx[o.VendorID]
			if v == nil {
				v = &itemShoppingVendor{
					ID: o.VendorID, Name: o.VendorName, X: o.X, Y: o.Y,
				}
				vendorIdx[o.VendorID] = v
				vendorOrder = append(vendorOrder, o.VendorID)
			}
			v.ItemIDs = append(v.ItemIDs, o.ItemID)
		}
		vendors := make([]itemShoppingVendor, 0, len(vendorOrder))
		for _, vid := range vendorOrder {
			v := vendorIdx[vid]
			sort.Ints(v.ItemIDs)
			vendors = append(vendors, *v)
		}
		sort.Slice(vendors, func(i, j int) bool { return vendors[i].Name < vendors[j].Name })

		resp.Stops = append(resp.Stops, itemShoppingStop{
			ZoneShort: st.Zone,
			ZoneName:  zoneName[st.Zone],
			Reason:    string(st.Reason),
			Alignment: shoproute.Alignment(st.Zone),
			Items:     items,
			Vendors:   vendors,
		})
	}

	for _, id := range plan.Uncovered {
		name := itemName[id]
		if name == "" {
			if it, err := h.db.GetItem(id); err == nil && it != nil {
				name = it.Name
			}
		}
		entry := shoppingItem{ID: id, Name: name}
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

	for short, items := range candidateItems {
		resp.CandidateZones = append(resp.CandidateZones, itemShoppingZone{
			ZoneShort: short,
			ZoneName:  zoneName[short],
			Alignment: shoproute.Alignment(short),
			ItemCount: len(items),
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

func (h *itemsHandler) search(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	f := db.ItemFilter{
		Query:    r.URL.Query().Get("q"),
		BaneBody: queryInt(r, "bane_body", 0),
		Race:     queryInt(r, "race", 0),
		Class:    queryInt(r, "class", 0),
		MinLevel: queryInt(r, "min_level", 0),
		MaxLevel: queryInt(r, "max_level", 0),
		Slot:     queryInt(r, "slot", 0),
		ItemType: queryInt(r, "item_type", -1),
		MinSTR:   queryInt(r, "min_str", 0),
		MinSTA:   queryInt(r, "min_sta", 0),
		MinAGI:   queryInt(r, "min_agi", 0),
		MinDEX:   queryInt(r, "min_dex", 0),
		MinWIS:   queryInt(r, "min_wis", 0),
		MinINT:   queryInt(r, "min_int", 0),
		MinCHA:   queryInt(r, "min_cha", 0),
		MinHP:    queryInt(r, "min_hp", 0),
		MinMana:  queryInt(r, "min_mana", 0),
		MinAC:    queryInt(r, "min_ac", 0),
		MinMR:    queryInt(r, "min_mr", 0),
		MinCR:    queryInt(r, "min_cr", 0),
		MinDR:    queryInt(r, "min_dr", 0),
		MinFR:    queryInt(r, "min_fr", 0),
		MinPR:    queryInt(r, "min_pr", 0),
		Limit:    limit,
		Offset:   queryInt(r, "offset", 0),
	}
	result, err := h.db.SearchItems(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

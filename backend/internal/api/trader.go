package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trader"
)

// traderHandler powers the (developer-tab) Bazaar Trader Tracker: it lists
// trader characters (those with a BZR price file), their current satchel
// listings, captured snapshot history, and the sale sessions inferred by
// diffing successive snapshots. The inference math lives in internal/trader.
type traderHandler struct {
	store    *trader.Store
	capturer *trader.Capturer
	cfgMgr   *config.Manager
	db       *db.DB
}

// traderCharacter summarizes one trader character for the picker.
type traderCharacter struct {
	Name          string `json:"name"`
	HasBZR        bool   `json:"has_bzr"`
	ListingCount  int    `json:"listing_count"`  // priced entries in the BZR file
	ForSaleCount  int    `json:"for_sale_count"` // listings with price > 0
	SnapshotCount int    `json:"snapshot_count"`
	LastCaptured  int64  `json:"last_captured,omitempty"` // unix seconds, 0 if none
}

// GET /api/trader/characters
// Lists every trader character (has a BZR file on disk OR captured snapshots),
// with enough summary data for the page's character picker.
func (h *traderHandler) characters(w http.ResponseWriter, r *http.Request) {
	eqPath := h.cfgMgr.Get().EQPath

	// Union of on-disk BZR characters and those with stored snapshots, so a
	// trader still shows up after the EQ path changes or the BZR file moves.
	names := map[string]bool{}
	var order []string
	add := func(n string) {
		if n == "" || names[n] {
			return
		}
		names[n] = true
		order = append(order, n)
	}

	bzrChars, err := trader.FindBZRCharacters(eqPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, n := range bzrChars {
		add(n)
	}
	if snapChars, err := h.store.CharactersWithSnapshots(); err == nil {
		for _, n := range snapChars {
			add(n)
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })

	out := make([]traderCharacter, 0, len(order))
	for _, name := range order {
		tc := traderCharacter{Name: name}
		if path := trader.FindBZRFile(eqPath, name); path != "" {
			tc.HasBZR = true
			if listing, err := trader.ParseBZR(path, name); err == nil {
				tc.ListingCount = len(listing.Items)
				for _, it := range listing.Items {
					if it.Price > 0 {
						tc.ForSaleCount++
					}
				}
			}
		}
		if cnt, err := h.store.SnapshotCount(name); err == nil {
			tc.SnapshotCount = cnt
		}
		if latest, ok, err := h.store.LatestSnapshot(name); err == nil && ok {
			tc.LastCaptured = latest.TakenAt.Unix()
		}
		out = append(out, tc)
	}
	writeJSON(w, http.StatusOK, out)
}

// traderListing is one BZR-priced item joined with whether it's currently in a
// Trader's Satchel (from the latest snapshot).
type traderListing struct {
	Name      string `json:"name"`
	Price     int64  `json:"price"`
	InSatchel int    `json:"in_satchel"` // count currently stocked, 0 if not
	ItemID    int    `json:"item_id,omitempty"`
	Icon      int    `json:"icon,omitempty"`
}

// GET /api/trader/{char}/listings
// Returns the trader's BZR price list, annotated with how many of each item are
// currently sitting in a Trader's Satchel (from the most recent snapshot).
func (h *traderHandler) listings(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	eqPath := h.cfgMgr.Get().EQPath

	path := trader.FindBZRFile(eqPath, char)
	if path == "" {
		writeJSON(w, http.StatusOK, []traderListing{})
		return
	}
	listing, err := trader.ParseBZR(path, char)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Current satchel stock by item name (from the latest snapshot).
	stock := map[string]int{}
	itemIDByName := map[string]int{}
	if latest, ok, err := h.store.LatestSnapshot(char); err == nil && ok {
		for _, it := range latest.Satchel {
			stock[normName(it.Name)] += it.Count
			itemIDByName[normName(it.Name)] = it.ItemID
		}
	}

	out := make([]traderListing, 0, len(listing.Items))
	var ids []int
	for _, it := range listing.Items {
		l := traderListing{Name: it.Name, Price: it.Price}
		key := normName(it.Name)
		l.InSatchel = stock[key]
		if id, ok := itemIDByName[key]; ok {
			l.ItemID = id
			ids = append(ids, id)
		}
		out = append(out, l)
	}
	h.attachIcons(ids, func(id, icon int) {
		for i := range out {
			if out[i].ItemID == id {
				out[i].Icon = icon
			}
		}
	})
	writeJSON(w, http.StatusOK, out)
}

// GET /api/trader/{char}/sessions
// Returns the sale sessions inferred by diffing each consecutive pair of stored
// snapshots, newest session first.
func (h *traderHandler) sessions(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")

	snaps, err := h.store.ListSnapshots(char)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var listing *trader.BZRListing
	if path := trader.FindBZRFile(h.cfgMgr.Get().EQPath, char); path != "" {
		listing, _ = trader.ParseBZR(path, char)
	}

	sessions := make([]*trader.Session, 0)
	var iconIDs []int
	for i := 1; i < len(snaps); i++ {
		sess := trader.InferSales(snaps[i-1], snaps[i], listing)
		if len(sess.Sold) == 0 && len(sess.Restocked) == 0 {
			continue // no satchel movement; skip empty sessions
		}
		sessions = append(sessions, sess)
		for _, s := range sess.Sold {
			iconIDs = append(iconIDs, s.ItemID)
		}
		for _, s := range sess.Restocked {
			iconIDs = append(iconIDs, s.ItemID)
		}
	}

	icons := h.lookupIcons(iconIDs)
	for _, sess := range sessions {
		for i := range sess.Sold {
			sess.Sold[i].Icon = icons[sess.Sold[i].ItemID]
		}
		for i := range sess.Restocked {
			sess.Restocked[i].Icon = icons[sess.Restocked[i].ItemID]
		}
	}

	// Newest first.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ToTime.After(sessions[j].ToTime)
	})
	writeJSON(w, http.StatusOK, sessions)
}

// traderSnapshotInfo is a lightweight snapshot summary for the history list.
type traderSnapshotInfo struct {
	TakenAt    int64 `json:"taken_at"`
	ItemCount  int   `json:"item_count"`  // distinct satchel entries
	TotalQty   int   `json:"total_qty"`   // summed counts
	OnPerson   int64 `json:"on_person"`   // copper
	BankCopper int64 `json:"bank_copper"` // copper
}

// GET /api/trader/{char}/snapshots
// Returns the raw captured snapshot history (oldest first) for transparency.
func (h *traderHandler) snapshots(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	snaps, err := h.store.ListSnapshots(char)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]traderSnapshotInfo, 0, len(snaps))
	for _, s := range snaps {
		info := traderSnapshotInfo{
			TakenAt:    s.TakenAt.Unix(),
			ItemCount:  len(s.Satchel),
			OnPerson:   s.OnPersonCopper,
			BankCopper: s.BankCopper,
		}
		for _, it := range s.Satchel {
			info.TotalQty += it.Count
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

// captureResponse reports the outcome of a manual capture.
type captureResponse struct {
	Captured  bool   `json:"captured"` // true if a NEW snapshot was stored
	Reason    string `json:"reason,omitempty"`
	TakenAt   int64  `json:"taken_at,omitempty"`
	ItemCount int    `json:"item_count,omitempty"`
}

// POST /api/trader/{char}/capture
// Forces a snapshot capture from the character's newest export, storing it only
// if it differs from the last one. Backs the page's "Capture now" button.
func (h *traderHandler) capture(w http.ResponseWriter, r *http.Request) {
	char := chi.URLParam(r, "char")
	if h.cfgMgr.Get().EQPath == "" {
		writeJSON(w, http.StatusOK, captureResponse{Reason: "EQ directory is not configured."})
		return
	}
	snap, stored, err := h.capturer.Capture(char)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if snap == nil {
		writeJSON(w, http.StatusOK, captureResponse{
			Reason: "No inventory or quarmy export found for this character. Run /output inventory in-game first.",
		})
		return
	}
	resp := captureResponse{
		Captured:  stored,
		TakenAt:   snap.TakenAt.Unix(),
		ItemCount: len(snap.Satchel),
	}
	if !stored {
		resp.Reason = "No change since the last snapshot."
	}
	writeJSON(w, http.StatusOK, resp)
}

// normName lowercases and trims an item name for case-insensitive matching
// between BZR entries and satchel contents.
func normName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// lookupIcons returns an itemID→icon map for the given IDs (deduped), empty on
// error or when the DB is unavailable.
func (h *traderHandler) lookupIcons(ids []int) map[int]int {
	if h.db == nil || len(ids) == 0 {
		return map[int]int{}
	}
	icons, err := h.db.ItemIcons(ids)
	if err != nil {
		return map[int]int{}
	}
	return icons
}

// attachIcons looks up icons for ids and invokes set(id, icon) for each found.
func (h *traderHandler) attachIcons(ids []int, set func(id, icon int)) {
	for id, icon := range h.lookupIcons(ids) {
		set(id, icon)
	}
}

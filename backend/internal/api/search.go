package api

import (
	"net/http"
	"sync"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type searchHandler struct{ db *db.DB }

type globalSearchResult struct {
	Items  []db.Item  `json:"items"`
	Spells []db.Spell `json:"spells"`
	NPCs   []db.NPC   `json:"npcs"`
	Zones  []db.Zone  `json:"zones"`
}

func (h *searchHandler) global(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 5)
	if limit > 10 {
		limit = 10
	}

	var (
		mu     sync.Mutex
		result globalSearchResult
		errStr string
		wg     sync.WaitGroup
	)

	setErr := func(msg string) {
		mu.Lock()
		if errStr == "" {
			errStr = msg
		}
		mu.Unlock()
	}

	wg.Add(4)

	go func() {
		defer wg.Done()
		res, err := h.db.SearchItems(q, 0, limit, 0)
		if err != nil {
			setErr(err.Error())
			return
		}
		mu.Lock()
		result.Items = res.Items
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		res, err := h.db.SearchSpells(q, -1, 0, 0, limit, 0)
		if err != nil {
			setErr(err.Error())
			return
		}
		mu.Lock()
		result.Spells = res.Items
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		res, err := h.db.SearchNPCs(q, limit, 0)
		if err != nil {
			setErr(err.Error())
			return
		}
		mu.Lock()
		result.NPCs = res.Items
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		res, err := h.db.SearchZones(q, limit, 0)
		if err != nil {
			setErr(err.Error())
			return
		}
		mu.Lock()
		result.Zones = res.Items
		mu.Unlock()
	}()

	wg.Wait()

	if errStr != "" {
		writeError(w, http.StatusInternalServerError, errStr)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

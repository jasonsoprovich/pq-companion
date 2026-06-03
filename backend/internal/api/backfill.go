package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/backfill"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type backfillHandler struct {
	registry *backfill.Registry
	mgr      *config.Manager
	tailer   *logparser.Tailer
	hub      *ws.Hub
}

// info handles GET /api/backfill — the available sections plus the characters
// that have a log file (each has its own log and so its own data to backfill),
// with the active character flagged so the UI can pre-select it.
func (h *backfillHandler) info(w http.ResponseWriter, r *http.Request) {
	chars := []string{}
	for _, d := range logparser.DiscoverCharacters(h.mgr.Get().EQPath) {
		chars = append(chars, d.Name)
	}
	sort.Slice(chars, func(i, j int) bool {
		return strings.ToLower(chars[i]) < strings.ToLower(chars[j])
	})
	active := h.mgr.Get().Character
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			active = c
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sections":   h.registry.Sections(),
		"characters": chars,
		"active":     active,
	})
}

// run handles POST /api/backfill {character, sections:[]} — replays the
// character's log once and populates the selected trackers. Returns per-section
// inserted/updated counts. Each tracker dedups, so re-running is safe.
func (h *backfillHandler) run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Character string   `json:"character"`
		Sections  []string `json:"sections"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	if len(req.Sections) == 0 {
		writeError(w, http.StatusBadRequest, "no sections selected")
		return
	}
	eqPath := h.mgr.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}
	logPath := filepath.Join(eqPath, "eqlog_"+req.Character+"_pq.proj.txt")
	results, err := h.registry.Run(logPath, req.Character, req.Sections, func(done, total int64) {
		if h.hub != nil {
			h.hub.Broadcast(ws.Event{Type: "backfill:progress", Data: map[string]any{
				"character": req.Character,
				"done":      done,
				"total":     total,
			}})
		}
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results":   results,
		"character": req.Character,
	})
}

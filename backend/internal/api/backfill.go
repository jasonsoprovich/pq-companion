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

// archiveInfo summarizes the rotated-out .bak.zip / .bak.txt logs available
// for one character, so the UI can label the "scan archives too" option.
type archiveInfo struct {
	Count  int    `json:"count"`
	Bytes  int64  `json:"bytes"`  // total uncompressed size across all archives
	Oldest string `json:"oldest"` // YYYY-MM-DD of the earliest archive, or ""
}

// info handles GET /api/backfill — the available sections plus the characters
// that have a log file (each has its own log and so its own data to backfill),
// with the active character flagged so the UI can pre-select it, plus a
// per-character summary of how many archived logs Archive & Trim has left
// alongside the live one.
func (h *backfillHandler) info(w http.ResponseWriter, r *http.Request) {
	eqPath := h.mgr.Get().EQPath
	chars := []string{}
	for _, d := range logparser.DiscoverCharacters(eqPath) {
		chars = append(chars, d.Name)
	}
	sort.Slice(chars, func(i, j int) bool {
		return strings.ToLower(chars[i]) < strings.ToLower(chars[j])
	})
	archives := map[string]archiveInfo{}
	for _, c := range chars {
		found := logparser.DiscoverArchives(eqPath, c)
		if len(found) == 0 {
			continue
		}
		var bytes int64
		for _, a := range found {
			bytes += a.Bytes
		}
		archives[c] = archiveInfo{
			Count:  len(found),
			Bytes:  bytes,
			Oldest: found[0].ArchivedAt.Format("2006-01-02"),
		}
	}
	active := h.mgr.Get().Character
	if h.tailer != nil {
		if c := h.tailer.ActiveCharacter(); c != "" {
			active = c
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sections":   h.registry.Sections(),
		"characters": chars,
		"archives":   archives,
		"active":     active,
	})
}

// run handles POST /api/backfill {character, sections:[], scope} — replays
// the character's log(s) and populates the selected trackers. Returns
// per-section inserted/updated counts. scope "all" also scans every
// archived log (.bak.zip / .bak.txt) next to the live one, oldest first;
// anything else (incl. "" and "current") scans only the live log. Each
// tracker dedups, so re-running — and the archive/live overlap — is safe.
func (h *backfillHandler) run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Character string   `json:"character"`
		Sections  []string `json:"sections"`
		Scope     string   `json:"scope"`
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
	livePath := filepath.Join(eqPath, "eqlog_"+req.Character+"_pq.proj.txt")
	paths := []string{livePath}
	if req.Scope == "all" {
		archives := logparser.DiscoverArchives(eqPath, req.Character)
		paths = make([]string, 0, len(archives)+1)
		for _, a := range archives {
			paths = append(paths, a.Path) // oldest first
		}
		paths = append(paths, livePath) // live log last
	}
	results, err := h.registry.RunMulti(paths, req.Character, req.Sections, func(done, total int64) {
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

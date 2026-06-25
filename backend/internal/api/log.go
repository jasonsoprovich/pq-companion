package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type logHandler struct {
	tailer *logparser.Tailer
	mgr    *config.Manager
}

// status handles GET /api/log/status — returns the current tailer state plus
// file size information needed for the cleanup notification.
func (h *logHandler) status(w http.ResponseWriter, r *http.Request) {
	s := h.tailer.Status()

	type response struct {
		Enabled    bool   `json:"enabled"`
		FilePath   string `json:"file_path"`
		FileExists bool   `json:"file_exists"`
		Tailing    bool   `json:"tailing"`
		Offset     int64  `json:"offset"`
		SizeBytes  int64  `json:"size_bytes"`
		LargeFile  bool   `json:"large_file"`
		RawFeed    bool   `json:"raw_feed"`
	}

	var sizeBytes int64
	var largeFile bool
	if s.FileExists && s.FilePath != "" {
		if fi, err := logparser.GetFileInfo(s.FilePath); err == nil {
			sizeBytes = fi.SizeBytes
			largeFile = fi.LargeFile
		}
	}

	json.NewEncoder(w).Encode(response{
		Enabled:    s.Enabled,
		FilePath:   s.FilePath,
		FileExists: s.FileExists,
		Tailing:    s.Tailing,
		Offset:     s.Offset,
		SizeBytes:  sizeBytes,
		LargeFile:  largeFile,
		RawFeed:    h.tailer.RawFeed(),
	})
}

// rawFeed handles POST /api/log/raw-feed — toggles whether unrecognised log
// lines (chat, system messages) are broadcast to the live feed as log:raw.
// Body: {"enabled": true}. The setting lives on the tailer, so it persists
// across renderer navigation until the backend restarts.
func (h *logHandler) rawFeed(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	h.tailer.SetRawFeed(req.Enabled)
	writeJSON(w, http.StatusOK, map[string]bool{"raw_feed": req.Enabled})
}

// info handles GET /api/log/info — returns full file metadata including
// oldest/newest entry timestamps (more expensive: scans file content).
func (h *logHandler) info(w http.ResponseWriter, r *http.Request) {
	s := h.tailer.Status()
	if !s.FileExists || s.FilePath == "" {
		http.Error(w, `{"error":"log file not found"}`, http.StatusNotFound)
		return
	}

	fi, err := logparser.GetFileInfo(s.FilePath)
	if err != nil {
		http.Error(w, `{"error":"could not read log file"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(fi)
}

// browse handles GET /api/log/browse — reads a chosen log file backward from
// the tail and returns a page of lines, newest-first. Unlike the live feed
// (fed only by the tailer) this works while the game is closed, and unlike
// replay it does not drive the app pipeline — it is a read-only viewer.
//
// Query params:
//
//	file          eqlog base name (required)
//	q             case-insensitive message filter (optional)
//	type          exact event type filter, e.g. "log:combat_hit" (optional)
//	before_offset byte cursor from a prior page's next_offset (optional)
//	limit         max lines to return (optional, default 300)
func (h *logHandler) browse(w http.ResponseWriter, r *http.Request) {
	full, errMsg := resolveLogFile(h.mgr, r.URL.Query().Get("file"))
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	q := r.URL.Query()
	beforeOffset, _ := strconv.ParseInt(q.Get("before_offset"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	res, err := logparser.BrowseLines(r.Context(), full, beforeOffset, q.Get("q"), q.Get("type"), limit)
	if err != nil {
		// The client abandoned this search (kept typing); nothing to send.
		if errors.Is(err, context.Canceled) {
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// cleanup handles POST /api/log/cleanup — backs up the log file and purges
// entries older than the retention window (see logparser.purgeKeepDays).
func (h *logHandler) cleanup(w http.ResponseWriter, r *http.Request) {
	s := h.tailer.Status()
	if !s.FileExists || s.FilePath == "" {
		http.Error(w, `{"error":"log file not found"}`, http.StatusNotFound)
		return
	}

	backupPath, err := logparser.BackupAndPurge(s.FilePath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"backup_path": backupPath})
}

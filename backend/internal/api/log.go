package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type logHandler struct {
	tailer *logparser.Tailer
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
	})
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

// cleanup handles POST /api/log/cleanup — backs up the log file and purges
// entries older than 7 days.
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

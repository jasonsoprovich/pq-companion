package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
)

type backupHandler struct {
	mgr *backup.Manager
}

// list handles GET /api/backups — returns all backups newest-first.
func (h *backupHandler) list(w http.ResponseWriter, r *http.Request) {
	backups, err := h.mgr.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list backups")
		return
	}
	if backups == nil {
		backups = []*backup.Backup{}
	}
	json.NewEncoder(w).Encode(map[string]any{"backups": backups})
}

// get handles GET /api/backups/{id}.
func (h *backupHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b, err := h.mgr.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get backup")
		return
	}
	json.NewEncoder(w).Encode(b)
}

// create handles POST /api/backups — body:
// {"name":"…","notes":"…","include_logs":false}.
func (h *backupHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Notes       string `json:"notes"`
		IncludeLogs bool   `json:"include_logs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	create := h.mgr.Create
	if req.IncludeLogs {
		create = h.mgr.CreateWithLogs
	}
	b, err := create(req.Name, req.Notes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(b)
}

// delete handles DELETE /api/backups/{id}.
func (h *backupHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Delete(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// files handles GET /api/backups/{id}/files — the files in a snapshot.
func (h *backupHandler) files(w http.ResponseWriter, r *http.Request) {
	files, err := h.mgr.Files(chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"files": files})
}

// restore handles POST /api/backups/{id}/restore — optional body
// {"files":["Osui_spellsets.ini"]} restores only those files.
func (h *backupHandler) restore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Files []string `json:"files"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if err := h.mgr.RestoreFiles(id, req.Files); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "restored"})
}

// lock handles PUT /api/backups/{id}/lock.
func (h *backupHandler) lock(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Lock(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "locked"})
}

// unlock handles PUT /api/backups/{id}/unlock.
func (h *backupHandler) unlock(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Unlock(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "unlocked"})
}

// prune handles POST /api/backups/prune — body: {"max_backups": N}.
func (h *backupHandler) prune(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxBackups int `json:"max_backups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MaxBackups <= 0 {
		writeError(w, http.StatusBadRequest, "max_backups must be a positive integer")
		return
	}
	deleted, err := h.mgr.Prune(req.MaxBackups)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]int{"deleted": deleted})
}

// history handles GET /api/backups/history — every tracked settings file
// with its saved versions.
func (h *backupHandler) history(w http.ResponseWriter, r *http.Request) {
	files, err := h.mgr.TrackedFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"files": files})
}

// captureHistory handles POST /api/backups/history/capture — save any
// changed files now instead of waiting for the next pass.
func (h *backupHandler) captureHistory(w http.ResponseWriter, r *http.Request) {
	n, err := h.mgr.CaptureFiles(true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]int{"saved": n})
}

func versionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "vid"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid version id")
		return 0, false
	}
	return id, true
}

// historyVersion handles GET /api/backups/history/{vid} — a saved version's
// text, for previewing before a restore.
func (h *backupHandler) historyVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := versionID(w, r)
	if !ok {
		return
	}
	name, data, err := h.mgr.VersionContent(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"file_name": name, "content": string(data)})
}

// restoreVersion handles POST /api/backups/history/{vid}/restore.
func (h *backupHandler) restoreVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := versionID(w, r)
	if !ok {
		return
	}
	if err := h.mgr.RestoreVersion(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "restored"})
}

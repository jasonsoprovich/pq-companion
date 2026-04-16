package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

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
		http.Error(w, `{"error":"failed to list backups"}`, http.StatusInternalServerError)
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
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get backup"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(b)
}

// create handles POST /api/backups — body: {"name":"…","notes":"…"}.
func (h *backupHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	b, err := h.mgr.Create(req.Name, req.Notes)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
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
		http.Error(w, `{"error":"`+err.Error()+`"}`, status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// restore handles POST /api/backups/{id}/restore.
func (h *backupHandler) restore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.mgr.Restore(id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, backup.ErrNotFound) {
			status = http.StatusNotFound
		}
		http.Error(w, `{"error":"`+err.Error()+`"}`, status)
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
		http.Error(w, `{"error":"`+err.Error()+`"}`, status)
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
		http.Error(w, `{"error":"`+err.Error()+`"}`, status)
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
		http.Error(w, `{"error":"max_backups must be a positive integer"}`, http.StatusBadRequest)
		return
	}
	deleted, err := h.mgr.Prune(req.MaxBackups)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]int{"deleted": deleted})
}

package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/appbackup"
)

type appBackupHandler struct {
	mgr *appbackup.Manager
}

// export handles POST /api/app/export. Body: {"destination_path": "<abs path>"}.
// Returns the final bundle path (which may differ from destination if the
// caller didn't include the .pqcb extension) and the manifest contents.
func (h *appBackupHandler) export(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DestinationPath string `json:"destination_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.DestinationPath == "" {
		writeError(w, http.StatusBadRequest, "destination_path is required")
		return
	}
	bundlePath, manifest, err := h.mgr.Export(body.DestinationPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bundle_path": bundlePath,
		"manifest":    manifest,
	})
}

// importPreview handles POST /api/app/import/preview. Body: {"bundle_path": "..."}.
// Reads the manifest only — does not stage anything. Used by the renderer to
// show a confirm dialog before the destructive StageImport.
func (h *appBackupHandler) importPreview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BundlePath string `json:"bundle_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.BundlePath == "" {
		writeError(w, http.StatusBadRequest, "bundle_path is required")
		return
	}
	manifest, err := h.mgr.PreviewImport(body.BundlePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"manifest": manifest})
}

// stageImport handles POST /api/app/import. Extracts the bundle into the
// staging dir and drops a sentinel so the next backend startup applies the
// swap. Returns the manifest and a restart_required flag.
func (h *appBackupHandler) stageImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BundlePath string `json:"bundle_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.BundlePath == "" {
		writeError(w, http.StatusBadRequest, "bundle_path is required")
		return
	}
	manifest, err := h.mgr.StageImport(body.BundlePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"manifest":         manifest,
		"restart_required": true,
	})
}

// pendingStatus handles GET /api/app/import/pending. Reports whether a
// staged import is waiting for the next backend startup.
func (h *appBackupHandler) pendingStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"pending": h.mgr.HasPendingImport()})
}

// cancelImport handles DELETE /api/app/import. Drops any staged files so the
// next startup does nothing extra.
func (h *appBackupHandler) cancelImport(w http.ResponseWriter, r *http.Request) {
	if err := h.mgr.CancelStagedImport(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/pipertts"
)

// piperHandler exposes the user-installed Piper local-TTS integration to the
// Settings UI and the audio engine: install status, on-demand synthesis
// (cached WAV path), and cache clearing. Piper config lives in the standard
// config.Preferences (piper_*), saved through the normal config PUT — this
// handler never writes config, it only reads it and drives synthesis.
type piperHandler struct {
	cfgMgr *config.Manager
	svc    *pipertts.Service
}

// piperConfig translates the saved preferences into a pipertts.Config.
func (h *piperHandler) piperConfig() pipertts.Config {
	p := h.cfgMgr.Get().Preferences
	return pipertts.Config{
		Enabled:   p.PiperEnabled,
		ExePath:   p.PiperExePath,
		ModelPath: p.PiperModelPath,
		Mode:      p.PiperMode,
		ServerURL: p.PiperServerURL,
	}
}

// GET /api/piper/status
// Reports whether the configured piper executable + voice model are present and
// valid (and the piper version, best-effort). Powers the Settings status card.
func (h *piperHandler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, pipertts.DetectStatus(r.Context(), h.piperConfig()))
}

// POST /api/piper/synthesize  body: {"text": "..."}  ->  {"path": "..."}
// Returns the cached WAV path for text in the configured Piper voice,
// synthesizing + caching on a miss. Used by the trigger-save pre-generate step,
// the settings "Test voice" button, and lazy fire-time fallback. A disabled or
// misconfigured install (or a spawn failure) responds 503 so the frontend can
// fall back to Web Speech without treating it as a hard error.
func (h *piperHandler) synthesize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	path, err := h.svc.Synthesize(r.Context(), h.piperConfig(), req.Text)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// POST /api/piper/clear-cache  ->  {"removed": N}
// Deletes every generated TTS WAV. Safe to call anytime; a fresh WAV is
// regenerated on the next synthesize.
func (h *piperHandler) clearCache(w http.ResponseWriter, _ *http.Request) {
	removed, err := h.svc.ClearCache()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

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
	}
}

// piperStatusResponse composes the pure install-detection check with the
// Service's live warm-worker and cache state into the single JSON object the
// Settings card fetches.
type piperStatusResponse struct {
	pipertts.Status
	Mode        string `json:"mode"`
	WarmRunning bool   `json:"warm_running"`
	WarmError   string `json:"warm_error,omitempty"`
	CacheFiles  int    `json:"cache_files"`
	CacheBytes  int64  `json:"cache_bytes"`
}

// GET /api/piper/status
// Reports whether the configured piper executable + voice model are present
// and valid (and the piper version, best-effort), plus the current mode,
// whether a warm worker is alive, and cache size. Powers the Settings status
// card. As a side effect, reconciles the warm worker against the live config
// (stopping — never starting — one that no longer matches), so a stale warm
// worker doesn't linger after the user disables Piper or changes its path;
// the frontend already re-fetches this on every config save.
func (h *piperHandler) status(w http.ResponseWriter, r *http.Request) {
	cfg := h.piperConfig()
	h.svc.ReconcileWarm(cfg)
	base := pipertts.DetectStatus(r.Context(), cfg)
	running, warmErr := h.svc.WarmStatus()
	files, bytes, _ := h.svc.CacheStats()
	writeJSON(w, http.StatusOK, piperStatusResponse{
		Status:      base,
		Mode:        cfg.EffectiveMode(),
		WarmRunning: running,
		WarmError:   warmErr,
		CacheFiles:  files,
		CacheBytes:  bytes,
	})
}

// POST /api/piper/synthesize  body: {"text": "...", "force"?: bool}  ->  {"path": "..."}
// Returns the cached WAV path for text in the configured Piper voice,
// synthesizing + caching on a miss. Used by the trigger-save pre-generate step,
// the settings "Test voice" button, and lazy fire-time fallback. A disabled or
// misconfigured install (or a spawn failure) responds 503 so the frontend can
// fall back to Web Speech without treating it as a hard error.
//
// force (used only by "Test voice") bypasses the cache-hit shortcut — see
// Service.Synthesize's doc for why that matters for actually exercising the
// currently selected spawn/warm mode.
func (h *piperHandler) synthesize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text  string `json:"text"`
		Force bool   `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	path, err := h.svc.Synthesize(r.Context(), h.piperConfig(), req.Text, req.Force)
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

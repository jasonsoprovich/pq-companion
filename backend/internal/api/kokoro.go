package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/kokorotts"
)

// kokoroHandler exposes the user-installed Kokoro local-TTS integration to
// the Settings UI and the audio engine — the sibling of piperHandler for the
// second local-TTS provider. Kokoro config lives in the standard
// config.Preferences (kokoro_*), saved through the normal config PUT — this
// handler never writes config, it only reads it and drives synthesis.
type kokoroHandler struct {
	cfgMgr *config.Manager
	svc    *kokorotts.Service
}

// kokoroConfig translates the saved preferences into a kokorotts.Config.
func (h *kokoroHandler) kokoroConfig() kokorotts.Config {
	p := h.cfgMgr.Get().Preferences
	return kokorotts.Config{
		Enabled: p.KokoroEnabled,
		BaseURL: p.KokoroBaseURL,
		Voice:   p.KokoroVoice,
		Speed:   p.KokoroSpeed,
	}
}

// kokoroStatusResponse composes the pure install-detection check with the
// Service's live cache state into the single JSON object the Settings card
// fetches.
type kokoroStatusResponse struct {
	kokorotts.Status
	CacheFiles int   `json:"cache_files"`
	CacheBytes int64 `json:"cache_bytes"`
}

// GET /api/kokoro/status
// Reports whether the configured kokoro-tts executable + model + voices file
// are present (and the kokoro-tts version, best-effort), plus cache size.
// Powers the Settings status card.
func (h *kokoroHandler) status(w http.ResponseWriter, r *http.Request) {
	cfg := h.kokoroConfig()
	base := kokorotts.DetectStatus(r.Context(), cfg)
	files, bytes, _ := h.svc.CacheStats()
	writeJSON(w, http.StatusOK, kokoroStatusResponse{
		Status:     base,
		CacheFiles: files,
		CacheBytes: bytes,
	})
}

// POST /api/kokoro/synthesize  body: {"text": "...", "force"?: bool}  ->  {"path": "..."}
// Returns the cached WAV path for text in the configured Kokoro voice,
// synthesizing + caching on a miss. Used by the trigger-save pre-generate
// step, the settings "Test voice" button, and lazy fire-time fallback. A
// disabled or misconfigured install (or a spawn failure) responds 503 so the
// frontend can fall back to Web Speech without treating it as a hard error.
func (h *kokoroHandler) synthesize(w http.ResponseWriter, r *http.Request) {
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
	path, err := h.svc.Synthesize(r.Context(), h.kokoroConfig(), req.Text, req.Force)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// POST /api/kokoro/clear-cache  ->  {"removed": N}
// Deletes every generated TTS WAV (shared with Piper's cache — see
// internal/tts). Safe to call anytime; a fresh WAV is regenerated on the next
// synthesize.
func (h *kokoroHandler) clearCache(w http.ResponseWriter, _ *http.Request) {
	removed, err := h.svc.ClearCache()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
}

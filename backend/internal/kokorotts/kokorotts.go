// Package kokorotts integrates a self-hosted Kokoro-FastAPI server
// (github.com/remsky/Kokoro-FastAPI, wrapping the Apache-2.0 Kokoro-82M
// model behind an OpenAI-compatible /v1/audio/speech + /v1/audio/voices
// HTTP API) as a second opt-in, local voice provider alongside Piper — see
// internal/pipertts for the sibling package this one deliberately keeps
// separate from, not shared with.
//
// Unlike Piper (a spawned subprocess), Kokoro here is a long-running service
// the user starts themselves (typically `docker run` — see the project's
// README) and this app talks to over plain HTTP, the same "point it at a
// URL" shape as the Zeal pipe or the quarm manifest fetcher. That also means
// no exe/model file paths to configure and no subprocess lifecycle to
// manage: DetectStatus's one GET /v1/audio/voices call both confirms the
// service is reachable AND returns the voice catalog to populate the
// Settings dropdown, so the whole setup is "enter a URL, pick a voice."
package kokorotts

import "strings"

// defaultVoice is used whenever Config.Voice is empty. af_heart is
// Kokoro-FastAPI's own flagship voice, bundled in every image regardless of
// which other voices an operator has configured — same reasoning distill-
// reader's DEFAULT_TTS_VOICES uses for the same provider.
const defaultVoice = "af_heart"

// defaultBaseURL is shown as the Settings field's placeholder and used as a
// fallback if the caller passes an empty string somewhere unexpected — the
// well-known default for a locally `docker run` Kokoro-FastAPI container.
const defaultBaseURL = "http://localhost:8880"

// Config is the resolved Kokoro configuration for one request, translated
// from config.Preferences by the API layer so this package stays free of an
// import cycle on config.
type Config struct {
	Enabled bool
	BaseURL string // e.g. "http://localhost:8880"
	Voice   string // e.g. "af_heart"
	// Speed is the API's speed multiplier (1.0 = normal). Zero/negative is
	// treated as 1.0 by EffectiveSpeed.
	Speed float64
}

// EffectiveBaseURL returns cfg.BaseURL with any trailing slash trimmed, or
// defaultBaseURL when unset.
func (c Config) EffectiveBaseURL() string {
	u := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if u == "" {
		return defaultBaseURL
	}
	return u
}

// EffectiveVoice returns cfg.Voice, or defaultVoice when unset.
func (c Config) EffectiveVoice() string {
	v := strings.TrimSpace(c.Voice)
	if v == "" {
		return defaultVoice
	}
	return v
}

// EffectiveSpeed returns cfg.Speed, or 1.0 when zero/negative.
func (c Config) EffectiveSpeed() float64 {
	if c.Speed <= 0 {
		return 1.0
	}
	return c.Speed
}

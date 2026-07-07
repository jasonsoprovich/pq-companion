// Package pipertts integrates the user-installed Piper neural TTS engine as an
// opt-in, local voice provider for trigger and alert callouts.
//
// Piper is NOT bundled with PQ Companion — it is a program the user installs
// themselves (the frozen MIT standalone piper.exe, or `pip install piper-tts`),
// exactly like the Zeal/EQW integrations. The backend detects the configured
// executable + voice model, spawns piper per phrase (Mode A) to produce a WAV,
// caches the WAV content-addressed under ~/.pq-companion/tts-cache/, and hands
// the path back to the renderer, which plays it through the existing
// pq-audio:// pipeline. Any failure is surfaced as an error so the frontend can
// fall back to Web Speech — a broken external tool must never wedge alerts.
//
// This is the first subprocess the Go backend ever spawns, so synth.go keeps
// the hygiene tight: args are passed as a slice (never a shell string), every
// spawn has a hard timeout and is killed on expiry, output size is capped, and
// concurrent spawns are bounded by a semaphore. See docs/piper-tts-plan.md.
package pipertts

import "strings"

// Config is the resolved Piper configuration for one synthesis request,
// translated from config.Preferences by the API layer so this package stays
// free of an import cycle on config.
type Config struct {
	Enabled   bool
	ExePath   string
	ModelPath string
	// Mode is "spawn" (v1) or "http" (phase 2). Empty is treated as "spawn".
	Mode      string
	ServerURL string
}

// spawnMode reports whether this config should synthesize via a per-phrase
// subprocess spawn (the only mode implemented in v1). An empty Mode defaults to
// spawn.
func (c Config) spawnMode() bool {
	return c.Mode == "" || c.Mode == "spawn"
}

// VoiceName derives a human-ish voice label from the model file name, e.g.
// "en_US-amy-medium" from ".../en_US-amy-medium.onnx". Empty when no model is
// configured.
func (c Config) VoiceName() string {
	base := c.ModelPath
	if base == "" {
		return ""
	}
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".onnx")
}

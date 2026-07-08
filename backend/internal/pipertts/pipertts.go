// Package pipertts integrates the user-installed Piper neural TTS engine as an
// opt-in, local voice provider for trigger and alert callouts.
//
// Piper is NOT bundled with PQ Companion — it is a program the user installs
// themselves (the frozen MIT standalone piper.exe, the same binary on every
// supported platform), exactly like the Zeal/EQW integrations. The backend
// detects the configured executable + voice model and synthesizes each phrase
// to a WAV, cached content-addressed under ~/.pq-companion/tts-cache/; the
// renderer plays the returned path through the existing pq-audio:// pipeline.
// Any failure is surfaced as an error so the frontend can fall back to Web
// Speech — a broken external tool must never wedge alerts.
//
// Two synthesis modes (Config.Mode):
//   - "spawn" (default, synth.go) — a fresh piper subprocess per phrase. Simple
//     and robust, but pays a model-reload cost on every cache miss.
//   - "warm" (opt-in, warm.go) — one persistent piper subprocess per (exe,
//     model), spawned with --output_dir so it loads the model once and stays
//     alive across requests. Falls back transparently to "spawn" on any
//     failure — a broken warm worker must never fail a request outright.
//
// This is the first subprocess (and, with warm mode, the first persistent
// request/response subprocess) the Go backend ever manages, so both synth.go
// and warm.go keep the hygiene tight: args are passed as a slice (never a
// shell string), every operation has a hard timeout, output size is capped,
// and warm.go relies on ordinary OS pipe-close semantics (rather than any
// app-wide graceful-shutdown machinery, which doesn't exist in this codebase)
// so an abnormal parent exit doesn't orphan the child. See
// docs/piper-tts-plan.md.
package pipertts

import "strings"

// Config is the resolved Piper configuration for one synthesis request,
// translated from config.Preferences by the API layer so this package stays
// free of an import cycle on config.
type Config struct {
	Enabled   bool
	ExePath   string
	ModelPath string
	// Mode is "spawn" (default, per-request subprocess) or "warm" (Phase 2,
	// one persistent subprocess kept alive across requests — see warm.go).
	// Empty is treated as "spawn".
	Mode string
}

// EffectiveMode normalizes Mode for display/comparison: "warm" if explicitly
// set, else "spawn" (covers "", "spawn", and any unrecognized future value —
// Service.Synthesize's cold path is the fallback for everything but "warm").
// Exported for the API layer's status response.
func (c Config) EffectiveMode() string {
	if c.Mode == "warm" {
		return "warm"
	}
	return "spawn"
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

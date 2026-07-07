package pipertts

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Status describes the detected Piper install for the Settings UI. It mirrors
// the shape of the Zeal/EQW status cards: found/valid flags plus an optional
// version string. Ready is the single bit the frontend uses to decide whether
// a "piper:local" voice can actually synthesize.
type Status struct {
	Enabled bool `json:"enabled"`

	ExePath   string `json:"exe_path,omitempty"`
	ModelPath string `json:"model_path,omitempty"`

	// ExeFound is true when ExePath points at an existing regular file.
	ExeFound bool `json:"exe_found"`
	// ModelFound is true when ModelPath points at an existing .onnx file.
	ModelFound bool `json:"model_found"`
	// ModelConfigFound is true when the sibling "<model>.onnx.json" (which
	// piper requires alongside the voice) exists.
	ModelConfigFound bool `json:"model_config_found"`

	// Version is the piper build string from "<exe> --version", best-effort.
	// Empty when the exe isn't found or doesn't support the flag.
	Version string `json:"version,omitempty"`

	// VoiceName is the label the configured voice appears under in the UI,
	// derived from the model file name (e.g. "en_US-amy-medium").
	VoiceName string `json:"voice_name,omitempty"`

	// Ready is true when the exe, model, and model config are all present, so a
	// Piper voice can actually be synthesized.
	Ready bool `json:"ready"`

	// Error carries a short human-readable reason the install isn't ready, for
	// the settings card (e.g. "voice model config (.onnx.json) not found").
	Error string `json:"error,omitempty"`
}

// versionProbeTimeout bounds the "<exe> --version" call so a hung or wrong
// executable can't stall the status endpoint.
const versionProbeTimeout = 5 * time.Second

// DetectStatus inspects the configured Piper executable and voice model and
// reports what's present. It performs at most one short, bounded subprocess
// call (the version probe) and never synthesizes audio.
func DetectStatus(ctx context.Context, cfg Config) Status {
	st := Status{
		Enabled:   cfg.Enabled,
		ExePath:   strings.TrimSpace(cfg.ExePath),
		ModelPath: strings.TrimSpace(cfg.ModelPath),
		VoiceName: cfg.VoiceName(),
	}

	if st.ExePath != "" {
		if info, err := os.Stat(st.ExePath); err == nil && info.Mode().IsRegular() {
			st.ExeFound = true
		}
	}
	if st.ModelPath != "" {
		if info, err := os.Stat(st.ModelPath); err == nil && info.Mode().IsRegular() {
			st.ModelFound = true
		}
		// The standard piper-voices convention keeps the ".onnx" in the sidecar
		// name (ModelPath already ends in ".onnx", so appending just ".json"
		// yields "<voice>.onnx.json" — NOT ModelPath + ".onnx.json", which would
		// double the extension into "<voice>.onnx.onnx.json" and never match).
		if info, err := os.Stat(st.ModelPath + ".json"); err == nil && info.Mode().IsRegular() {
			st.ModelConfigFound = true
		} else if info, err := os.Stat(modelConfigPath(st.ModelPath)); err == nil && info.Mode().IsRegular() {
			// Some voices ship "<voice>.json" instead of "<voice>.onnx.json".
			st.ModelConfigFound = true
		}
	}

	if st.ExeFound {
		st.Version = probeVersion(ctx, st.ExePath)
	}

	st.Ready = st.ExeFound && st.ModelFound && st.ModelConfigFound
	st.Error = readinessError(st)
	return st
}

// modelConfigPath returns the alternate "<name>.json" config path for a
// "<name>.onnx" model (some Hugging Face voices use this form).
func modelConfigPath(modelPath string) string {
	return strings.TrimSuffix(modelPath, ".onnx") + ".json"
}

// readinessError returns the first missing prerequisite as a short message, or
// "" when the install is ready. Only meaningful once a path has been entered.
func readinessError(st Status) string {
	switch {
	case st.ExePath == "" && st.ModelPath == "":
		return ""
	case st.ExePath != "" && !st.ExeFound:
		return "piper executable not found at the configured path"
	case st.ModelPath != "" && !st.ModelFound:
		return "voice model (.onnx) not found at the configured path"
	case st.ModelFound && !st.ModelConfigFound:
		return "voice model config (.onnx.json) not found next to the model"
	case st.ExePath == "":
		return "piper executable path not set"
	case st.ModelPath == "":
		return "voice model path not set"
	default:
		return ""
	}
}

// probeVersion runs "<exe> --version" with a short timeout and returns a
// trimmed one-line version string. Best-effort: returns "" on any failure
// (including exes that don't support the flag), never an error — a missing
// version must not make an otherwise-working install look broken.
func probeVersion(ctx context.Context, exePath string) string {
	ctx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, exePath, "--version").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	if len(line) > 64 {
		line = line[:64]
	}
	return line
}

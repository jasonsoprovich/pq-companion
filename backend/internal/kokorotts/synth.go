package kokorotts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// synthTimeout bounds a single /v1/audio/speech call. Kokoro-FastAPI runs
	// CPU-bound synthesis (unless the operator's container has GPU access),
	// so this is generous — matches the timeout distill-reader settled on
	// for the same provider.
	synthTimeout = 60 * time.Second

	// maxTextLen mirrors pipertts's cap — alert callouts are short; anything
	// past this is almost certainly a misconfiguration.
	maxTextLen = 1000

	// maxOutputBytes rejects a WAV larger than this — a wide safety margin
	// against a misbehaving server flooding memory/disk.
	maxOutputBytes = 25 << 20

	// maxConcurrentRequests bounds how many synthesis requests are in flight
	// at once, so a burst of trigger fires can't overwhelm the user's own
	// Kokoro container.
	maxConcurrentRequests = 4
)

var requestSem = make(chan struct{}, maxConcurrentRequests)

// errSynthUnavailable is returned when the service can't synthesize
// (disabled or misconfigured). The API layer maps it to a 503 so the
// frontend falls back to Web Speech.
var errSynthUnavailable = errors.New("kokoro not available")

var httpClient = &http.Client{}

// speechRequest is the OpenAI-compatible body Kokoro-FastAPI's
// /v1/audio/speech expects.
type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed"`
}

// synthesizeToFile POSTs text to {cfg.BaseURL}/v1/audio/speech and writes
// the returned WAV bytes to outPath, atomically (a temp file in the same
// directory renamed into place only on success).
func synthesizeToFile(ctx context.Context, cfg Config, text, outPath string) error {
	if !cfg.Enabled {
		return fmt.Errorf("%w: disabled", errSynthUnavailable)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("empty text")
	}
	if len(text) > maxTextLen {
		return fmt.Errorf("text too long (%d > %d chars)", len(text), maxTextLen)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("%w: base URL not set", errSynthUnavailable)
	}

	select {
	case requestSem <- struct{}{}:
		defer func() { <-requestSem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	reqBody, err := json.Marshal(speechRequest{
		Model:          "kokoro",
		Input:          text,
		Voice:          cfg.EffectiveVoice(),
		ResponseFormat: "wav",
		Speed:          cfg.EffectiveSpeed(),
	})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, synthTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.EffectiveBaseURL()+"/v1/audio/speech", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("kokoro request timed out after %s", synthTimeout)
		}
		return fmt.Errorf("kokoro request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := strings.TrimSpace(string(body))
		if msg != "" {
			if len(msg) > 200 {
				msg = msg[:200]
			}
			return fmt.Errorf("kokoro request failed (%d): %s", resp.StatusCode, msg)
		}
		return fmt.Errorf("kokoro request failed (%d)", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(outPath), "kokoro-*.wav.tmp")
	if err != nil {
		return fmt.Errorf("create temp wav: %w", err)
	}
	tmpPath := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, io.LimitReader(resp.Body, maxOutputBytes+1))
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write response: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("finalize temp wav: %w", err)
	}
	if written > maxOutputBytes {
		return fmt.Errorf("kokoro output too large (>%d bytes)", maxOutputBytes)
	}
	if written == 0 {
		return errors.New("kokoro returned no audio")
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("finalize wav: %w", err)
	}
	success = true
	return nil
}

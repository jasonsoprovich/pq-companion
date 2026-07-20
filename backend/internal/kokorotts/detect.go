package kokorotts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// detectTimeout bounds the GET /v1/audio/voices probe so a hung or
// unreachable service can't stall the status endpoint.
const detectTimeout = 5 * time.Second

// maxVoicesResponseBytes caps how much of the voices response is read —
// generous for a few dozen short voice ids, tight enough to reject a
// misconfigured URL that happens to return an unrelated, large response.
const maxVoicesResponseBytes = 1 << 20 // 1 MB

// Status describes the detected Kokoro-FastAPI service for the Settings UI.
type Status struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url,omitempty"`
	Voice   string `json:"voice,omitempty"`

	// Reachable is true when GET {BaseURL}/v1/audio/voices returned a valid
	// response. False for any connection error, timeout, non-2xx status, or
	// unparseable body.
	Reachable bool `json:"reachable"`

	// Voices lists every voice id the service reported, for the Settings
	// voice dropdown. Empty when unreachable.
	Voices []string `json:"voices,omitempty"`

	// Ready is true when the service is reachable and reported at least one
	// voice, so a Kokoro voice can actually be synthesized.
	Ready bool `json:"ready"`

	// Error carries a short human-readable reason the install isn't ready.
	Error string `json:"error,omitempty"`
}

// DetectStatus probes {cfg.BaseURL}/v1/audio/voices and reports whether the
// service is reachable and which voices it offers. This single call serves
// double duty as both the "is it up" health check (Kokoro-FastAPI has no
// dedicated /health endpoint) and the voice-catalog fetch. Never
// synthesizes audio.
func DetectStatus(ctx context.Context, cfg Config) Status {
	st := Status{
		Enabled: cfg.Enabled,
		BaseURL: strings.TrimSpace(cfg.BaseURL),
		Voice:   cfg.EffectiveVoice(),
	}

	if st.BaseURL == "" {
		st.Error = "Kokoro base URL not set"
		return st
	}
	if _, err := url.ParseRequestURI(cfg.EffectiveBaseURL()); err != nil {
		st.Error = "Kokoro base URL is not a valid URL"
		return st
	}

	voices, err := fetchVoices(ctx, cfg)
	if err != nil {
		st.Error = fmt.Sprintf("Kokoro service not reachable at %s — is it running? (%s)", cfg.EffectiveBaseURL(), err)
		return st
	}
	st.Reachable = true
	st.Voices = voices

	if len(voices) == 0 {
		st.Error = "Kokoro service is reachable but reported no voices"
		return st
	}
	st.Ready = true
	return st
}

// fetchVoices performs the GET /v1/audio/voices call and parses the
// OpenAI-compatible {"voices": [{"id": "..."}]} response shape (confirmed
// against the Kokoro-FastAPI docs — not memorized).
func fetchVoices(ctx context.Context, cfg Config) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.EffectiveBaseURL()+"/v1/audio/voices", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxVoicesResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxVoicesResponseBytes {
		return nil, fmt.Errorf("response too large")
	}

	var parsed struct {
		Voices []struct {
			ID string `json:"id"`
		} `json:"voices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	voices := make([]string, 0, len(parsed.Voices))
	for _, v := range parsed.Voices {
		if v.ID != "" {
			voices = append(voices, v.ID)
		}
	}
	return voices, nil
}

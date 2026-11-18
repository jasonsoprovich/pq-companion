package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

type patternConvertRequest struct {
	// To is "regex" (Text is plain text) or "plain" (Text is a regex).
	To   string `json:"to"`
	Text string `json:"text"`
}

type patternConvertResponse struct {
	OK     bool   `json:"ok"`
	Result string `json:"result,omitempty"`
	// Error explains why conversion failed: a plain-text syntax problem, or
	// why a regex can't be shown as plain text.
	Error string `json:"error,omitempty"`
}

// convertPattern handles POST /api/triggers/pattern/convert. The plain-text
// grammar lives in trigger.PlainToRegex / RegexToPlain so the editor and
// the engine agree on it; the stored pattern is always the regex.
func (h *triggerHandler) convertPattern(w http.ResponseWriter, r *http.Request) {
	var req patternConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var out string
	var err error
	switch req.To {
	case "regex":
		out, err = trigger.PlainToRegex(req.Text)
	case "plain":
		out, err = trigger.RegexToPlain(req.Text)
	default:
		writeError(w, http.StatusBadRequest, `to must be "regex" or "plain"`)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, patternConvertResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, patternConvertResponse{OK: true, Result: out})
}

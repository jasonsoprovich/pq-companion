package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError responds with a JSON {"error": msg} body. 5xx responses are
// also logged via slog so that the underlying cause (e.g. a SQLite open
// failure mid-session) lands in server.log alongside the chi request line.
// Without this, the user only ever sees the toast in the UI.
func writeError(w http.ResponseWriter, status int, msg string) {
	if status >= 500 {
		slog.Error("api error response", "status", status, "msg", msg)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// queryInt reads an integer query param, returning defaultVal if absent or invalid.
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defaultVal
	}
	return v
}

// queryIntList reads a comma-separated list of non-negative integers
// (e.g. "?targetType=4,2,40"), skipping invalid entries. Returns nil when
// the param is absent or holds no valid values.
func queryIntList(r *http.Request, key string) []int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v < 0 {
			continue
		}
		out = append(out, v)
	}
	return out
}

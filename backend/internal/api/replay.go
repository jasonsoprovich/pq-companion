package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// replayHandler exposes the log replay pipeline: list candidate log files,
// probe a file's timestamp range, and drive play/pause/stop. Replay reads
// log files strictly read-only.
type replayHandler struct {
	mgr      *config.Manager
	replayer *logparser.Replayer
}

// replayFile is one eqlog candidate for the file picker.
type replayFile struct {
	Name      string    `json:"name"` // base file name
	Character string    `json:"character"`
	SizeBytes int64     `json:"size_bytes"`
	Modified  time.Time `json:"modified"`
}

// files handles GET /api/replay/files — eqlog files in the configured EQ dir.
func (h *replayHandler) files(w http.ResponseWriter, r *http.Request) {
	eqPath := h.mgr.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusBadRequest, "EQ path is not configured")
		return
	}
	matches, err := filepath.Glob(filepath.Join(eqPath, "eqlog_*_pq.proj.txt"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]replayFile, 0, len(matches))
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.IsDir() {
			continue
		}
		base := filepath.Base(m)
		character := strings.TrimSuffix(strings.TrimPrefix(base, "eqlog_"), "_pq.proj.txt")
		out = append(out, replayFile{
			Name:      base,
			Character: character,
			SizeBytes: info.Size(),
			Modified:  info.ModTime(),
		})
	}
	// Most recently played characters first.
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	writeJSON(w, http.StatusOK, out)
}

// resolveLogFile validates a client-supplied base name and returns the full
// path inside the configured EQ dir. Rejects anything that isn't a plain
// eqlog file name (no separators, no traversal).
func (h *replayHandler) resolveLogFile(name string) (string, string) {
	return resolveLogFile(h.mgr, name)
}

// resolveLogFile is the shared name validator used by both the replay and log
// browse handlers. It rejects anything that isn't a plain eqlog file name (no
// separators, no traversal) and confirms the file exists inside the EQ dir.
func resolveLogFile(mgr *config.Manager, name string) (string, string) {
	eqPath := mgr.Get().EQPath
	if eqPath == "" {
		return "", "EQ path is not configured"
	}
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) ||
		!strings.HasPrefix(name, "eqlog_") || !strings.HasSuffix(name, ".txt") {
		return "", "invalid log file name"
	}
	full := filepath.Join(eqPath, name)
	if _, err := os.Stat(full); err != nil {
		return "", "log file not found"
	}
	return full, ""
}

// info handles GET /api/replay/info?file=<name> — the first and last line
// timestamps, used to bound the replay range picker.
func (h *replayHandler) info(w http.ResponseWriter, r *http.Request) {
	full, errMsg := h.resolveLogFile(r.URL.Query().Get("file"))
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	first, last, err := probeLogRange(full)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if first.IsZero() || last.IsZero() {
		writeError(w, http.StatusUnprocessableEntity, "no parseable EQ log lines found in file")
		return
	}
	writeJSON(w, http.StatusOK, map[string]time.Time{"first": first, "last": last})
}

// probeRead caps how many bytes are scanned from each end of the file when
// probing its timestamp range.
const probeRead = 256 * 1024

// probeLogRange returns the first and last valid EQ line timestamps without
// reading the whole file: the head chunk yields the first, the tail chunk
// the last.
func probeLogRange(path string) (first, last time.Time, err error) {
	f, err := os.Open(path)
	if err != nil {
		return first, last, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return first, last, err
	}

	// Head → first timestamp.
	head := bufio.NewScanner(f)
	head.Buffer(make([]byte, 64*1024), probeRead)
	read := 0
	for head.Scan() && read < probeRead {
		read += len(head.Bytes()) + 1
		if ts, _, ok := logparser.ParseRawLine(head.Text()); ok {
			first = ts
			break
		}
	}

	// Tail → last timestamp.
	off := info.Size() - probeRead
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && len(buf) == 0 {
		return first, last, err
	}
	for _, line := range strings.Split(string(buf), "\n") {
		if ts, _, ok := logparser.ParseRawLine(strings.TrimRight(line, "\r")); ok {
			last = ts
		}
	}
	return first, last, nil
}

// start handles POST /api/replay/start.
// Body: {"file": "eqlog_X_pq.proj.txt", "from": RFC3339, "to": RFC3339, "speed": 1}
// from/to default to the file's full range when omitted.
func (h *replayHandler) start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File  string     `json:"file"`
		From  *time.Time `json:"from"`
		To    *time.Time `json:"to"`
		Speed float64    `json:"speed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	full, errMsg := h.resolveLogFile(req.File)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	from, to := time.Time{}, time.Time{}
	if req.From != nil {
		from = *req.From
	}
	if req.To != nil {
		to = *req.To
	}
	if from.IsZero() || to.IsZero() {
		first, last, err := probeLogRange(full)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if from.IsZero() {
			from = first
		}
		if to.IsZero() {
			to = last.Add(time.Second) // inclusive of the final line
		}
	}

	if err := h.replayer.Start(full, filepath.Base(full), from, to, req.Speed); err != nil {
		switch {
		case errors.Is(err, logparser.ErrReplayRangeInverted):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, logparser.ErrReplayAlreadyActive):
			writeError(w, http.StatusConflict, err.Error())
		default: // file-open failure and anything else
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, h.replayer.Status())
}

// pause/resume/stop handle the corresponding POST endpoints.
func (h *replayHandler) pause(w http.ResponseWriter, r *http.Request) {
	h.replayer.Pause()
	writeJSON(w, http.StatusOK, h.replayer.Status())
}

func (h *replayHandler) resume(w http.ResponseWriter, r *http.Request) {
	h.replayer.Resume()
	writeJSON(w, http.StatusOK, h.replayer.Status())
}

func (h *replayHandler) stop(w http.ResponseWriter, r *http.Request) {
	h.replayer.Stop()
	writeJSON(w, http.StatusOK, h.replayer.Status())
}

// status handles GET /api/replay/status.
func (h *replayHandler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.replayer.Status())
}

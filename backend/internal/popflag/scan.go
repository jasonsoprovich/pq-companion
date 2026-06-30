package popflag

import (
	"bufio"
	"os"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// SeerBurst is one Seer guided-meditation block recovered from a log file.
type SeerBurst struct {
	Text       string    // the matching log messages, joined by "\n"
	ObservedAt time.Time // timestamp of the last line in the burst
	LineCount  int
}

// ScanLogForSeer reads an EQ log file and returns the MOST RECENT contiguous run
// of Seer guided-meditation lines — the same lines the live Consumer buffers off
// the tail, but recovered on demand from the file. This lets a player who
// meditated before the app was watching (or on another character) re-sync their
// flags without copy-pasting out of the log by hand.
//
// found is false when the log holds no Seer reading. A run is broken by any
// non-matching line, mirroring Consumer.HandleLine's flush-on-non-match, so the
// scan and the live tail agree on what counts as one reading.
func ScanLogForSeer(path string) (burst SeerBurst, found bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return SeerBurst{}, false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// EQ log lines are normally short, but allow generous headroom (the
	// replayer/backfill scanners use a 1MB cap) so a stray long line can't abort
	// the scan mid-file.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var cur []string
	var curTS time.Time
	flush := func() {
		if len(cur) == 0 {
			return
		}
		// Each completed run overwrites the previous, so the last one standing is
		// the most recent reading in the file.
		burst = SeerBurst{Text: strings.Join(cur, "\n"), ObservedAt: curTS, LineCount: len(cur)}
		found = true
		cur = nil
	}
	for sc.Scan() {
		ts, msg, ok := logparser.ParseRawLine(sc.Text())
		if ok && MatchSeerLine(msg) {
			cur = append(cur, msg)
			curTS = ts
			continue
		}
		flush()
	}
	flush()
	if err := sc.Err(); err != nil {
		return SeerBurst{}, false, err
	}
	return burst, found, nil
}

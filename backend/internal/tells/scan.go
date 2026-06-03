package tells

import (
	"bufio"
	"fmt"
	"os"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// ScanFile reads an entire EQ log file and stores every direct tell it
// contains, attributing each to character and stamping the zone in effect at
// that point in the log. Duplicate rows are ignored (see tells_unique), so
// re-scanning is safe and idempotent. Returns the number of newly-inserted
// rows.
//
// This is the optional historical backfill behind the Tell Tracker's "scan
// existing logs" button — it is never run automatically because large logs can
// take a while to walk.
func ScanFile(store *Store, path, character string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	inserted := 0
	zone := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		ts, msg, ok := logparser.ParseRawLine(line)
		if !ok {
			continue
		}
		// Track zone so tells get the same zone stamp they would have had live.
		if ev, ok := logparser.ParseLine(line); ok && ev.Type == logparser.EventZone {
			if zd, ok := ev.Data.(logparser.ZoneData); ok {
				zone = zd.ZoneName
			}
			continue
		}
		p, ok := ParseTell(msg)
		if !ok {
			continue
		}
		ins, err := store.Insert(Input{
			Character: character,
			Peer:      p.Peer,
			Direction: p.Direction,
			Message:   p.Message,
			Zone:      zone,
			TS:        ts,
		})
		if err != nil {
			return inserted, err
		}
		if ins {
			inserted++
		}
	}
	if err := sc.Err(); err != nil {
		return inserted, fmt.Errorf("read log: %w", err)
	}
	return inserted, nil
}

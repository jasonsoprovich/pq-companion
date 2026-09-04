package logparser

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// lastZoneScanCap bounds how far back from EOF LastZoneInLog will read before
// giving up. A character who played a long session in a single zone and then
// camped can have megabytes of combat/chat spam after their last "You have
// entered" line; 64 MiB covers all but pathological raid logs, and a miss just
// means the zone isn't seeded (the live parser fills it in on the next zone).
const lastZoneScanCap = 64 << 20

// lastZoneChunk is the backward read granularity for LastZoneInLog.
const lastZoneChunk = 1 << 20

// LastZoneInLog scans an EQ character's log file backward from the end for the
// most recent "You have entered <Zone>." line and returns the zone long name
// and that line's timestamp. ok is false when the log can't be read or holds
// no usable zone-in line within lastZoneScanCap bytes of the end.
//
// Used at startup to seed characters.last_zone so the Recap tab can show
// "Camped in …" without waiting for a live zone-in.
func LastZoneInLog(eqPath, character string) (zone string, ts time.Time, ok bool) {
	path := logFilePath(eqPath, character)
	if path == "" {
		return "", time.Time{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, false
	}
	defer f.Close() //nolint:errcheck

	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return "", time.Time{}, false
	}

	// Read backward in chunks, carrying the leading partial line forward so a
	// zone-in that straddles a chunk boundary is still parsed intact.
	var carry string
	pos := info.Size()
	var scanned int64
	for pos > 0 && scanned < lastZoneScanCap {
		readSize := int64(lastZoneChunk)
		if readSize > pos {
			readSize = pos
		}
		pos -= readSize
		scanned += readSize

		buf := make([]byte, readSize)
		if _, err := f.ReadAt(buf, pos); err != nil && !errors.Is(err, io.EOF) {
			return "", time.Time{}, false
		}

		lines := strings.Split(string(buf)+carry, "\n")
		// When not yet at BOF, lines[0] is an incomplete fragment — hold it
		// for the next (earlier) chunk rather than parsing it now.
		firstComplete := 0
		if pos > 0 {
			firstComplete = 1
			carry = lines[0]
		}
		for i := len(lines) - 1; i >= firstComplete; i-- {
			ev, ok := ParseLine(strings.TrimRight(lines[i], "\r"))
			if !ok || ev.Type != EventZone {
				continue
			}
			zd, _ := ev.Data.(ZoneData)
			if isNonZoneEnterLine(zd.ZoneName) {
				continue
			}
			return zd.ZoneName, ev.Timestamp, true
		}
	}
	return "", time.Time{}, false
}

// isNonZoneEnterLine reports whether a string captured by reZone is actually
// one of EQ's non-zone "You have entered …" system messages rather than a real
// zone name — e.g. "You have entered an area where levitation effects do not
// function." or "… an area where Bind Affinity is not allowed."
func isNonZoneEnterLine(captured string) bool {
	return strings.HasPrefix(captured, "an area where ")
}

// Package live holds the app's convention for values read from a live feed
// (the Zeal pipe, the log), so an overlay can tell a current reading from one
// the feed stopped updating.
//
// A live field travels with two siblings named after it: "<field>_at", when
// the feed last confirmed the value (not when it last changed), and
// "<field>_source", which feed it came from. For example:
//
//	"hp_percent": 62, "hp_at": "2026-11-04T20:15:03Z", "hp_source": "pipe"
//
// A feed that de-duplicates repeats still re-sends an unchanged value at least
// every Heartbeat while it keeps arriving, so "<field>_at" stays current. The
// frontend (frontend/src/lib/live.ts) holds the per-field TTLs: past its
// stale TTL a value is dimmed, past its gone TTL it's blanked.
package live

import "time"

// Sources.
const (
	SourcePipe = "pipe" // the Zeal pipe
	SourceLog  = "log"  // the EQ log
)

// Heartbeat is the longest a feed goes without re-sending a value it is
// still receiving. Frontend TTLs must stay well above it.
const Heartbeat = 2 * time.Second

// Due reports whether an unchanged value last sent at lastSent should be
// re-sent at now to refresh its "<field>_at".
func Due(lastSent, now time.Time) bool {
	return now.Sub(lastSent) >= Heartbeat
}

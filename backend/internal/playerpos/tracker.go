// Package playerpos broadcasts the player's live map position, derived from the
// Zeal named pipe, to the renderer so maps can draw a "you are here" arrow.
//
// Its only real job is deciding *when* to broadcast. The pipe delivers a player
// snapshot every tick, and forwarding each one would put a WebSocket frame on
// the wire several times a second forever, for a payload that usually has not
// changed — the app already had one performance problem from an over-eager
// broadcast path and does not need another.
package playerpos

import (
	"math"
	"sync"
	"time"
)

// State is a player position in map space, ready for the renderer to draw.
type State struct {
	// Zone is the short name, resolved from the pipe's zone id. Empty means the
	// zone is unknown, in which case the position cannot be placed on any map.
	Zone string `json:"zone"`
	// X and Y are map-space coordinates, already negated from game coordinates
	// to match the geometry pipeline (map_f1 = -game_x, map_f2 = -game_y). Done
	// here rather than in the renderer so there is exactly one place in the
	// codebase that knows the transform.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
	// Heading is EQ's 0-512 counter-clockwise value, passed through unchanged.
	Heading float64 `json:"heading"`
}

// Broadcast throttling.
const (
	// minInterval caps the broadcast rate. At 5 Hz an arrow moves smoothly at
	// running speed while costing a fraction of the frames the pipe offers.
	minInterval = 200 * time.Millisecond
	// heartbeat forces a broadcast even when nothing has changed.
	//
	// Load-bearing, not a nicety: the renderer times out a stale position and
	// hides the arrow, which is the correct response to Zeal dying. Without a
	// heartbeat, standing still is indistinguishable from a dead pipe, and the
	// arrow would vanish exactly when a player stops to fight something.
	heartbeat = 2 * time.Second
	// moveEpsilon and headingEpsilon are the smallest changes worth a frame.
	// EQ jitters position slightly while standing, and a fraction of a unit is
	// far below one screen pixel at any usable zoom.
	moveEpsilon    = 1.5
	headingEpsilon = 4 // of 512, i.e. ~2.8 degrees
)

// Tracker holds the latest position and rate-limits broadcasts.
type Tracker struct {
	mu        sync.Mutex
	cur       State
	have      bool
	lastSent  State
	lastAt    time.Time
	broadcast func(State)
	now       func() time.Time // injectable for tests
}

// New returns a Tracker that calls broadcast when a position is worth sending.
func New(broadcast func(State)) *Tracker {
	return &Tracker{broadcast: broadcast, now: time.Now}
}

// Update records a pipe player snapshot in *game* coordinates and broadcasts it
// if it is worth sending. zone is the resolved short name; an empty zone still
// updates state but never broadcasts, since an unplaceable position is worse
// than none — it would draw the arrow at the right coordinates on the wrong map.
func (t *Tracker) Update(zone string, gameX, gameY, gameZ, heading float64) {
	s := State{
		Zone: zone,
		// Same negation the geometry pipeline applies.
		X:       -gameX,
		Y:       -gameY,
		Z:       gameZ,
		Heading: heading,
	}

	t.mu.Lock()
	t.cur, t.have = s, true
	send := zone != "" && t.shouldSendLocked(s)
	if send {
		t.lastSent = s
		t.lastAt = t.now()
	}
	fn := t.broadcast
	t.mu.Unlock()

	if send && fn != nil {
		fn(s)
	}
}

// shouldSendLocked applies the rate limit. Caller holds the lock.
func (t *Tracker) shouldSendLocked(s State) bool {
	elapsed := t.now().Sub(t.lastAt)
	if elapsed < minInterval {
		return false
	}
	if elapsed >= heartbeat {
		return true
	}
	if s.Zone != t.lastSent.Zone {
		return true
	}
	if math.Hypot(s.X-t.lastSent.X, s.Y-t.lastSent.Y) >= moveEpsilon {
		return true
	}
	if math.Abs(s.Z-t.lastSent.Z) >= moveEpsilon {
		return true
	}
	return math.Abs(s.Heading-t.lastSent.Heading) >= headingEpsilon
}

// Snapshot returns the last known position.
func (t *Tracker) Snapshot() (State, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cur, t.have && t.cur.Zone != ""
}

// Reset clears the position. Called when the pipe drops, so a stale arrow does
// not linger somewhere the player no longer is — which is worse than no arrow,
// because it looks authoritative.
func (t *Tracker) Reset() {
	t.mu.Lock()
	t.cur, t.have = State{}, false
	t.lastSent, t.lastAt = State{}, time.Time{}
	t.mu.Unlock()
}

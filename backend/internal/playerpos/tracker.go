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
	// minInterval caps the broadcast rate, matched to the pipe's own default
	// cadence (~100 ms, /pipedelay) so we forward what arrives rather than
	// dropping every other frame.
	//
	// This started at 200 ms to be frugal, but halving the source rate is
	// exactly what made the arrow read as laggy next to the in-game one: the
	// game redraws its own arrow every frame, so any visible stepping is ours.
	// Going faster than the pipe would buy nothing, so this is the floor worth
	// having.
	minInterval = 100 * time.Millisecond
	// heartbeat forces a broadcast even when nothing has changed.
	//
	// Load-bearing, not a nicety: the renderer times out a stale position and
	// hides the arrow, which is the correct response to Zeal dying. Without a
	// heartbeat, standing still is indistinguishable from a dead pipe, and the
	// arrow would vanish exactly when a player stops to fight something.
	heartbeat = 2 * time.Second
	// moveEpsilon and headingEpsilon are the smallest changes worth a frame.
	// EQ jitters position slightly while standing, so some floor is needed or a
	// stationary player streams frames forever.
	//
	// Small, though: these suppress updates until the change accumulates past
	// the threshold, which converts smooth movement into a jump every few
	// frames. The old 1.5-unit floor was over a screen pixel at the zoom the
	// overlay actually uses, and 2.8 degrees of turn is plainly visible on an
	// arrow. Set below perceptibility instead of below a guess at it.
	moveEpsilon    = 0.4
	headingEpsilon = 1.5 // of 512, i.e. ~1 degree
	// raidMinInterval is the floor for raid frames, slower than the group's:
	// up to 72 raiders, nearly always one of them moving, would otherwise put
	// a full roster on the wire ten times a second.
	raidMinInterval = 250 * time.Millisecond
)

// GroupMemberInput is one groupmate's position in *game* coordinates, as it
// arrives from a Zeal MsgGroup frame. Only members Zeal resolves in the local
// player's own zone carry a position, so this is always the in-zone subset.
type GroupMemberInput struct {
	Name    string
	GameX   float64
	GameY   float64
	GameZ   float64
	Heading float64
}

// GroupMemberState is one groupmate's position in map space (same negation as
// State), ready for the renderer.
type GroupMemberState struct {
	Name    string  `json:"name"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Z       float64 `json:"z"`
	Heading float64 `json:"heading"`
}

// GroupState is the payload of the player:group_positions broadcast — the
// groupmates who are on the same map as the local player, so the renderer can
// draw a faint secondary arrow for each.
type GroupState struct {
	Zone    string             `json:"zone"`
	Members []GroupMemberState `json:"members"`
}

// RaidMemberInput is one raider's position in *game* coordinates, from a
// Zeal MsgRaid frame. Only raiders in the local player's zone carry one.
type RaidMemberInput struct {
	Name    string
	Class   int
	Group   int
	GameX   float64
	GameY   float64
	GameZ   float64
	Heading float64
}

// RaidMemberState is one raider's position in map space (same negation as
// State). Class is Zeal's 1-indexed class id and Group the raid group, 0
// for ungrouped.
type RaidMemberState struct {
	Name    string  `json:"name"`
	Class   int     `json:"class"`
	Group   int     `json:"group"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Z       float64 `json:"z"`
	Heading float64 `json:"heading"`
}

// RaidState is the payload of the player:raid_positions broadcast: the
// raiders on the local player's map, for the live map to draw.
type RaidState struct {
	Zone    string            `json:"zone"`
	Members []RaidMemberState `json:"members"`
}

// TargetState is the payload of the player:target broadcast: the local
// player's Zeal target in map space. Name "" means no target, or none whose
// position Zeal reports (it withholds it past 250 units). Type is Zeal's
// target type: 0 PC, 1 NPC, 2 NPC corpse, 3 PC corpse.
type TargetState struct {
	Zone string  `json:"zone"`
	Name string  `json:"name"`
	Type int     `json:"type"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}

// Tracker holds the latest position and rate-limits broadcasts.
type Tracker struct {
	mu        sync.Mutex
	cur       State
	have      bool
	lastSent  State
	lastAt    time.Time
	broadcast func(State)
	now       func() time.Time // injectable for tests

	groupBroadcast func(GroupState)
	lastGroupSent  GroupState
	lastGroupAt    time.Time

	raidBroadcast func(RaidState)
	lastRaidSent  RaidState
	lastRaidAt    time.Time

	targetBroadcast func(TargetState)
	lastTargetSent  TargetState
	lastTargetAt    time.Time
}

// New returns a Tracker that calls broadcast when a position is worth sending.
func New(broadcast func(State)) *Tracker {
	return &Tracker{broadcast: broadcast, now: time.Now}
}

// SetGroupBroadcast registers the sink for group-position updates. Optional —
// when unset, UpdateGroup is a no-op.
func (t *Tracker) SetGroupBroadcast(fn func(GroupState)) {
	t.mu.Lock()
	t.groupBroadcast = fn
	t.mu.Unlock()
}

// SetRaidBroadcast registers the sink for raid-position updates. Optional —
// when unset, UpdateRaid is a no-op.
func (t *Tracker) SetRaidBroadcast(fn func(RaidState)) {
	t.mu.Lock()
	t.raidBroadcast = fn
	t.mu.Unlock()
}

// SetTargetBroadcast registers the sink for target-position updates.
// Optional — when unset, UpdateTarget is a no-op.
func (t *Tracker) SetTargetBroadcast(fn func(TargetState)) {
	t.mu.Lock()
	t.targetBroadcast = fn
	t.mu.Unlock()
}

// UpdateTarget records the local player's target from a Zeal player frame,
// in *game* coordinates. known is false when there's no target or Zeal
// gave no position for it. Losing the target sends one empty frame, then
// stays quiet; a target is sent on change, and on the heartbeat.
func (t *Tracker) UpdateTarget(zone, name string, typ int, gameX, gameY, gameZ float64, known bool) {
	ts := TargetState{Zone: zone}
	if known && name != "" && zone != "" {
		ts = TargetState{Zone: zone, Name: name, Type: typ, X: -gameX, Y: -gameY, Z: gameZ}
	}

	t.mu.Lock()
	fn := t.targetBroadcast
	send := fn != nil && t.shouldSendTargetLocked(ts)
	if send {
		t.lastTargetSent = ts
		t.lastTargetAt = t.now()
	}
	t.mu.Unlock()

	if send {
		fn(ts)
	}
}

// shouldSendTargetLocked applies the rate limit to target frames. Caller
// holds the lock.
func (t *Tracker) shouldSendTargetLocked(ts TargetState) bool {
	prev := t.lastTargetSent
	if ts.Name == "" {
		// Gone: say so once.
		return prev.Name != ""
	}
	elapsed := t.now().Sub(t.lastTargetAt)
	if ts.Name != prev.Name || ts.Type != prev.Type || ts.Zone != prev.Zone {
		return true
	}
	if elapsed < minInterval {
		return false
	}
	return elapsed >= heartbeat || moved(ts.X, ts.Y, ts.Z, 0, prev.X, prev.Y, prev.Z, 0)
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

// UpdateGroup records the in-zone groupmate positions from a Zeal MsgGroup
// frame and broadcasts them if the set changed enough to be worth a frame. An
// empty member list still broadcasts once (so the renderer drops stale arrows
// when the group scatters across zones or disbands), then stays quiet.
func (t *Tracker) UpdateGroup(zone string, members []GroupMemberInput) {
	gs := GroupState{Zone: zone, Members: make([]GroupMemberState, 0, len(members))}
	for _, m := range members {
		gs.Members = append(gs.Members, GroupMemberState{
			Name:    m.Name,
			X:       -m.GameX, // same negation as State
			Y:       -m.GameY,
			Z:       m.GameZ,
			Heading: m.Heading,
		})
	}

	t.mu.Lock()
	fn := t.groupBroadcast
	send := fn != nil && t.shouldSendGroupLocked(gs)
	if send {
		t.lastGroupSent = gs
		t.lastGroupAt = t.now()
	}
	t.mu.Unlock()

	if send {
		fn(gs)
	}
}

// shouldSendGroupLocked applies the same style of rate limit as the self
// arrow: a hard floor, a heartbeat ceiling, and a between-those change test.
// Caller holds the lock.
func (t *Tracker) shouldSendGroupLocked(gs GroupState) bool {
	elapsed := t.now().Sub(t.lastGroupAt)
	if elapsed < minInterval {
		return false
	}
	if elapsed >= heartbeat {
		return true
	}
	if gs.Zone != t.lastGroupSent.Zone || len(gs.Members) != len(t.lastGroupSent.Members) {
		return true
	}
	prev := make(map[string]GroupMemberState, len(t.lastGroupSent.Members))
	for _, m := range t.lastGroupSent.Members {
		prev[m.Name] = m
	}
	for _, m := range gs.Members {
		p, ok := prev[m.Name]
		if !ok {
			return true
		}
		if moved(m.X, m.Y, m.Z, m.Heading, p.X, p.Y, p.Z, p.Heading) {
			return true
		}
	}
	return false
}

// moved reports whether a position or heading changed enough for a frame.
func moved(x, y, z, h, px, py, pz, ph float64) bool {
	return math.Hypot(x-px, y-py) >= moveEpsilon ||
		math.Abs(z-pz) >= moveEpsilon ||
		math.Abs(h-ph) >= headingEpsilon
}

// UpdateRaid records the in-zone raiders' positions from a Zeal MsgRaid
// frame and broadcasts them under the same rules as UpdateGroup, with a
// slower floor.
func (t *Tracker) UpdateRaid(zone string, members []RaidMemberInput) {
	rs := RaidState{Zone: zone, Members: make([]RaidMemberState, 0, len(members))}
	for _, m := range members {
		rs.Members = append(rs.Members, RaidMemberState{
			Name:    m.Name,
			Class:   m.Class,
			Group:   m.Group,
			X:       -m.GameX, // same negation as State
			Y:       -m.GameY,
			Z:       m.GameZ,
			Heading: m.Heading,
		})
	}

	t.mu.Lock()
	fn := t.raidBroadcast
	send := fn != nil && t.shouldSendRaidLocked(rs)
	if send {
		t.lastRaidSent = rs
		t.lastRaidAt = t.now()
	}
	t.mu.Unlock()

	if send {
		fn(rs)
	}
}

// shouldSendRaidLocked is shouldSendGroupLocked for raid frames, where a
// raider changing group also counts. Caller holds the lock.
func (t *Tracker) shouldSendRaidLocked(rs RaidState) bool {
	elapsed := t.now().Sub(t.lastRaidAt)
	if elapsed < raidMinInterval {
		return false
	}
	if elapsed >= heartbeat {
		return true
	}
	if rs.Zone != t.lastRaidSent.Zone || len(rs.Members) != len(t.lastRaidSent.Members) {
		return true
	}
	prev := make(map[string]RaidMemberState, len(t.lastRaidSent.Members))
	for _, m := range t.lastRaidSent.Members {
		prev[m.Name] = m
	}
	for _, m := range rs.Members {
		p, ok := prev[m.Name]
		if !ok || m.Group != p.Group {
			return true
		}
		if moved(m.X, m.Y, m.Z, m.Heading, p.X, p.Y, p.Z, p.Heading) {
			return true
		}
	}
	return false
}

// ResetTarget clears the target and, if one was being shown, sends one
// empty frame. Called when the pipe drops.
func (t *Tracker) ResetTarget() {
	t.mu.Lock()
	fn := t.targetBroadcast
	had := t.lastTargetSent.Name != ""
	t.lastTargetSent = TargetState{}
	t.lastTargetAt = time.Time{}
	t.mu.Unlock()
	if fn != nil && had {
		fn(TargetState{})
	}
}

// ResetRaid clears raid state and, if a sink is set, emits one empty frame
// so the renderer drops any lingering raiders. Called when the pipe drops.
func (t *Tracker) ResetRaid() {
	t.mu.Lock()
	fn := t.raidBroadcast
	had := len(t.lastRaidSent.Members) > 0
	t.lastRaidSent = RaidState{}
	t.lastRaidAt = time.Time{}
	t.mu.Unlock()
	if fn != nil && had {
		fn(RaidState{})
	}
}

// ResetGroup clears group state and, if a sink is set, emits one empty frame so
// the renderer drops any lingering arrows. Called when the pipe drops.
func (t *Tracker) ResetGroup() {
	t.mu.Lock()
	fn := t.groupBroadcast
	had := len(t.lastGroupSent.Members) > 0
	t.lastGroupSent = GroupState{}
	t.lastGroupAt = time.Time{}
	t.mu.Unlock()
	if fn != nil && had {
		fn(GroupState{})
	}
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

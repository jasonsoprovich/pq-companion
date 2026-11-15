package raidtrack

import (
	"log/slog"
	"strings"
	"time"
)

// The raid roster log records who joined and left the raid. Unlike frames,
// it isn't tied to a session: Zeal's raid frame lists every raider wherever
// they are, so joins and leaves are still caught while the observer is out
// of the raid's zone (porting out to fetch someone, dead at bind).

const (
	// leaveGrace is how long a raider has to be missing from a live roster
	// to count as having left, so a dropped frame isn't a leave.
	leaveGrace = 15 * time.Second

	// rosterReset is how long the roster can go unseen (zoning, camping,
	// leaving the raid) before it's forgotten. The next roster after that
	// is a fresh start, recorded as "present" rather than as joins.
	rosterReset = 10 * time.Minute
)

// Roster event kinds.
const (
	RosterPresent = "present" // already in the raid when the observer's roster started
	RosterJoin    = "join"    // joined the raid
	RosterLeave   = "leave"   // left the raid
)

// RosterEvent is one change to the raid's membership. Character is the
// observer whose Zeal saw it.
type RosterEvent struct {
	T         int64  `json:"t"` // unix ms
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Class     int    `json:"class"`
	Level     int    `json:"level"`
	Character string `json:"-"`
}

// rosterMember is one raider the roster log knows about.
type rosterMember struct {
	name     string
	class    int
	level    int
	lastSeen time.Time
}

// trackRosterLocked compares the latest raid roster with the known one and
// records joins and leaves. Called from tick. Caller holds r.mu.
func (r *Recorder) trackRosterLocked(now time.Time) {
	live := len(r.roster) > 0 && !r.rosterAt.IsZero() && now.Sub(r.rosterAt) <= rosterFresh
	if !live {
		if r.known != nil && (r.rosterAt.IsZero() || now.Sub(r.rosterAt) > rosterReset) {
			r.known = nil
		}
		return
	}
	fresh := r.known == nil
	if fresh {
		r.known = map[string]*rosterMember{}
	}
	for _, m := range r.roster {
		if m.Name == "" {
			continue
		}
		key := strings.ToLower(m.Name)
		if k, ok := r.known[key]; ok {
			k.lastSeen = now
			if m.Level > 0 {
				k.level = m.Level
			}
			continue
		}
		r.known[key] = &rosterMember{name: m.Name, class: m.Class, level: m.Level, lastSeen: now}
		kind := RosterJoin
		if fresh {
			kind = RosterPresent
		}
		r.writeRoster(RosterEvent{T: now.UnixMilli(), Kind: kind, Name: m.Name, Class: m.Class, Level: m.Level, Character: r.character})
	}
	for key, k := range r.known {
		if now.Sub(k.lastSeen) <= leaveGrace {
			continue
		}
		delete(r.known, key)
		r.writeRoster(RosterEvent{T: k.lastSeen.UnixMilli(), Kind: RosterLeave, Name: k.name, Class: k.class, Level: k.level, Character: r.character})
	}
}

func (r *Recorder) writeRoster(ev RosterEvent) {
	if err := r.store.addRosterEvent(ev); err != nil {
		slog.Warn("raidtrack: roster write failed", "err", err)
	}
}

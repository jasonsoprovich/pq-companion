package raidtrack

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// A raid night is a run of one character's sessions with short gaps
// between them. A session covers one zone, and ends when the observer
// zones, dies and respawns elsewhere, or ports out to fetch someone. A
// night ties those pieces back together, so a raid that moved through
// several zones, or was left for a while, plays back as one evening.

const (
	// nightGap is the longest break between two sessions of one night.
	nightGap = 45 * time.Minute

	// rosterLead is how far before a night's first session its roster log
	// is read, to catch the raid forming before it zoned in.
	rosterLead = time.Hour

	// awayGap is how long a raider has to be missing from the observer's
	// zone before it counts as having left the zone. A shorter gap is
	// interpolated over in playback (lib/raidReplay MAX_GAP_MS is 10 s).
	awayGap = 15 * time.Second

	// stillRadius and stillMin define standing still: staying within
	// stillRadius units of one spot for at least stillMin.
	stillRadius = 15.0
	stillMin    = 2 * time.Minute
)

// Derived event kinds: a raider leaving or entering the observer's zone,
// and joining or leaving the raid (from the roster log).
const (
	EventZoneOut   = "zone_out"
	EventZoneIn    = "zone_in"
	EventRaidJoin  = "raid_join"
	EventRaidLeave = "raid_leave"
)

// Night is one raid night: its sessions oldest first, and totals over them.
type Night struct {
	// ID is the first session's id.
	ID          int64     `json:"id"`
	Character   string    `json:"character"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
	Sessions    []Session `json:"sessions"`
	PeakMembers int       `json:"peak_members"`
	Kills       int       `json:"kills"`
	Deaths      int       `json:"deaths"`

	// rosterFrom and rosterTo bound the roster log read for this night
	// (unix ms), kept clear of the character's neighbouring nights.
	rosterFrom, rosterTo int64
}

// RaiderSummary is one raider's night. Times are unix ms; 0 means unknown.
type RaiderSummary struct {
	Name  string `json:"name"`
	Class int    `json:"class"`
	Level int    `json:"level"`
	// FirstSeen and LastSeen are the first and last time the raider was in
	// the roster or in the observer's zone.
	FirstSeen int64 `json:"first_seen"`
	LastSeen  int64 `json:"last_seen"`
	// JoinedAt is when they joined the raid, 0 when they were already in
	// it when the roster log started (or the night has no roster log).
	JoinedAt int64 `json:"joined_at"`
	// LeftAt is when they left the raid for good, 0 when they stayed.
	LeftAt int64 `json:"left_at"`
	// InZoneMs is the time spent in the observer's zone.
	InZoneMs int64 `json:"in_zone_ms"`
	// Away counts the times they left the observer's zone mid-session.
	Away   int `json:"away"`
	Deaths int `json:"deaths"`
	// StillMs is the time spent standing still for stillMin or longer;
	// LongestStillMs the longest single stretch. Positions are all there
	// is, so a healer holding a spot counts the same as someone AFK.
	StillMs        int64 `json:"still_ms"`
	LongestStillMs int64 `json:"longest_still_ms"`
}

// NightData is a whole night for playback.
type NightData struct {
	Night      Night       `json:"night"`
	Recordings []Recording `json:"recordings"`
	// Events are the derived events (zone_out, zone_in, raid_join,
	// raid_leave), oldest first. Each recording keeps its own events.
	Events  []Event         `json:"events"`
	Summary []RaiderSummary `json:"summary"`
	// RosterTracked is false for nights recorded before the roster log
	// existed, so joins and leaves are unknown.
	RosterTracked bool `json:"roster_tracked"`
}

// GroupNights groups sessions into nights, newest night first.
func GroupNights(ss []Session) []Night {
	sorted := append([]Session(nil), ss...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Character != sorted[j].Character {
			return sorted[i].Character < sorted[j].Character
		}
		return sorted[i].StartedAt.Before(sorted[j].StartedAt)
	})
	var nights []Night
	for _, s := range sorted {
		n := len(nights)
		if n > 0 {
			last := &nights[n-1]
			if last.Character == s.Character && s.StartedAt.Sub(last.EndedAt) <= nightGap {
				last.add(s)
				continue
			}
		}
		nights = append(nights, Night{ID: s.ID, Character: s.Character, StartedAt: s.StartedAt, EndedAt: s.EndedAt})
		nights[len(nights)-1].add(s)
	}
	// Roster windows: from rosterLead before the night to nightGap after,
	// stopping short of the character's previous and next nights.
	for i := range nights {
		n := &nights[i]
		n.rosterFrom = n.StartedAt.Add(-rosterLead).UnixMilli()
		n.rosterTo = n.EndedAt.Add(nightGap).UnixMilli()
		if i > 0 && nights[i-1].Character == n.Character {
			n.rosterFrom = max(n.rosterFrom, nights[i-1].EndedAt.UnixMilli()+1)
		}
		if i+1 < len(nights) && nights[i+1].Character == n.Character {
			n.rosterTo = min(n.rosterTo, nights[i+1].StartedAt.UnixMilli()-1)
		}
	}
	sort.SliceStable(nights, func(i, j int) bool { return nights[i].StartedAt.After(nights[j].StartedAt) })
	return nights
}

func (n *Night) add(s Session) {
	n.Sessions = append(n.Sessions, s)
	if s.EndedAt.After(n.EndedAt) {
		n.EndedAt = s.EndedAt
	}
	n.PeakMembers = max(n.PeakMembers, s.PeakMembers)
	n.Kills += s.Kills
	n.Deaths += s.Deaths
}

// ListNights returns every raid night, newest first.
func (s *Store) ListNights() ([]Night, error) {
	ss, err := s.ListSessions()
	if err != nil {
		return nil, err
	}
	return GroupNights(ss), nil
}

// GetNight loads the night whose first session is id, for playback. ok is
// false when there's no such night.
func (s *Store) GetNight(id int64) (data NightData, ok bool, err error) {
	nights, err := s.ListNights()
	if err != nil {
		return data, false, err
	}
	for _, n := range nights {
		if n.ID == id {
			data.Night, ok = n, true
			break
		}
	}
	if !ok {
		return data, false, nil
	}
	for _, ss := range data.Night.Sessions {
		rec, found, err := s.GetRecording(ss.ID)
		if err != nil {
			return data, false, fmt.Errorf("load session %d: %w", ss.ID, err)
		}
		if found {
			data.Recordings = append(data.Recordings, rec)
		}
	}
	roster, err := s.RosterBetween(data.Night.Character, data.Night.rosterFrom, data.Night.rosterTo)
	if err != nil {
		return data, false, err
	}
	data.RosterTracked = len(roster) > 0
	data.Summary, data.Events = summarize(data.Recordings, roster)
	return data, true, nil
}

// raiderTrack follows one raider through a recording's frames.
type raiderTrack struct {
	sum *RaiderSummary

	firstT, lastT int64 // presence in the current recording, 0 before any
	anchor        [3]float64
	anchorT       int64
}

// closeStill ends the raider's current still stretch at their last frame.
func (rt *raiderTrack) closeStill() {
	if rt.anchorT == 0 {
		return
	}
	if d := rt.lastT - rt.anchorT; d >= stillMin.Milliseconds() {
		rt.sum.StillMs += d
		rt.sum.LongestStillMs = max(rt.sum.LongestStillMs, d)
	}
	rt.anchorT = 0
}

// summarize builds each raider's summary and the derived events from a
// night's recordings (oldest first) and its roster log.
func summarize(recs []Recording, roster []RosterEvent) ([]RaiderSummary, []Event) {
	sums := map[string]*RaiderSummary{}
	get := func(name string, class, level int) *RaiderSummary {
		key := strings.ToLower(name)
		rs, ok := sums[key]
		if !ok {
			rs = &RaiderSummary{Name: name}
			sums[key] = rs
		}
		if class > 0 {
			rs.Class = class
		}
		if level > 0 {
			rs.Level = max(rs.Level, level)
		}
		return rs
	}
	seen := func(rs *RaiderSummary, t int64) {
		if rs.FirstSeen == 0 || t < rs.FirstSeen {
			rs.FirstSeen = t
		}
		rs.LastSeen = max(rs.LastSeen, t)
	}
	var events []Event
	gap := awayGap.Milliseconds()

	for _, rec := range recs {
		if len(rec.Frames) == 0 {
			continue
		}
		tracks := map[int]*raiderTrack{}
		for _, m := range rec.Members {
			tracks[m.Index] = &raiderTrack{sum: get(m.Name, m.Class, m.Level)}
		}
		start, end := rec.Frames[0].T, rec.Frames[len(rec.Frames)-1].T
		for _, f := range rec.Frames {
			for _, row := range f.M {
				if len(row) < 4 {
					continue
				}
				rt := tracks[row[0]]
				if rt == nil {
					continue
				}
				pos := [3]float64{float64(row[1]), float64(row[2]), float64(row[3])}
				seen(rt.sum, f.T)
				switch {
				case rt.lastT == 0:
					rt.firstT = f.T
					if f.T-start > gap {
						events = append(events, Event{T: f.T, Kind: EventZoneIn, Name: rt.sum.Name})
					}
				case f.T-rt.lastT <= gap:
					rt.sum.InZoneMs += f.T - rt.lastT
				default:
					rt.closeStill()
					rt.sum.Away++
					events = append(events,
						Event{T: rt.lastT, Kind: EventZoneOut, Name: rt.sum.Name},
						Event{T: f.T, Kind: EventZoneIn, Name: rt.sum.Name})
				}
				if rt.anchorT != 0 && dist(rt.anchor, pos) > stillRadius {
					rt.closeStill()
				}
				rt.lastT = f.T
				if rt.anchorT == 0 {
					rt.anchor, rt.anchorT = pos, f.T
				}
			}
		}
		for _, rt := range tracks {
			if rt.lastT == 0 {
				continue
			}
			rt.closeStill()
			if end-rt.lastT > gap {
				rt.sum.Away++
				events = append(events, Event{T: rt.lastT, Kind: EventZoneOut, Name: rt.sum.Name})
			}
		}
		for _, ev := range rec.Events {
			if ev.Kind == EventDeath {
				if rs, ok := sums[strings.ToLower(ev.Name)]; ok {
					rs.Deaths++
				}
			}
		}
	}

	// JoinedAt is only a raider's first roster event, so someone who was
	// there from the start, left and came back isn't shown as joining late.
	inRoster := map[string]bool{}
	for _, ev := range roster {
		rs := get(ev.Name, ev.Class, ev.Level)
		seen(rs, ev.T)
		key := strings.ToLower(ev.Name)
		first := !inRoster[key]
		inRoster[key] = true
		switch ev.Kind {
		case RosterJoin:
			if first {
				rs.JoinedAt = ev.T
			}
			rs.LeftAt = 0
			events = append(events, Event{T: ev.T, Kind: EventRaidJoin, Name: ev.Name})
		case RosterPresent:
			rs.LeftAt = 0
		case RosterLeave:
			rs.LeftAt = ev.T
			events = append(events, Event{T: ev.T, Kind: EventRaidLeave, Name: ev.Name})
		}
	}

	out := make([]RaiderSummary, 0, len(sums))
	for _, rs := range sums {
		out = append(out, *rs)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	sort.SliceStable(events, func(i, j int) bool { return events[i].T < events[j].T })
	if events == nil {
		events = []Event{}
	}
	return out, events
}

func dist(a, b [3]float64) float64 {
	dx, dy, dz := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

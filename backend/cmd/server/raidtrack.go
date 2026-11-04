package main

import (
	"strconv"

	"github.com/jasonsoprovich/pq-companion/backend/internal/raidtrack"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// raidTrackMembers converts a Zeal raid frame for the raid replay recorder.
// Raiders outside the observer's zone come through with no location or with
// (0,0,0); both are recorded as not in zone.
func raidTrackMembers(members []zealpipe.RaidMember) []raidtrack.Member {
	out := make([]raidtrack.Member, 0, len(members))
	for _, m := range members {
		rm := raidtrack.Member{Name: m.Name, Class: int(m.Class), Level: int(m.Level), HPPct: -1}
		rm.Group, _ = strconv.Atoi(m.Group)
		if l := m.Loc; l != nil && (l.X != 0 || l.Y != 0 || l.Z != 0) {
			rm.HasLoc = true
			rm.X, rm.Y, rm.Z = l.GameX(), l.GameY(), l.Z
			if m.Heading != nil {
				rm.Heading = *m.Heading
			}
		}
		if m.HPCur != nil && m.HPMax != nil && *m.HPMax > 0 {
			rm.HPPct = *m.HPCur * 100 / *m.HPMax
		}
		out = append(out, rm)
	}
	return out
}

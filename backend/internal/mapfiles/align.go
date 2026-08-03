package mapfiles

import (
	"math"
	"sort"
	"strings"
)

// Alignment reports whether a pack's map for a zone describes the same place
// the server does.
//
// This exists because a map pack is drawn for whichever EverQuest its author
// plays, and that is not this one. Project Quarm is a Luclin-era server; a
// modern pack's Bazaar is the revamped Bazaar, with a Plane of Knowledge zone
// line and trader halls named after gems — none of which exist here. The lines
// render perfectly and describe a different building.
//
// That failure is worse than a missing map, because nothing about it looks
// wrong: the drawing is clean, our own pins land where the server says they
// are, and only someone who knows the zone notices the two disagree.
type Alignment struct {
	// Landmarks is how many named points were matched between the pack and the
	// server's own data.
	Landmarks int
	// OffsetUnits is the median distance between matched pairs.
	OffsetUnits float64
	// Mismatch is true when the two disagree by more than a zone's worth.
	Mismatch bool
}

// Named is one labelled point from the server's data, in map space.
type Named struct {
	Label string
	// Category decides whether this point can be trusted as a landmark; see
	// fixedCategories.
	Category string
	X, Y     float64
}

// fixedCategories are the POI kinds that describe a fixed place.
//
// NPCs are excluded, and that exclusion is the difference between a check that
// works and one that condemns good maps. A pack labels a named mob where its
// author saw it — a camp, a patrol point, a spawn the server has since moved —
// while our coordinates come from spawn2. Measured on Brewall's set, that gap
// alone put Vex Thal 3024 units out and Temple of Veeshan 4474, on zones whose
// line work is plainly the same place. Judged on doors, zone lines and
// tradeskill containers instead, both agree to within a few units.
var fixedCategories = map[string]bool{
	"zone_line": true, "tradeskill": true, "succor": true,
	"locked": true, "switch": true, "teleport": true, "door": true,
}

// mismatchUnits is the offset above which the two maps are calling different
// places by the same name.
//
// Measured across the whole 2024-01-09 Brewall set against quarm.db, using only
// fixed landmarks: 73 zones are judgeable and their median offset is 7 units.
// Exactly two exceed 150 — bazaar at 698 and highpass at 182 — and rendering
// both side by side against our own extraction settles which is which. Highpass
// is unmistakably the same winding pass in both, so 182 is noise; the Bazaar is
// a different building entirely. The threshold therefore sits in the gap
// between them rather than at the edge of the noise.
const mismatchUnits = 400

// minLandmarks is how many matched names are needed before judging.
//
// Two can agree by chance — a generic "Bankers" in the right quarter of a city
// — and a wrong verdict here is expensive in both directions: a false mismatch
// discredits a good map, and a false pass leaves the user trusting a wrong one.
const minLandmarks = 3

// CheckAlignment compares a pack zone's labelled points against the server's,
// matching on name.
//
// Names rather than geometry because names are the only thing the two sources
// share: both label the same NPCs and zone lines, while the line work has no
// correspondence at all. Returns Landmarks 0 when there is not enough overlap
// to judge, which callers must treat as "unknown", never as "fine".
func CheckAlignment(pack *Zone, known []Named) Alignment {
	if pack == nil || len(pack.Points) == 0 || len(known) == 0 {
		return Alignment{}
	}
	byName := map[string][]Point{}
	for _, p := range pack.Points {
		if k := normalizeLabel(p.Label); k != "" {
			byName[k] = append(byName[k], p)
		}
	}

	var dists []float64
	for _, n := range known {
		if !fixedCategories[n.Category] {
			continue
		}
		k := normalizeLabel(n.Label)
		if k == "" {
			continue
		}
		cands, ok := byName[k]
		if !ok {
			continue
		}
		// Nearest candidate: a name can legitimately appear several times (four
		// bankers, two zone lines out), and pairing with the closest is the only
		// reading that does not manufacture an offset from an arbitrary choice.
		best := math.Inf(1)
		for _, c := range cands {
			d := math.Hypot(float64(c.X)-n.X, float64(c.Y)-n.Y)
			if d < best {
				best = d
			}
		}
		dists = append(dists, best)
	}
	if len(dists) < minLandmarks {
		return Alignment{}
	}
	sort.Float64s(dists)
	median := dists[len(dists)/2]
	return Alignment{
		Landmarks:   len(dists),
		OffsetUnits: median,
		Mismatch:    median > mismatchUnits,
	}
}

// normalizeLabel reduces a label to letters and digits, so "to_Shadow_Haven"
// and "to Shadow Haven" match, and returns "" for anything too short to
// identify a place.
func normalizeLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() < 5 {
		return ""
	}
	return b.String()
}

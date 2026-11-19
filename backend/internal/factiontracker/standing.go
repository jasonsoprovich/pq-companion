package factiontracker

import (
	"strconv"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Standing estimation.
//
// EQMacEmu keeps a per-character "personal" faction value, clamped to the
// faction's faction_list min_cap..max_cap (usually -2000..2000; see
// Client::UpdatePersonalFaction). What /con shows is personal plus a fixed
// per-character modifier: the faction's race modifier (faction_list.base
// plus any r<race> faction_list_mod row), class (c<class>) and deity
// (d<deity>) modifiers — see ZoneDatabase::GetFactionData and
// CalculateFaction. So Claws of Veeshan (base -800) at personal -2000 cons
// as -2800, and needs personal >= +300 to reach dubious.
//
// The tracker can't read personal faction, so it keeps a range [lo, hi]
// the value must lie in:
//   - "could not possibly get any worse/better" pins it to min_cap/max_cap
//     exactly;
//   - a clean /con narrows it to that bucket's interval (or replaces it
//     when the estimate has drifted outside — XP potions multiply faction
//     changes by an amount we can't see, so drift is expected);
//   - each change since shifts it: by the DB value when the change matched
//     a kill or quest turn-in, or widens it by up to maxStep when it didn't.

// FactionInfo is what the standing estimate needs to know about one faction
// for the active character. Modifier is the sum of the race, class and deity
// modifiers added to personal faction before /con picks a bucket.
// ModifierExact is false when the character's race, class or deity isn't
// known and the faction has a modifier for that attribute.
type FactionInfo struct {
	MinCap        int  `json:"min_cap"`
	MaxCap        int  `json:"max_cap"`
	Modifier      int  `json:"modifier"`
	ModifierExact bool `json:"modifier_exact"`
}

// FactionInfoResolver looks up FactionInfo by faction_list id for a
// character. ok=false when the faction isn't in the game DB.
type FactionInfoResolver func(characterID, factionID int) (info FactionInfo, ok bool)

// Range sources.
const (
	RangeSourceCap = "cap" // pinned by a "could not possibly get any…" line
	RangeSourceCon = "con" // set from a clean /con reading
)

// defaultMaxStep is how far one unattributed faction change may move the
// estimate before any change for the faction has been attributed. Ordinary
// kill hits are 1-20 points.
const defaultMaxStep = 25

// inferWindow is how long an attributed faction value stays usable for
// inferring the size of later unattributed changes to the same faction.
const inferWindow = 30 * time.Minute

// inferKeep bounds how many recent attributed values are kept per faction
// and direction.
const inferKeep = 5

// recentHit is one attributed faction change, kept for inference.
type recentHit struct {
	value int
	at    time.Time
}

// bucketTotalRange returns the inclusive range of total (modified) faction
// values that /con reports as bucket b, per CalculateFaction. Open ends are
// returned as very large magnitudes, ok=false for an unknown bucket.
func bucketTotalRange(b logparser.FactionBucket) (lo, hi int, ok bool) {
	const inf = 1 << 20
	switch b {
	case logparser.BucketAlly:
		return 1100, inf, true
	case logparser.BucketWarmly:
		return 750, 1099, true
	case logparser.BucketKindly:
		return 500, 749, true
	case logparser.BucketAmiable:
		return 100, 499, true
	case logparser.BucketIndifferent:
		return 0, 99, true
	case logparser.BucketApprehensive:
		return -100, -1, true
	case logparser.BucketDubious:
		return -500, -101, true
	case logparser.BucketThreatening:
		return -750, -501, true
	case logparser.BucketScowling:
		return -inf, -751, true
	}
	return 0, 0, false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// conPersonalRange converts a /con bucket into the personal-value range it
// implies for info, clamped to the faction's caps. ok=false when the
// bucket is unknown or the interval misses the caps entirely.
func conPersonalRange(b logparser.FactionBucket, info FactionInfo) (lo, hi int, ok bool) {
	tlo, thi, ok := bucketTotalRange(b)
	if !ok {
		return 0, 0, false
	}
	lo = clampInt(tlo-info.Modifier, info.MinCap, info.MaxCap)
	hi = clampInt(thi-info.Modifier, info.MinCap, info.MaxCap)
	if tlo-info.Modifier > info.MaxCap || thi-info.Modifier < info.MinCap {
		return 0, 0, false
	}
	return lo, hi, true
}

// setRange pins t's estimate to [lo, hi].
func (t *Tally) setRange(lo, hi int, source string, at time.Time) {
	t.RangeKnown = true
	t.RangeLo, t.RangeHi = lo, hi
	t.RangeSource = source
	ts := at
	t.RangeAt = &ts
}

// shiftRange moves a known estimate by an attributed change of v points.
func (t *Tally) shiftRange(v int, info FactionInfo) {
	if !t.RangeKnown {
		return
	}
	t.RangeLo = clampInt(t.RangeLo+v, info.MinCap, info.MaxCap)
	t.RangeHi = clampInt(t.RangeHi+v, info.MinCap, info.MaxCap)
}

// widenRange applies an unattributed change: at least one point in its
// direction, at most t's max step.
func (t *Tally) widenRange(better bool, info FactionInfo) {
	if !t.RangeKnown {
		return
	}
	step := t.MaxStep
	if step <= 0 {
		step = defaultMaxStep
	}
	if better {
		t.RangeLo = clampInt(t.RangeLo+1, info.MinCap, info.MaxCap)
		t.RangeHi = clampInt(t.RangeHi+step, info.MinCap, info.MaxCap)
	} else {
		t.RangeLo = clampInt(t.RangeLo-step, info.MinCap, info.MaxCap)
		t.RangeHi = clampInt(t.RangeHi-1, info.MinCap, info.MaxCap)
	}
}

// applyCon narrows a known estimate with a clean /con reading, or sets it
// from the reading when there's none yet or the estimate has drifted
// outside what the server just reported.
func (t *Tally) applyCon(b logparser.FactionBucket, info FactionInfo, at time.Time) {
	lo, hi, ok := conPersonalRange(b, info)
	if !ok {
		return
	}
	if t.RangeKnown {
		nlo, nhi := max(t.RangeLo, lo), min(t.RangeHi, hi)
		if nlo <= nhi {
			t.RangeLo, t.RangeHi = nlo, nhi
			return
		}
	}
	t.setRange(lo, hi, RangeSourceCon, at)
}

// clearRange drops the estimate.
func (t *Tally) clearRange() {
	t.RangeKnown = false
	t.RangeLo, t.RangeHi = 0, 0
	t.RangeSource = ""
	t.RangeAt = nil
}

// playerRaces and playerDeities are the server ids a player character can
// have, used to tell whether an unknown race/deity could change a faction's
// modifier.
var (
	playerRaces   = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 128, 130}
	playerDeities = []int{201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 396}
)

// ComputeInfo derives FactionInfo the way ZoneDatabase::GetFactionData
// does for a player: race modifier = base + r<race> mod (base alone when
// the race has no row), plus c<class> and d<deity> mods. race, class
// (1-based server id) and deity are 0 when unknown; an unknown attribute
// whose mods could matter makes the modifier inexact.
func ComputeInfo(base, minCap, maxCap int, mods map[string]int, race, class, deity int) FactionInfo {
	info := FactionInfo{MinCap: minCap, MaxCap: maxCap, ModifierExact: true}
	anyMod := func(prefix string, ids []int) bool {
		for _, id := range ids {
			if _, ok := mods[prefix+itoa(id)]; ok {
				return true
			}
		}
		return false
	}
	info.Modifier = base
	if race > 0 {
		info.Modifier += mods["r"+itoa(race)]
	} else if anyMod("r", playerRaces) {
		info.ModifierExact = false
	}
	if class > 0 {
		info.Modifier += mods["c"+itoa(class)]
	} else if anyMod("c", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}) {
		info.ModifierExact = false
	}
	if deity > 0 {
		info.Modifier += mods["d"+itoa(deity)]
	} else if anyMod("d", playerDeities) {
		info.ModifierExact = false
	}
	return info
}

func itoa(v int) string { return strconv.Itoa(v) }

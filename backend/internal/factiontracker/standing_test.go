package factiontracker

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

const covID = 430

// covKillResolver stubs a Kael giant: +5 Claws of Veeshan.
func covKillResolver(mobName string) ([]NPCFactionHit, bool) {
	if mobName != "a Kromzek warrior" {
		return nil, false
	}
	return []NPCFactionHit{{FactionID: covID, FactionName: "Claws of Veeshan", Value: 5}}, true
}

// covInfo mirrors quarm.db: caps -2000..2000, base -800, no player mods.
var covInfo = FactionInfo{MinCap: -2000, MaxCap: 2000, Modifier: -800, ModifierExact: true}

func newStandingEngine() *Engine {
	e := NewEngine(nil, covKillResolver)
	e.SetFactionIDResolver(func(name string) (int, bool) {
		return covID, name == "Claws of Veeshan"
	})
	e.SetPrimaryFactionResolver(func(npc string) (int, string, bool) {
		return covID, "Claws of Veeshan", npc == "a Claws of Veeshan sentry"
	})
	e.SetFactionInfoResolver(func(_, factionID int) (FactionInfo, bool) {
		return covInfo, factionID == covID
	})
	e.SetCharacter(testCharID, nil)
	return e
}

func feedCapped(tr *Engine, faction, direction string, ts time.Time) {
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventFactionChanged,
		Timestamp: ts,
		Data:      logparser.FactionChangedData{Faction: faction, Direction: direction, AtCap: true},
	})
}

func wantRange(t *testing.T, tl Tally, lo, hi int) {
	t.Helper()
	if !tl.RangeKnown || tl.RangeLo != lo || tl.RangeHi != hi {
		t.Fatalf("range = known:%v [%d, %d], want [%d, %d]", tl.RangeKnown, tl.RangeLo, tl.RangeHi, lo, hi)
	}
}

func TestStanding_CapLinePinsAndKillsShift(t *testing.T) {
	e := newStandingEngine()
	ts := time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC)

	feedCapped(e, "Claws of Veeshan", "worse", ts)
	tl := tallyFor(t, e.State(), "Claws of Veeshan")
	wantRange(t, tl, -2000, -2000)
	if tl.RangeSource != RangeSourceCap || tl.Better != 0 || tl.Worse != 0 {
		t.Fatalf("cap line: source %q better %d worse %d", tl.RangeSource, tl.Better, tl.Worse)
	}
	if tl.Info == nil || tl.Info.Modifier != -800 {
		t.Fatalf("Info not filled: %+v", tl.Info)
	}

	for i := 0; i < 3; i++ {
		at := ts.Add(time.Duration(i+1) * time.Minute)
		feedKill(e, "a Kromzek warrior", at)
		feedFactionChanged(e, "Claws of Veeshan", "better", at)
	}
	tl = tallyFor(t, e.State(), "Claws of Veeshan")
	wantRange(t, tl, -1985, -1985)
	if tl.MaxStep != 5 {
		t.Fatalf("MaxStep = %d, want 5", tl.MaxStep)
	}
}

func TestStanding_UnseenKillsInferredFromRecentHits(t *testing.T) {
	e := newStandingEngine()
	ts := time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC)
	feedCapped(e, "Claws of Veeshan", "worse", ts)
	feedKill(e, "a Kromzek warrior", ts.Add(time.Minute))
	feedFactionChanged(e, "Claws of Veeshan", "better", ts.Add(time.Minute))
	// Raid kills out of view: no kill line, same faction, same direction.
	feedFactionChanged(e, "Claws of Veeshan", "better", ts.Add(2*time.Minute))
	feedFactionChanged(e, "Claws of Veeshan", "better", ts.Add(3*time.Minute))

	tl := tallyFor(t, e.State(), "Claws of Veeshan")
	if tl.Inferred != 2 || tl.Unresolved != 0 || tl.EstimatedNet != 15 || tl.Better != 3 {
		t.Fatalf("inferred %d unresolved %d net %d better %d", tl.Inferred, tl.Unresolved, tl.EstimatedNet, tl.Better)
	}
	wantRange(t, tl, -1985, -1985)
}

func TestStanding_UnresolvedWidens(t *testing.T) {
	e := newStandingEngine()
	ts := time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC)
	feedCapped(e, "Claws of Veeshan", "worse", ts)
	feedFactionChanged(e, "Claws of Veeshan", "better", ts.Add(time.Minute))

	tl := tallyFor(t, e.State(), "Claws of Veeshan")
	wantRange(t, tl, -1999, -2000+defaultMaxStep)
	if tl.Unresolved != 1 {
		t.Fatalf("Unresolved = %d, want 1", tl.Unresolved)
	}
}

func TestStanding_ConSetsNarrowsAndCorrects(t *testing.T) {
	e := newStandingEngine()
	ts := time.Date(2026, 11, 19, 20, 0, 0, 0, time.UTC)

	// No anchor yet: threatening (-750..-501 total) => personal 50..299.
	feedConsidered(e, "a Claws of Veeshan sentry", logparser.BucketThreatening, ts)
	tl := tallyFor(t, e.State(), "Claws of Veeshan")
	wantRange(t, tl, 50, 299)
	if tl.RangeSource != RangeSourceCon {
		t.Fatalf("source = %q, want con", tl.RangeSource)
	}

	// Pinned at the floor, then a scowling /con agrees: unchanged.
	feedCapped(e, "Claws of Veeshan", "worse", ts.Add(time.Minute))
	feedConsidered(e, "a Claws of Veeshan sentry", logparser.BucketScowling, ts.Add(2*time.Minute))
	wantRange(t, tallyFor(t, e.State(), "Claws of Veeshan"), -2000, -2000)

	// The estimate says scowling but the server says dubious (e.g. an XP
	// potion multiplied the gains): the /con wins.
	feedConsidered(e, "a Claws of Veeshan sentry", logparser.BucketDubious, ts.Add(3*time.Minute))
	wantRange(t, tallyFor(t, e.State(), "Claws of Veeshan"), 300, 699)
}

func TestStanding_SuspectConIgnored(t *testing.T) {
	e := newStandingEngine()
	e.SetIllusionProvider(func() bool { return true })
	feedConsidered(e, "a Claws of Veeshan sentry", logparser.BucketThreatening, time.Now())
	if tl := tallyFor(t, e.State(), "Claws of Veeshan"); tl.RangeKnown {
		t.Fatalf("suspect /con set a range: %+v", tl)
	}
}

func TestStanding_ResetClearsRange(t *testing.T) {
	e := newStandingEngine()
	feedCapped(e, "Claws of Veeshan", "worse", time.Now())
	e.Reset()
	if tl := tallyFor(t, e.State(), "Claws of Veeshan"); tl.RangeKnown || tl.MaxStep != 0 {
		t.Fatalf("reset left range: %+v", tl)
	}
}

func TestComputeInfo(t *testing.T) {
	mods := map[string]int{"r128": -200, "c5": 50, "d203": 100}
	cases := []struct {
		name               string
		race, class, deity int
		want               int
		exact              bool
	}{
		{"all known with mods", 128, 5, 203, -800 - 200 + 50 + 100, true},
		{"race without row uses base", 1, 1, 396, -800, true},
		{"unknown deity with deity mods", 128, 5, 0, -800 - 200 + 50, false},
		{"unknown race with race mods", 0, 5, 203, -800 + 50 + 100, false},
	}
	for _, c := range cases {
		got := ComputeInfo(-800, -2000, 2000, mods, c.race, c.class, c.deity)
		if got.Modifier != c.want || got.ModifierExact != c.exact {
			t.Errorf("%s: got %+v, want modifier %d exact %v", c.name, got, c.want, c.exact)
		}
	}
	if got := ComputeInfo(-800, -2000, 2000, map[string]int{}, 0, 0, 0); !got.ModifierExact {
		t.Errorf("no mods at all should be exact: %+v", got)
	}
}

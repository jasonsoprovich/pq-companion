package factiontracker

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// koalindlResolver stubs quarm.db's npc_faction_entries for "a Koalindl"
// (npc_faction_id 61 in the real DB), matching the faction hits verified
// against real Project Quarm session logs while researching this feature.
func koalindlResolver(mobName string) ([]NPCFactionHit, bool) {
	if mobName != "a Koalindl" {
		return nil, false
	}
	return []NPCFactionHit{
		{FactionID: 341, FactionName: "Priests of Life", Value: -100},
		{FactionID: 280, FactionName: "Knights of Thunder", Value: -30},
		{FactionID: 262, FactionName: "Guards of Qeynos", Value: -50},
		{FactionID: 221, FactionName: "Bloodsabers", Value: 25},
		{FactionID: 219, FactionName: "Antonius Bayle", Value: -15},
		{FactionID: 5063, FactionName: "KoS-ME", Value: 0}, // never logged; must be skipped
	}, true
}

func feedKill(tr *Engine, target string, ts time.Time) {
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventKill,
		Timestamp: ts,
		Data:      logparser.KillData{Killer: "You", Target: target},
	})
}

func feedFactionChanged(tr *Engine, faction, direction string, ts time.Time) {
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventFactionChanged,
		Timestamp: ts,
		Data:      logparser.FactionChangedData{Faction: faction, Direction: direction},
	})
}

func tallyFor(t *testing.T, st State, faction string) Tally {
	t.Helper()
	for _, tl := range st.Tallies {
		if tl.FactionName == faction {
			return tl
		}
	}
	t.Fatalf("no tally for faction %q in %+v", faction, st.Tallies)
	return Tally{}
}

func TestKillCorrelation_EstimatesMatchDBHits(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{
		{FactionID: 341, FactionName: "Priests of Life"},
		{FactionID: 221, FactionName: "Bloodsabers"},
	})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)
	feedFactionChanged(e, "Knights of Thunder", "worse", base) // not tracked, ignored
	feedFactionChanged(e, "Guards of Qeynos", "worse", base)   // not tracked, ignored
	feedFactionChanged(e, "Bloodsabers", "better", base)
	feedFactionChanged(e, "Antonius Bayle", "worse", base) // not tracked, ignored

	st := e.State()
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 1 || pol.EstimatedNet != -100 || pol.Unresolved != 0 {
		t.Errorf("Priests of Life = %+v, want Worse=1 EstimatedNet=-100 Unresolved=0", pol)
	}
	blood := tallyFor(t, st, "Bloodsabers")
	if blood.Better != 1 || blood.EstimatedNet != 25 || blood.Unresolved != 0 {
		t.Errorf("Bloodsabers = %+v, want Better=1 EstimatedNet=25 Unresolved=0", blood)
	}
}

func TestFactionChanged_NotTracked_NoTally(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 341, FactionName: "Priests of Life"}})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Bloodsabers", "better", base)

	st := e.State()
	if len(st.Tallies) != 1 {
		t.Fatalf("Tallies = %+v, want exactly 1 (only Priests of Life tracked)", st.Tallies)
	}
}

func TestFactionChanged_NoKillContext_CountsAsUnresolved(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 100, FactionName: "Silent Fist Clan"}})

	ts := time.Date(2025, 10, 19, 18, 55, 54, 0, time.Local)
	// Quest/hail-triggered faction change — no preceding kill, so no DB
	// correlation is possible, mirroring the "Silent Fist Clan" induction
	// line observed in a real session log with no kill line before it.
	feedFactionChanged(e, "Silent Fist Clan", "better", ts)

	st := e.State()
	sfc := tallyFor(t, st, "Silent Fist Clan")
	if sfc.Better != 1 || sfc.Unresolved != 1 || sfc.EstimatedNet != 0 {
		t.Errorf("Silent Fist Clan = %+v, want Better=1 Unresolved=1 EstimatedNet=0", sfc)
	}
}

func TestKillCorrelation_ExpiresOutsideWindow(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 341, FactionName: "Priests of Life"}})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	// Arrives well after correlationWindow — must NOT be attributed to the
	// stale kill (guards against a kill's estimate leaking into an unrelated
	// later change of the same faction, e.g. a quest turn-in).
	late := base.Add(correlationWindow + time.Second)
	feedFactionChanged(e, "Priests of Life", "worse", late)

	st := e.State()
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 1 || pol.EstimatedNet != 0 || pol.Unresolved != 1 {
		t.Errorf("Priests of Life = %+v, want Worse=1 EstimatedNet=0 Unresolved=1 (stale kill must not match)", pol)
	}
}

func TestKillCorrelation_RepeatedKillsEachConsumeOwnHit(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 341, FactionName: "Priests of Life"}})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	// Four back-to-back kills of the same NPC, as observed in a real
	// session log, each producing its own faction line moments later.
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)
	feedKill(e, "a Koalindl", base.Add(66*time.Second))
	feedFactionChanged(e, "Priests of Life", "worse", base.Add(66*time.Second))
	feedKill(e, "a Koalindl", base.Add(2*66*time.Second))
	feedFactionChanged(e, "Priests of Life", "worse", base.Add(2*66*time.Second))

	st := e.State()
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 3 || pol.EstimatedNet != -300 || pol.Unresolved != 0 {
		t.Errorf("Priests of Life = %+v, want Worse=3 EstimatedNet=-300 Unresolved=0", pol)
	}
}

func TestSetTracked_PreservesTallyForStillTrackedFaction(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{
		{FactionID: 341, FactionName: "Priests of Life"},
		{FactionID: 221, FactionName: "Bloodsabers"},
	})
	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)

	// Drop Bloodsabers, keep Priests of Life — its tally must survive.
	e.SetTracked([]TrackedFaction{{FactionID: 341, FactionName: "Priests of Life"}})

	st := e.State()
	if len(st.Tallies) != 1 {
		t.Fatalf("Tallies = %+v, want exactly 1 after dropping Bloodsabers", st.Tallies)
	}
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 1 || pol.EstimatedNet != -100 {
		t.Errorf("Priests of Life = %+v, want Worse=1 EstimatedNet=-100 (preserved across SetTracked)", pol)
	}
}

func TestReset_ZeroesTalliesKeepsTrackedSet(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 341, FactionName: "Priests of Life"}})
	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)

	e.Reset()

	st := e.State()
	if len(st.Tallies) != 1 {
		t.Fatalf("Tallies = %+v, want faction still tracked after Reset", st.Tallies)
	}
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 0 || pol.EstimatedNet != 0 || pol.Unresolved != 0 {
		t.Errorf("Priests of Life = %+v, want all-zero after Reset", pol)
	}
}

func TestUnresolvableKill_NoResolverMatch_StillTalliesDirectionOnly(t *testing.T) {
	e := NewEngine(nil, koalindlResolver)
	e.SetTracked([]TrackedFaction{{FactionID: 1, FactionName: "Some Other Faction"}})

	ts := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "an unresolvable mob", ts) // koalindlResolver returns ok=false
	feedFactionChanged(e, "Some Other Faction", "better", ts)

	st := e.State()
	sof := tallyFor(t, st, "Some Other Faction")
	if sof.Better != 1 || sof.EstimatedNet != 0 || sof.Unresolved != 1 {
		t.Errorf("Some Other Faction = %+v, want Better=1 EstimatedNet=0 Unresolved=1", sof)
	}
}

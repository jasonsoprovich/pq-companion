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

const testCharID = 1

func newEngine() *Engine {
	e := NewEngine(nil, koalindlResolver)
	e.SetCharacter(testCharID, nil)
	return e
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

// herladTelchaDialogueResolver stubs the Herald Telcha quest-script dialogue
// extracted into quest_sources.json — matches the real Green Goblin Skin
// turn-in branch verified against the live quest scripts repo.
func heraldTelchaDialogueResolver(npcName, text string) ([]NPCFactionHit, bool) {
	if npcName != "Herald Telcha" || text != "Green Goblin Skin! You have indeed been busy!" {
		return nil, false
	}
	return []NPCFactionHit{
		{FactionID: 451, FactionName: "Brood of Di`Zok", Value: 3},
		{FactionID: 307, FactionName: "Sarnak Collective", Value: 3},
		{FactionID: 259, FactionName: "Goblins of Mountain Death", Value: -1},
	}, true
}

func feedNPCDialogue(tr *Engine, npc, text string, ts time.Time) {
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventNPCDialogue,
		Timestamp: ts,
		Data:      logparser.NPCDialogueData{NPCName: npc, Text: text},
	})
}

func feedConsidered(tr *Engine, npc string, bucket logparser.FactionBucket, ts time.Time) {
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventConsidered,
		Timestamp: ts,
		Data:      logparser.ConsideredData{TargetName: npc, Bucket: bucket},
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
	e := newEngine()

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)
	feedFactionChanged(e, "Knights of Thunder", "worse", base)
	feedFactionChanged(e, "Guards of Qeynos", "worse", base)
	feedFactionChanged(e, "Bloodsabers", "better", base)
	feedFactionChanged(e, "Antonius Bayle", "worse", base)

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

// TestQuestDialogueCorrelation_EstimatesMatchScriptDeltas verifies quest
// turn-in faction changes get the exact numeric delta from the quest
// script's own Faction() calls (via SetQuestDialogueResolver), the same way
// TestKillCorrelation_EstimatesMatchDBHits verifies kill-driven changes get
// the exact delta from npc_faction_entries.
func TestQuestDialogueCorrelation_EstimatesMatchScriptDeltas(t *testing.T) {
	e := newEngine()
	e.SetQuestDialogueResolver(heraldTelchaDialogueResolver)

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedNPCDialogue(e, "Herald Telcha", "Green Goblin Skin! You have indeed been busy!", base)
	feedFactionChanged(e, "Brood of Di`Zok", "better", base)
	feedFactionChanged(e, "Sarnak Collective", "better", base)
	feedFactionChanged(e, "Goblins of Mountain Death", "worse", base)

	st := e.State()
	brood := tallyFor(t, st, "Brood of Di`Zok")
	if brood.Better != 1 || brood.EstimatedNet != 3 || brood.Unresolved != 0 {
		t.Errorf("Brood of Di`Zok = %+v, want Better=1 EstimatedNet=3 Unresolved=0", brood)
	}
	goblins := tallyFor(t, st, "Goblins of Mountain Death")
	if goblins.Worse != 1 || goblins.EstimatedNet != -1 || goblins.Unresolved != 0 {
		t.Errorf("Goblins of Mountain Death = %+v, want Worse=1 EstimatedNet=-1 Unresolved=0", goblins)
	}
}

// TestQuestDialogueCorrelation_UnmatchedDialogue_NoResolverMatch verifies an
// ordinary say-line (no quest match) leaves a following faction change
// direction-only, same as an unresolvable kill.
func TestQuestDialogueCorrelation_UnmatchedDialogue_NoResolverMatch(t *testing.T) {
	e := newEngine()
	e.SetQuestDialogueResolver(heraldTelchaDialogueResolver)

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedNPCDialogue(e, "Herald Telcha", "Hail to you, lesser being!", base)
	feedFactionChanged(e, "Brood of Di`Zok", "better", base)

	st := e.State()
	brood := tallyFor(t, st, "Brood of Di`Zok")
	if brood.Better != 1 || brood.EstimatedNet != 0 || brood.Unresolved != 1 {
		t.Errorf("Brood of Di`Zok = %+v, want Better=1 EstimatedNet=0 Unresolved=1", brood)
	}
}

// TestBackfillHandler_ReplaysLogIntoMergeFunc checks the backfill handler
// wiring: events fed through HandleEvent flow through a fresh Engine (same
// kill/quest-dialogue resolution as live tracking) and the final tallies
// reach the merge callback at Finalize, exactly once per faction.
func TestBackfillHandler_ReplaysLogIntoMergeFunc(t *testing.T) {
	merged := map[string]Tally{}
	h := NewBackfillHandler(koalindlResolver, nil, nil, nil, func(tally Tally) (bool, error) {
		merged[tally.FactionName] = tally
		return true, nil
	})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	h.HandleEvent(logparser.LogEvent{Type: logparser.EventKill, Timestamp: base, Data: logparser.KillData{Killer: "You", Target: "a Koalindl"}})
	h.HandleEvent(logparser.LogEvent{Type: logparser.EventFactionChanged, Timestamp: base, Data: logparser.FactionChangedData{Faction: "Priests of Life", Direction: "worse"}})
	h.Finalize()

	if h.Inserted() != 1 {
		t.Fatalf("Inserted() = %d, want 1", h.Inserted())
	}
	pol, ok := merged["Priests of Life"]
	if !ok {
		t.Fatal("expected a merged tally for Priests of Life")
	}
	if pol.Worse != 1 || pol.EstimatedNet != -100 {
		t.Errorf("Priests of Life = %+v, want Worse=1 EstimatedNet=-100", pol)
	}
}

// TestFactionChanged_AnyFaction_CreatesTally is the core Phase 3 behavior
// change: a faction that was never pinned/seeded still gets a tally entry
// the moment a "got better/worse" line names it, mirroring how the Lockout
// and Player trackers record everything encountered rather than only
// user-curated entries.
func TestFactionChanged_AnyFaction_CreatesTally(t *testing.T) {
	e := newEngine()
	e.SetFactionIDResolver(func(name string) (int, bool) {
		if name == "Guards of Qeynos" {
			return 262, true
		}
		return 0, false
	})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Guards of Qeynos", "worse", base) // never pinned, never seeded

	st := e.State()
	gq := tallyFor(t, st, "Guards of Qeynos")
	if gq.Worse != 1 || gq.EstimatedNet != -50 || gq.FactionID != 262 {
		t.Errorf("Guards of Qeynos = %+v, want Worse=1 EstimatedNet=-50 FactionID=262", gq)
	}
}

func TestFactionChanged_NoKillContext_CountsAsUnresolved(t *testing.T) {
	e := newEngine()

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
	e := newEngine()

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
	e := newEngine()

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

// TestSetCharacter_SeedsFromPersistedTallies simulates what main.go does on
// every character switch: load every persisted tally for the new character
// (not filtered by wishlist) and hand it to SetCharacter as the seed set.
func TestSetCharacter_SeedsFromPersistedTallies(t *testing.T) {
	e := newEngine()
	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)
	feedFactionChanged(e, "Bloodsabers", "better", base)

	persisted := e.State().Tallies

	// Switch to a different character, seeding from what would have been
	// loaded from user.db for them (here, just Priests of Life's row).
	var polSeed Tally
	for _, t := range persisted {
		if t.FactionName == "Priests of Life" {
			polSeed = t
		}
	}
	e.SetCharacter(2, []Tally{polSeed})

	st := e.State()
	if len(st.Tallies) != 1 {
		t.Fatalf("Tallies = %+v, want exactly 1 after switching character", st.Tallies)
	}
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 1 || pol.EstimatedNet != -100 {
		t.Errorf("Priests of Life = %+v, want Worse=1 EstimatedNet=-100 (seeded from persisted tally)", pol)
	}
}

func TestReset_ZeroesTalliesKeepsThemRecorded(t *testing.T) {
	e := newEngine()
	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)

	var cleared int
	e.SetClearPersistedFunc(func(characterID int) { cleared = characterID })
	e.Reset()

	if cleared != testCharID {
		t.Errorf("ClearPersistedFunc called with characterID=%d, want %d", cleared, testCharID)
	}
	st := e.State()
	pol := tallyFor(t, st, "Priests of Life")
	if pol.Worse != 0 || pol.EstimatedNet != 0 || pol.Unresolved != 0 || pol.LastBucket != "" {
		t.Errorf("Priests of Life = %+v, want all-zero after Reset", pol)
	}
}

func TestUnresolvableKill_NoResolverMatch_StillTalliesDirectionOnly(t *testing.T) {
	e := newEngine()

	ts := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "an unresolvable mob", ts) // koalindlResolver returns ok=false
	feedFactionChanged(e, "Some Other Faction", "better", ts)

	st := e.State()
	sof := tallyFor(t, st, "Some Other Faction")
	if sof.Better != 1 || sof.EstimatedNet != 0 || sof.Unresolved != 1 {
		t.Errorf("Some Other Faction = %+v, want Better=1 EstimatedNet=0 Unresolved=1", sof)
	}
}

func TestPersistFunc_CalledOnEveryMutation(t *testing.T) {
	e := newEngine()

	var gotCharID int
	var gotTally Tally
	calls := 0
	e.SetPersistFunc(func(characterID int, tally Tally) {
		calls++
		gotCharID = characterID
		gotTally = tally
	})

	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base)
	feedFactionChanged(e, "Priests of Life", "worse", base)

	if calls != 1 {
		t.Fatalf("PersistFunc called %d times, want 1", calls)
	}
	if gotCharID != testCharID {
		t.Errorf("PersistFunc characterID = %d, want %d", gotCharID, testCharID)
	}
	if gotTally.Worse != 1 || gotTally.EstimatedNet != -100 {
		t.Errorf("PersistFunc tally = %+v, want Worse=1 EstimatedNet=-100", gotTally)
	}
}

func TestConsidered_CreatesTallyViaPrimaryResolver(t *testing.T) {
	e := newEngine()
	e.SetPrimaryFactionResolver(func(npcName string) (int, string, bool) {
		if npcName == "a priest" {
			return 341, "Priests of Life", true
		}
		return 0, "", false
	})

	ts := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedConsidered(e, "a priest", logparser.BucketDubious, ts)

	pol := tallyFor(t, e.State(), "Priests of Life")
	if pol.LastBucket != string(logparser.BucketDubious) || pol.LastConsideredAt == nil || !pol.LastConsideredAt.Equal(ts) {
		t.Errorf("Priests of Life = %+v, want LastBucket=dubious LastConsideredAt=%v", pol, ts)
	}
	if pol.FactionID != 341 {
		t.Errorf("Priests of Life FactionID = %d, want 341 (from primary resolver)", pol.FactionID)
	}
	if pol.LastConsiderSuspect {
		t.Error("LastConsiderSuspect = true, want false (no illusion provider set)")
	}
}

func TestConsidered_FlaggedSuspectWhenIllusioned(t *testing.T) {
	e := newEngine()
	e.SetPrimaryFactionResolver(func(npcName string) (int, string, bool) { return 341, "Priests of Life", true })
	e.SetIllusionProvider(func() bool { return true })

	ts := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedConsidered(e, "a priest", logparser.BucketAlly, ts)

	pol := tallyFor(t, e.State(), "Priests of Life")
	if !pol.LastConsiderSuspect {
		t.Error("LastConsiderSuspect = false, want true (illusion provider reports illusioned)")
	}
}

func TestConsidered_UnresolvableNPC_NoTallyCreated(t *testing.T) {
	e := newEngine()
	e.SetPrimaryFactionResolver(func(npcName string) (int, string, bool) { return 0, "", false })

	ts := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedConsidered(e, "an unresolvable npc", logparser.BucketAlly, ts)

	st := e.State()
	if len(st.Tallies) != 0 {
		t.Fatalf("Tallies = %+v, want empty (resolver could not place the NPC)", st.Tallies)
	}
}

func TestFactionIDResolver_FillsInIDOnFirstSight(t *testing.T) {
	e := newEngine()
	e.SetFactionIDResolver(func(name string) (int, bool) {
		if name == "Silent Fist Clan" {
			return 100, true
		}
		return 0, false
	})

	ts := time.Date(2025, 10, 19, 18, 55, 54, 0, time.Local)
	feedFactionChanged(e, "Silent Fist Clan", "better", ts)

	sfc := tallyFor(t, e.State(), "Silent Fist Clan")
	if sfc.FactionID != 100 {
		t.Errorf("Silent Fist Clan FactionID = %d, want 100 (from FactionIDResolver)", sfc.FactionID)
	}
}

func TestSetCharacter_ClearsPendingAcrossCharacters(t *testing.T) {
	e := newEngine()
	base := time.Date(2025, 10, 19, 18, 58, 50, 0, time.Local)
	feedKill(e, "a Koalindl", base) // pending correlation for character 1

	e.SetCharacter(2, nil) // switch character — pending must not carry over

	feedFactionChanged(e, "Priests of Life", "worse", base.Add(time.Second))
	pol := tallyFor(t, e.State(), "Priests of Life")
	if pol.EstimatedNet != 0 || pol.Unresolved != 1 {
		t.Errorf("Priests of Life = %+v, want EstimatedNet=0 Unresolved=1 (stale pending kill must not leak across characters)", pol)
	}
}

package spelltimer

import (
	"strconv"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const tepidDeedsID = 185 // ENC slow, shares nothing with Sha's but used as a second candidate

func shasLand(target string) logparser.SpellLandedData {
	return logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellID:    shasAdvantageID,
		SpellName:  shasAdvantageName,
		TargetName: target,
	}
}

// ambiguousLand is a land text two spells share, so the parser can't name
// the spell.
func ambiguousLand(target string) logparser.SpellLandedData {
	return logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		TargetName: target,
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: shasAdvantageID, SpellName: shasAdvantageName},
			{SpellID: tepidDeedsID, SpellName: "Tepid Deeds"},
		},
	}
}

func landEvent(seq uint64, data logparser.SpellLandedData) logparser.LogEvent {
	return logparser.LogEvent{Type: logparser.EventSpellLanded, Seq: seq, Data: data}
}

// landLive runs one land line through the batch the way the tailer does:
// prime, the parsed pipeline, end of batch.
func landLive(e *Engine, seq uint64, data logparser.SpellLandedData) {
	e.PrimeLands([]logparser.LogEvent{landEvent(seq, data)})
	e.onSpellLandedSeq(time.Now(), seq, data)
	e.EndBatch()
}

func tagged(name, target string, id int) string {
	return timerKey(name, target+targetIDKeySep+strconv.Itoa(id))
}

// Two same-named mobs each slowed by someone else get their own spawn-tagged
// timers, and each clears on its own death.
func TestSpellAction_NamesakesClearOnOwnDeath(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	landLive(e, 1, shasLand("a bloodguard"))
	e.HandlePipeSpellAction(shasAdvantageID, 50, 702, "a bloodguard", false)
	landLive(e, 2, shasLand("a bloodguard"))

	for _, id := range []int{701, 702} {
		if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", id)]; !ok {
			t.Fatalf("expected #%d's timer, got %v", id, keysOf(e.timers))
		}
	}
	e.HandlePipeDeath(701, "a bloodguard")
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 701)]; ok {
		t.Error("expected #701 cleared")
	}
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 702)]; !ok {
		t.Error("expected #702 kept")
	}
}

// The attempt and success packets of one cast are one entry, and a success
// arriving after the entry was used doesn't make it usable again.
func TestSpellAction_SuccessMergesIntoAttempt(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	if len(e.spellActions) != 1 || !e.spellActions[0].success {
		t.Fatalf("expected one success entry, got %d", len(e.spellActions))
	}

	e2 := spawnIDEngine(t)
	e2.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	landLive(e2, 1, shasLand("a bloodguard"))
	e2.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	if len(e2.spellActions) != 1 || !e2.spellActions[0].consumed {
		t.Fatalf("success after the land must merge into the consumed entry")
	}
	e2.PrimeLands([]logparser.LogEvent{landEvent(2, shasLand("a bloodguard"))})
	if _, ok := e2.landCorr[2]; ok {
		t.Error("a second land must not reuse the consumed cast")
	}
	e2.EndBatch()
}

// A resist on one mob and a landed cast on its namesake: the success flag
// says which one the land line is for.
func TestSpellAction_SuccessBreaksTie(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false) // resisted
	e.HandlePipeSpellAction(shasAdvantageID, 50, 702, "a bloodguard", false)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 702, "a bloodguard", true)
	landLive(e, 1, shasLand("a bloodguard"))
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 702)]; !ok {
		t.Fatalf("expected #702's timer, got %v", keysOf(e.timers))
	}
	if e.spellActions[0].consumed {
		t.Error("the resisted cast must stay unconsumed")
	}
}

// Two namesakes with only bystander attempts can't be told apart: no tag,
// nothing consumed, the timer is built as before.
func TestSpellAction_AmbiguousLeavesUntagged(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	e.HandlePipeSpellAction(shasAdvantageID, 51, 702, "a bloodguard", false)
	landLive(e, 1, shasLand("a bloodguard"))
	if _, ok := e.timers[timerKey(shasAdvantageName, "a bloodguard")]; !ok {
		t.Fatalf("expected the untagged timer, got %v", keysOf(e.timers))
	}
	for _, a := range e.spellActions {
		if a.consumed {
			t.Error("an ambiguous land must consume nothing")
		}
	}
}

// The packet's spell id settles a land text two spells share, even with no
// cast of our own to go by.
func TestSpellAction_ResolvesAmbiguousSpell(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 51, 701, "a bloodguard", false)
	landLive(e, 1, ambiguousLand("a bloodguard"))
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 701)]; !ok {
		t.Fatalf("expected Sha's Advantage on #701, got %v", keysOf(e.timers))
	}
}

// Matches that agree on the target but not on the spell, or not on the
// caster, tag the target and leave the rest to the log path.
func TestSpellAction_FieldsSetOnlyWhenAgreed(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	e.HandlePipeSpellAction(tepidDeedsID, 51, 701, "a bloodguard", false)
	e.mu.Lock()
	c, found := e.correlateLandLocked(ambiguousLand("a bloodguard"), time.Now())
	e.mu.Unlock()
	if !found || c.targetID != 701 || c.spellID != 0 || c.spellName != "" || c.sourceID != 0 {
		t.Fatalf("got %+v found=%v", c, found)
	}
}

// Our own spawn id as the source makes the cast ours, with no cast-start
// line, so cast_by_me scope keeps it; another caster's doesn't count.
func TestSpellAction_SourceSetsCastByMe(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeCastByMe }, nil, nil, nil, nil, nil)
	e.SetSelfSpawnID(intPtr(50))

	e.HandlePipeSpellAction(shasAdvantageID, 51, 702, "a bloodguard", true)
	landLive(e, 1, shasLand("a bloodguard"))
	if len(e.timers) != 0 {
		t.Fatalf("another caster's slow must be dropped under cast_by_me, got %v", keysOf(e.timers))
	}
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	landLive(e, 2, shasLand("a bloodguard"))
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 701)]; !ok {
		t.Fatalf("expected our own slow kept and tagged, got %v", keysOf(e.timers))
	}
}

// The packet's target beats a conflicting id from the cast-history
// heuristic.
func TestSpellAction_PacketBeatsHeuristic(t *testing.T) {
	e := spawnIDEngine(t)
	settlePipeTarget(e, "a bloodguard", 801)
	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellCast,
		Data: logparser.SpellCastData{SpellName: shasAdvantageName},
	})
	e.HandlePipeSpellAction(shasAdvantageID, 50, 802, "a bloodguard", true)
	landLive(e, 1, shasLand("a bloodguard"))
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 802)]; !ok {
		t.Fatalf("expected the packet's #802, got %v", keysOf(e.timers))
	}
}

// Trigger timers started while their line is matched read the same answer:
// several triggers on one line, without consuming again; a trigger for a
// different candidate spell gets no packet tag; an unrelated custom trigger
// is untouched.
func TestSpellAction_TriggersShareLineCorrelation(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", false)
	data := ambiguousLand("a bloodguard")
	e.PrimeLands([]logparser.LogEvent{landEvent(7, data)})
	now := time.Now()
	e.SetLineSeq(7)
	e.StartExternal("Slow A", "debuff", 60, 0, now, nil, shasAdvantageID, "a bloodguard", "", false, "", false)
	e.StartExternal("Slow B", "debuff", 60, 0, now, nil, shasAdvantageID, "a bloodguard", "", false, "", false)
	e.StartExternal("Tepid", "debuff", 60, 0, now, nil, tepidDeedsID, "a bloodguard", "", false, "", false)
	e.StartExternal("Custom", "custom", 60, 0, now, nil, 0, "", "", false, "", false)
	e.SetLineSeq(0)
	// Slow B is the same spell as Slow A, so StartExternal's own dedup folds
	// it into Slow A as before; what matters is that reading the line again
	// didn't consume anything or drop the answer.
	if c, ok := e.landCorr[7]; !ok || c.targetID != 701 {
		t.Errorf("line 7's answer changed after several triggers: %+v ok=%v", c, ok)
	}
	if len(e.spellActions) != 1 || !e.spellActions[0].consumed {
		t.Error("expected the one action consumed once")
	}
	for _, k := range []string{
		tagged("Slow A", "a bloodguard", 701),
		timerKey("Tepid", "a bloodguard"),
		timerKey("Custom", ""),
	} {
		if _, ok := e.timers[k]; !ok {
			t.Errorf("expected %q, got %v", k, keysOf(e.timers))
		}
	}
	// The parsed land then reads the same answer (absorbing its trigger
	// twins, as before).
	e.onSpellLandedSeq(now, 7, data)
	e.EndBatch()
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 701)]; !ok {
		t.Errorf("expected the landed timer on #701, got %v", keysOf(e.timers))
	}
}

// Two lands on namesakes in one batch: each line gets its own answer, and
// trigger and parsed paths agree on both.
func TestSpellAction_SameSecondNamesakes(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	e.HandlePipeSpellAction(shasAdvantageID, 51, 702, "a bloodguard", false)
	d := shasLand("a bloodguard")
	e.PrimeLands([]logparser.LogEvent{landEvent(1, d), landEvent(2, d)})
	now := time.Now()
	for seq, id := range map[uint64]int{1: 701, 2: 702} {
		e.SetLineSeq(seq)
		e.StartExternal("Slow "+strconv.Itoa(id), "debuff", 60, 0, now, nil, shasAdvantageID, "a bloodguard", "", false, "", false)
	}
	e.SetLineSeq(0)
	for _, id := range []int{701, 702} {
		name := "Slow " + strconv.Itoa(id)
		if _, ok := e.timers[tagged(name, "a bloodguard", id)]; !ok {
			t.Errorf("expected %s on #%d, got %v", name, id, keysOf(e.timers))
		}
	}
	e.onSpellLandedSeq(now, 1, d)
	e.onSpellLandedSeq(now, 2, d)
	e.EndBatch()
	// Same log second, two spawns: both landed timers survive.
	for _, id := range []int{701, 702} {
		if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", id)]; !ok {
			t.Errorf("expected the landed timer on #%d, got %v", id, keysOf(e.timers))
		}
	}
}

// An action arriving between the trigger and parsed phases doesn't change
// the line's answer.
func TestSpellAction_LateActionDoesNotChangeLine(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	d := shasLand("a bloodguard")
	e.PrimeLands([]logparser.LogEvent{landEvent(1, d)})
	e.HandlePipeSpellAction(shasAdvantageID, 50, 702, "a bloodguard", true)
	e.onSpellLandedSeq(time.Now(), 1, d)
	e.EndBatch()
	if _, ok := e.timers[tagged(shasAdvantageName, "a bloodguard", 701)]; !ok {
		t.Fatalf("expected #701, got %v", keysOf(e.timers))
	}
}

// A zone change earlier in the batch drops the old zone's actions before the
// land is matched; a stale action is ignored too.
func TestSpellAction_ZoneAndStaleIgnored(t *testing.T) {
	e := spawnIDEngine(t)
	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	e.PrimeLands([]logparser.LogEvent{
		{Type: logparser.EventZone, Seq: 1, Data: logparser.ZoneData{ZoneName: "Somewhere"}},
		landEvent(2, shasLand("a bloodguard")),
	})
	if _, ok := e.landCorr[2]; ok {
		t.Error("an old zone's action must not match")
	}
	e.EndBatch()

	e.HandlePipeSpellAction(shasAdvantageID, 50, 702, "a bloodguard", true)
	e.spellActions[len(e.spellActions)-1].at = time.Now().Add(-spellActionTTL - time.Second)
	e.PrimeLands([]logparser.LogEvent{landEvent(3, shasLand("a bloodguard"))})
	if _, ok := e.landCorr[3]; ok {
		t.Error("a stale action must not match")
	}
	e.EndBatch()
}

// A big batch keeps every line's answer until the batch is done, and replay
// lines (Seq 0) are never correlated.
func TestSpellAction_BatchLifetimeAndReplay(t *testing.T) {
	e := spawnIDEngine(t)
	var evs []logparser.LogEvent
	for i := 1; i <= 600; i++ {
		name := "mob" + strconv.Itoa(i)
		if i <= 200 {
			e.HandlePipeSpellAction(shasAdvantageID, 50, 1000+i, name, true)
		}
		evs = append(evs, landEvent(uint64(i), shasLand(name)))
	}
	e.PrimeLands(evs)
	if len(e.landCorr) == 0 {
		t.Fatal("expected correlations")
	}
	// The first lines' actions fell out of the 256 cap only if more than 256
	// were sent; all 200 fit, so line 1 must still be answered.
	if c, ok := e.landCorr[1]; !ok || c.targetID != 1001 {
		t.Errorf("line 1: got %+v ok=%v", c, ok)
	}
	e.EndBatch()
	if e.landCorr != nil {
		t.Error("EndBatch must drop the batch's answers")
	}

	e.HandlePipeSpellAction(shasAdvantageID, 50, 701, "a bloodguard", true)
	e.PrimeLands([]logparser.LogEvent{landEvent(0, shasLand("a bloodguard"))})
	if len(e.landCorr) != 0 || e.spellActions[len(e.spellActions)-1].consumed {
		t.Error("a replayed line must not be correlated")
	}
	e.EndBatch()
}

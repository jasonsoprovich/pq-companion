package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// The five item clickies that share the "Your eyes tingle." land text. With no
// "begin casting" line, soleClickableCandidate can't pick one — unless the
// owned-inventory provider narrows it to the single clicky the player carries.
func eyesTingleCandidates() []logparser.SpellLandedCandidate {
	return []logparser.SpellLandedCandidate{
		{SpellID: 46, SpellName: "Ultravision"},
		{SpellID: 79, SpellName: "Spirit Sight"},
		{SpellID: 80, SpellName: "See Invisible"},
		{SpellID: 276, SpellName: "Serpent Sight"},
		{SpellID: 539, SpellName: "Chill Sight"},
		{SpellID: 1575, SpellName: "Acumen"},
		{SpellID: 2248, SpellName: "Acumen"},
		{SpellID: 2886, SpellName: "Acumen of Dar Khura"},
	}
}

// ownershipEngine builds a DB-backed engine whose owned-items provider returns
// the given item IDs (so we can simulate carrying the Sigil Earring of Veracity
// = item 29861, clickeffect Acumen 2248).
func ownershipEngine(t *testing.T, ownedItems []int) *Engine {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Vortikai", -1 }
	return NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil,
		func() []int { return ownedItems }, nil)
}

func TestSoleClickableCandidate_NarrowsByOwnership(t *testing.T) {
	// Carrying the Sigil Earring (29861 -> Acumen 2248) resolves the collision.
	e := ownershipEngine(t, []int{29861})
	if got := e.soleClickableCandidate(eyesTingleCandidates()); got != "Acumen" {
		t.Fatalf("owned earring: got %q, want %q", got, "Acumen")
	}

	// Carrying none of the colliding clicky items stays ambiguous.
	e2 := ownershipEngine(t, []int{12345})
	if got := e2.soleClickableCandidate(eyesTingleCandidates()); got != "" {
		t.Fatalf("no owned clicky: got %q, want empty", got)
	}
}

func TestOnSpellLanded_AcumenTracksWhenEarringOwned(t *testing.T) {
	e := ownershipEngine(t, []int{29861})
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindYou,
		Candidates: eyesTingleCandidates(),
	})
	timers := e.GetState().Timers
	if len(timers) != 1 || timers[0].SpellName != "Acumen" {
		t.Fatalf("expected one Acumen buff timer, got %+v", timers)
	}
	if timers[0].Category != CategoryBuff {
		t.Errorf("Acumen category = %s, want buff", timers[0].Category)
	}
}

// AoE mez lands on several mobs from one cast. Each land must produce its own
// per-target timer — so one mob's break can't clear the rest — and each must
// inherit the trigger's display threshold from the single pending arm stashed
// on cast-begin. Regression test for the report that AoE mez collapsed to one
// timer and that any break made it vanish for every mob.
func TestOnSpellLanded_AoEMezTracksPerTarget(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil, nil, nil)

	now := time.Now()
	// Cast-begin stashes the trigger's metadata as a deferred-render pending
	// arm (no visible timer yet) and records the recent local cast the
	// detrimental scope filter requires for non-self targets.
	e.StartExternal("Mesmerization", "mez", 24, 8, now, nil, 0, "", "")
	e.mu.Lock()
	e.lastCastSpell = "Mesmerization"
	e.lastCastAt = now
	e.mu.Unlock()

	mobs := []string{"a gnoll", "a kobold", "a bat"}
	for _, m := range mobs {
		e.onSpellLanded(now, logparser.SpellLandedData{
			Kind:       logparser.SpellLandedKindOther,
			SpellName:  "Mesmerization",
			TargetName: m,
		})
	}

	if len(e.timers) != len(mobs) {
		t.Fatalf("AoE mez should track one timer per mob, got %d: %v",
			len(e.timers), keysOf(e.timers))
	}
	for _, m := range mobs {
		tm, ok := e.timers[timerKey("Mesmerization", m)]
		if !ok {
			t.Errorf("missing per-target mez timer for %q", m)
			continue
		}
		if !isDetrimentalCategory(tm.Category) {
			t.Errorf("%s: category %s is not detrimental", m, tm.Category)
		}
		if tm.DisplayThresholdSecs != 8 {
			t.Errorf("%s: threshold %d, want 8 (grafted from the pending arm)",
				m, tm.DisplayThresholdSecs)
		}
	}

	// One worn-off line peels a single mob; the rest keep ticking.
	e.StopExternal("Mesmerization", 0)
	if len(e.timers) != len(mobs)-1 {
		t.Fatalf("one mez break should clear one mob, got %d timers", len(e.timers))
	}
}

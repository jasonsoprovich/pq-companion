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
		func() []int { return ownedItems })
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

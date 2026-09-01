package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Regression: another player's illusion ("<name>'s image shimmers.") is an
// ambiguous cast_on_other land — the log never names the race. It used to be
// disambiguated with the ACTIVE character's clicky inventory, so every other
// player's illusion showed up as whatever lone illusion mask the local player
// carried (a Mask of Deception → "Illusion: Dark Elf"). It must instead
// collapse to a generic "Illusion" timer keyed to the caster.
func TestOnSpellLanded_OtherPlayerIllusionCollapsesToGeneric(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	charCtx := func() (string, string, int) { return "/eq", "Vortikai", -1 }
	// Owned-items provider returns the Mask of Deception (2472 -> Illusion:
	// Dark Elf 590) — the exact item that used to poison the result.
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil,
		func() []int { return []int{2472} }, nil, nil)

	now := time.Now()
	e.onSpellLanded(now, logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		TargetName: "Sneakyrogue",
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 588, SpellName: "Illusion: Wood Elf"},
			{SpellID: 582, SpellName: "Illusion: Human"},
			{SpellID: 590, SpellName: "Illusion: Dark Elf"},
		},
	})

	timers := e.GetState().Timers
	if len(timers) != 1 {
		t.Fatalf("expected one timer, got %d: %+v", len(timers), timers)
	}
	tm := timers[0]
	if tm.SpellName != illusionCombinedName {
		t.Errorf("SpellName = %q, want %q", tm.SpellName, illusionCombinedName)
	}
	if tm.TargetName != "Sneakyrogue" {
		t.Errorf("TargetName = %q, want %q", tm.TargetName, "Sneakyrogue")
	}
	if tm.Category != CategoryBuff {
		t.Errorf("Category = %s, want buff", tm.Category)
	}
	if tm.DurationSeconds <= 0 {
		t.Errorf("DurationSeconds = %v, want > 0", tm.DurationSeconds)
	}
}

// A SELF illusion land (you cast it) still resolves to the specific race when a
// recent cast names it — the guard only blocks the cast_on_other path.
func TestOnSpellLanded_SelfIllusionKeepsSpecificRace(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	charCtx := func() (string, string, int) { return "/eq", "Vortikai", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil, nil, nil, nil)

	now := time.Now()
	e.mu.Lock()
	e.lastCastSpell = "Illusion: Wood Elf"
	e.lastCastAt = now
	e.mu.Unlock()

	e.onSpellLanded(now, logparser.SpellLandedData{
		Kind: logparser.SpellLandedKindYou,
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 588, SpellName: "Illusion: Wood Elf"},
			{SpellID: 590, SpellName: "Illusion: Dark Elf"},
		},
	})

	timers := e.GetState().Timers
	if len(timers) != 1 || timers[0].SpellName != "Illusion: Wood Elf" {
		t.Fatalf("expected one 'Illusion: Wood Elf' timer, got %+v", timers)
	}
}

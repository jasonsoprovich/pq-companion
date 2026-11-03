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
		func() []int { return ownedItems }, nil, nil)
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
		func() string { return scopeAnyone }, nil, nil, nil, nil, nil)

	now := time.Now()
	// Cast-begin stashes the trigger's metadata as a deferred-render pending
	// arm (no visible timer yet) and records the recent local cast the
	// detrimental scope filter requires for non-self targets.
	e.StartExternal("Mesmerization", "mez", 24, 8, now, nil, 0, "", "", false, "", false)
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
			t.Errorf("%s: threshold %v, want 8 (grafted from the pending arm)",
				m, tm.DisplayThresholdSecs)
		}
	}

	// One worn-off line peels a single mob; the rest keep ticking.
	e.StopExternal("Mesmerization", 0)
	if len(e.timers) != len(mobs)-1 {
		t.Fatalf("one mez break should clear one mob, got %d timers", len(e.timers))
	}
}

// enchanterClassLevelsIdx is spells_new.classes14 (0-indexed into
// db.Spell.ClassLevels), the real Enchanter column — distinct from the
// synthetic enchanterClassIdx used by the classFilterAllowsSpell unit tests
// in clicky_test.go, which build their own fake Spell literals.
const enchanterClassLevelsIdx = 13

// scope=anyone now extends the caster-restriction lift to detrimentals
// (previously they always used cast_by_me semantics regardless of scope —
// see the raid enchanter report that "Anyone + class filter" should surface
// every enchanter's mez/tash, not just the player's own). Tashanian is a
// real single-target enchanter debuff with no recent local cast recorded —
// i.e. exactly the shape of "another enchanter tashed this mob."
func TestOnSpellLanded_ScopeAnyone_DetrimentalTrackedWithoutLocalCast(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone },
		func() (bool, int) { return true, enchanterClassLevelsIdx },
		nil, nil, nil, nil)

	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  "Tashanian",
		TargetName: "a gnoll",
	})
	if _, ok := e.timers[timerKey("Tashanian", "a gnoll")]; !ok {
		t.Error("scope=anyone should track another enchanter's Tashanian with no local cast recorded")
	}
}

// The class filter must still gate detrimentals under scope=anyone, the same
// way it already gates buffs — otherwise lifting the caster restriction would
// flood the overlay with every class's debuffs. Turgur's Insects is a
// shaman-only debuff the enchanter class filter should reject.
func TestOnSpellLanded_ScopeAnyone_DetrimentalStillHonoursClassFilter(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone },
		func() (bool, int) { return true, enchanterClassLevelsIdx },
		nil, nil, nil, nil)

	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  "Turgur's Insects",
		TargetName: "a gnoll",
	})
	if _, ok := e.timers[timerKey("Turgur's Insects", "a gnoll")]; ok {
		t.Error("scope=anyone + class filter should drop an off-class (shaman) debuff")
	}
}

// Regression guard for the original cast_by_me fix: scope=cast_by_me (as
// opposed to anyone) must still drop a detrimental with no recent local cast,
// even though the scope switch above now shares more code between the two.
func TestOnSpellLanded_ScopeCastByMe_StillDropsUnmatchedDetrimental(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeCastByMe }, nil, nil, nil, nil, nil)

	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  "Tashanian",
		TargetName: "a gnoll",
	})
	if len(e.timers) != 0 {
		t.Error("scope=cast_by_me without a recent local cast should still drop another caster's Tashanian")
	}
}

// A detrimental the user marked "hide" must never get a timer, while other
// detrimentals still do.
func TestOnSpellLanded_HiddenDetrimentalSkipped(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil, nil, nil, nil)
	e.SetHiddenDetrimProvider(func(name string) bool { return name == "Tashanian" })

	land := func(spell string) {
		e.onSpellLanded(time.Now(), logparser.SpellLandedData{
			Kind:       logparser.SpellLandedKindOther,
			SpellName:  spell,
			TargetName: "a gnoll",
		})
	}
	land("Tashanian")
	if _, ok := e.timers[timerKey("Tashanian", "a gnoll")]; ok {
		t.Error("hidden detrimental should not create a timer")
	}
	land("Slow")
	if _, ok := e.timers[timerKey("Slow", "a gnoll")]; !ok {
		t.Error("a detrimental that isn't hidden should still create a timer")
	}
}

// Staff of the Serpent (enchanter epic) casts Speed of the Shissar, a spell no
// class can scribe. Aimed at a pet, it used to fall to the class filter, which
// only exempted all-255 clickies landing on the player — so the pet overlay
// never saw the haste. The player's own "You begin casting" line now marks it
// as theirs. Another player's identical land (no local cast) stays filtered.
func TestOnSpellLanded_ClickyOnPetPassesClassFilter(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", 60 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeCastByMe },
		func() (bool, int) { return true, enchanterClassLevelsIdx },
		nil, nil, nil, nil)

	land := logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		TargetName: "Kebeker",
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 1939, SpellName: "Speed of the Shissar"},
			{SpellID: 2692, SpellName: "Beta Shissar"},
			{SpellID: 2895, SpellName: "Speed of the Brood"},
		},
	}

	e.HandlePipeTarget("Kebeker")
	e.Handle(logparser.LogEvent{Type: logparser.EventSpellCast, Timestamp: time.Now(),
		Data: logparser.SpellCastData{SpellName: "Speed of the Shissar"}})
	e.onSpellLanded(time.Now(), land)
	tm, ok := e.timers[timerKey("Speed of the Shissar", "Kebeker")]
	if !ok {
		t.Fatalf("own Speed of the Shissar on the pet was dropped; timers=%v", e.timers)
	}
	if tm.Category != CategoryBuff || tm.DurationSeconds <= 0 {
		t.Errorf("got category %s duration %v, want a buff with a duration", tm.Category, tm.DurationSeconds)
	}

	// Someone else's cast on their own pet: no local cast line.
	e2 := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone },
		func() (bool, int) { return true, enchanterClassLevelsIdx },
		nil, nil, nil, nil)
	land.TargetName = "Gygr`s warder"
	e2.onSpellLanded(time.Now(), land)
	if len(e2.timers) != 0 {
		t.Errorf("another player's clicky should stay filtered, got %v", e2.timers)
	}
}

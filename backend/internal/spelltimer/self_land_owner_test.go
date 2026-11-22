package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Real spells_new class columns (0-indexed into db.Spell.ClassLevels).
const (
	clericClassLevelsIdx       = 1
	shadowKnightClassLevelsIdx = 4
)

// selfLandEngine builds a DB-backed engine for "Batt" with the given scope,
// class filter (classIdx < 0 = off) and "Only show spells I cast" setting.
func selfLandEngine(t *testing.T, scope string, classIdx int, onlyMine bool) *Engine {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Batt", -1 }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scope },
		func() (bool, int) { return classIdx >= 0, classIdx },
		nil, nil, nil, nil)
	e.SetOnlyMyCastsProvider(func() bool { return onlyMine })
	return e
}

func castLocally(e *Engine, spell string) {
	e.mu.Lock()
	e.lastCastSpell = spell
	e.lastCastAt = time.Now()
	e.mu.Unlock()
}

func landOnMe(e *Engine, spell string) bool {
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:      logparser.SpellLandedKindYou,
		SpellName: spell,
	})
	_, ok := e.timers[timerKey(spell, "Batt")]
	return ok
}

// A shadow knight groupmate's Aura of Darkness lands its recourse on the
// whole group. No class can cast the recourse itself, so it used to pass the
// class filter as if it were the player's own item clicky.
func TestSelfLand_OthersRecourseDroppedByClassFilter(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, clericClassLevelsIdx, false)
	if landOnMe(e, "Aura of Darkness Recourse") {
		t.Error("cleric with class filter should not track a groupmate's Aura of Darkness Recourse")
	}
}

// Without the class filter, cast_by_me still keeps every buff on the player,
// a groupmate's recourse included: unchanged default behaviour.
func TestSelfLand_OthersRecourseKeptWithoutFilters(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, -1, false)
	if !landOnMe(e, "Aura of Darkness Recourse") {
		t.Error("cast_by_me without filters should keep buffs landing on the player")
	}
}

// The shadow knight's own recourse is theirs: it follows their own cast of
// the parent spell, so the class filter keeps it.
func TestSelfLand_OwnRecourseKeptByClassFilter(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, shadowKnightClassLevelsIdx, true)
	castLocally(e, "Aura of Darkness")
	if !landOnMe(e, "Aura of Darkness Recourse") {
		t.Error("the caster's own recourse should be tracked under class filter + only my casts")
	}
}

// Real item clickies (no class can cast them, no recourse parent) keep the
// existing exemption: landing on the player still counts as the player's.
func TestSelfLand_ItemClickyStillExempt(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, clericClassLevelsIdx, false)
	if !landOnMe(e, "Shield of the Eighth") {
		t.Error("an item clicky landing on the player should still pass the class filter")
	}
}

// "Only show spells I cast" drops a buff another player cast on you and
// keeps your own.
func TestSelfLand_OnlyMyCasts(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, -1, true)
	if landOnMe(e, "Symbol of Transal") {
		t.Error("only my casts: another cleric's Symbol on me should be dropped")
	}
	castLocally(e, "Symbol of Transal")
	if !landOnMe(e, "Symbol of Transal") {
		t.Error("only my casts: my own Symbol on me should be tracked")
	}
}

func TestSelfLand_OnlyMyCastsAppliesToSelfScope(t *testing.T) {
	e := selfLandEngine(t, scopeSelf, -1, true)
	if landOnMe(e, "Symbol of Transal") {
		t.Error("only my casts should apply under self scope too")
	}
}

func TestSelfLand_OnlyMyCastsIgnoredUnderAnyone(t *testing.T) {
	e := selfLandEngine(t, scopeAnyone, -1, true)
	if !landOnMe(e, "Symbol of Transal") {
		t.Error("only my casts is ignored under anyone scope")
	}
}

// With the Zeal pipe, the spell action's source decides, ahead of the log
// timing heuristics.
func TestSelfLandIsMine_PipeSource(t *testing.T) {
	e := selfLandEngine(t, scopeCastByMe, -1, true)
	spell, err := e.db.GetSpellByExactName("Symbol of Transal")
	if err != nil || spell == nil {
		t.Fatalf("lookup: %v", err)
	}
	e.mu.Lock()
	e.selfSpawnID = 100
	e.mu.Unlock()
	castLocally(e, "Symbol of Transal")
	if e.selfLandIsMine(landCorrelation{sourceID: 200}, spell.Name, spell) {
		t.Error("pipe source of another spawn should win over a matching local cast")
	}
	if !e.selfLandIsMine(landCorrelation{sourceID: 100}, spell.Name, spell) {
		t.Error("pipe source of the player should count as mine")
	}
}

package spelltimer

import (
	"testing"
	"time"
)

func intp(n int) *int { return &n }

// seedCharm puts one charm timer and one unrelated debuff timer in the engine.
func seedCharm(e *Engine) {
	e.timers[timerKey("Beguile", "a sarnak conscript")] = &ActiveTimer{
		ID: "Beguile@a sarnak conscript", SpellName: "Beguile",
		TargetName: "a sarnak conscript", Category: CategoryBuff, IsCharm: true,
	}
	e.timers[timerKey("Tashani", "a sarnak conscript")] = &ActiveTimer{
		ID: "Tashani@a sarnak conscript", SpellName: "Tashani",
		TargetName: "a sarnak conscript", Category: CategoryDebuff,
	}
}

func hasCharmTimer(e *Engine) bool {
	for _, t := range e.timers {
		if t.IsCharm {
			return true
		}
	}
	return false
}

func TestSetPipePetID_PetGoneClearsCharmAfterMissStreak(t *testing.T) {
	e := newTestEngine()
	seedCharm(e)

	e.SetPipePetID(intp(5001)) // pet acquired
	if !hasCharmTimer(e) {
		t.Fatal("charm timer should still be present right after pet acquire")
	}

	// A single dropped frame must NOT clear the timer.
	e.SetPipePetID(nil)
	if !hasCharmTimer(e) {
		t.Fatalf("charm timer cleared after only 1 miss (threshold %d)", petLossMissThreshold)
	}
	// A non-nil frame in between resets the streak.
	e.SetPipePetID(intp(5001))
	for i := 0; i < petLossMissThreshold-1; i++ {
		e.SetPipePetID(nil)
	}
	if !hasCharmTimer(e) {
		t.Fatal("charm timer cleared before the miss streak was reached")
	}
	e.SetPipePetID(nil) // threshold hit
	if hasCharmTimer(e) {
		t.Fatal("charm timer should be cleared once pet_id has been absent for the full streak")
	}
	// The unrelated debuff timer is untouched.
	if _, ok := e.timers[timerKey("Tashani", "a sarnak conscript")]; !ok {
		t.Error("pet-loss cleared a non-charm timer")
	}
}

func TestSetPipePetID_IdChangeClearsCharmImmediately(t *testing.T) {
	e := newTestEngine()
	seedCharm(e)

	e.SetPipePetID(intp(5001))
	e.SetPipePetID(intp(5002)) // re-charm: new pet, possibly same name
	if hasCharmTimer(e) {
		t.Fatal("a changed pet_id should clear the old charm timer at once")
	}
}

func TestSetPipePetID_NilWhenAlreadyNilIsNoop(t *testing.T) {
	e := newTestEngine()
	seedCharm(e)
	for i := 0; i < 10; i++ {
		e.SetPipePetID(nil) // older Zeal / no pet — every frame
	}
	if !hasCharmTimer(e) {
		t.Fatal("charm timer cleared with no pet ever reported (older Zeal path)")
	}
}

func TestResetPipePetID_DoesNotClearCharm(t *testing.T) {
	e := newTestEngine()
	seedCharm(e)
	e.SetPipePetID(intp(5001))
	e.ResetPipePetID() // pipe disconnected — pet may still be alive
	if !hasCharmTimer(e) {
		t.Fatal("pipe disconnect must not clear the charm timer")
	}
	if e.pipePetID != nil || e.pipePetMisses != 0 {
		t.Errorf("ResetPipePetID left state: id=%v misses=%d", e.pipePetID, e.pipePetMisses)
	}
}

// Charm pets are usually one of the zone's many same-named NPCs. A namesake's
// death must not clear the buffs on the live pet, while debuffs under that
// name (which could be on the mob that died) still clear as before.
func TestRemoveOnKill_KeepsLivePetBuffs(t *testing.T) {
	e := newTestEngine()
	pet := "Rygan Anisher"
	e.SetPetNameProvider(func() string { return pet })
	now := time.Now()
	e.timers[timerKey("Speed of the Shissar", "Rygan Anisher")] = &ActiveTimer{
		SpellName: "Speed of the Shissar", TargetName: "Rygan Anisher", Category: CategoryBuff,
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(time.Hour),
	}
	e.timers[timerKey("Tashanian", "Rygan Anisher")] = &ActiveTimer{
		SpellName: "Tashanian", TargetName: "Rygan Anisher", Category: CategoryDebuff,
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(time.Hour),
	}

	e.removeOnKill("Rygan Anisher", false, false)
	if _, ok := e.timers[timerKey("Speed of the Shissar", "Rygan Anisher")]; !ok {
		t.Error("namesake death cleared the live pet's buff")
	}
	if _, ok := e.timers[timerKey("Tashanian", "Rygan Anisher")]; ok {
		t.Error("untagged debuff under the dead mob's name should still clear")
	}

	// No pet (or a different one): the buff clears like any other.
	pet = ""
	e.removeOnKill("Rygan Anisher", false, false)
	if len(e.timers) != 0 {
		t.Errorf("without a namesake pet the buff should clear, got %v", e.timers)
	}
}

package spelltimer

import (
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestReconcileTargetEffects(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * time.Minute)
	before := now.Add(-6 * time.Second)
	const pet = "a Shissar Revenant"
	const petID = 812
	tagged := func(id int) string { return pet + targetIDKeySep + strconv.Itoa(id) }

	type seed struct {
		key     string
		timer   ActiveTimer
		survive bool
	}
	seeds := []seed{
		// Listed in the reply: kept.
		{timerKey("Speed of the Shissar", pet),
			ActiveTimer{SpellName: "Speed of the Shissar", TargetName: pet, Category: CategoryBuff, StartsAt: old}, true},
		// Not listed, untagged buff on the pet: dropped.
		{timerKey("Primal Essence", pet),
			ActiveTimer{SpellName: "Primal Essence", TargetName: pet, Category: CategoryBuff, StartsAt: old}, false},
		// Not listed but landed after the reply's margin: kept (may race it).
		{timerKey("Celerity", pet),
			ActiveTimer{SpellName: "Celerity", TargetName: pet, Category: CategoryBuff, StartsAt: now}, true},
		// Not listed, debuff tagged to the pet's spawn id: dropped.
		{timerKey("Tashanian", tagged(petID)),
			ActiveTimer{SpellName: "Tashanian", TargetName: pet, Category: CategoryDebuff, StartsAt: old}, false},
		// Not listed, debuff on a same-named enemy (other spawn id): kept.
		{timerKey("Tashani", tagged(999)),
			ActiveTimer{SpellName: "Tashani", TargetName: pet, Category: CategoryDebuff, StartsAt: old}, true},
		// Not listed, untagged debuff: can't tell pet from enemy, kept.
		{timerKey("Clinging Darkness", pet),
			ActiveTimer{SpellName: "Clinging Darkness", TargetName: pet, Category: CategoryDot, StartsAt: old}, true},
		// The charm timer is never judged by the list.
		{timerKey("Boltran`s Agacerie", ""),
			ActiveTimer{SpellName: "Boltran`s Agacerie", TargetName: pet, Category: CategoryMez, StartsAt: old, IsCharm: true}, true},
		// Custom trigger rows are never judged by the list.
		{timerKey("Pet buff reminder", pet),
			ActiveTimer{SpellName: "Pet buff reminder", TargetName: pet, Category: CategoryCustom, StartsAt: old}, true},
		// A different target entirely.
		{timerKey("Primal Essence", "Osui"),
			ActiveTimer{SpellName: "Primal Essence", TargetName: "Osui", Category: CategoryBuff, StartsAt: old}, true},
	}

	run := func(t *testing.T, keepExpired bool) {
		e := newTestEngine()
		if keepExpired {
			e.keepExpiredFn = func() bool { return true }
		}
		for _, s := range seeds {
			tm := s.timer
			tm.ID = s.key
			tm.ExpiresAt = now.Add(10 * time.Minute)
			e.timers[s.key] = &tm
		}
		listed := map[string]bool{"Speed of the Shissar": true}
		keep := func(name string) bool { return listed[name] }

		removed := e.ReconcileTargetEffects("A Shissar Revenant", petID, keep, before)
		sort.Strings(removed)
		if want := []string{"Primal Essence", "Tashanian"}; !equalStrings(removed, want) {
			t.Fatalf("removed = %q, want %q", removed, want)
		}
		for _, s := range seeds {
			tm, present := e.timers[s.key]
			switch {
			case s.survive && (!present || !tm.ExpiresAt.After(now)):
				t.Errorf("%s: should be untouched", s.key)
			case !s.survive && !keepExpired && present:
				t.Errorf("%s: should be deleted", s.key)
			case !s.survive && keepExpired && (!present || tm.ExpiresAt.After(time.Now())):
				t.Errorf("%s: keep-expired should flag it overdue, not delete it", s.key)
			}
		}

		// A second reply must not re-report already-handled timers.
		if again := e.ReconcileTargetEffects(pet, petID, keep, before); len(again) != 0 {
			t.Errorf("second reconcile removed %q again", again)
		}
	}
	t.Run("delete", func(t *testing.T) { run(t, false) })
	t.Run("keep expired", func(t *testing.T) { run(t, true) })
}

func TestReconcileTargetEffects_UnknownPetIDSkipsTagged(t *testing.T) {
	e := newTestEngine()
	old := time.Now().Add(-time.Minute)
	key := timerKey("Primal Essence", "a Shissar Revenant"+targetIDKeySep+"812")
	e.timers[key] = &ActiveTimer{ID: key, SpellName: "Primal Essence", TargetName: "a Shissar Revenant",
		Category: CategoryBuff, StartsAt: old, ExpiresAt: time.Now().Add(time.Minute)}
	// No Zeal pet id (0): a spawn-tagged timer can't be matched to the pet.
	if removed := e.ReconcileTargetEffects("a Shissar Revenant", 0, func(string) bool { return false }, time.Now()); len(removed) != 0 {
		t.Fatalf("removed %q with no pet id", removed)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

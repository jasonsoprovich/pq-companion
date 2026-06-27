package raidthreat

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
)

type fakeDmg []combat.MobDamage

func (f fakeDmg) RaidThreatDamage() []combat.MobDamage { return f }

type fakePersonal map[string]int64

func (f fakePersonal) PersonalHate() map[string]int64 { return f }

func newAsm(dmg DamageSource, personal PersonalSource, classMods, playerMods map[string]int) *Assembler {
	if dmg == nil {
		dmg = fakeDmg(nil)
	}
	if personal == nil {
		personal = fakePersonal(nil)
	}
	return NewAssembler(nil, dmg, personal,
		func() bool { return true },
		func() map[string]int { return classMods },
		func() map[string]int { return playerMods })
}

func mob(name string, atk ...combat.AttackerDamage) combat.MobDamage {
	return combat.MobDamage{Mob: name, Attackers: atk}
}

// findMob / findPlayer locate rows in a snapshot, or nil.
func findMob(s RaidThreatState, name string) *RaidMob {
	for i := range s.Mobs {
		if s.Mobs[i].Name == name {
			return &s.Mobs[i]
		}
	}
	return nil
}

func findPlayer(m *RaidMob, name string) *RaidEntry {
	if m == nil {
		return nil
	}
	for i := range m.Players {
		if m.Players[i].Name == name {
			return &m.Players[i]
		}
	}
	return nil
}

func TestDisabledReturnsEmpty(t *testing.T) {
	a := NewAssembler(nil,
		fakeDmg{mob("a dragon", combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 1000})},
		fakePersonal{"a dragon": 500},
		func() bool { return false }, nil, nil)
	if s := a.GetState(); s.InCombat || len(s.Mobs) != 0 {
		t.Errorf("disabled state = %+v, want empty/not-in-combat", s)
	}
}

func TestDamageToHateRankingAndYouSplice(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon",
			combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 1000},
			combat.AttackerDamage{Name: "Tank", Class: "Warrior", Damage: 1000},
			combat.AttackerDamage{Name: "You", Class: "Enchanter", Damage: 300}, // dropped; personal used
		)},
		fakePersonal{"a dragon": 5000},
		nil, nil)

	s := a.GetState()
	m := findMob(s, "a dragon")
	if m == nil {
		t.Fatal("a dragon not in snapshot")
	}
	// Wizard: no adjustment → 1000. Warrior: +30% default → 1300. You: personal 5000.
	if got := findPlayer(m, "Narya"); got == nil || got.Hate != 1000 {
		t.Errorf("Narya hate = %v, want 1000", got)
	}
	if got := findPlayer(m, "Tank"); got == nil || got.Hate != 1300 {
		t.Errorf("Tank hate = %v, want 1300 (+30%% tank default)", got)
	}
	you := findPlayer(m, "You")
	if you == nil || you.Hate != 5000 || !you.IsYou {
		t.Errorf("You row = %v, want hate 5000 from personal meter", you)
	}
	// Ranking + top + pct.
	if m.Players[0].Name != "You" || m.TopHate != 5000 {
		t.Errorf("top = %s/%d, want You/5000", m.Players[0].Name, m.TopHate)
	}
	if got := findPlayer(m, "Tank").HatePct; got < 0.25 || got > 0.27 {
		t.Errorf("Tank pct = %v, want ~0.26", got)
	}
}

func TestUserClassModOverridesDefault(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon", combat.AttackerDamage{Name: "Tank", Class: "Warrior", Damage: 1000})},
		nil,
		map[string]int{"Warrior": 0}, // explicit 0 beats the +30 default
		nil)
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Tank"); got == nil || got.Hate != 1000 {
		t.Errorf("Tank hate = %v, want 1000 (override to 0)", got)
	}
}

func TestPlayerModStacksOnClassMod(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon", combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 1000})},
		nil, nil,
		map[string]int{"Narya": 20}) // Wizard default 0 + player 20 = +20%
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Narya"); got == nil || got.Hate != 1200 {
		t.Errorf("Narya hate = %v, want 1200 (+20%% player mod)", got)
	}
}

func TestPetGetsNeutralModAndFlag(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon", combat.AttackerDamage{
			Name: "Gybartik", Class: "Warrior", OwnerName: "Magebot", Damage: 1000, IsPet: true,
		})},
		nil, nil, nil)
	m := findMob(a.GetState(), "a dragon")
	p := findPlayer(m, "Gybartik")
	if p == nil || p.Hate != 1000 {
		t.Errorf("pet hate = %v, want 1000 (neutral, no owner tank boost)", p)
	}
	if !p.IsPet || p.OwnerName != "Magebot" || len(p.Confidence) != 0 {
		t.Errorf("pet row = %+v, want IsPet/owner Magebot/no flags", p)
	}
}

func TestConfidenceFlags(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon",
			combat.AttackerDamage{Name: "Dotter", Class: "Necromancer", Damage: 100},
			combat.AttackerDamage{Name: "Healer", Class: "Cleric", Damage: 100},
			combat.AttackerDamage{Name: "Mystery", Class: "", Damage: 100},
		)},
		nil, nil, nil)
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Dotter").Confidence; len(got) != 1 || got[0] != ConfDoTUndercount {
		t.Errorf("necro confidence = %v, want [dot_undercount]", got)
	}
	if got := findPlayer(m, "Healer").Confidence; len(got) != 1 || got[0] != ConfHealUndercount {
		t.Errorf("cleric confidence = %v, want [heal_undercount]", got)
	}
	if got := findPlayer(m, "Mystery").Confidence; len(got) != 1 || got[0] != ConfClassUnknown {
		t.Errorf("unknown-class confidence = %v, want [class_unknown]", got)
	}
}

func TestPersonalOnlyMobAppears(t *testing.T) {
	// A mob you hold hate on (e.g. you mezzed it) that no combat damage touched
	// still shows, as a You-only row.
	a := newAsm(nil, fakePersonal{"a sleeper": 250}, nil, nil)
	m := findMob(a.GetState(), "a sleeper")
	if m == nil || len(m.Players) != 1 || !m.Players[0].IsYou {
		t.Fatalf("personal-only mob = %+v, want single You row", m)
	}
}

func TestPipeTargetHighlights(t *testing.T) {
	a := newAsm(
		fakeDmg{
			mob("a dragon", combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 1000}),
			mob("a whelp", combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 5000}),
		}, nil, nil, nil)
	a.SetPipeTarget("a dragon")
	s := a.GetState()
	if m := findMob(s, "a dragon"); m == nil || !m.IsTarget {
		t.Errorf("a dragon IsTarget = false, want true (pipe target)")
	}
	if m := findMob(s, "a whelp"); m == nil || m.IsTarget {
		t.Errorf("a whelp IsTarget = true, want false")
	}
}

package raidthreat

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type fakeDmg []combat.MobDamage

func (f fakeDmg) RaidThreatDamage() []combat.MobDamage { return f }

// settableDmg is a mutable damage source for tests that change damage between
// snapshots (e.g. damage accruing after a taunt).
type settableDmg struct{ mobs []combat.MobDamage }

func (s *settableDmg) RaidThreatDamage() []combat.MobDamage { return s.mobs }

func tauntEv(mob, taunter string) logparser.LogEvent {
	return logparser.LogEvent{Type: logparser.EventTaunt, Data: logparser.TauntData{Mob: mob, Taunter: taunter}}
}

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
		func() map[string]int { return playerMods },
		nil)
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
		func() bool { return false }, nil, nil, nil)
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
	// No class boost by default — every row is honest observed damage.
	if got := findPlayer(m, "Narya"); got == nil || got.Hate != 1000 {
		t.Errorf("Narya hate = %v, want 1000", got)
	}
	if got := findPlayer(m, "Tank"); got == nil || got.Hate != 1000 {
		t.Errorf("Tank hate = %v, want 1000 (no default class boost)", got)
	}
	you := findPlayer(m, "You")
	if you == nil || you.Hate != 5000 || !you.IsYou {
		t.Errorf("You row = %v, want hate 5000 from personal meter", you)
	}
	// Ranking + top + pct.
	if m.Players[0].Name != "You" || m.TopHate != 5000 {
		t.Errorf("top = %s/%d, want You/5000", m.Players[0].Name, m.TopHate)
	}
	if got := findPlayer(m, "Tank").HatePct; got < 0.19 || got > 0.21 {
		t.Errorf("Tank pct = %v, want ~0.20", got)
	}
}

func TestUserClassModApplied(t *testing.T) {
	a := newAsm(
		fakeDmg{mob("a dragon", combat.AttackerDamage{Name: "Tank", Class: "Warrior", Damage: 1000})},
		nil,
		map[string]int{"Warrior": 50}, // opt-in: known aggro-mod gear
		nil)
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Tank"); got == nil || got.Hate != 1500 {
		t.Errorf("Tank hate = %v, want 1500 (+50%% class mod)", got)
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

// ── Taunt model ─────────────────────────────────────────────────────────────

func TestTauntSetsToTopPlusBump(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 500},
	)}, nil, nil, nil)
	a.Handle(tauntEv("a dragon", "Borg")) // Borg below Narya → jumps to 2000 + 10
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Borg"); got == nil || got.Hate != 2010 {
		t.Fatalf("Borg after taunt = %v, want 2010 (top 2000 + 10)", got)
	}
	if m.Players[0].Name != "Borg" || m.TopHate != 2010 {
		t.Errorf("top = %s/%d, want Borg/2010", m.Players[0].Name, m.TopHate)
	}
}

// TestTauntCasingMismatchStillApplies covers the EQ inconsistency where a taunt
// emote capitalises the mob's leading article ("A shadow reaver says ...") while
// its damage line / clean spawn name is lowercase. The taunt offset must key to
// the same canonical mob as the damage, or it would be recorded but never
// merged into the displayed hate.
func TestTauntCasingMismatchStillApplies(t *testing.T) {
	a := newAsm(fakeDmg{mob("a shadow reaver",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 500},
	)}, nil, nil, nil)
	// Emote subject is capitalised — the bug fold target.
	a.Handle(tauntEv("A shadow reaver", "Borg"))
	m := findMob(a.GetState(), "a shadow reaver")
	if got := findPlayer(m, "Borg"); got == nil || got.Hate != 2010 {
		t.Fatalf("Borg after capitalised-emote taunt = %v, want 2010", got)
	}
}

func TestTauntNoOpWhenAlreadyTop(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 3000},
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 500},
	)}, nil, nil, nil)
	a.Handle(tauntEv("a dragon", "Borg")) // already top → unchanged
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Borg"); got == nil || got.Hate != 3000 {
		t.Errorf("Borg = %v, want 3000 (taunt no-op when already top)", got)
	}
}

func TestTauntThenDamageAccrues(t *testing.T) {
	sd := &settableDmg{mobs: []combat.MobDamage{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 500},
	)}}
	a := NewAssembler(nil, sd, fakePersonal(nil),
		func() bool { return true },
		func() map[string]int { return nil },
		func() map[string]int { return nil }, nil)
	a.Handle(tauntEv("a dragon", "Borg")) // offset = 2010 - 500 = 1510
	// Borg keeps swinging: base 500 → 800. Displayed should be 800 + 1510 = 2310.
	sd.mobs[0].Attackers[1].Damage = 800
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Borg"); got == nil || got.Hate != 2310 {
		t.Errorf("Borg = %v, want 2310 (offset + accrued damage)", got)
	}
}

func TestTauntMapsSelfNameToYou(t *testing.T) {
	a := NewAssembler(nil,
		fakeDmg{mob("a dragon", combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000})},
		fakePersonal{"a dragon": 300},
		func() bool { return true },
		func() map[string]int { return nil },
		func() map[string]int { return nil },
		func() string { return "Osui" }) // you are Osui
	a.Handle(tauntEv("a dragon", "Osui")) // emote names Osui → applies to the You row
	you := findPlayer(findMob(a.GetState(), "a dragon"), "You")
	if you == nil || you.Hate != 2010 {
		t.Errorf("You after self-taunt = %v, want 2010 (top 2000 + 10)", you)
	}
}

func TestTauntOnlyPlayerAppears(t *testing.T) {
	// A tank who taunts before doing any damage still shows, at top + 10.
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
	)}, nil, nil, nil)
	a.Handle(tauntEv("a dragon", "Borg"))
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Borg"); got == nil || got.Hate != 2010 {
		t.Errorf("taunt-only Borg = %v, want 2010", got)
	}
}

func departureEv(target, spellName string, candidates ...string) logparser.LogEvent {
	data := logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  spellName,
		TargetName: target,
	}
	for _, c := range candidates {
		data.Candidates = append(data.Candidates, logparser.SpellLandedCandidate{SpellName: c})
	}
	return logparser.LogEvent{Type: logparser.EventSpellLanded, Data: data}
}

// A raid member evacuating/succoring/porting away resets their hate to zero
// server-side, but our raid estimate is built from combat's cumulative
// per-attacker damage, which never resets on its own. Regression for a real
// report: a wizard/druid's own threat meter cleared correctly on their own
// evac, but everyone else watching the raid meter kept seeing their stale,
// pre-evac hate.
func TestDepartureZeroesPlayer(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 500},
	)}, nil, nil, nil)
	a.Handle(departureEv("Narya", "Evacuate"))
	m := findMob(a.GetState(), "a dragon")
	if got := findPlayer(m, "Narya"); got == nil || got.Hate != 0 {
		t.Errorf("Narya after Evacuate = %v, want 0", got)
	}
	// Borg is untouched.
	if got := findPlayer(m, "Borg"); got == nil || got.Hate != 500 {
		t.Errorf("Borg after Narya's Evacuate = %v, want 500 (unaffected)", got)
	}
}

// The ambiguous "creates a mystic portal." text is shared by Succor's zone
// variants and several unrelated Circle spells — any candidate resolving to a
// departure spell should trigger the zeroing.
func TestDepartureAmbiguousCandidateStillApplies(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Druid", Damage: 1500},
	)}, nil, nil, nil)
	a.Handle(departureEv("Narya", "", "Succor: East", "Circle of Karana"))
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Narya"); got == nil || got.Hate != 0 {
		t.Errorf("Narya after ambiguous Succor/Circle candidates = %v, want 0", got)
	}
}

func TestDepartureIgnoresUnrelatedSpell(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
	)}, nil, nil, nil)
	a.Handle(departureEv("Narya", "Complete Heal"))
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Narya"); got == nil || got.Hate != 2000 {
		t.Errorf("Narya after unrelated spell = %v, want 2000 (unaffected)", got)
	}
}

func TestDepartureThenDamageAccrues(t *testing.T) {
	sd := &settableDmg{mobs: []combat.MobDamage{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
	)}}
	a := NewAssembler(nil, sd, fakePersonal(nil),
		func() bool { return true },
		func() map[string]int { return nil },
		func() map[string]int { return nil }, nil)
	a.Handle(departureEv("Narya", "Evacuate")) // offset = -2000
	// Narya zones back in and re-engages: cumulative damage climbs to 2300.
	sd.mobs[0].Attackers[0].Damage = 2300
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Narya"); got == nil || got.Hate != 300 {
		t.Errorf("Narya after re-engaging = %v, want 300 (re-accrued since departure)", got)
	}
}

func TestKillClearsDepartureOffset(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
	)}, nil, nil, nil)
	a.Handle(departureEv("Narya", "Evacuate"))
	a.Handle(logparser.LogEvent{Type: logparser.EventKill, Data: logparser.KillData{Killer: "You", Target: "a dragon"}})
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Narya"); got == nil || got.Hate != 2000 {
		t.Errorf("Narya after kill = %v, want 2000 (departure offset cleared)", got)
	}
}

func TestKillClearsTauntOffset(t *testing.T) {
	a := newAsm(fakeDmg{mob("a dragon",
		combat.AttackerDamage{Name: "Narya", Class: "Wizard", Damage: 2000},
		combat.AttackerDamage{Name: "Borg", Class: "Warrior", Damage: 100},
	)}, nil, nil, nil)
	a.Handle(tauntEv("a dragon", "Borg")) // Borg → 2010
	a.Handle(logparser.LogEvent{Type: logparser.EventKill, Data: logparser.KillData{Killer: "You", Target: "a dragon"}})
	// Offset cleared; Borg falls back to base damage hate.
	if got := findPlayer(findMob(a.GetState(), "a dragon"), "Borg"); got == nil || got.Hate != 100 {
		t.Errorf("Borg after kill = %v, want 100 (taunt offset cleared)", got)
	}
}

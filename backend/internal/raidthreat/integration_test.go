package raidthreat_test

import (
	"encoding/json"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/raidthreat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/threat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// TestRaidThreatLiveIntegration drives realistic raid combat lines through the
// REAL parser, combat tracker, and personal threat tracker, then asks the
// assembler for the raid view — the same path main.go wires up. It is the
// end-to-end smoke for the feature: it proves mob keys line up between the
// combat and personal meters, that class resolution and pet roll-up flow into
// the per-player rows, and that the tank boost / ranking come out sane.
func TestRaidThreatLiveIntegration(t *testing.T) {
	// Class resolver: who is what. Mirrors what the character/who tracker feeds
	// the combat tracker in production.
	classOf := map[string]string{
		"Borg":    "Warrior",     // tank → +30% default
		"Narya":   "Wizard",      // pure DPS → neutral
		"Plague":  "Necromancer", // DoT class → dot_undercount flag
		"Magebot": "Magician",    // pet owner
	}

	hub := ws.NewHub() // combat tracker broadcasts unconditionally; give it a real hub
	go hub.Run()
	ct := combat.NewTracker(hub, func() string { return "You" })
	ct.SetClassResolvers(
		func() string { return "Enchanter" }, // you
		func(name string) string { return classOf[name] },
	)
	tt := threat.NewTracker(nil, nil, nil) // damage-only personal hate is fine here

	asm := raidthreat.NewAssembler(nil, ct, tt,
		func() bool { return true },
		func() map[string]int { return nil },                         // class mods: use defaults
		func() map[string]int { return map[string]int{"Narya": 15} }, // a per-player override
	)

	// A pull on "a sand giant": you engage first (seeds the fight), then the
	// raid piles on — melee, nukes, and a mage pet.
	lines := []string{
		"You slash a sand giant for 100 points of damage.",
		"You slash a sand giant for 120 points of damage.",
		"a sand giant was hit by non-melee for 250 points of damage.", // your nuke
		"Borg slashes a sand giant for 350 points of damage.",         // tank melee
		"Borg slashes a sand giant for 300 points of damage.",
		"Narya hit a sand giant for 800 points of non-melee damage.",  // wizard nuke
		"Plague hit a sand giant for 150 points of non-melee damage.", // necro direct nuke
		"Gybartik says 'My leader is Magebot.'",                       // pet → owner
		"Gybartik slashes a sand giant for 90 points of damage.",      // mage pet melee
	}
	const ts = "[Sat Jun 27 20:00:00 2026] " // ParseLine needs the EQ timestamp prefix
	for _, ln := range lines {
		ev, ok := logparser.ParseLine(ts + ln)
		if !ok {
			t.Fatalf("parser did not recognise: %q", ln)
		}
		ct.Handle(ev)
		tt.Handle(ev)
	}

	state := asm.GetState()
	pretty, _ := json.MarshalIndent(state, "", "  ")
	t.Logf("assembled raid threat:\n%s", pretty)

	if !state.InCombat || len(state.Mobs) != 1 {
		t.Fatalf("want 1 mob in combat, got in_combat=%v mobs=%d", state.InCombat, len(state.Mobs))
	}
	mob := state.Mobs[0]
	if mob.Name != "a sand giant" {
		t.Fatalf("mob name = %q, want \"a sand giant\"", mob.Name)
	}

	get := func(name string) *raidthreat.RaidEntry {
		for i := range mob.Players {
			if mob.Players[i].Name == name {
				return &mob.Players[i]
			}
		}
		return nil
	}

	// You row comes from the personal meter (470 = 100+120+250 observed damage).
	you := get("You")
	if you == nil || !you.IsYou || you.Hate != 470 {
		t.Fatalf("You row = %+v, want personal hate 470", you)
	}
	// Tank: 650 melee × +30% default = 845.
	if borg := get("Borg"); borg == nil || borg.Hate != 845 {
		t.Fatalf("Borg (tank) = %+v, want 845 (650 × 1.30)", borg)
	}
	// Wizard: 800 × +15% player override = 920.
	if narya := get("Narya"); narya == nil || narya.Hate != 920 {
		t.Fatalf("Narya (wizard) = %+v, want 920 (800 × 1.15)", narya)
	}
	// Necro flagged as DoT-undercount; direct nuke 150 at neutral.
	necro := get("Plague")
	if necro == nil || necro.Hate != 150 || len(necro.Confidence) == 0 {
		t.Fatalf("Plague (necro) = %+v, want 150 with dot_undercount flag", necro)
	}
	// Mage pet rolled to owner: own row, neutral mod, flagged as pet.
	pet := get("Gybartik")
	if pet == nil || !pet.IsPet || pet.OwnerName != "Magebot" || pet.Hate != 90 {
		t.Fatalf("pet row = %+v, want IsPet/owner Magebot/hate 90", pet)
	}

	// Ranking: top is the wizard (920); bars are relative to it.
	if mob.TopHate != 920 || mob.Players[0].Name != "Narya" {
		t.Fatalf("top = %s/%d, want Narya/920", mob.Players[0].Name, mob.TopHate)
	}
}

package trigger

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
)

// unverifiedBuffDurations are pack buff timers that disagree with the plain
// level-60 formula. Quarm's spell_modifiers table (absent from the public
// dumps, LIMITATIONS.md §11.1) is live for client-cast beneficial spells and
// could explain them, so they stay as-is until checked in-game.
var unverifiedBuffDurations = map[int]bool{
	3441: true, // Blessing of Replenishment
	1561: true, // Legacy of Thorn
	2528: true, // Regrowth of Dar Khura
	1461: true, // Call of Sky
	1463: true, // Call of Fire
	2039: true, // Shroud of Stealth
	412:  true, // Shield of Lava
	1416: true, // Arch Lich
	3463: true, // Ferocity
}

// TestPackDurationsMatchSpellFormula guards against hand-entered pack timer
// durations drifting from the server's duration formula. Pack values are the
// formula at level 60; spelltimer rescales them to the caster's real level,
// which only works when the value is recognisably formula-derived.
func TestPackDurationsMatchSpellFormula(t *testing.T) {
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	defer database.Close()

	check := func(pack, name string, spellID int, secs float64) {
		if spellID <= 0 || secs <= 0 || unverifiedBuffDurations[spellID] {
			return
		}
		if _, ok := disciplineReuseBySpell[spellID]; ok {
			return
		}
		spell, err := database.GetSpell(spellID)
		if err != nil || spell == nil {
			t.Errorf("%s/%s: spell %d not found", pack, name, spellID)
			return
		}
		want := float64(spelltimer.SpellDurationTicks(spell, 60)) * 6
		if want > 0 && secs != want {
			t.Errorf("%s/%s (spell %d %s): pack %gs, formula at 60 = %gs",
				pack, name, spellID, spell.Name, secs, want)
		}
	}
	// Ring War signature-spell alerts time the boss's recast, not the
	// debuff, and only borrow the spell for its icon.
	recastTimers := map[string]bool{}
	for _, tr := range raidSignatureSpellAlerts() {
		recastTimers[tr.Name] = true
	}
	for _, p := range AllPacks() {
		for _, tr := range p.Triggers {
			if tr.TimerType != TimerTypeBuff && tr.TimerType != TimerTypeDetrimental {
				continue
			}
			if tr.PackName == "Raid Alerts" && recastTimers[tr.Name] {
				continue
			}
			// Boss-caster timers count down the boss's recast, not the
			// debuff; packs_raid_test.go checks those against npc_spells.
			if len(tr.BossCasters) > 0 {
				continue
			}
			check(p.PackName, tr.Name, tr.SpellID, tr.TimerDurationSecs)
			for _, x := range tr.ExtraPatterns {
				id, secs := x.SpellID, x.TimerDurationSecs
				if id == 0 {
					id = tr.SpellID
				}
				if secs == 0 {
					secs = tr.TimerDurationSecs
				}
				check(p.PackName, tr.Name+" "+x.Pattern, id, secs)
			}
		}
	}
}

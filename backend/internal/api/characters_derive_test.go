package api

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// osui is the base profile from testdata/Osui-Quarmy.txt: a level-60 Dark Elf
// (race 6) Enchanter (class 13, 0-indexed). Base attributes are the Quarmy
// values; the +6 INT below comes from Innate Intelligence rank 3 (an AA).
func osui() character.Character {
	return character.Character{
		Name: "Osui", Level: 60, Class: 13, Race: 6,
		BaseSTR: 60, BaseSTA: 65, BaseCHA: 95, BaseDEX: 75,
		BaseINT: 114, BaseAGI: 90, BaseWIS: 83,
	}
}

// TestDeriveBlock_Base checks the naked (base) layer: vitals derived purely
// from base attributes + always-on AA bonuses, no gear, no buffs.
func TestDeriveBlock_Base(t *testing.T) {
	h := &charactersHandler{}
	aa := db.AABonuses{INT: 6} // Innate Intelligence rank 3

	b := h.deriveBlock(osui(), aa, spellHasteSplit{}, skillCaps{defense: 100}, statBlock{}, 0, nil)

	if b.INT != 120 {
		t.Errorf("base INT = %d, want 120 (114 + 6 AA)", b.INT)
	}
	// BaseHP(Enc,60,STA 65)=876, +5 = 881.
	if b.HP != 881 {
		t.Errorf("base HP = %d, want 881", b.HP)
	}
	// BaseMana(Enc,60,INT 120)=1575.
	if b.Mana != 1575 {
		t.Errorf("base Mana = %d, want 1575", b.Mana)
	}
	// Dark Elf racial floors — DR/PR are 15, not the old hardcoded 25.
	if b.DR != 15 || b.PR != 15 || b.MR != 25 {
		t.Errorf("base resists MR/DR/PR = %d/%d/%d, want 25/15/15", b.MR, b.DR, b.PR)
	}
}

// TestDeriveBlock_Compounding feeds gear + a buff sized so the result lands on
// Osui's known in-game numbers (HP 4132, AC 931). This proves the layering
// composes correctly: item HP sits inside the AA HP-percent while buff HP is
// added after; a buff's STA compounds into HP; item AC takes the caster path
// and buff AC is treated as spell AC ÷3.
func TestDeriveBlock_Compounding(t *testing.T) {
	h := &charactersHandler{}
	aa := db.AABonuses{INT: 6}

	// Gear: +82 STA (→ total 147), +31 AGI (→ total 121), 1280 item HP, 470 AC.
	item := statBlock{STA: 82, AGI: 31, HP: 1280, AC: 470}
	// Buff: Aegolism-like — 1775 flat HP and 129 spell AC (÷3 → +43 mitigation).
	buffs := []resolvedBuff{{id: 999, delta: db.BuffStatDelta{HP: 1775, AC: 129}}}

	b := h.deriveBlock(osui(), aa, spellHasteSplit{}, skillCaps{defense: 100}, item, 0, buffs)

	if b.STA != 147 {
		t.Fatalf("total STA = %d, want 147", b.STA)
	}
	if b.AGI != 121 {
		t.Fatalf("total AGI = %d, want 121", b.AGI)
	}
	if b.HP != 4132 {
		t.Errorf("HP = %d, want 4132", b.HP)
	}
	if b.AC != 931 {
		t.Errorf("AC = %d, want 931", b.AC)
	}
}

// bard is a level-60 Half Elf (race 7) Bard (class 8, but 7 when 0-indexed).
// Attribute values are placeholders — only the class matters for these tests.
func bard() character.Character {
	return character.Character{
		Name: "Fiddle", Level: 60, Class: 7, Race: 7,
		BaseSTR: 75, BaseSTA: 75, BaseCHA: 90, BaseDEX: 85,
		BaseINT: 80, BaseAGI: 85, BaseWIS: 70,
	}
}

// TestDeriveBlock_BardBuffManaRegenExcluded verifies bards ignore buff-sourced
// mana regen (Clarity/KEI) while every other class receives it, and that worn
// item + AA mana regen and buff-sourced HP regen still apply to bards.
func TestDeriveBlock_BardBuffManaRegenExcluded(t *testing.T) {
	h := &charactersHandler{}

	// Clarity-like buff: +12 mana/tick, +8 HP/tick.
	clarity := []resolvedBuff{{id: 42, delta: db.BuffStatDelta{ManaRegen: 12, Regen: 8}}}
	// Worn item + AA mana regen that must survive regardless of class.
	item := statBlock{ManaRegen: 4}
	aa := db.AABonuses{ManaRegen: 3}

	enc := h.deriveBlock(osui(), aa, spellHasteSplit{}, skillCaps{defense: 100}, item, 0, clarity)
	if enc.ManaRegen != 19 { // 4 item + 3 AA + 12 buff
		t.Errorf("enchanter ManaRegen = %d, want 19", enc.ManaRegen)
	}
	if enc.Breakdown.ManaRegen.Buff != 12 {
		t.Errorf("enchanter buff split = %d, want 12", enc.Breakdown.ManaRegen.Buff)
	}

	brd := h.deriveBlock(bard(), aa, spellHasteSplit{}, skillCaps{defense: 100}, item, 0, clarity)
	if brd.ManaRegen != 7 { // 4 item + 3 AA, no buff
		t.Errorf("bard ManaRegen = %d, want 7 (item+AA only, no buff)", brd.ManaRegen)
	}
	if brd.Breakdown.ManaRegen.Buff != 0 {
		t.Errorf("bard buff split = %d, want 0", brd.Breakdown.ManaRegen.Buff)
	}
	if brd.Breakdown.ManaRegen.Item != 4 || brd.Breakdown.ManaRegen.AA != 3 {
		t.Errorf("bard item/AA split = %d/%d, want 4/3",
			brd.Breakdown.ManaRegen.Item, brd.Breakdown.ManaRegen.AA)
	}
	// HP regen from the same buff is not bard-excluded.
	if brd.Breakdown.Regen.Buff != 8 {
		t.Errorf("bard buff HP regen = %d, want 8 (not excluded)", brd.Breakdown.Regen.Buff)
	}
}

// TestDeriveBlock_BuffStaRaisesHP isolates the compounding bug the old additive
// model couldn't express: a pure +STA buff must raise HP beyond any flat HP it
// carries, because base HP scales with STA.
func TestDeriveBlock_BuffStaRaisesHP(t *testing.T) {
	h := &charactersHandler{}
	base := h.deriveBlock(osui(), db.AABonuses{}, spellHasteSplit{}, skillCaps{defense: 100}, statBlock{}, 0, nil)
	// A buff granting only +100 STA (no flat HP).
	buffed := h.deriveBlock(osui(), db.AABonuses{}, spellHasteSplit{}, skillCaps{defense: 100}, statBlock{}, 0,
		[]resolvedBuff{{id: 1, delta: db.BuffStatDelta{STA: 100}}})
	if buffed.HP <= base.HP {
		t.Errorf("STA buff did not raise HP: base %d, buffed %d", base.HP, buffed.HP)
	}
}

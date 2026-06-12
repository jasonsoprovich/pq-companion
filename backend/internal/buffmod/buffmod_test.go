package buffmod_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// requireTestdata skips the test when the named file under testdata/ is not
// present. testdata/ is gitignored (real game exports), so CI lacks it.
func requireTestdata(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "testdata", name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("testdata fixture %s not present", path)
	}
	return path
}

// repoRoot returns the repo root, derived from this test file's location.
// This file lives at backend/internal/buffmod/.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func openDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(repoRoot(t), "backend", "data", "quarm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// TestComputeOsui exercises the full pipeline against the real Osui quarmy
// export fixture. Per testdata, Osui has Dragon Scaled Mask equipped (focus
// 2335 = Extended Enhancement III, +15% beneficial) and the AAs Spell Casting
// Reinforcement rank 3 (+30% beneficial) and Spell Casting Reinforcement
// Mastery rank 1 (+20% beneficial). Resolving against a long-duration L60
// beneficial buff should land at +65%.
func TestComputeOsui(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")

	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	var sawDragonMask, sawSCR, sawSCRM bool
	for _, m := range res.Contributors {
		if m.Source == "item" && m.SourceItemName == "Dragon Scaled Mask" && m.SPA == buffmod.SPADuration && m.Percent == 15 {
			sawDragonMask = true
		}
		if m.Source == "aa" && m.SourceAAID == 21 && m.SourceAARank == 3 && m.Percent == 30 {
			sawSCR = true
		}
		if m.Source == "aa" && m.SourceAAID == 113 && m.SourceAARank == 1 && m.Percent == 20 {
			sawSCRM = true
		}
	}
	if !sawDragonMask {
		t.Errorf("expected Dragon Scaled Mask +15%% beneficial duration in contributors")
	}
	if !sawSCR {
		t.Errorf("expected Spell Casting Reinforcement rank 3 (+30%%) in contributors")
	}
	if !sawSCRM {
		t.Errorf("expected Spell Casting Reinforcement Mastery rank 1 (+20%%) in contributors")
	}

	// Resolve against Aegolism (id 1447): a long-duration L60 beneficial
	// Cleric buff. All three contributors should apply → +65%.
	aegolism, err := gameDB.GetSpell(1447)
	if err != nil {
		t.Fatalf("GetSpell(Aegolism): %v", err)
	}
	r := buffmod.Resolve(
		aegolism.ID, aegolism.Name,
		buffmod.SpellLevel(aegolism.ClassLevels), 60,
		aegolism.BuffDuration*6,
		buffmod.SpellTypeBeneficial, aegolism.EffectIDs[:],
		res.Contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	// 50% AAs (SCR3=30 + SCRM1=20) × 15% item (Dragon Mask) → 1.50 × 1.15 - 1
	// = 0.725 → 72%. Stored as integer % for display.
	if r.DurationAAPercent != 50 {
		t.Errorf("Aegolism AA duration %% = %d, want 50", r.DurationAAPercent)
	}
	if r.DurationItemPercent != 15 {
		t.Errorf("Aegolism item duration %% = %d, want 15", r.DurationItemPercent)
	}
	if r.DurationPercent != 72 {
		t.Errorf("Aegolism combined duration %% = %d, want 72 (1.50 × 1.15 = 1.725)", r.DurationPercent)
	}
}

// TestKEIOsuiExtendedDuration locks in the user-confirmed math: KEI base
// 9000s at L60 caster × (1 + 0.50 AA) × (1 + 0.15 item) = 15525s.
func TestKEIOsuiExtendedDuration(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")
	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	kei, err := gameDB.GetSpell(2570)
	if err != nil {
		t.Fatalf("GetSpell(KEI): %v", err)
	}
	r := buffmod.Resolve(
		kei.ID, kei.Name,
		buffmod.SpellLevel(kei.ClassLevels), 60,
		9000,
		buffmod.SpellTypeBeneficial, kei.EffectIDs[:],
		res.Contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if r.DurationAAPercent != 50 {
		t.Errorf("KEI AA duration %% = %d, want 50", r.DurationAAPercent)
	}
	if r.DurationItemPercent != 15 {
		t.Errorf("KEI item duration %% = %d, want 15", r.DurationItemPercent)
	}
	if r.ExtendedDurationSec != 15525 {
		t.Errorf("KEI extended duration = %ds, want 15525", r.ExtendedDurationSec)
	}
}

// TestKEIRejectsEH1 — Koadic's Endless Intellect is a L60 wizard spell.
// Gatorscale Sleeves' focus is Enhancement Haste I (max_level=20), so it must
// not contribute cast-time reduction to KEI. Duration should still resolve to
// +65% from the three beneficial-duration contributors.
func TestKEIRejectsEH1(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")
	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	kei, err := gameDB.GetSpell(2570)
	if err != nil {
		t.Fatalf("GetSpell(KEI): %v", err)
	}
	r := buffmod.Resolve(
		kei.ID, kei.Name,
		buffmod.SpellLevel(kei.ClassLevels), // L60 — rejects EH1 (max_level=20)
		60,                                  // caster level
		kei.BuffDuration*6,
		buffmod.SpellTypeBeneficial, kei.EffectIDs[:],
		res.Contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if r.DurationAAPercent != 50 || r.DurationItemPercent != 15 {
		t.Errorf("KEI duration: AA=%d item=%d, want AA=50 item=15", r.DurationAAPercent, r.DurationItemPercent)
	}
	// KEI's only matching cast-time foci should be EH3 (Ring of Shissar) and
	// Wind of the Nightcrawler (Slippers); both are −30%, only one applies.
	if r.CastTimePercent != 30 {
		t.Errorf("KEI cast time %% = %d, want 30 (best of EH3 / Wind of Nightcrawler)", r.CastTimePercent)
	}
}

// TestKEIBlocksEH1 asserts that a low-level focus (Enhancement Haste I, max
// level 20) is correctly filtered out for a L60 spell like KEI. Without the
// SpellLevel-based check, EH1 would erroneously match every beneficial buff.
func TestKEIBlocksEH1(t *testing.T) {
	gameDB := openDB(t)
	eh1, err := gameDB.GetSpell(2357)
	if err != nil {
		t.Fatalf("GetSpell(EH1): %v", err)
	}
	kei, err := gameDB.GetSpell(2570)
	if err != nil {
		t.Fatalf("GetSpell(KEI): %v", err)
	}
	// Synthetic contributor: a Gatorscale-style item with EH1 as its focus.
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   2458,
			SourceItemName: "Gatorscale Sleeves",
			FocusSpellID:   eh1.ID,
			FocusSpellName: eh1.Name,
			SPA:            buffmod.SPACastTime,
			Percent:        30,
			Limits: buffmod.Limits{
				MaxLevel:       20,
				SpellType:      buffmod.SpellTypeBeneficial,
				MinDurationSec: 72,
			},
		},
	}
	r := buffmod.Resolve(
		kei.ID, kei.Name,
		buffmod.SpellLevel(kei.ClassLevels), 60,
		kei.BuffDuration*6,
		buffmod.SpellTypeBeneficial, kei.EffectIDs[:],
		contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if r.CastTimePercent != 0 {
		t.Errorf("KEI + only-EH1 cast time %% = %d, want 0 (max_level filter)", r.CastTimePercent)
	}
	if len(r.Applied) != 0 {
		t.Errorf("KEI + only-EH1 applied count = %d, want 0", len(r.Applied))
	}
}

// TestSpellHasteCap confirms that SPA 127 contributions are clamped to 50%
// even when items + AAs would otherwise stack higher. Uses synthetic
// contributors: a 35% item focus + a 25% AA on a generic L60 beneficial
// buff would normally resolve to 60% cast-time reduction; the cap reins
// it back to 50.
func TestSpellHasteCap(t *testing.T) {
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   9999,
			SourceItemName: "Synthetic Focus 35",
			FocusSpellID:   1,
			FocusSpellName: "Synthetic SPA 127 Focus",
			SPA:            buffmod.SPACastTime,
			Percent:        35,
			Limits:         buffmod.Limits{SpellType: buffmod.SpellTypeBeneficial},
		},
		{
			Source:       "aa",
			SourceAAID:   1,
			SourceAAName: "Synthetic Spell Haste AA",
			SourceAARank: 1,
			SPA:          buffmod.SPACastTime,
			Percent:      25,
			Limits:       buffmod.Limits{SpellType: buffmod.SpellTypeBeneficial},
		},
	}
	r := buffmod.Resolve(
		1, "Synthetic Buff",
		60, 60, 600,
		buffmod.SpellTypeBeneficial,
		[]int{},
		contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if r.CastTimePercent != buffmod.SpellHasteCapPercent {
		t.Errorf("CastTimePercent = %d, want %d (capped)", r.CastTimePercent, buffmod.SpellHasteCapPercent)
	}
	if got := buffmod.SpellHasteSummary(contributors); got != buffmod.SpellHasteCapPercent {
		t.Errorf("SpellHasteSummary = %d, want %d (capped)", got, buffmod.SpellHasteCapPercent)
	}
}

// TestBardExemptOnInClassSpell confirms the bard exemption is unconditional:
// even when the clicked / cast spell IS in the bard's spell line (so the
// off-class gate would not fire on its own), bards still receive no SPA 128
// duration extensions. Bards are the lone class exempted from focus/AA
// duration boosts — until further user-confirmed testing, this is enforced
// regardless of in-class vs off-class.
func TestBardExemptOnInClassSpell(t *testing.T) {
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   9100,
			SourceItemName: "Blessed Coldain Prayer Shawl (synthetic)",
			FocusSpellID:   1,
			FocusSpellName: "Synthetic SPA 128 Focus",
			SPA:            buffmod.SPADuration,
			Percent:        20,
			Limits:         buffmod.Limits{SpellType: buffmod.SpellTypeBeneficial},
		},
	}
	// Bard-castable spell: ClassLevels[7] populated, off-class gate would
	// NOT fire on its own (since bard can cast it). The bard exemption
	// must still zero out the extension.
	var bardInClass [15]int
	for i := range bardInClass {
		bardInClass[i] = 255
	}
	bardInClass[buffmod.BardClassIdx] = 50 // bard learns at L50
	r := buffmod.Resolve(
		1, "Resonance (synthetic)",
		50, 60, 720, // 12 min base
		buffmod.SpellTypeBeneficial,
		[]int{},
		contributors,
		buffmod.BardClassIdx,
		bardInClass,
	)
	if r.DurationItemPercent != 0 {
		t.Errorf("bard on in-class spell item duration %% = %d, want 0 (bard exempt regardless of class match)", r.DurationItemPercent)
	}
	if r.ExtendedDurationSec != 720 {
		t.Errorf("bard on in-class spell extended duration = %ds, want 720 (unchanged)", r.ExtendedDurationSec)
	}
}

// TestOffClassClickyDurationGate confirms that when the caster's class cannot
// normally cast a spell (e.g. an Enchanter clicking a wizard-only Wand of
// Deflection), AA/item duration extensions do NOT apply — the click effect
// falls back to its base duration. Player-cast spells always pass the gate
// because the player by definition can cast their own class's spells.
func TestOffClassClickyDurationGate(t *testing.T) {
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   9001,
			SourceItemName: "Synthetic Duration Item",
			FocusSpellID:   1,
			FocusSpellName: "Synthetic SPA 128 Focus",
			SPA:            buffmod.SPADuration,
			Percent:        15,
			Limits:         buffmod.Limits{SpellType: buffmod.SpellTypeBeneficial},
		},
		{
			Source:       "aa",
			SourceAAID:   1,
			SourceAAName: "Synthetic Duration AA",
			SourceAARank: 1,
			SPA:          buffmod.SPADuration,
			Percent:      30,
			Limits:       buffmod.Limits{SpellType: buffmod.SpellTypeBeneficial},
		},
	}
	// Wizard-only spell: every class except Wizard (index 11) marked as
	// cannot-cast (255). Wizard sits at level 60.
	var wizOnly [15]int
	for i := range wizOnly {
		wizOnly[i] = 255
	}
	wizOnly[11] = 60

	// Caster = Enchanter (13) clicking a wizard-only spell. Item duration
	// focuses never apply to an off-class clicky (its effective level is 255,
	// above every era focus's max-level cap), but AA duration extensions have
	// no class/level limit and DO apply to clickies in-game.
	off := buffmod.Resolve(
		1, "Wand of Deflection (synthetic)",
		60, 60, 600,
		buffmod.SpellTypeBeneficial,
		[]int{},
		contributors,
		13, // Enchanter
		wizOnly,
	)
	if off.DurationAAPercent != 30 {
		t.Errorf("off-class clicky AA duration = %d, want 30 (AAs apply to clickies)", off.DurationAAPercent)
	}
	if off.DurationItemPercent != 0 {
		t.Errorf("off-class clicky item duration = %d, want 0 (item focus gated)", off.DurationItemPercent)
	}
	if off.ExtendedDurationSec != 780 {
		t.Errorf("off-class extended = %ds, want 780 (600 × 1.30 AA)", off.ExtendedDurationSec)
	}

	// Control: same caster class casting their own in-class spell. Build a
	// spell ClassLevels with Enchanter (13) able to cast → extensions apply.
	var encInClass [15]int
	encInClass[13] = 60
	in := buffmod.Resolve(
		1, "Synthetic Enchanter Buff",
		60, 60, 600,
		buffmod.SpellTypeBeneficial,
		[]int{},
		contributors,
		13, // Enchanter
		encInClass,
	)
	if in.DurationAAPercent != 30 {
		t.Errorf("in-class AA duration = %d, want 30", in.DurationAAPercent)
	}
	if in.DurationItemPercent != 15 {
		t.Errorf("in-class item duration = %d, want 15", in.DurationItemPercent)
	}

	// Regression for the Primal Avatar black-screen crash: Applied must never
	// be nil (a nil slice marshals to JSON null, which the frontend then
	// dereferences as r.applied.length and crashes the render). The off-class
	// gate keeps the matching AA, so Applied carries exactly that one entry.
	if off.Applied == nil {
		t.Error("off-class Applied is nil; must be non-nil to avoid JSON null")
	}
	if len(off.Applied) != 1 {
		t.Errorf("off-class Applied count = %d, want 1 (the AA only)", len(off.Applied))
	}
	if len(off.Applied) == 1 && off.Applied[0].Source != "aa" {
		t.Errorf("off-class Applied[0] source = %q, want \"aa\"", off.Applied[0].Source)
	}
}

// TestBardDurationExempt confirms that bard casters (class index 7) never
// receive SPA 128 duration extensions, even when AA + item focuses would
// otherwise apply. Reuses Osui's contributors (which produce +50% AA / +15%
// item on a generic beneficial buff) and resolves with casterClass=Bard —
// the resulting Resolution should be a no-op on duration. Cast-time (SPA 127)
// behaviour is independent of this rule.
func TestBardDurationExempt(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")
	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	kei, err := gameDB.GetSpell(2570)
	if err != nil {
		t.Fatalf("GetSpell(KEI): %v", err)
	}
	r := buffmod.Resolve(
		kei.ID, kei.Name,
		buffmod.SpellLevel(kei.ClassLevels), 60,
		9000,
		buffmod.SpellTypeBeneficial, kei.EffectIDs[:],
		res.Contributors,
		buffmod.BardClassIdx,
		kei.ClassLevels,
	)
	if r.DurationAAPercent != 0 {
		t.Errorf("bard AA duration %% = %d, want 0 (bard exempt)", r.DurationAAPercent)
	}
	if r.DurationItemPercent != 0 {
		t.Errorf("bard item duration %% = %d, want 0 (bard exempt)", r.DurationItemPercent)
	}
	if r.ExtendedDurationSec != 9000 {
		t.Errorf("bard extended duration = %ds, want 9000 (unchanged)", r.ExtendedDurationSec)
	}
}

// TestMultiClassSpellLevelFocusGate — Celerity (id 171) is learned at
// different levels per class: Enchanter 39, Shaman 56, Beastlord 63. The
// Batfang Headband's focus (Extended Enhancement II, id 2334) carries SPA 134
// max_level=44, so in-game it extends an Enchanter's Celerity but NOT a
// Shaman's — EQMacEmu compares spell.classes[casterClass], not the lowest
// class level. Regression for the user report where a 60 Shaman's Celerity
// showed +15% (4 extra minutes) it doesn't get in-game.
func TestMultiClassSpellLevelFocusGate(t *testing.T) {
	gameDB := openDB(t)
	celerity, err := gameDB.GetSpell(171)
	if err != nil {
		t.Fatalf("GetSpell(Celerity): %v", err)
	}
	const (
		shamanIdx    = 9 // 0-indexed EQ class
		enchanterIdx = 13
		shamanLevel  = 56
		enchanterLvl = 39
	)
	if got := buffmod.SpellLevelForClass(celerity.ClassLevels, shamanIdx); got != shamanLevel {
		t.Fatalf("SpellLevelForClass(Celerity, SHM) = %d, want %d", got, shamanLevel)
	}
	if got := buffmod.SpellLevelForClass(celerity.ClassLevels, enchanterIdx); got != enchanterLvl {
		t.Fatalf("SpellLevelForClass(Celerity, ENC) = %d, want %d", got, enchanterLvl)
	}
	// Unknown class falls back to the lowest class level (Enchanter's 39).
	if got := buffmod.SpellLevelForClass(celerity.ClassLevels, buffmod.CasterClassUnknown); got != enchanterLvl {
		t.Fatalf("SpellLevelForClass(Celerity, unknown) = %d, want %d (min fallback)", got, enchanterLvl)
	}

	// Synthetic Batfang Headband contributor: EH2 = +15% duration,
	// max_level 44, beneficial only.
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   10108,
			SourceItemName: "Batfang Headband",
			FocusSpellID:   2334,
			FocusSpellName: "Extended Enhancement II",
			SPA:            buffmod.SPADuration,
			Percent:        15,
			Limits: buffmod.Limits{
				MaxLevel:  44,
				SpellType: buffmod.SpellTypeBeneficial,
			},
		},
	}
	base := 30 * 60 // 30 min Celerity at L60, value irrelevant to the gate

	// Shaman: spell level 56 > max_level 44 → focus must NOT apply.
	shm := buffmod.Resolve(
		celerity.ID, celerity.Name,
		buffmod.SpellLevelForClass(celerity.ClassLevels, shamanIdx), 60,
		base,
		buffmod.SpellTypeBeneficial, celerity.EffectIDs[:],
		contributors,
		shamanIdx,
		celerity.ClassLevels,
	)
	if shm.DurationItemPercent != 0 {
		t.Errorf("Shaman Celerity item duration %% = %d, want 0 (L56 > EH2 max_level 44)", shm.DurationItemPercent)
	}
	if shm.ExtendedDurationSec != base {
		t.Errorf("Shaman Celerity extended = %ds, want %ds (unchanged)", shm.ExtendedDurationSec, base)
	}

	// Enchanter: spell level 39 ≤ 44 → focus applies.
	enc := buffmod.Resolve(
		celerity.ID, celerity.Name,
		buffmod.SpellLevelForClass(celerity.ClassLevels, enchanterIdx), 60,
		base,
		buffmod.SpellTypeBeneficial, celerity.EffectIDs[:],
		contributors,
		enchanterIdx,
		celerity.ClassLevels,
	)
	if enc.DurationItemPercent != 15 {
		t.Errorf("Enchanter Celerity item duration %% = %d, want 15 (L39 ≤ EH2 max_level 44)", enc.DurationItemPercent)
	}
}

// TestExclusionFilter — Complete Heal (id 1292) is in the SPA-137 exclusion
// list of Extended Enhancement III, so the Dragon Mask focus must not apply.
// The two AAs have no exclude list, so 30 + 20 = 50% should still resolve.
func TestExclusionFilter(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")
	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	completeHeal, err := gameDB.GetSpell(1292)
	if err != nil {
		t.Fatalf("GetSpell(Complete Heal): %v", err)
	}
	r := buffmod.Resolve(
		completeHeal.ID, completeHeal.Name,
		buffmod.SpellLevel(completeHeal.ClassLevels), 60, 0,
		buffmod.SpellTypeBeneficial,
		completeHeal.EffectIDs[:],
		res.Contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if r.DurationAAPercent != 50 {
		t.Errorf("Complete Heal AA duration %% = %d, want 50 (AAs apply)", r.DurationAAPercent)
	}
	if r.DurationItemPercent != 0 {
		t.Errorf("Complete Heal item duration %% = %d, want 0 (mask excluded)", r.DurationItemPercent)
	}
}

// TestIncludeEffectLimit — focuses with a positive SPA-137 base only apply to
// spells that CONTAIN that effect (EQMacEmu SE_LimitEffect include semantics).
// Summoning Haste (Encyclopedia Necrotheurgia line) carries 137:33, so its 30%
// cast-time reduction applies to pet summons (SPA 33) and nothing else.
func TestIncludeEffectLimit(t *testing.T) {
	contributors := []buffmod.Modifier{
		{
			Source:         "item",
			SourceItemID:   9002,
			SourceItemName: "Synthetic Summoning Haste Item",
			FocusSpellID:   2352,
			FocusSpellName: "Summoning Haste II",
			SPA:            buffmod.SPACastTime,
			Percent:        30,
			Limits: buffmod.Limits{
				SpellType:      buffmod.SpellTypeUnset,
				MaxLevel:       44,
				IncludeEffects: []int{33}, // SE_SummonPet
			},
		},
	}

	pet := buffmod.Resolve(
		1, "Synthetic Pet Summon",
		40, 60, 0,
		buffmod.SpellTypeBeneficial,
		[]int{33, 254, 254, 254},
		contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if pet.CastTimePercent != 30 {
		t.Errorf("pet summon cast time %% = %d, want 30 (include-effect matched)", pet.CastTimePercent)
	}

	nonPet := buffmod.Resolve(
		2, "Synthetic Nuke",
		40, 60, 0,
		buffmod.SpellTypeDetrimental,
		[]int{0, 254, 254, 254},
		contributors,
		buffmod.CasterClassUnknown, [15]int{},
	)
	if nonPet.CastTimePercent != 0 {
		t.Errorf("non-pet cast time %% = %d, want 0 (include-effect 33 not present)", nonPet.CastTimePercent)
	}
}

// TestIncludeSpellLimit — SPA 141 with a positive base whitelists specific
// spell IDs; a negative base excludes that spell ID.
func TestIncludeSpellLimit(t *testing.T) {
	whitelist := []buffmod.Modifier{
		{
			Source:  "item",
			SPA:     buffmod.SPADuration,
			Percent: 20,
			Limits: buffmod.Limits{
				SpellType:     buffmod.SpellTypeUnset,
				IncludeSpells: []int{1447}, // Aegolism only
			},
		},
	}
	hit := buffmod.Resolve(
		1447, "Aegolism", 60, 60, 600,
		buffmod.SpellTypeBeneficial, []int{},
		whitelist, buffmod.CasterClassUnknown, [15]int{},
	)
	if hit.DurationItemPercent != 20 {
		t.Errorf("whitelisted spell item duration %% = %d, want 20", hit.DurationItemPercent)
	}
	miss := buffmod.Resolve(
		2570, "Koadic's Endless Intellect", 60, 60, 600,
		buffmod.SpellTypeBeneficial, []int{},
		whitelist, buffmod.CasterClassUnknown, [15]int{},
	)
	if miss.DurationItemPercent != 0 {
		t.Errorf("non-whitelisted spell item duration %% = %d, want 0", miss.DurationItemPercent)
	}

	exclude := []buffmod.Modifier{
		{
			Source:  "item",
			SPA:     buffmod.SPADuration,
			Percent: 20,
			Limits: buffmod.Limits{
				SpellType:     buffmod.SpellTypeUnset,
				ExcludeSpells: []int{1292}, // never Complete Heal
			},
		},
	}
	blocked := buffmod.Resolve(
		1292, "Complete Heal", 60, 60, 600,
		buffmod.SpellTypeBeneficial, []int{},
		exclude, buffmod.CasterClassUnknown, [15]int{},
	)
	if blocked.DurationItemPercent != 0 {
		t.Errorf("excluded spell item duration %% = %d, want 0", blocked.DurationItemPercent)
	}
}

// TestPermanentIllusionFlag — Osui has the Permanent Illusion AA (eqmacid 55)
// in the fixture export, so Compute must surface it; HasIllusionEffect gates
// the override to spells that actually contain SPA 58 (SE_Illusion).
func TestPermanentIllusionFlag(t *testing.T) {
	requireTestdata(t, "Osui-Quarmy.txt")
	gameDB := openDB(t)
	eqPath := filepath.Join(repoRoot(t), "testdata")
	res, err := buffmod.Compute(eqPath, "Osui", gameDB)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if !res.PermanentIllusion {
		t.Error("Compute(Osui).PermanentIllusion = false, want true (AA 55 rank 1 in fixture)")
	}
	if !buffmod.HasIllusionEffect([]int{58, 254, 254}) {
		t.Error("HasIllusionEffect([58 ...]) = false, want true")
	}
	if buffmod.HasIllusionEffect([]int{86, 254, 254}) {
		t.Error("HasIllusionEffect without SPA 58 = true, want false")
	}
}

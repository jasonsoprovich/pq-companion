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
	)
	if r.CastTimePercent != 0 {
		t.Errorf("KEI + only-EH1 cast time %% = %d, want 0 (max_level filter)", r.CastTimePercent)
	}
	if len(r.Applied) != 0 {
		t.Errorf("KEI + only-EH1 applied count = %d, want 0", len(r.Applied))
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
	)
	if r.DurationAAPercent != 50 {
		t.Errorf("Complete Heal AA duration %% = %d, want 50 (AAs apply)", r.DurationAAPercent)
	}
	if r.DurationItemPercent != 0 {
		t.Errorf("Complete Heal item duration %% = %d, want 0 (mask excluded)", r.DurationItemPercent)
	}
}

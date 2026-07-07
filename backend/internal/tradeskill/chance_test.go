package tradeskill

import "testing"

func TestEffectiveSkill(t *testing.T) {
	tests := []struct {
		name    string
		raw     int
		mod     int
		wantEff int
	}{
		{"no mod", 60, 0, 60},
		{"plus 10pct floors down", 60, 10, 66}, // floor(60*1.10)=66
		{"plus 5pct", 200, 5, 210},             // floor(200*1.05)=210
		{"mod caps at 252", 250, 10, 252},      // floor(275)->252
		{"already above cap raw", 300, 0, 252}, // raw clamped by hard cap
		{"negative raw treated as 0", -5, 10, 0},
	}
	for _, tc := range tests {
		if got := EffectiveSkill(tc.raw, tc.mod); got != tc.wantEff {
			t.Errorf("%s: EffectiveSkill(%d,%d)=%d, want %d", tc.name, tc.raw, tc.mod, got, tc.wantEff)
		}
	}
}

func TestChance(t *testing.T) {
	tests := []struct {
		name        string
		raw         int
		trivial     int
		mod         int
		aa          int
		nofail      bool
		wantSuccess float64
		wantEff     int
		wantFloor   int
		wantAtFloor bool
		wantReach   bool
		wantAtTriv  bool
	}{
		// Spirit of Wolf potion (Alchemy), pqdi recipe 1229: trivial 37.
		{"SoW at trivial", 37, 37, 0, 0, false, 66, 37, 37 + 29, false, true, true},
		{"SoW mid", 50, 37, 0, 0, false, 79, 50, 66, false, true, true},
		{"SoW at 5pct floor breakpoint", 66, 37, 0, 0, false, 95, 66, 66, true, true, true},
		{"SoW above breakpoint clamps at 95", 100, 37, 0, 0, false, 95, 100, 66, true, true, true},
		{"SoW below trivial", 20, 37, 0, 0, false, 49, 20, 66, false, true, false},

		// A +10% skill-mod robe at raw 60 reaches the trivial-37 floor (eff 66).
		{"skillmod reaches floor early", 60, 37, 10, 0, false, 95, 66, 66, true, true, true},

		// trivial >= 68 branch: breakpoint = ceil(43.5 + 0.75*trivial).
		{"triv100 at trivial", 100, 100, 0, 0, false, 76.5, 100, 119, false, true, true},
		{"triv100 at floor breakpoint", 119, 100, 0, 0, false, 95, 119, 119, true, true, true},

		// Low clamp: chance can never fall below 5%.
		{"far below trivial clamps at 5", 10, 200, 0, 0, false, 5, 10, 194, false, true, false},

		// Unreachable floor: trivial 300 needs eff 269 > 252 cap.
		{"triv300 floor unreachable", 252, 300, 0, 0, false, 78.5, 252, 269, false, false, false},

		// No-fail recipe is always 100%.
		{"nofail is 100", 1, 200, 0, 0, true, 100, 1, 194, true, true, false},

		// AA fail-reduction lifts the clamped chance toward 100.
		{"aa 50pct reduces failure", 37, 37, 0, 50, false, 83, 37, 66, false, true, true},
	}
	for _, tc := range tests {
		got := Chance(tc.raw, tc.trivial, tc.mod, tc.aa, 0, tc.nofail)
		if got.Success != tc.wantSuccess {
			t.Errorf("%s: Success=%v, want %v", tc.name, got.Success, tc.wantSuccess)
		}
		if got.Failure != round1(100-tc.wantSuccess) {
			t.Errorf("%s: Failure=%v, want %v", tc.name, got.Failure, round1(100-tc.wantSuccess))
		}
		if got.EffSkill != tc.wantEff {
			t.Errorf("%s: EffSkill=%d, want %d", tc.name, got.EffSkill, tc.wantEff)
		}
		if got.FloorSkill != tc.wantFloor {
			t.Errorf("%s: FloorSkill=%d, want %d", tc.name, got.FloorSkill, tc.wantFloor)
		}
		if got.AtFloor != tc.wantAtFloor {
			t.Errorf("%s: AtFloor=%v, want %v", tc.name, got.AtFloor, tc.wantAtFloor)
		}
		if got.FloorReachable != tc.wantReach {
			t.Errorf("%s: FloorReachable=%v, want %v", tc.name, got.FloorReachable, tc.wantReach)
		}
		if got.AtTrivial != tc.wantAtTriv {
			t.Errorf("%s: AtTrivial=%v, want %v", tc.name, got.AtTrivial, tc.wantAtTriv)
		}
	}
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func TestSuccessAtTrivial(t *testing.T) {
	// trivial < 68: success at trivial is always 66% (34% fail) — the "trivial
	// isn't safe" number, independent of current skill.
	for _, cur := range []int{1, 20, 37, 100} {
		if got := Chance(cur, 37, 0, 0, 0, false).SuccessAtTrivial; got != 66 {
			t.Errorf("trivial 37 at skill %d: SuccessAtTrivial=%v, want 66", cur, got)
		}
	}
	// trivial >= 68: 0.25*trivial + 51.5 -> at 100 that's 76.5%.
	if got := Chance(50, 100, 0, 0, 0, false).SuccessAtTrivial; got != 76.5 {
		t.Errorf("trivial 100: SuccessAtTrivial=%v, want 76.5", got)
	}
	// A no-fail recipe is 100% at trivial too.
	if got := Chance(1, 50, 0, 0, 0, true).SuccessAtTrivial; got != 100 {
		t.Errorf("nofail: SuccessAtTrivial=%v, want 100", got)
	}
}

// TestSuccessAtCap covers the "even at max skill" best case — the honest floor
// for a below-cap character, which is NOT their current-skill failure.
func TestSuccessAtCap(t *testing.T) {
	// Grimrose's tailoring case: skill 224, cap 250, a very high trivial (335) the
	// character can never trivialize. At 224 the failure is high, but at their 250
	// cap it's far lower — the panel must report the cap number, not the current.
	r := Chance(224, 335, 0, 0, 250, false)
	if r.FloorReachableAtCap {
		t.Errorf("floor should be unreachable even at cap 250, got reachable")
	}
	// success at cap 250: 250 - 0.75*335 + 51.5 = 50.25 -> 50.3 (one decimal).
	if r.SuccessAtCap != 50.3 {
		t.Errorf("SuccessAtCap=%v, want 50.3", r.SuccessAtCap)
	}
	if r.FailureAtCap != 49.7 {
		t.Errorf("FailureAtCap=%v, want 49.7", r.FailureAtCap)
	}
	// The current-skill failure is much worse than the cap failure — the whole
	// point of the fix.
	if !(r.Failure > r.FailureAtCap) {
		t.Errorf("current failure %v should exceed cap failure %v", r.Failure, r.FailureAtCap)
	}

	// When the 5% floor IS reachable within the cap, FloorReachableAtCap is true.
	if got := Chance(50, 100, 0, 0, 250, false); !got.FloorReachableAtCap {
		t.Errorf("trivial 100 floor (%d) is reachable at cap 250, want reachable", got.FloorSkill)
	}

	// No cap known: cap fields stay zero and reachability falls back to the hard
	// 252 cap.
	if got := Chance(224, 335, 0, 0, 0, false); got.CapSkill != 0 || got.SuccessAtCap != 0 {
		t.Errorf("no cap: expected zero cap fields, got %+v", got)
	}
}

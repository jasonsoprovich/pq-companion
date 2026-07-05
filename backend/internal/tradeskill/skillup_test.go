package tradeskill

import (
	"math"
	"testing"
)

func TestTradeStat(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		ts, str, dex, intel, wis int
		want                     int
		label                    string
	}{
		// General case subtracts 15; uses the better of WIS/INT.
		{"alchemy caster", 59, 60, 75, 114, 83, 114 - 15, "WIS/INT"},
		{"pottery low stats", 69, 70, 70, 20, 30, 30 - 15, "WIS/INT"},
		{"general floors at 0", 60, 10, 10, 10, 10, 0, "WIS/INT"},
		// Blacksmithing uses max(STR,INT,WIS), no -15.
		{"blacksmith str", 63, 150, 60, 90, 100, 150, "STR/INT/WIS"},
		// Fletching / Make Poison use max(DEX,INT,WIS), no -15.
		{"fletching dex", 64, 60, 120, 80, 70, 120, "DEX/INT/WIS"},
		{"poison dex", 56, 60, 95, 110, 70, 110, "DEX/INT/WIS"},
	} {
		got, label := TradeStat(tc.ts, tc.str, tc.dex, tc.intel, tc.wis)
		if got != tc.want || label != tc.label {
			t.Errorf("%s: TradeStat=%d/%q, want %d/%q", tc.name, got, label, tc.want, tc.label)
		}
	}
}

func TestRollProbs(t *testing.T) {
	if realRollProb(0) != 0 || realRollProb(1) != 0 {
		t.Error("realRollProb should be 0 at/below 1")
	}
	if realRollProb(1500) != 1 {
		t.Error("realRollProb should clamp to 1")
	}
	if got := realRollProb(1000); math.Abs(got-999.0/999.0) > 1e-9 {
		t.Errorf("realRollProb(1000)=%v", got)
	}
	// Second check: 1 at low skill, 0.5 mid, 0.05 floor near cap.
	if skillRollProb(15) != 1 || skillRollProb(10) != 1 {
		t.Error("skillRollProb should be 1 for skill <= 15")
	}
	if skillRollProb(100) != 0.5 {
		t.Errorf("skillRollProb(100)=%v, want 0.5", skillRollProb(100))
	}
	if skillRollProb(190) != 0.05 || skillRollProb(240) != 0.05 {
		t.Error("skillRollProb should floor at 0.05 for skill >= 190")
	}
}

func TestEstimateSkillUp(t *testing.T) {
	// Already at/above where skill-ups stop.
	maxed := EstimateSkillUp(120, 100, 0, 200, 4, 0, 0, false)
	if !maxed.Maxed || maxed.AttemptsToTarget != 0 {
		t.Errorf("expected maxed, got %+v", maxed)
	}

	// Capped by class/level below trivial.
	capped := EstimateSkillUp(50, 200, 100, 150, 4, 0, 0, false)
	if !capped.AtCap || capped.TargetSkill != 100 || capped.PointsToGo != 50 {
		t.Errorf("expected cap at 100 with 50 to go, got %+v", capped)
	}

	// No stat or no difficulty -> impractical.
	if !EstimateSkillUp(50, 200, 0, 0, 4, 0, 0, false).Impractical {
		t.Error("tradeStat 0 should be impractical")
	}
	if !EstimateSkillUp(50, 200, 0, 100, 0, 0, 0, false).Impractical {
		t.Error("difficulty 0 should be impractical")
	}

	// Single-point case, hand-computed: current 100, trivial 101, tradeStat 200,
	// difficulty 4. p(success)=75.75%; p1succ=.4995 p1fail=.24925; P2(100)=.5;
	// pUp = .5*(.7575*.4995 + .2425*.24925) ≈ .2194 -> ~4.56 attempts.
	one := EstimateSkillUp(100, 101, 0, 200, 4, 0, 0, false)
	if one.PointsToGo != 1 {
		t.Errorf("PointsToGo=%d, want 1", one.PointsToGo)
	}
	if math.Abs(one.AttemptsToNext-4.6) > 0.2 {
		t.Errorf("AttemptsToNext=%v, want ~4.6", one.AttemptsToNext)
	}
	if math.Abs(one.AttemptsToTarget-5) > 1 {
		t.Errorf("AttemptsToTarget=%v, want ~5", one.AttemptsToTarget)
	}

	// Skilling up gets slower as you approach the cap: attempts-to-next at a
	// near-cap skill should exceed that at a low skill.
	low := EstimateSkillUp(50, 250, 0, 150, 4, 0, 0, false)
	high := EstimateSkillUp(185, 250, 0, 150, 4, 0, 0, false)
	if !(high.AttemptsToNext > low.AttemptsToNext) {
		t.Errorf("expected near-cap slower: low=%v high=%v", low.AttemptsToNext, high.AttemptsToNext)
	}

	// A higher item skill-mod (more successes) should not increase the estimate.
	noMod := EstimateSkillUp(60, 120, 0, 120, 4, 0, 0, false)
	withMod := EstimateSkillUp(60, 120, 0, 120, 4, 15, 0, false)
	if withMod.AttemptsToTarget > noMod.AttemptsToTarget {
		t.Errorf("mod should not slow skilling: noMod=%v withMod=%v",
			noMod.AttemptsToTarget, withMod.AttemptsToTarget)
	}
}

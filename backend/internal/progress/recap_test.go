package progress

import (
	"testing"
	"time"
)

func TestBuildRecap_AggregatesJournalEvents(t *testing.T) {
	since := time.Unix(1_700_000_000, 0)
	now := since.Add(30 * 24 * time.Hour)

	events := []Event{
		{Character: "Osui", At: since.Add(time.Hour), Kind: KindLevel, Value: 51, Delta: 1},
		{Character: "Osui", At: since.Add(2 * time.Hour), Kind: KindLevel, Value: 53, Delta: 2},
		{Character: "Osui", At: since.Add(3 * time.Hour), Kind: KindAA, Value: 5},
		{Character: "Osui", At: since.Add(4 * time.Hour), Kind: KindAA, Value: 6},
		{Character: "Osui", At: since.Add(5 * time.Hour), Kind: KindSpell, Detail: "Mesmerization"},
		// Baking (skill_id 60) is a tradeskill; Swimming (skill_id 50) isn't.
		{Character: "Osui", At: since.Add(6 * time.Hour), Kind: KindSkill, Detail: "Baking", Value: 10},
		{Character: "Osui", At: since.Add(48 * time.Hour), Kind: KindSkill, Detail: "Swimming", Value: 20},
	}

	// ActiveDays is sourced from the login-day table, not the journal —
	// these two dates mirror the two distinct calendar days the events
	// above land on (hour 1-6, then hour 48).
	loginDays := []string{
		since.Add(time.Hour).Local().Format("2006-01-02"),
		since.Add(48 * time.Hour).Local().Format("2006-01-02"),
	}
	r := BuildRecap("Osui", events, loginDays, nil, nil, since, now)

	if r.StartLevel != 50 {
		t.Errorf("StartLevel = %d, want 50", r.StartLevel)
	}
	if r.EndLevel != 53 {
		t.Errorf("EndLevel = %d, want 53", r.EndLevel)
	}
	if r.LevelsGained != 3 {
		t.Errorf("LevelsGained = %d, want 3", r.LevelsGained)
	}
	if r.AAsGained != 2 {
		t.Errorf("AAsGained = %d, want 2", r.AAsGained)
	}
	if r.SpellsScribed != 1 {
		t.Errorf("SpellsScribed = %d, want 1", r.SpellsScribed)
	}
	if r.TradeskillUps != 1 {
		t.Errorf("TradeskillUps = %d, want 1", r.TradeskillUps)
	}
	if r.SkillUps != 1 {
		t.Errorf("SkillUps = %d, want 1", r.SkillUps)
	}
	// Events land on two distinct calendar days (hour 1-6, then hour 48).
	if r.ActiveDays != 2 {
		t.Errorf("ActiveDays = %d, want 2", r.ActiveDays)
	}
	if len(r.DailyActivity) != 2 {
		t.Fatalf("DailyActivity has %d buckets, want 2", len(r.DailyActivity))
	}
	if r.DailyActivity[0].Count != 6 {
		t.Errorf("first day Count = %d, want 6", r.DailyActivity[0].Count)
	}
	if r.HasSnapshotData {
		t.Error("HasSnapshotData = true with nil snapshots, want false")
	}
}

func TestBuildRecap_LevelDrainNetsAgainstGains(t *testing.T) {
	since := time.Unix(1_700_000_000, 0)
	now := since.Add(time.Hour)

	events := []Event{
		{Character: "Osui", At: since.Add(10 * time.Minute), Kind: KindLevel, Value: 55, Delta: 1},
		{Character: "Osui", At: since.Add(20 * time.Minute), Kind: KindLevel, Value: 54, Delta: -1},
	}

	r := BuildRecap("Osui", events, nil, nil, nil, since, now)
	if r.StartLevel != 54 {
		t.Errorf("StartLevel = %d, want 54", r.StartLevel)
	}
	if r.EndLevel != 54 {
		t.Errorf("EndLevel = %d, want 54", r.EndLevel)
	}
	if r.LevelsGained != 0 {
		t.Errorf("LevelsGained = %d, want 0 (net of +1/-1)", r.LevelsGained)
	}
}

func TestBuildRecap_UsesSnapshotsForCoinAndFallbackLevels(t *testing.T) {
	since := time.Unix(1_700_000_000, 0)
	now := since.Add(30 * 24 * time.Hour)

	start := &Snapshot{Character: "Osui", TakenAt: since, Level: 50, Copper: 100_000}
	end := &Snapshot{Character: "Osui", TakenAt: now, Level: 55, Copper: 250_000}

	// No journal events at all (e.g. this character's log doesn't reach
	// back to `since`) — the recap should still report level bounds and
	// coin delta from snapshots alone.
	r := BuildRecap("Osui", nil, nil, start, end, since, now)

	if !r.HasSnapshotData {
		t.Fatal("HasSnapshotData = false, want true")
	}
	if r.CoinDelta != 150_000 {
		t.Errorf("CoinDelta = %d, want 150000", r.CoinDelta)
	}
	if r.CurrentCopper != 250_000 {
		t.Errorf("CurrentCopper = %d, want 250000", r.CurrentCopper)
	}
	if r.StartLevel != 50 || r.EndLevel != 55 {
		t.Errorf("StartLevel/EndLevel = %d/%d, want 50/55", r.StartLevel, r.EndLevel)
	}
}

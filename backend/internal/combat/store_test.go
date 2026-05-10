package combat

import (
	"path/filepath"
	"testing"
	"time"
)

// newTestStore opens a fresh HistoryStore against a temp-dir SQLite file.
// Caller may rely on t.TempDir cleanup; an explicit Close is wired via
// t.Cleanup so the connection drains before the dir is removed.
func newTestStore(t *testing.T) *HistoryStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenHistoryStore(path)
	if err != nil {
		t.Fatalf("OpenHistoryStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// sampleFight builds a small FightSummary suitable for round-tripping
// through the store. Reused across tests so the assertions can pivot on
// stable known values.
func sampleFight(npc string, start time.Time) FightSummary {
	return FightSummary{
		StartTime:     start,
		EndTime:       start.Add(10 * time.Second),
		Duration:      10,
		PrimaryTarget: npc,
		TotalDamage:   1500,
		TotalDPS:      150,
		YouDamage:     900,
		YouDPS:        90,
		TotalHeal:     400,
		TotalHPS:      40,
		YouHeal:       250,
		YouHPS:        25,
		Combatants: []EntityStats{
			{Name: "Osui", TotalDamage: 900, HitCount: 9, MaxHit: 200, DPS: 90, ActiveDPS: 100, ActiveSeconds: 9, CritCount: 2, CritDamage: 400},
			{Name: "Sandrian", TotalDamage: 600, HitCount: 6, MaxHit: 150, DPS: 60, ActiveDPS: 75, ActiveSeconds: 8},
		},
		Healers: []HealerStats{
			{Name: "Cleric1", TotalHeal: 400, HealCount: 2, MaxHeal: 250, HPS: 40, ActiveHPS: 50, ActiveSeconds: 8},
		},
	}
}

func TestSaveAndGetFight(t *testing.T) {
	s := newTestStore(t)
	start := time.Now().Truncate(time.Second)

	id, err := s.SaveFight(sampleFight("Aten Ha Ra", start), "The Hole", "Osui")
	if err != nil {
		t.Fatalf("SaveFight: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive insert id, got %d", id)
	}

	got, err := s.GetFight(id)
	if err != nil {
		t.Fatalf("GetFight: %v", err)
	}
	if got == nil {
		t.Fatal("GetFight returned nil for existing id")
	}
	if got.NPCName != "Aten Ha Ra" || got.Zone != "The Hole" || got.CharacterName != "Osui" {
		t.Errorf("identity mismatch: got %+v", got)
	}
	if got.TotalDamage != 1500 || got.YouDamage != 900 {
		t.Errorf("damage mismatch: got total=%d you=%d", got.TotalDamage, got.YouDamage)
	}
	if len(got.Combatants) != 2 || got.Combatants[0].Name != "Osui" || got.Combatants[0].CritCount != 2 {
		t.Errorf("combatants round-trip mismatch: %+v", got.Combatants)
	}
	if len(got.Healers) != 1 || got.Healers[0].Name != "Cleric1" {
		t.Errorf("healers round-trip mismatch: %+v", got.Healers)
	}
	// time.Time round-trips through unix nanos; equality must hold.
	if !got.StartTime.Equal(start) {
		t.Errorf("StartTime mismatch: got %v want %v", got.StartTime, start)
	}
}

func TestGetFightMissingReturnsNil(t *testing.T) {
	s := newTestStore(t)
	got, err := s.GetFight(99999)
	if err != nil {
		t.Fatalf("expected nil error for missing id, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil fight for missing id, got %+v", got)
	}
}

func TestListFightsReturnsNewestFirst(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().Truncate(time.Second).Add(-time.Hour)

	// Save in oldest-first order; list should come back newest-first.
	for i := 0; i < 5; i++ {
		_, err := s.SaveFight(sampleFight("a gnoll", base.Add(time.Duration(i)*time.Minute)), "z", "Osui")
		if err != nil {
			t.Fatalf("SaveFight #%d: %v", i, err)
		}
	}

	out, err := s.ListFights(FightFilter{})
	if err != nil {
		t.Fatalf("ListFights: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("expected 5 fights, got %d", len(out))
	}
	for i := 1; i < len(out); i++ {
		if !out[i-1].StartTime.After(out[i].StartTime) {
			t.Errorf("ordering broken: fight %d (%v) not after fight %d (%v)",
				i-1, out[i-1].StartTime, i, out[i].StartTime)
		}
	}
}

func TestListFightsFilterByNPC(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	for _, npc := range []string{"a gnoll", "Aten Ha Ra", "an orc warrior", "Aten Ha Ra"} {
		if _, err := s.SaveFight(sampleFight(npc, now), "z", "Osui"); err != nil {
			t.Fatalf("SaveFight: %v", err)
		}
	}

	// Substring match, case-insensitive.
	out, err := s.ListFights(FightFilter{NPCName: "aten"})
	if err != nil {
		t.Fatalf("ListFights: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 'aten*' matches, got %d", len(out))
	}
	for _, f := range out {
		if f.NPCName != "Aten Ha Ra" {
			t.Errorf("unexpected NPC in filter: %q", f.NPCName)
		}
	}
}

func TestListFightsFilterByDateRange(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().Truncate(time.Second).Add(-24 * time.Hour)

	// Three fights across three hours.
	for i := 0; i < 3; i++ {
		if _, err := s.SaveFight(sampleFight("a gnoll", base.Add(time.Duration(i)*time.Hour)), "z", "Osui"); err != nil {
			t.Fatalf("SaveFight: %v", err)
		}
	}

	// Window covers only the middle fight.
	mid := base.Add(time.Hour)
	out, err := s.ListFights(FightFilter{
		StartTime: mid.Add(-time.Minute),
		EndTime:   mid.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ListFights: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 fight in middle window, got %d", len(out))
	}
	if !out[0].StartTime.Equal(mid) {
		t.Errorf("expected mid fight, got %v", out[0].StartTime)
	}
}

func TestPruneOlderThan(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	// 3 old, 2 fresh.
	for i := 0; i < 3; i++ {
		_, _ = s.SaveFight(sampleFight("a gnoll", now.Add(-48*time.Hour-time.Duration(i)*time.Minute)), "z", "Osui")
	}
	for i := 0; i < 2; i++ {
		_, _ = s.SaveFight(sampleFight("a wolf", now.Add(-time.Duration(i)*time.Minute)), "z", "Osui")
	}

	removed, err := s.PruneOlderThan(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("PruneOlderThan: %v", err)
	}
	if removed != 3 {
		t.Errorf("expected 3 rows removed, got %d", removed)
	}
	out, _ := s.ListFights(FightFilter{})
	if len(out) != 2 {
		t.Errorf("expected 2 fights remaining, got %d", len(out))
	}
}

func TestDeleteFightAndDeleteAll(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	id1, _ := s.SaveFight(sampleFight("a gnoll", now), "z", "Osui")
	_, _ = s.SaveFight(sampleFight("a wolf", now), "z", "Osui")

	n, err := s.DeleteFight(id1)
	if err != nil {
		t.Fatalf("DeleteFight: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 row deleted, got %d", n)
	}

	// Deleting a missing id is a 0-row no-op, not an error.
	if n, err := s.DeleteFight(99999); err != nil || n != 0 {
		t.Errorf("DeleteFight(missing) = %d, %v; want 0, nil", n, err)
	}

	if n, err := s.DeleteAll(); err != nil || n != 1 {
		t.Errorf("DeleteAll = %d, %v; want 1, nil", n, err)
	}

	out, _ := s.ListFights(FightFilter{})
	if len(out) != 0 {
		t.Errorf("expected empty after DeleteAll, got %d", len(out))
	}
}

func TestFacetsReturnsDistinctSortedNonEmpty(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	// Three characters, two zones; an empty-zone row to verify the WHERE
	// filter drops blanks rather than surfacing them as empty options.
	for _, row := range []struct {
		npc, zone, char string
	}{
		{"a gnoll", "East Karana", "Osui"},
		{"a wolf", "East Karana", "Osui"},
		{"Aten Ha Ra", "The Hole", "Nariana"},
		{"a kobold", "", "Osui"},  // empty zone: should not appear in zones
		{"a goblin", "Mistmoore", ""}, // empty character: not in characters
	} {
		if _, err := s.SaveFight(sampleFight(row.npc, now), row.zone, row.char); err != nil {
			t.Fatalf("SaveFight: %v", err)
		}
	}

	f, err := s.Facets()
	if err != nil {
		t.Fatalf("Facets: %v", err)
	}
	wantChars := []string{"Nariana", "Osui"} // sorted, no empty
	if len(f.Characters) != len(wantChars) {
		t.Fatalf("Characters = %v, want %v", f.Characters, wantChars)
	}
	for i := range wantChars {
		if f.Characters[i] != wantChars[i] {
			t.Errorf("Characters[%d] = %q, want %q", i, f.Characters[i], wantChars[i])
		}
	}
	wantZones := []string{"East Karana", "Mistmoore", "The Hole"}
	if len(f.Zones) != len(wantZones) {
		t.Fatalf("Zones = %v, want %v", f.Zones, wantZones)
	}
	for i := range wantZones {
		if f.Zones[i] != wantZones[i] {
			t.Errorf("Zones[%d] = %q, want %q", i, f.Zones[i], wantZones[i])
		}
	}
}

func TestCount(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	for i := 0; i < 4; i++ {
		_, _ = s.SaveFight(sampleFight("a gnoll", now), "z", "Osui")
	}
	_, _ = s.SaveFight(sampleFight("Aten Ha Ra", now), "z", "Osui")

	total, err := s.Count(FightFilter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 5 {
		t.Errorf("Count() = %d, want 5", total)
	}

	gnolls, err := s.Count(FightFilter{NPCName: "gnoll"})
	if err != nil {
		t.Fatalf("Count(gnoll): %v", err)
	}
	if gnolls != 4 {
		t.Errorf("Count(gnoll) = %d, want 4", gnolls)
	}
}

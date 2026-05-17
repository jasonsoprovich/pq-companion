package players

import (
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsert_NewNamed(t *testing.T) {
	s := openTest(t)
	now := time.Unix(1_700_000_000, 0)
	err := s.Upsert(SightingInput{
		Name: "Foo", Level: 60, Class: "Necromancer", Race: "Iksar", Guild: "Drowsy Disciples",
		Zone: "Plane of Knowledge", ObservedAt: now,
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.Get("Foo")
	if err != nil || got == nil {
		t.Fatalf("Get: %v got=%v", err, got)
	}
	if got.LastSeenLevel != 60 || got.Class != "Necromancer" || got.Race != "Iksar" || got.Guild != "Drowsy Disciples" {
		t.Errorf("inserted row wrong: %+v", got)
	}
	if got.LastAnonymous {
		t.Error("expected last_anonymous = false")
	}
	if got.SightingsCount != 1 {
		t.Errorf("count = %d, want 1", got.SightingsCount)
	}
}

func TestUpsert_AnonymousAfterNamed_PreservesClass(t *testing.T) {
	s := openTest(t)
	// First sighting: full info.
	if err := s.Upsert(SightingInput{
		Name: "Bar", Level: 55, Class: "Wizard", Race: "Erudite", Guild: "Mages United",
		Zone: "Sol B", ObservedAt: time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	// Second sighting: same player now anonymous in a different zone.
	if err := s.Upsert(SightingInput{
		Name: "Bar", Anonymous: true, Zone: "Plane of Fear", ObservedAt: time.Unix(1_700_001_000, 0),
	}); err != nil {
		t.Fatalf("anonymous upsert: %v", err)
	}
	got, _ := s.Get("Bar")
	if got == nil {
		t.Fatal("Get returned nil after anon sighting")
	}
	if got.Class != "Wizard" {
		t.Errorf("Class wiped by anonymous sighting: got %q, want Wizard", got.Class)
	}
	if got.Race != "Erudite" {
		t.Errorf("Race wiped: got %q, want Erudite", got.Race)
	}
	if got.Guild != "Mages United" {
		t.Errorf("Guild wiped: got %q, want Mages United", got.Guild)
	}
	if got.LastSeenLevel != 55 {
		t.Errorf("Level wiped: got %d, want 55", got.LastSeenLevel)
	}
	if !got.LastAnonymous {
		t.Error("LastAnonymous should be true after anon sighting")
	}
	if got.LastSeenZone != "Plane of Fear" {
		t.Errorf("Zone not updated: got %q, want Plane of Fear", got.LastSeenZone)
	}
	if got.SightingsCount != 2 {
		t.Errorf("SightingsCount = %d, want 2", got.SightingsCount)
	}
}

func TestUpsert_AnonymousFirstThenNamed_FillsClass(t *testing.T) {
	s := openTest(t)
	if err := s.Upsert(SightingInput{
		Name: "Baz", Anonymous: true, Zone: "Lavastorm", ObservedAt: time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatalf("first anon upsert: %v", err)
	}
	// Same player /who's themselves non-anonymous later.
	if err := s.Upsert(SightingInput{
		Name: "Baz", Level: 42, Class: "Druid", Race: "Wood Elf", Zone: "Lavastorm",
		ObservedAt: time.Unix(1_700_001_000, 0),
	}); err != nil {
		t.Fatalf("named upsert: %v", err)
	}
	got, _ := s.Get("Baz")
	if got == nil || got.Class != "Druid" || got.LastSeenLevel != 42 {
		t.Errorf("expected named info filled in: %+v", got)
	}
	if got.LastAnonymous {
		t.Error("LastAnonymous should be cleared after named sighting")
	}
}

func TestUpsert_LevelChangeRecordsHistoryRow(t *testing.T) {
	s := openTest(t)
	name := "Qux"
	s.Upsert(SightingInput{Name: name, Level: 50, Class: "Cleric", Zone: "PoK", ObservedAt: time.Unix(1, 0)})
	s.Upsert(SightingInput{Name: name, Level: 50, Class: "Cleric", Zone: "PoK", ObservedAt: time.Unix(2, 0)})
	s.Upsert(SightingInput{Name: name, Level: 51, Class: "Cleric", Zone: "PoK", ObservedAt: time.Unix(3, 0)})
	s.Upsert(SightingInput{Name: name, Level: 52, Class: "Cleric", Zone: "PoK", ObservedAt: time.Unix(4, 0)})

	hist, err := s.LevelHistory(name)
	if err != nil {
		t.Fatalf("LevelHistory: %v", err)
	}
	// We expect 3 rows: 50 (first sighting), 51, 52 — second 50 collapses.
	if len(hist) != 3 {
		t.Fatalf("history rows = %d, want 3 — %+v", len(hist), hist)
	}
	wantLevels := []int{50, 51, 52}
	for i, h := range hist {
		if h.Level != wantLevels[i] {
			t.Errorf("hist[%d].Level = %d, want %d", i, h.Level, wantLevels[i])
		}
	}
}

func TestUpsert_AnonymousDoesNotRecordHistory(t *testing.T) {
	s := openTest(t)
	s.Upsert(SightingInput{Name: "Z", Level: 60, Class: "Bard", Zone: "PoK", ObservedAt: time.Unix(1, 0)})
	// Anonymous re-sighting: no level given (0), should not insert history row.
	s.Upsert(SightingInput{Name: "Z", Anonymous: true, Zone: "PoK", ObservedAt: time.Unix(2, 0)})
	hist, _ := s.LevelHistory("Z")
	if len(hist) != 1 {
		t.Errorf("history rows = %d, want 1 (only the first named sighting)", len(hist))
	}
}

func TestSearch_FiltersAndOrdersDesc(t *testing.T) {
	s := openTest(t)
	s.Upsert(SightingInput{Name: "Alpha", Level: 60, Class: "Wizard", Zone: "A", ObservedAt: time.Unix(100, 0)})
	s.Upsert(SightingInput{Name: "Beta", Level: 55, Class: "Wizard", Zone: "B", ObservedAt: time.Unix(200, 0)})
	s.Upsert(SightingInput{Name: "Gamma", Level: 50, Class: "Cleric", Zone: "A", ObservedAt: time.Unix(150, 0)})

	all, err := s.Search(SearchFilters{})
	if err != nil {
		t.Fatalf("Search all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("Search all = %d rows, want 3", len(all))
	}
	if all[0].Name != "Beta" {
		t.Errorf("first row = %q, want newest (Beta)", all[0].Name)
	}

	wiz, _ := s.Search(SearchFilters{Class: "Wizard"})
	if len(wiz) != 2 {
		t.Errorf("Wizard count = %d, want 2", len(wiz))
	}

	zoneA, _ := s.Search(SearchFilters{Zone: "A"})
	if len(zoneA) != 2 {
		t.Errorf("Zone A count = %d, want 2", len(zoneA))
	}

	contains, _ := s.Search(SearchFilters{NameContains: "et"})
	if len(contains) != 1 || contains[0].Name != "Beta" {
		t.Errorf("NameContains 'et' = %+v, want [Beta]", contains)
	}
}

func TestDeleteAndClear(t *testing.T) {
	s := openTest(t)
	s.Upsert(SightingInput{Name: "X", Level: 10, Class: "Druid", Zone: "Z", ObservedAt: time.Unix(1, 0)})
	s.Upsert(SightingInput{Name: "Y", Level: 11, Class: "Druid", Zone: "Z", ObservedAt: time.Unix(2, 0)})
	if err := s.Delete("X"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	all, _ := s.Search(SearchFilters{})
	if len(all) != 1 || all[0].Name != "Y" {
		t.Errorf("after delete = %+v, want [Y]", all)
	}
	n, err := s.Clear()
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if n != 1 {
		t.Errorf("Clear deleted = %d, want 1", n)
	}
	all2, _ := s.Search(SearchFilters{})
	if len(all2) != 0 {
		t.Errorf("after clear = %d rows, want 0", len(all2))
	}
}

func TestUpdateGuild_PreservesOtherFields(t *testing.T) {
	s := openTest(t)
	// First, a full /who sighting.
	if err := s.Upsert(SightingInput{
		Name: "Osui", Level: 60, Class: "Enchanter", Race: "Halfling",
		Zone: "Plane of Knowledge", ObservedAt: time.Unix(1_700_000_000, 0),
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Then /guildstat sets the guild without touching anything else.
	if err := s.UpdateGuild("Osui", "Seekers of Souls", "Plane of Knowledge", time.Unix(1_700_000_100, 0)); err != nil {
		t.Fatalf("UpdateGuild: %v", err)
	}
	got, _ := s.Get("Osui")
	if got == nil {
		t.Fatal("expected row")
	}
	if got.Guild != "Seekers of Souls" {
		t.Errorf("guild = %q, want Seekers of Souls", got.Guild)
	}
	if got.LastSeenLevel != 60 || got.Class != "Enchanter" || got.Race != "Halfling" {
		t.Errorf("UpdateGuild clobbered other fields: %+v", got)
	}
}

func TestUpdateGuild_NewPlayer(t *testing.T) {
	s := openTest(t)
	if err := s.UpdateGuild("Stranger", "Outsiders", "Plane of Knowledge", time.Unix(1_700_000_000, 0)); err != nil {
		t.Fatalf("UpdateGuild: %v", err)
	}
	got, _ := s.Get("Stranger")
	if got == nil {
		t.Fatal("expected row")
	}
	if got.Guild != "Outsiders" || got.LastSeenLevel != 0 || got.Class != "" {
		t.Errorf("new row from UpdateGuild = %+v", got)
	}
}

func TestSearch_ClassFilterExpandsToTitleAliases(t *testing.T) {
	s := openTest(t)
	ts := time.Unix(1_700_000_000, 0)

	// Three enchanters at different level brackets — each shows up under a
	// different title in /who. The filter should pull all three when the
	// user picks "Enchanter".
	for _, r := range []struct{ name, class string }{
		{"Lowlvl", "Enchanter"},
		{"Mid", "Illusionist"},
		{"High", "Phantasmist"},
		// Unrelated class — should not appear.
		{"Healer", "Cleric"},
	} {
		if err := s.Upsert(SightingInput{Name: r.name, Class: r.class, ObservedAt: ts}); err != nil {
			t.Fatalf("upsert %s: %v", r.name, err)
		}
	}

	got, err := s.Search(SearchFilters{Class: "Enchanter"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	for _, want := range []string{"Lowlvl", "Mid", "High"} {
		if !names[want] {
			t.Errorf("expected %s in results, got %v", want, names)
		}
	}
	if names["Healer"] {
		t.Errorf("Cleric leaked into Enchanter filter")
	}
}

func TestSearch_ClassFilterDirectTitleStillExactMatches(t *testing.T) {
	s := openTest(t)
	ts := time.Unix(1_700_000_000, 0)
	s.Upsert(SightingInput{Name: "Lowlvl", Class: "Enchanter", ObservedAt: ts})
	s.Upsert(SightingInput{Name: "High", Class: "Phantasmist", ObservedAt: ts})

	// Specific title — should only match that one title, not expand.
	got, _ := s.Search(SearchFilters{Class: "Phantasmist"})
	if len(got) != 1 || got[0].Name != "High" {
		t.Errorf("specific-title filter should match exactly; got %+v", got)
	}
}

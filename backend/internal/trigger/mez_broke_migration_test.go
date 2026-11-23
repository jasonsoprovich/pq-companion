package trigger

import (
	"regexp"
	"testing"
	"time"
)

func insertMezBroke(t *testing.T, s *Store, pattern string) {
	t.Helper()
	tr := &Trigger{
		ID: "mez-old", Name: "Mez Broke", Enabled: true, Pattern: pattern,
		PackName: "Enchanter", CreatedAt: time.Now().UTC(),
		Actions: []Action{{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5}},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateMezBrokeDropNonMezzes_UntouchedOldPattern(t *testing.T) {
	s := openTestStore(t)
	insertMezBroke(t, s, oldEnchanterMezPattern)
	if err := s.MigrateMezBrokeDropNonMezzes(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.FindByPackAndName("Enchanter", "Mez Broke")
	if got.Pattern != enchanterMezBrokeDef() {
		t.Fatalf("pattern = %q, want shipped def", got.Pattern)
	}
	re := regexp.MustCompile(got.Pattern)
	if re.MatchString("Your Instill spell has worn off.") {
		t.Error("Instill must not match Mez Broke")
	}
	if !re.MatchString("Your Mesmerize spell has worn off.") {
		t.Error("Mesmerize must still match")
	}
}

func TestMigrateMezBrokeDropNonMezzes_EditedPatternKeepsEdits(t *testing.T) {
	s := openTestStore(t)
	edited := `Your (?:Mesmerize|Instill|My Custom Mez|Wake of Tranquility) spell has worn off\.`
	insertMezBroke(t, s, edited)
	if err := s.MigrateMezBrokeDropNonMezzes(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.FindByPackAndName("Enchanter", "Mez Broke")
	want := `Your (?:Mesmerize|My Custom Mez) spell has worn off\.`
	if got.Pattern != want {
		t.Fatalf("pattern = %q, want %q", got.Pattern, want)
	}
	// Runs once: a later re-add of Instill by the user is left alone.
	got.Pattern = edited
	if err := s.Update(got); err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateMezBrokeDropNonMezzes(); err != nil {
		t.Fatal(err)
	}
	again, _ := s.FindByPackAndName("Enchanter", "Mez Broke")
	if again.Pattern != edited {
		t.Error("migration must only run once")
	}
}

func TestMigrateMezBrokeDropNonMezzes_NoTrigger(t *testing.T) {
	s := openTestStore(t)
	if err := s.MigrateMezBrokeDropNonMezzes(); err != nil {
		t.Fatal(err)
	}
}

func TestDropAlternatives(t *testing.T) {
	drop := map[string]bool{"Instill": true}
	cases := map[string]string{
		`Your (?:Instill) spell`:         `Your (?:Instill) spell`, // never empty the group
		`Your Mesmerize spell`:           `Your Mesmerize spell`,
		`Your (?:A|Instill|B) spell`:     `Your (?:A|B) spell`,
		`Your (?:A|Instills|B) spell`:    `Your (?:A|Instills|B) spell`,
	}
	for in, want := range cases {
		if got := dropAlternatives(in, drop); got != want {
			t.Errorf("dropAlternatives(%q) = %q, want %q", in, got, want)
		}
	}
}

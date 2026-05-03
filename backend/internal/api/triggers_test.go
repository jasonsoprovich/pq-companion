package api

import (
	"sort"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

func newPack(class *int, count int) trigger.TriggerPack {
	triggers := make([]trigger.Trigger, count)
	for i := range triggers {
		triggers[i] = trigger.Trigger{Name: "t", Pattern: "x", Enabled: true}
	}
	return trigger.TriggerPack{PackName: "Test", Class: class, Triggers: triggers}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDefaultPackCharacters_ClassAgnosticPackUsesAllChars(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
		{Name: "Beasty", Class: trigger.ClassBeastlord, Level: 60},
	}
	pack := newPack(nil, 2)
	defaultPackCharacters(&pack, chars, "Wizzy")

	want := []string{"Beasty", "Wizzy"}
	for i, tr := range pack.Triggers {
		got := sortedCopy(tr.Characters)
		if !equalStrings(got, want) {
			t.Errorf("trigger[%d] characters = %v, want %v", i, got, want)
		}
	}
}

func TestDefaultPackCharacters_ClassPackPrefersActiveMatchingChar(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
		{Name: "OtherWiz", Class: trigger.ClassWizard, Level: 55},
		{Name: "Beasty", Class: trigger.ClassBeastlord, Level: 60},
	}
	pack := newPack(trigger.ClassPtr(trigger.ClassWizard), 1)
	defaultPackCharacters(&pack, chars, "Wizzy")

	got := pack.Triggers[0].Characters
	if !equalStrings(got, []string{"Wizzy"}) {
		t.Errorf("expected only active char Wizzy, got %v", got)
	}
}

func TestDefaultPackCharacters_ClassPackFallsBackToOtherMatchingChars(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
		{Name: "OtherWiz", Class: trigger.ClassWizard, Level: 55},
		{Name: "Beasty", Class: trigger.ClassBeastlord, Level: 60},
	}
	pack := newPack(trigger.ClassPtr(trigger.ClassWizard), 1)
	defaultPackCharacters(&pack, chars, "Beasty") // active is not a wizard

	got := sortedCopy(pack.Triggers[0].Characters)
	want := []string{"OtherWiz", "Wizzy"}
	if !equalStrings(got, want) {
		t.Errorf("expected wizard chars %v, got %v", want, got)
	}
}

func TestDefaultPackCharacters_NoMatchingClassDisablesTriggers(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
	}
	pack := newPack(trigger.ClassPtr(trigger.ClassBeastlord), 2)
	defaultPackCharacters(&pack, chars, "Wizzy")

	for i, tr := range pack.Triggers {
		if len(tr.Characters) != 0 {
			t.Errorf("trigger[%d] expected empty Characters when no class match, got %v", i, tr.Characters)
		}
		if tr.Enabled {
			t.Errorf("trigger[%d] expected Enabled=false when no class match, got true", i)
		}
	}
}

func TestDefaultPackCharacters_EmptyStoreDisablesClassPack(t *testing.T) {
	pack := newPack(trigger.ClassPtr(trigger.ClassWizard), 1)
	defaultPackCharacters(&pack, nil, "")

	if pack.Triggers[0].Enabled {
		t.Errorf("expected Enabled=false when no characters exist at all")
	}
}

func TestDefaultPackCharacters_EmptyStoreLeavesClassAgnosticEnabled(t *testing.T) {
	pack := newPack(nil, 1)
	defaultPackCharacters(&pack, nil, "")

	if !pack.Triggers[0].Enabled {
		t.Errorf("class-agnostic triggers should stay enabled even with empty store")
	}
}

func TestDefaultPackCharacters_DoesNotOverrideExplicitCharacters(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
		{Name: "Beasty", Class: trigger.ClassBeastlord, Level: 60},
	}
	pack := newPack(trigger.ClassPtr(trigger.ClassWizard), 1)
	pack.Triggers[0].Characters = []string{"Beasty"}
	defaultPackCharacters(&pack, chars, "Wizzy")

	if !equalStrings(pack.Triggers[0].Characters, []string{"Beasty"}) {
		t.Errorf("explicit Characters list was overridden: got %v", pack.Triggers[0].Characters)
	}
}

func TestDefaultPackCharacters_ActiveCaseInsensitive(t *testing.T) {
	chars := []character.Character{
		{Name: "Wizzy", Class: trigger.ClassWizard, Level: 60},
		{Name: "OtherWiz", Class: trigger.ClassWizard, Level: 55},
	}
	pack := newPack(trigger.ClassPtr(trigger.ClassWizard), 1)
	defaultPackCharacters(&pack, chars, "wIzZy")

	if !equalStrings(pack.Triggers[0].Characters, []string{"Wizzy"}) {
		t.Errorf("expected case-insensitive match to pin to Wizzy, got %v", pack.Triggers[0].Characters)
	}
}

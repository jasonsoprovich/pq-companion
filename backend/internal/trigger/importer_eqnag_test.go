package trigger

import (
	"os"
	"path/filepath"
	"testing"
)

func loadEQNagPreview(t *testing.T) ImportPreview {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "import", "eqnag-trigger-database.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	prev, err := DetectAndParse("eqnag-trigger-database.json", data)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if prev.Format != FormatEQNag {
		t.Fatalf("format = %q, want eqnag", prev.Format)
	}
	return prev
}

func findTrigger(prev ImportPreview, name string) *ImportedTrigger {
	for i := range prev.Triggers {
		if prev.Triggers[i].Trigger.Name == name {
			return &prev.Triggers[i]
		}
	}
	return nil
}

func TestEQNagImport(t *testing.T) {
	prev := loadEQNagPreview(t)
	if len(prev.Triggers) == 0 {
		t.Fatal("no triggers parsed")
	}
	// Every imported trigger must have a name and a pattern, and a bad-regex
	// trigger must be disabled + flagged.
	for _, it := range prev.Triggers {
		if it.Trigger.Name == "" || it.Trigger.Pattern == "" {
			t.Errorf("trigger %q has empty name/pattern", it.Trigger.Name)
		}
		if !it.RegexOK && it.Trigger.Enabled {
			t.Errorf("trigger %q has bad regex but is enabled", it.Trigger.Name)
		}
		// No play_sound actions — EQNag audio is an opaque id, routed to TTS.
		for _, a := range it.Trigger.Actions {
			if a.Type == ActionPlaySound {
				t.Errorf("trigger %q kept a play_sound action", it.Trigger.Name)
			}
		}
	}
}

func TestEQNagRampage(t *testing.T) {
	prev := loadEQNagPreview(t)
	it := findTrigger(prev, "Rampage")
	if it == nil {
		t.Fatal("Rampage trigger not found")
	}
	// (?<char>…) → (?P<char>…) so it compiles, and the display text keeps {char}.
	if !it.RegexOK {
		t.Errorf("Rampage pattern should compile: %q", it.Trigger.Pattern)
	}
	var sawOverlay, sawSpeak bool
	for _, a := range it.Trigger.Actions {
		switch a.Type {
		case ActionOverlayText:
			sawOverlay = true
			if a.Text != "Rampage - {char}" {
				t.Errorf("overlay text = %q, want %q", a.Text, "Rampage - {char}")
			}
		case ActionTextToSpeech:
			sawSpeak = true
			if a.Text != "Rampage on {char}" {
				t.Errorf("speak text = %q, want %q", a.Text, "Rampage on {char}")
			}
		}
	}
	if !sawOverlay || !sawSpeak {
		t.Errorf("Rampage missing actions: overlay=%v speak=%v", sawOverlay, sawSpeak)
	}
	if it.OriginalGroup != "Common/Combat" {
		t.Errorf("group = %q, want Common/Combat", it.OriginalGroup)
	}
}

func TestEQNagSpellCapture(t *testing.T) {
	prev := loadEQNagPreview(t)
	it := findTrigger(prev, "Capture spell casting")
	if it == nil {
		t.Fatal("Capture spell casting trigger not found")
	}
	// 8 alternative phrases → 1 primary + 7 extra patterns.
	if got := len(it.Trigger.ExtraPatterns); got != 7 {
		t.Errorf("extra patterns = %d, want 7", got)
	}
	// Primary "^You begin casting (.*)\." — the unnamed group is named with the
	// stored variable so {SpellBeingCast} resolves.
	if want := `^You begin casting (?P<SpellBeingCast>.*)\.`; it.Trigger.Pattern != want {
		t.Errorf("primary pattern = %q, want %q", it.Trigger.Pattern, want)
	}
	if !it.RegexOK {
		t.Errorf("primary should compile: %q", it.Trigger.Pattern)
	}
	// An extra phrase that referenced ${SpellBeingCast} becomes an inline named
	// capture so it self-captures and compiles.
	foundInterrupt := false
	for _, ep := range it.Trigger.ExtraPatterns {
		if ep.Pattern == `^Your (?P<SpellBeingCast>.+?) spell is interrupted\.` {
			foundInterrupt = true
		}
	}
	if !foundInterrupt {
		t.Errorf("interrupt extra pattern not rewritten as expected; got %+v", it.Trigger.ExtraPatterns)
	}
}

func TestEQNagTimer(t *testing.T) {
	prev := loadEQNagPreview(t)
	it := findTrigger(prev, "Mod Rod Ready")
	if it == nil {
		t.Skip("Mod Rod Ready not in fixture")
	}
	if it.Trigger.TimerType != TimerTypeDetrimental {
		t.Errorf("timer type = %q, want detrimental", it.Trigger.TimerType)
	}
	if it.Trigger.TimerDurationSecs != 300 {
		t.Errorf("timer duration = %d, want 300", it.Trigger.TimerDurationSecs)
	}
	// "ending soon" speak → a TTS fading alert.
	if len(it.Trigger.TimerAlerts) == 0 {
		t.Errorf("expected a fading TimerAlert from endingSoonSpeakPhrase")
	}
}

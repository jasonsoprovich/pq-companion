package trigger

import (
	"os"
	"path/filepath"
	"testing"
)

func loadEQLPPreview(t *testing.T) ImportPreview {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "import", "eqlogparser-triggers.tgf"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	prev, err := DetectAndParse("eqlogparser-triggers.tgf", data)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if prev.Format != FormatEQLogParser {
		t.Fatalf("format = %q, want eqlogparser", prev.Format)
	}
	return prev
}

func TestEQLPImport(t *testing.T) {
	prev := loadEQLPPreview(t)
	if len(prev.Triggers) < 200 {
		t.Fatalf("trigger count = %d, want >= 200", len(prev.Triggers))
	}
	for _, it := range prev.Triggers {
		if it.Trigger.Name == "" || it.Trigger.Pattern == "" {
			t.Errorf("trigger %q has empty name/pattern", it.Trigger.Name)
		}
		if !it.RegexOK && it.Trigger.Enabled {
			t.Errorf("trigger %q has bad regex but is enabled", it.Trigger.Name)
		}
		for _, a := range it.Trigger.Actions {
			if a.Type == ActionPlaySound {
				t.Errorf("trigger %q kept a play_sound action", it.Trigger.Name)
			}
		}
	}
}

func TestEQLPEnrageTimer(t *testing.T) {
	prev := loadEQLPPreview(t)
	it := findTrigger(prev, "Enrage")
	if it == nil {
		t.Fatal("Enrage trigger not found")
	}
	tr := it.Trigger
	if tr.TimerType != TimerTypeDetrimental {
		t.Errorf("timer type = %q, want detrimental", tr.TimerType)
	}
	if tr.TimerDurationSecs != 16 {
		t.Errorf("duration = %d, want 16", tr.TimerDurationSecs)
	}
	// AltTimerName "Enraged:  {s1}" → per-target timer keyed on group S1.
	if tr.TimerKeyCapture != "S1" || tr.TimerTargetCapture != "S1" {
		t.Errorf("timer captures = key=%q target=%q, want S1/S1", tr.TimerKeyCapture, tr.TimerTargetCapture)
	}
	// EndEarlyPattern → worn-off.
	if tr.WornOffPattern == "" {
		t.Errorf("expected worn-off pattern from EndEarlyPattern")
	}
	if !it.RegexOK {
		t.Errorf("Enrage pattern should compile: %q", tr.Pattern)
	}
}

func TestEQLPLiteralEscaped(t *testing.T) {
	prev := loadEQLPPreview(t)
	// A non-regex pattern with parentheses must be escaped so it still compiles.
	it := findTrigger(prev, "Words of Acquisition (Beza)")
	if it == nil {
		t.Skip("literal trigger not in fixture")
	}
	if !it.RegexOK {
		t.Errorf("escaped literal should compile: %q", it.Trigger.Pattern)
	}
	if it.Trigger.Pattern == "Words of Acquisition (Beza)" {
		t.Errorf("pattern was not escaped: %q", it.Trigger.Pattern)
	}
}

func TestEQLPWarningAlert(t *testing.T) {
	prev := loadEQLPPreview(t)
	it := findTrigger(prev, "Silence of the Shadows")
	if it == nil {
		t.Skip("warning trigger not in fixture")
	}
	if len(it.Trigger.TimerAlerts) == 0 {
		t.Errorf("expected a fading TimerAlert from WarningTextToSpeak")
	}
}

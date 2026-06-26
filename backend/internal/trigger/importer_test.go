package trigger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDetectAndParse runs the detector + parsers against the real sample export
// files shipped in testdata/import. It asserts format detection and that the
// GINA/PQC paths produce usable triggers; EQNag/EQLogParser are expected to
// report "coming later" until their parsers land (phases 2 & 3).
func TestDetectAndParse(t *testing.T) {
	tests := []struct {
		file       string
		wantFormat ImportFormat
		minCount   int
	}{
		{"GINA_Generic.gtp", FormatGINA, 1},
		{"Grokii_GINA_Enchanter.gtp", FormatGINA, 1},
		{"Grokii_GINA_CharmBreak.gtp", FormatGINA, 1},
		{"GINA_FromJemi.gtp", FormatGINA, 10},
		{"CharmBreak_ShareData.xml", FormatGINA, 1},
		{"pq-triggers.json", FormatPQC, 1},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "import", tt.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			prev, err := DetectAndParse(tt.file, data)
			if err != nil {
				t.Fatalf("DetectAndParse: %v", err)
			}
			if prev.Format != tt.wantFormat {
				t.Errorf("format = %q, want %q", prev.Format, tt.wantFormat)
			}
			if len(prev.Triggers) < tt.minCount {
				t.Errorf("trigger count = %d, want >= %d", len(prev.Triggers), tt.minCount)
			}
			for i, it := range prev.Triggers {
				if strings.TrimSpace(it.Trigger.Name) == "" {
					t.Errorf("trigger %d has empty name", i)
				}
				if it.Trigger.Pattern == "" {
					t.Errorf("trigger %d (%s) has empty pattern", i, it.Trigger.Name)
				}
				// A regex flagged not-OK must be imported disabled with a warning.
				if !it.RegexOK {
					if it.Trigger.Enabled {
						t.Errorf("trigger %q has bad regex but is enabled", it.Trigger.Name)
					}
					if len(it.Warnings) == 0 {
						t.Errorf("trigger %q has bad regex but no warning", it.Trigger.Name)
					}
				}
			}
		})
	}
}

// TestGINAMediaNoBogusSound verifies the fix for the old bug where GINA's
// boolean <PlayMediaFile> was mapped to a SoundPath of "True"/"False". No
// imported GINA trigger should carry a play_sound action (GINA exports include
// no recoverable filename); sound intent becomes a TTS fallback + warning.
func TestGINAMediaNoBogusSound(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "import", "GINA_FromJemi.gtp"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	prev, err := DetectAndParse("GINA_FromJemi.gtp", data)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	for _, it := range prev.Triggers {
		for _, a := range it.Trigger.Actions {
			if a.Type == ActionPlaySound {
				t.Errorf("trigger %q has a play_sound action %q — should be a TTS fallback", it.Trigger.Name, a.SoundPath)
			}
			if a.SoundPath == "True" || a.SoundPath == "False" {
				t.Errorf("trigger %q has bogus SoundPath %q", it.Trigger.Name, a.SoundPath)
			}
		}
	}
}

// TestNormalizeActionText covers the ${X} → {X} rewrite for foreign braced
// capture references.
func TestNormalizeActionText(t *testing.T) {
	cases := map[string]string{
		"":                      "",
		"plain text":            "plain text",
		"Charm broke on ${2}!":  "Charm broke on {2}!",
		"Casting ${SpellName}.": "Casting {SpellName}.",
		"keep $1 and {3}":       "keep $1 and {3}", // bare $N and {N} untouched here
	}
	for in, want := range cases {
		if got := normalizeActionText(in); got != want {
			t.Errorf("normalizeActionText(%q) = %q, want %q", in, got, want)
		}
	}
}

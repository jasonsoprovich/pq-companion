package popflag

import (
	"testing"
	"time"
)

func TestMatchEvent(t *testing.T) {
	cases := []struct {
		kind, name string
		want       string // expected flag ID, or "" for no match
	}{
		{"kill", "Terris Thule", "ponb_terris"},
		{"kill", "terris thule", "ponb_terris"},           // case-insensitive
		{"kill", "the Keeper of Sorrows", "potor_keeper"}, // article-insensitive (rule has "the")
		{"kill", "Keeper of Sorrows", "potor_keeper"},     // and without the article
		// The Zeks are detected from their projection's grant pair instead:
		// Vallon's split copies print "has been slain" long before the real
		// kill, and Tallon/the Warlord die without one (2026-11-04 raid log).
		{"kill", "Vallon Zek", ""},
		{"kill", "Tallon Zek", ""},
		{"kill", "a gnoll pup", ""},      // random mob → no match
		{"zone", "Plane of Justice", ""}, // not a key-ring zone
		// Key-ring zone-ins: zoning in IS the key-ring grant, and #popflags
		// can't report it (no qglobal), so the zone-in is the only signal.
		{"zone", "The Crypt of Decay", "pod_cod_keyring"},
		{"zone", "Halls of Honor", "pov_hoh_keyring"},
		{"zone", "Plane of Fire", "solro_pofire_keyring"},
		{"zone", "Plane of Earth", ""}, // poeartha/poearthb share a name
	}
	for _, tc := range cases {
		got := MatchEvent(tc.kind, tc.name)
		if tc.want == "" {
			if len(got) != 0 {
				t.Errorf("MatchEvent(%q,%q) = %v, want none", tc.kind, tc.name, got)
			}
			continue
		}
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("MatchEvent(%q,%q) = %v, want [%s]", tc.kind, tc.name, got, tc.want)
		}
	}
}

// TestEventRulesValid guards that every authored EventRule sits on a real node
// with a supported kind and non-empty match.
func TestEventRulesValid(t *testing.T) {
	for _, f := range Flags() {
		for _, r := range f.Events {
			if r.Kind != "kill" && r.Kind != "zone" && r.Kind != "say" && r.Kind != "loot" {
				t.Errorf("flag %q has unsupported event kind %q", f.ID, r.Kind)
			}
			if r.Match == "" {
				t.Errorf("flag %q has an event rule with empty match", f.ID)
			}
		}
	}
}

func TestSetAutoPrecedence(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	// Fresh auto detection inserts.
	ins, err := s.SetAuto(char, "ponb_terris")
	if err != nil {
		t.Fatalf("SetAuto: %v", err)
	}
	if !ins {
		t.Fatalf("expected first SetAuto to insert")
	}
	if r := doneByID(t, s, char)["ponb_terris"]; !r.Done || r.Source != SourceAuto {
		t.Errorf("expected auto-done row, got %+v", r)
	}

	// Re-detecting the same kill is a no-op (already present).
	if ins, _ := s.SetAuto(char, "ponb_terris"); ins {
		t.Errorf("re-detection should not insert again")
	}

	// Auto must never overwrite a manual row.
	if err := s.SetManual(char, "pod_grummus", false); err != nil {
		t.Fatalf("SetManual: %v", err)
	}
	if ins, _ := s.SetAuto(char, "pod_grummus"); ins {
		t.Errorf("auto should not overwrite a manual row")
	}
	if r := doneByID(t, s, char)["pod_grummus"]; r.Done || r.Source != SourceManual {
		t.Errorf("manual row should survive auto: %+v", r)
	}

	// Auto must never overwrite a seer row.
	if _, err := s.ApplySeer(char, map[string]string{"saryrn": "1"}, "r", time.Unix(1, 0)); err != nil {
		t.Fatalf("ApplySeer: %v", err)
	}
	if ins, _ := s.SetAuto(char, "potor_saryrn"); ins {
		t.Errorf("auto should not overwrite a seer row")
	}
	if r := doneByID(t, s, char)["potor_saryrn"]; r.Source != SourceSeer {
		t.Errorf("seer row should survive auto: %+v", r)
	}
}

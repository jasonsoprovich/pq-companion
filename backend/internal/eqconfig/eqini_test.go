package eqconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetINIKey(t *testing.T) {
	t.Run("in-place replace preserves CRLF + other lines", func(t *testing.T) {
		in := "[Defaults]\r\nLog=FALSE\r\nWidth=1920\r\n"
		out := setINIKey(in, "Defaults", "Log", "TRUE")
		if !strings.Contains(out, "Log=TRUE") || strings.Contains(out, "Log=FALSE") {
			t.Errorf("Log not flipped: %q", out)
		}
		if !strings.Contains(out, "Width=1920") {
			t.Errorf("other key lost: %q", out)
		}
		if !strings.Contains(out, "\r\n") {
			t.Errorf("CRLF not preserved: %q", out)
		}
	})

	t.Run("case-insensitive key match, no duplicate", func(t *testing.T) {
		in := "[Defaults]\nlog=false\n"
		out := setINIKey(in, "Defaults", "Log", "TRUE")
		if strings.Count(strings.ToLower(out), "log=") != 1 {
			t.Errorf("expected single log key, got %q", out)
		}
		if !strings.Contains(out, "log=TRUE") {
			t.Errorf("value not set on existing key: %q", out)
		}
	})

	t.Run("insert into existing section when key missing", func(t *testing.T) {
		in := "[Defaults]\nWidth=1920\n[VideoMode]\nFoo=1\n"
		out := setINIKey(in, "Defaults", "Log", "TRUE")
		lines := strings.Split(out, "\n")
		// Log must appear inside [Defaults], before [VideoMode].
		logIdx, vmIdx := -1, -1
		for i, l := range lines {
			if strings.TrimSpace(l) == "Log=TRUE" {
				logIdx = i
			}
			if strings.TrimSpace(l) == "[VideoMode]" {
				vmIdx = i
			}
		}
		if logIdx < 0 || vmIdx < 0 || logIdx > vmIdx {
			t.Errorf("Log not inserted within [Defaults]: %q", out)
		}
	})

	t.Run("append section when missing", func(t *testing.T) {
		in := "[Zeal]\nOther=1\n"
		out := setINIKey(in, "Defaults", "Log", "TRUE")
		if !strings.Contains(out, "[Defaults]") || !strings.Contains(out, "Log=TRUE") {
			t.Errorf("section not appended: %q", out)
		}
	})
}

func TestReadAndSetLogRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, eqClientINI)
	if err := os.WriteFile(path, []byte("[Defaults]\r\nLog=FALSE\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := ReadLog(dir); !st.Found || st.Enabled {
		t.Errorf("initial ReadLog=%+v, want found+disabled", st)
	}
	if err := SetLog(dir, true); err != nil {
		t.Fatalf("SetLog: %v", err)
	}
	if st := ReadLog(dir); !st.Found || !st.Enabled {
		t.Errorf("after SetLog(true) ReadLog=%+v, want found+enabled", st)
	}
}

func TestSetExportOnCampCreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := SetExportOnCamp(dir, true); err != nil {
		t.Fatalf("SetExportOnCamp: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, zealINI))
	if err != nil {
		t.Fatalf("zeal.ini not created: %v", err)
	}
	if !strings.Contains(string(b), "ExportOnCamp=true") {
		t.Errorf("ExportOnCamp not written: %q", string(b))
	}
}

func TestSetBandolierBagSlot_ScopedToCharacterSection(t *testing.T) {
	// Two characters each carry their own BandolierBagSlot. Updating one must
	// not touch the other — the whole-file setINIKey would clobber the first
	// match, so this guards the section-scoped writer.
	in := "[Zeal_Osui]\r\nBandolierBagSlot=3\r\nAutoAASwitchLow=60\r\n" +
		"[Zeal_Nariana]\r\nBandolierBagSlot=5\r\n"
	out := setINIKeyInSection(in, "Zeal_Nariana", "BandolierBagSlot", "7")

	if osui, ok := getINIValueInSection(out, "Zeal_Osui", "BandolierBagSlot"); !ok || osui != "3" {
		t.Errorf("Osui bag changed to %q (ok=%v), want 3", osui, ok)
	}
	if nar, ok := getINIValueInSection(out, "Zeal_Nariana", "BandolierBagSlot"); !ok || nar != "7" {
		t.Errorf("Nariana bag = %q (ok=%v), want 7", nar, ok)
	}
	if !strings.Contains(out, "AutoAASwitchLow=60") {
		t.Errorf("unrelated key lost: %q", out)
	}
	if !strings.Contains(out, "\r\n") {
		t.Errorf("CRLF not preserved: %q", out)
	}
	// Exactly one BandolierBagSlot per character (no duplicates introduced).
	if n := strings.Count(out, "BandolierBagSlot="); n != 2 {
		t.Errorf("expected 2 BandolierBagSlot lines, got %d: %q", n, out)
	}
}

func TestSetBandolierBagSlot_InsertAndCreate(t *testing.T) {
	// Insert into an existing section that lacks the key.
	in := "[Zeal_Osui]\r\nAutoAASwitchLow=60\r\n"
	out := setINIKeyInSection(in, "Zeal_Osui", "BandolierBagSlot", "4")
	if v, ok := getINIValueInSection(out, "Zeal_Osui", "BandolierBagSlot"); !ok || v != "4" {
		t.Errorf("insert failed: %q (ok=%v) in %q", v, ok, out)
	}

	// Append a brand-new section when the character has none.
	in2 := "[Zeal]\r\nExportOnCamp=true\r\n"
	out2 := setINIKeyInSection(in2, "Zeal_Feane", "BandolierBagSlot", "2")
	if v, ok := getINIValueInSection(out2, "Zeal_Feane", "BandolierBagSlot"); !ok || v != "2" {
		t.Errorf("append-section failed: %q (ok=%v) in %q", v, ok, out2)
	}
	if !strings.Contains(out2, "ExportOnCamp=true") {
		t.Errorf("global [Zeal] section lost: %q", out2)
	}
}

func TestReadSetBandolierBagRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Read on a missing file → not found.
	if st := ReadBandolierBagSlot(dir, "Osui"); st.Found {
		t.Errorf("expected not-found on missing file, got %+v", st)
	}

	// Set creates the file with the character section.
	if err := SetBandolierBagSlot(dir, "Osui", 3); err != nil {
		t.Fatalf("SetBandolierBagSlot: %v", err)
	}
	if st := ReadBandolierBagSlot(dir, "Osui"); !st.Found || st.Slot != 3 {
		t.Errorf("after set 3, ReadBandolierBagSlot=%+v", st)
	}

	// 0 = disabled, still found.
	if err := SetBandolierBagSlot(dir, "Osui", 0); err != nil {
		t.Fatalf("SetBandolierBagSlot(0): %v", err)
	}
	if st := ReadBandolierBagSlot(dir, "Osui"); !st.Found || st.Slot != 0 {
		t.Errorf("after set 0, ReadBandolierBagSlot=%+v, want found+0", st)
	}

	// Another character reads independently.
	if st := ReadBandolierBagSlot(dir, "Nariana"); st.Found {
		t.Errorf("Nariana should be unset, got %+v", st)
	}

	// Out of range rejected.
	if err := SetBandolierBagSlot(dir, "Osui", 9); err == nil {
		t.Errorf("expected error for slot 9")
	}
}

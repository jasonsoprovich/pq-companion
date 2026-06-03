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

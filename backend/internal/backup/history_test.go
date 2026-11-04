package backup

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

func newHistoryManager(t *testing.T, keep int) (*Manager, string) {
	t.Helper()
	base := t.TempDir()
	eq := filepath.Join(base, "eq")
	if err := os.MkdirAll(eq, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(base, "config.yaml")
	cfg := "eq_path: " + eq + "\nserver_addr: :8080\nbackup:\n  file_history_keep: " +
		strconv.Itoa(keep) + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cm, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewManagerAt(cm, base, filepath.Join(base, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m, eq
}

// writeOld writes a file with a mod time past the settle window.
func writeOld(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyFile(t *testing.T) {
	cases := []struct {
		name, kind, char string
		ok               bool
	}{
		{"Osui_spellsets.ini", KindSpellsets, "Osui", true},
		{"osui_bandolier.ini", KindBandolier, "Osui", true},
		{"Osui_pq.proj.ini", KindCharacter, "Osui", true},
		{"UI_Osui_pq.proj.ini", KindUI, "Osui", true},
		{"BZR_Osui_pq.proj.ini", KindBazaar, "Osui", true},
		{"BZR_Osui.ini", KindBazaar, "Osui", true},
		{"eqclient.ini", KindClient, "", true},
		{"zeal.ini", KindZeal, "", true},
		{"eqclient - Copy.ini", "", "", false},
		{"UI_Osui_pq.proj - backup.ini", "", "", false},
	}
	for _, c := range cases {
		kind, char, ok := ClassifyFile(c.name)
		if kind != c.kind || char != c.char || ok != c.ok {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q, %v)", c.name, kind, char, ok, c.kind, c.char, c.ok)
		}
	}
}

func TestFileHistoryCaptureDedupePruneRestore(t *testing.T) {
	m, eq := newHistoryManager(t, 2)
	spells := filepath.Join(eq, "Osui_spellsets.ini")
	writeOld(t, spells, "v1")
	writeOld(t, filepath.Join(eq, "eqclient - Copy.ini"), "ignored")

	if n, err := m.CaptureFiles(false); err != nil || n != 1 {
		t.Fatalf("first capture = %d, %v; want 1", n, err)
	}
	// Unchanged content saves nothing, even forced.
	if n, _ := m.CaptureFiles(true); n != 0 {
		t.Fatalf("unchanged capture saved %d", n)
	}
	// A fresh write waits for the settle window unless forced.
	if err := os.WriteFile(spells, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, _ := m.CaptureFiles(false); n != 0 {
		t.Fatalf("unsettled file captured")
	}
	writeOld(t, spells, "v2")
	m.CaptureFiles(false)
	writeOld(t, spells, "v3")
	m.CaptureFiles(false)

	files, err := m.TrackedFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Character != "Osui" || !files[0].Exists {
		t.Fatalf("tracked files = %+v", files)
	}
	vs := files[0].Versions
	if len(vs) != 2 {
		t.Fatalf("kept %d versions, want 2 (keep limit)", len(vs))
	}
	_, data, _ := m.VersionContent(vs[1].ID)
	if string(data) != "v2" {
		t.Fatalf("older kept version = %q, want v2", data)
	}

	// Damage the file, then restore v2: the damaged copy is saved first.
	writeOld(t, spells, "broken")
	if err := m.RestoreVersion(vs[1].ID); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(spells)
	if string(got) != "v2" {
		t.Fatalf("restored content = %q", got)
	}
	files, _ = m.TrackedFiles()
	_, newest, _ := m.VersionContent(files[0].Versions[0].ID)
	if string(newest) != "broken" {
		t.Fatalf("pre-restore content not saved; newest = %q", newest)
	}
}

func TestAutoSnapshotSkipsUnchangedFolder(t *testing.T) {
	m, eq := newHistoryManager(t, 0)
	writeOld(t, filepath.Join(eq, "eqclient.ini"), "a")
	writeOld(t, filepath.Join(eq, "eqlog_Osui_pq.proj.txt"), "log line")

	if b, err := m.create("s1", "", TriggerScheduled, false); err != nil || b == nil {
		t.Fatalf("first scheduled = %v, %v", b, err)
	}
	if b, err := m.create("s2", "", TriggerScheduled, false); err != nil || b != nil {
		t.Fatalf("unchanged scheduled = %v, %v; want skipped", b, err)
	}
	// Manual snapshots always run, and can take the logs.
	b, err := m.CreateWithLogs("manual", "")
	if err != nil || b == nil || b.FileCount != 2 {
		t.Fatalf("manual with logs = %+v, %v", b, err)
	}
	files, err := m.Files(b.ID)
	if err != nil || len(files) != 2 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	var sawLog bool
	for _, f := range files {
		if f.Kind == "log" && f.Character == "Osui" {
			sawLog = true
		}
	}
	if !sawLog {
		t.Fatalf("log file not classified: %+v", files)
	}

	writeOld(t, filepath.Join(eq, "eqclient.ini"), "b")
	if b, _ := m.create("s3", "", TriggerScheduled, false); b == nil {
		t.Fatal("changed folder skipped")
	}
}

func TestRestoreSelectedFiles(t *testing.T) {
	m, eq := newHistoryManager(t, 0)
	a := filepath.Join(eq, "Osui_spellsets.ini")
	b := filepath.Join(eq, "eqclient.ini")
	writeOld(t, a, "a1")
	writeOld(t, b, "b1")
	snap, err := m.Create("snap", "")
	if err != nil {
		t.Fatal(err)
	}
	writeOld(t, a, "a2")
	writeOld(t, b, "b2")
	if err := m.RestoreFiles(snap.ID, []string{"Osui_spellsets.ini"}); err != nil {
		t.Fatal(err)
	}
	ga, _ := os.ReadFile(a)
	gb, _ := os.ReadFile(b)
	if string(ga) != "a1" || string(gb) != "b2" {
		t.Fatalf("after selective restore: a=%q b=%q", ga, gb)
	}
}

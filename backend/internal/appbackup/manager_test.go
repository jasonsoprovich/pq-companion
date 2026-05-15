package appbackup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// makeUserDB seeds a user.db with two rows in a single test table so we can
// verify exports/imports preserve content end-to-end.
func makeUserDB(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE test_rows (id INTEGER PRIMARY KEY, payload TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO test_rows (payload) VALUES ('one'), ('two')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

// readPayloads returns all payloads from test_rows, ordered.
func readPayloads(t *testing.T, path string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT payload FROM test_rows ORDER BY id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, p)
	}
	return out
}

func writeBackupZip(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write backup: %v", err)
	}
}

// TestExportImportRoundtrip exercises Export → StageImport → ApplyPendingImport
// and verifies user.db contents and backup zips arrive identically at the
// import destination.
func TestExportImportRoundtrip(t *testing.T) {
	srcHome := t.TempDir()
	srcDB := filepath.Join(srcHome, "user.db")
	srcBackups := filepath.Join(srcHome, "backups")

	makeUserDB(t, srcDB)
	writeBackupZip(t, srcBackups, "alpha.zip", "alpha-payload")
	writeBackupZip(t, srcBackups, "beta.zip", "beta-payload")

	mgr := New(srcDB, srcBackups, srcHome, "test-1.0.0")

	exportDest := filepath.Join(t.TempDir(), "out")
	bundlePath, manifest, err := mgr.Export(exportDest)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !filepath.IsAbs(bundlePath) && bundlePath == "" {
		t.Fatalf("Export returned empty bundle path")
	}
	if got, want := manifest.Stats.BackupCount, 2; got != want {
		t.Errorf("manifest.Stats.BackupCount = %d, want %d", got, want)
	}
	if got, want := manifest.FormatVersion, FormatVersion; got != want {
		t.Errorf("manifest.FormatVersion = %d, want %d", got, want)
	}
	// Preview should return the same shape without staging anything.
	preview, err := mgr.PreviewImport(bundlePath)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Stats.BackupCount != 2 {
		t.Errorf("PreviewImport BackupCount = %d, want 2", preview.Stats.BackupCount)
	}
	if mgr.HasPendingImport() {
		t.Fatal("PreviewImport should not stage anything")
	}

	// Fresh destination — new appHome with NO existing user.db or backups dir
	// yet, to simulate first-time import on a different machine.
	dstHome := t.TempDir()
	dstDB := filepath.Join(dstHome, "user.db")
	dstBackups := filepath.Join(dstHome, "backups")

	dstMgr := New(dstDB, dstBackups, dstHome, "test-1.0.0")

	if _, err := dstMgr.StageImport(bundlePath); err != nil {
		t.Fatalf("StageImport: %v", err)
	}
	if !dstMgr.HasPendingImport() {
		t.Fatal("HasPendingImport should be true after staging")
	}

	applied, err := dstMgr.ApplyPendingImport()
	if err != nil {
		t.Fatalf("ApplyPendingImport: %v", err)
	}
	if !applied {
		t.Fatal("ApplyPendingImport returned false despite pending import")
	}
	if dstMgr.HasPendingImport() {
		t.Fatal("HasPendingImport should be cleared after apply")
	}

	// Verify the swapped-in user.db has the original rows.
	got := readPayloads(t, dstDB)
	want := []string{"one", "two"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("imported user.db payloads = %v, want %v", got, want)
	}

	// Verify the backup zips arrived.
	for _, name := range []string{"alpha.zip", "beta.zip"} {
		b, err := os.ReadFile(filepath.Join(dstBackups, name))
		if err != nil {
			t.Errorf("missing imported backup %s: %v", name, err)
			continue
		}
		expected := name[:len(name)-len(".zip")] + "-payload"
		if string(b) != expected {
			t.Errorf("imported %s content = %q, want %q", name, string(b), expected)
		}
	}
}

// TestApplyPreservesPriorUserDB checks that the existing user.db on the
// import target is renamed aside (not deleted) so the user can recover.
func TestApplyPreservesPriorUserDB(t *testing.T) {
	srcHome := t.TempDir()
	srcDB := filepath.Join(srcHome, "user.db")
	srcBackups := filepath.Join(srcHome, "backups")
	makeUserDB(t, srcDB)
	writeBackupZip(t, srcBackups, "one.zip", "src-content")

	srcMgr := New(srcDB, srcBackups, srcHome, "v1")
	bundle, _, err := srcMgr.Export(filepath.Join(t.TempDir(), "b"))
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dstHome := t.TempDir()
	dstDB := filepath.Join(dstHome, "user.db")
	dstBackups := filepath.Join(dstHome, "backups")

	// Pre-existing user.db with different content so we can distinguish it.
	if err := os.MkdirAll(filepath.Dir(dstDB), 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := os.WriteFile(dstDB, []byte("preexisting-db-bytes"), 0o644); err != nil {
		t.Fatalf("write preexisting: %v", err)
	}
	writeBackupZip(t, dstBackups, "preexisting.zip", "preexisting-content")

	dstMgr := New(dstDB, dstBackups, dstHome, "v1")
	if _, err := dstMgr.StageImport(bundle); err != nil {
		t.Fatalf("StageImport: %v", err)
	}
	if _, err := dstMgr.ApplyPendingImport(); err != nil {
		t.Fatalf("ApplyPendingImport: %v", err)
	}

	// Look for *.preimport siblings — should contain the original bytes /
	// the original backup zip.
	entries, err := os.ReadDir(dstHome)
	if err != nil {
		t.Fatalf("readdir dstHome: %v", err)
	}
	var dbAside, backupsAside string
	for _, e := range entries {
		name := e.Name()
		switch {
		case !e.IsDir() && filepath.Ext(name) == ".preimport" && len(name) > len("user.db") && name[:len("user.db")] == "user.db":
			dbAside = filepath.Join(dstHome, name)
		case e.IsDir() && len(name) > len("backups") && name[:len("backups")] == "backups" && filepath.Ext(name) == ".preimport":
			backupsAside = filepath.Join(dstHome, name)
		}
	}
	if dbAside == "" {
		t.Fatal("no user.db.*.preimport aside-copy left behind")
	}
	if backupsAside == "" {
		t.Fatal("no backups.*.preimport aside-copy left behind")
	}

	gotDB, _ := os.ReadFile(dbAside)
	if string(gotDB) != "preexisting-db-bytes" {
		t.Errorf("aside user.db content = %q, want preserved preexisting bytes", string(gotDB))
	}
	gotZip, _ := os.ReadFile(filepath.Join(backupsAside, "preexisting.zip"))
	if string(gotZip) != "preexisting-content" {
		t.Errorf("aside backup zip content = %q, want preserved preexisting content", string(gotZip))
	}
}

// TestStageImportRefusesFutureFormat ensures import refuses bundles whose
// format version exceeds what the running app supports.
func TestStageImportRefusesFutureFormat(t *testing.T) {
	// Hand-craft a bundle with a bumped format version.
	tmp := t.TempDir()
	bundlePath := filepath.Join(tmp, "future.pqcb")
	// Use Export to build a real bundle, then we'd need to rewrite its
	// manifest. Simpler: build a bundle manually.
	if err := writeBogusFutureBundle(bundlePath); err != nil {
		t.Fatalf("build bogus bundle: %v", err)
	}
	mgr := New(filepath.Join(tmp, "user.db"), filepath.Join(tmp, "backups"), tmp, "v1")
	_, err := mgr.StageImport(bundlePath)
	if err == nil {
		t.Fatal("StageImport should refuse future format bundle")
	}
}

// writeBogusFutureBundle creates a minimal valid-shaped bundle but with a
// format_version one above what the build supports.
func writeBogusFutureBundle(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// Local import to avoid leaking archive/zip into the test file body.
	zw := newTestZip(f)
	defer zw.Close()
	manifest := fmt.Sprintf(`{"format_version":%d,"app_version":"x","exported_at":"now","files":[],"stats":{}}`, FormatVersion+1)
	w, err := zw.Create(manifestName)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(manifest)); err != nil {
		return err
	}
	// Add an empty user.db so the bundle isn't *obviously* malformed.
	dbw, err := zw.Create(userDBName)
	if err != nil {
		return err
	}
	_, err = dbw.Write([]byte("not really a sqlite file"))
	return err
}

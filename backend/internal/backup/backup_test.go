package backup_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// ─── Store tests ──────────────────────────────────────────────────────────────

func TestStoreOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	s, err := backup.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	// Second open on same file should also succeed (idempotent migration).
	s2, err := backup.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("second OpenStore: %v", err)
	}
	s2.Close()
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := backup.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	b := &backup.Backup{
		ID:        "abc123",
		Name:      "Test Backup",
		Notes:     "some notes",
		CreatedAt: time.Unix(1700000000, 0).UTC(),
		SizeBytes: 4096,
		FileCount: 3,
	}

	t.Run("insert", func(t *testing.T) {
		if err := s.Insert(b); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	})

	t.Run("get", func(t *testing.T) {
		got, err := s.Get("abc123")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Name != b.Name {
			t.Errorf("name: got %q, want %q", got.Name, b.Name)
		}
		if got.FileCount != b.FileCount {
			t.Errorf("file_count: got %d, want %d", got.FileCount, b.FileCount)
		}
	})

	t.Run("list", func(t *testing.T) {
		list, err := s.List()
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("len(list): got %d, want 1", len(list))
		}
		if list[0].ID != "abc123" {
			t.Errorf("id: got %q, want %q", list[0].ID, "abc123")
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := s.Delete("abc123"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		list, _ := s.List()
		if len(list) != 0 {
			t.Errorf("expected empty list after delete, got %d entries", len(list))
		}
	})
}

func TestStoreListOrderedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	s, err := backup.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	for i, ts := range []int64{1000, 2000, 3000} {
		b := &backup.Backup{
			ID:        string(rune('a' + i)),
			Name:      "Backup",
			CreatedAt: time.Unix(ts, 0).UTC(),
		}
		if err := s.Insert(b); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len: got %d, want 3", len(list))
	}
	// Expect newest (unix 3000) first.
	if list[0].CreatedAt.Unix() != 3000 {
		t.Errorf("first item not newest: got %d, want 3000", list[0].CreatedAt.Unix())
	}
}

// ─── Manager tests ────────────────────────────────────────────────────────────

// newTestManager creates a Manager pointing at a temporary EQ dir and backup dir.
func newTestManager(t *testing.T, eqPath string) *backup.Manager {
	t.Helper()
	base := t.TempDir()

	// Write a minimal config pointing at the temp EQ path.
	cfgPath := filepath.Join(base, "config.yaml")
	cfgContent := "eq_path: " + eqPath + "\nserver_addr: :8080\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	mgr, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	bm, err := backup.NewManagerAt(mgr, base, filepath.Join(base, "backups"))
	if err != nil {
		t.Fatalf("NewManagerAt: %v", err)
	}
	t.Cleanup(func() { bm.Close() })
	return bm
}

func TestManagerCreateAndList(t *testing.T) {
	eqDir := t.TempDir()
	// Write a couple of .ini files.
	for _, name := range []string{"eqclient.ini", "UI_Tester_pq.proj.ini"} {
		if err := os.WriteFile(filepath.Join(eqDir, name), []byte("[Settings]\nfoo=bar\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	bm := newTestManager(t, eqDir)

	b, err := bm.Create("My First Backup", "test notes")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if b.ID == "" {
		t.Error("expected non-empty ID")
	}
	if b.FileCount != 2 {
		t.Errorf("file_count: got %d, want 2", b.FileCount)
	}
	if b.SizeBytes == 0 {
		t.Error("expected non-zero size_bytes")
	}

	list, err := bm.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list): got %d, want 1", len(list))
	}
	if list[0].Name != "My First Backup" {
		t.Errorf("name: got %q, want %q", list[0].Name, "My First Backup")
	}
}

func TestManagerCreateNoEQPath(t *testing.T) {
	base := t.TempDir()
	cfgPath := filepath.Join(base, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("server_addr: :8080\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	mgr, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	bm, err := backup.NewManagerAt(mgr, base, filepath.Join(base, "backups"))
	if err != nil {
		t.Fatalf("NewManagerAt: %v", err)
	}
	defer bm.Close()

	_, err = bm.Create("fail", "")
	if err == nil {
		t.Fatal("expected error when eq_path is empty")
	}
}

func TestManagerCreateNoIniFiles(t *testing.T) {
	eqDir := t.TempDir() // empty — no .ini files
	bm := newTestManager(t, eqDir)
	_, err := bm.Create("fail", "")
	if err == nil {
		t.Fatal("expected error when no *.ini files found")
	}
}

func TestManagerDelete(t *testing.T) {
	eqDir := t.TempDir()
	os.WriteFile(filepath.Join(eqDir, "eqclient.ini"), []byte("[foo]\n"), 0o644)

	bm := newTestManager(t, eqDir)
	b, err := bm.Create("del test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := bm.Delete(b.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, _ := bm.List()
	if len(list) != 0 {
		t.Errorf("expected empty list after delete, got %d", len(list))
	}
}

func TestManagerDeleteNotFound(t *testing.T) {
	bm := newTestManager(t, t.TempDir())
	err := bm.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent backup")
	}
}

func TestManagerRestore(t *testing.T) {
	eqDir := t.TempDir()
	iniPath := filepath.Join(eqDir, "eqclient.ini")
	os.WriteFile(iniPath, []byte("[original]\nvalue=1\n"), 0o644)

	bm := newTestManager(t, eqDir)
	b, err := bm.Create("restore test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Overwrite the file.
	os.WriteFile(iniPath, []byte("[modified]\nvalue=2\n"), 0o644)

	if err := bm.Restore(b.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "[original]\nvalue=1\n" {
		t.Errorf("restored content wrong: got %q", string(data))
	}
}

func TestManagerRestoreNotFound(t *testing.T) {
	bm := newTestManager(t, t.TempDir())
	err := bm.Restore("nonexistent")
	if err == nil {
		t.Fatal("expected error restoring nonexistent backup")
	}
}

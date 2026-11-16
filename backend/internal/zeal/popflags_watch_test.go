package zeal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Every character's export is offered once per change; one the callback
// fails on is offered again on the next poll even though it didn't change.
func TestCheckPopFlagsFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("Osui-PoPFlags.txt", "a")
	b := write("Alt-PoPFlags_pq.proj.txt", "b")
	write("Osui-Inventory.txt", "not an export")

	calls := map[string]int{}
	failB := true
	w := &Watcher{}
	w.SetPopFlagsFileCallback(func(path string) error {
		calls[filepath.Base(path)]++
		if path == b && failB {
			return errors.New("half written")
		}
		return nil
	})

	w.checkPopFlagsFiles(dir)
	w.checkPopFlagsFiles(dir)
	if calls["Osui-PoPFlags.txt"] != 1 {
		t.Errorf("unchanged export offered %d times, want 1", calls["Osui-PoPFlags.txt"])
	}
	if calls["Alt-PoPFlags_pq.proj.txt"] != 2 {
		t.Errorf("failed export offered %d times, want 2 (retried)", calls["Alt-PoPFlags_pq.proj.txt"])
	}
	if calls["Osui-Inventory.txt"] != 0 {
		t.Error("other exports must be ignored")
	}

	failB = false
	w.checkPopFlagsFiles(dir)
	w.checkPopFlagsFiles(dir)
	if calls["Alt-PoPFlags_pq.proj.txt"] != 3 {
		t.Errorf("export offered %d times after it succeeded, want 3", calls["Alt-PoPFlags_pq.proj.txt"])
	}

	// A rewrite is offered again.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	w.checkPopFlagsFiles(dir)
	if calls["Osui-PoPFlags.txt"] != 2 {
		t.Errorf("changed export offered %d times, want 2", calls["Osui-PoPFlags.txt"])
	}
}

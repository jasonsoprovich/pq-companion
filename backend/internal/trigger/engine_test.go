package trigger

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// openTestStore creates a temporary user.db for testing.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// openTestEngine returns a Store and Engine backed by a temp database.
func openTestEngine(t *testing.T) (*Store, *Engine) {
	t.Helper()
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil)
	return s, e
}

func TestEngine_MatchAndFire(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "test1",
		Name:    "Mez Wore Off",
		Enabled: true,
		Pattern: `Your .+ spell has worn off\.`,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Should fire.
	e.Handle(time.Now(), "Your Mesmerization spell has worn off.")
	hist := e.GetHistory()
	if len(hist) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(hist))
	}
	if hist[0].TriggerName != "Mez Wore Off" {
		t.Errorf("expected trigger name %q, got %q", "Mez Wore Off", hist[0].TriggerName)
	}
	if hist[0].MatchedLine != "Your Mesmerization spell has worn off." {
		t.Errorf("unexpected matched line: %q", hist[0].MatchedLine)
	}

	// Should not fire on unrelated line.
	e.Handle(time.Now(), "You slash a gnoll for 100 points of damage.")
	if len(e.GetHistory()) != 1 {
		t.Errorf("unrelated line should not fire trigger")
	}
}

func TestEngine_DisabledTriggerDoesNotFire(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "disabled1",
		Name:    "Disabled Trigger",
		Enabled: false,
		Pattern: `You have been slain`,
		Actions: []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "You have been slain by a gnoll.")
	if len(e.GetHistory()) != 0 {
		t.Errorf("disabled trigger should not fire")
	}
}

func TestEngine_ReloadUpdatesPatterns(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "upd1",
		Name:    "Tell Alert",
		Enabled: true,
		Pattern: `\w+ tells you,`,
		Actions: []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Somedude tells you, 'hello'")
	if len(e.GetHistory()) != 1 {
		t.Fatalf("expected 1 entry after match")
	}

	// Disable and reload — should no longer fire.
	tr.Enabled = false
	if err := s.Update(tr); err != nil {
		t.Fatalf("Update: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Otherdude tells you, 'hi'")
	if len(e.GetHistory()) != 1 {
		t.Errorf("disabled trigger fired after reload")
	}
}

func TestEngine_HistoryRingBuffer(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "ring1",
		Name:    "Any Hit",
		Enabled: true,
		Pattern: `damage`,
		Actions: []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	for i := 0; i < historyMaxSize+10; i++ {
		e.Handle(time.Now(), "You slash a gnoll for 100 points of damage.")
	}
	if len(e.GetHistory()) != historyMaxSize {
		t.Errorf("expected history capped at %d, got %d", historyMaxSize, len(e.GetHistory()))
	}
}

func TestStore_CRUD(t *testing.T) {
	s := openTestStore(t)

	// Insert.
	tr := &Trigger{
		ID:      "crud1",
		Name:    "Test Trigger",
		Enabled: true,
		Pattern: `test`,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "Test!", DurationSecs: 3, Color: "#ffffff"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Get.
	got, err := s.Get("crud1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != tr.Name {
		t.Errorf("name mismatch: got %q want %q", got.Name, tr.Name)
	}
	if len(got.Actions) != 1 || got.Actions[0].Text != "Test!" {
		t.Errorf("actions not preserved: %+v", got.Actions)
	}

	// Update.
	got.Name = "Updated"
	got.Enabled = false
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := s.Get("crud1")
	if updated.Name != "Updated" || updated.Enabled {
		t.Errorf("update did not persist: %+v", updated)
	}

	// List.
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 trigger, got %d", len(list))
	}

	// Delete.
	if err := s.Delete("crud1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("crud1"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_ErrNotFound(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.Get("nonexistent"); err != ErrNotFound {
		t.Errorf("Get nonexistent: expected ErrNotFound, got %v", err)
	}
	if err := s.Delete("nonexistent"); err != ErrNotFound {
		t.Errorf("Delete nonexistent: expected ErrNotFound, got %v", err)
	}
	if err := s.Update(&Trigger{ID: "nonexistent", Actions: []Action{}}); err != ErrNotFound {
		t.Errorf("Update nonexistent: expected ErrNotFound, got %v", err)
	}
}

func TestInstallPack(t *testing.T) {
	s := openTestStore(t)

	pack := EnchanterPack()
	if err := InstallPack(s, pack); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(pack.Triggers) {
		t.Errorf("expected %d triggers after install, got %d", len(pack.Triggers), len(list))
	}

	// Re-installing should replace, not duplicate.
	if err := InstallPack(s, pack); err != nil {
		t.Fatalf("second InstallPack: %v", err)
	}
	list2, _ := s.List()
	if len(list2) != len(pack.Triggers) {
		t.Errorf("re-install duplicated triggers: expected %d, got %d", len(pack.Triggers), len(list2))
	}
}


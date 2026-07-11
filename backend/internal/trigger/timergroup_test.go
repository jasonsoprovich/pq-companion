package trigger

import (
	"errors"
	"testing"
)

func TestCreateTimerGroup_PersistsEmpty(t *testing.T) {
	s := openTestStore(t)

	g, err := s.CreateTimerGroup("Raid Timers")
	if err != nil {
		t.Fatalf("CreateTimerGroup: %v", err)
	}
	if g.Name != "Raid Timers" || g.Count != 0 || g.ID == "" {
		t.Fatalf("unexpected group: %+v", g)
	}

	groups, err := s.ListTimerGroups()
	if err != nil {
		t.Fatalf("ListTimerGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID || groups[0].Name != "Raid Timers" {
		t.Fatalf("unexpected listed groups: %+v", groups)
	}
}

func TestCreateTimerGroup_Rejects(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.CreateTimerGroup("   "); !errors.Is(err, ErrTimerGroupNameEmpty) {
		t.Errorf("blank name: got %v, want ErrTimerGroupNameEmpty", err)
	}
	if _, err := s.CreateTimerGroup("Raid Timers"); err != nil {
		t.Fatalf("CreateTimerGroup: %v", err)
	}
	if _, err := s.CreateTimerGroup("Raid Timers"); !errors.Is(err, ErrTimerGroupExists) {
		t.Errorf("duplicate name: got %v, want ErrTimerGroupExists", err)
	}
}

// ListTimerGroups' count must reflect triggers currently assigned via
// custom_group_id, and must not include triggers in other groups or the
// default (unassigned) window.
func TestListTimerGroups_CountsAssignedTriggers(t *testing.T) {
	s := openTestStore(t)
	a, err := s.CreateTimerGroup("Group A")
	if err != nil {
		t.Fatalf("CreateTimerGroup A: %v", err)
	}
	b, err := s.CreateTimerGroup("Group B")
	if err != nil {
		t.Fatalf("CreateTimerGroup B: %v", err)
	}

	t1 := makeTrigger("T1", "")
	t1.CustomGroupID = a.ID
	t2 := makeTrigger("T2", "")
	t2.CustomGroupID = a.ID
	t3 := makeTrigger("T3", "")
	t3.CustomGroupID = b.ID
	t4 := makeTrigger("T4", "") // default window — no group
	for _, tr := range []*Trigger{t1, t2, t3, t4} {
		if err := s.Insert(tr); err != nil {
			t.Fatalf("Insert %s: %v", tr.Name, err)
		}
	}

	groups, err := s.ListTimerGroups()
	if err != nil {
		t.Fatalf("ListTimerGroups: %v", err)
	}
	counts := make(map[string]int, len(groups))
	for _, g := range groups {
		counts[g.ID] = g.Count
	}
	if counts[a.ID] != 2 {
		t.Errorf("Group A count = %d, want 2", counts[a.ID])
	}
	if counts[b.ID] != 1 {
		t.Errorf("Group B count = %d, want 1", counts[b.ID])
	}
}

// Renaming a group must not touch its ID or any trigger's custom_group_id —
// unlike categories (keyed by name), triggers reference the group by ID, so
// no cascade is needed.
func TestRenameTimerGroup_KeepsIDAndAssignments(t *testing.T) {
	s := openTestStore(t)
	g, err := s.CreateTimerGroup("Raid Timers")
	if err != nil {
		t.Fatalf("CreateTimerGroup: %v", err)
	}
	tr := makeTrigger("Signature Spell", "")
	tr.CustomGroupID = g.ID
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := s.RenameTimerGroup(g.ID, "Emperor Timers"); err != nil {
		t.Fatalf("RenameTimerGroup: %v", err)
	}

	groups, err := s.ListTimerGroups()
	if err != nil {
		t.Fatalf("ListTimerGroups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != g.ID || groups[0].Name != "Emperor Timers" {
		t.Fatalf("unexpected groups after rename: %+v", groups)
	}

	stored, err := s.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.CustomGroupID != g.ID {
		t.Errorf("trigger's custom_group_id changed on rename: got %q, want %q", stored.CustomGroupID, g.ID)
	}
}

func TestRenameTimerGroup_NotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.RenameTimerGroup("bogus-id", "New Name"); !errors.Is(err, ErrTimerGroupNotFound) {
		t.Errorf("got %v, want ErrTimerGroupNotFound", err)
	}
}

// Deleting a timer group must reassign its triggers to the default window
// (custom_group_id = ""), never delete them.
func TestDeleteTimerGroup_ReassignsTriggersToDefault(t *testing.T) {
	s := openTestStore(t)
	g, err := s.CreateTimerGroup("Raid Timers")
	if err != nil {
		t.Fatalf("CreateTimerGroup: %v", err)
	}
	tr := makeTrigger("Signature Spell", "")
	tr.CustomGroupID = g.ID
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := s.DeleteTimerGroup(g.ID); err != nil {
		t.Fatalf("DeleteTimerGroup: %v", err)
	}

	groups, err := s.ListTimerGroups()
	if err != nil {
		t.Fatalf("ListTimerGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected group deleted, got %+v", groups)
	}

	stored, err := s.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.CustomGroupID != "" {
		t.Errorf("trigger not reassigned to default: custom_group_id = %q", stored.CustomGroupID)
	}
}

func TestDeleteTimerGroup_NotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.DeleteTimerGroup("bogus-id"); !errors.Is(err, ErrTimerGroupNotFound) {
		t.Errorf("got %v, want ErrTimerGroupNotFound", err)
	}
}

func TestReorderTimerGroups(t *testing.T) {
	s := openTestStore(t)
	a, _ := s.CreateTimerGroup("A")
	b, _ := s.CreateTimerGroup("B")

	if err := s.ReorderTimerGroups([]string{b.ID, a.ID}); err != nil {
		t.Fatalf("ReorderTimerGroups: %v", err)
	}

	groups, err := s.ListTimerGroups()
	if err != nil {
		t.Fatalf("ListTimerGroups: %v", err)
	}
	if len(groups) != 2 || groups[0].ID != b.ID || groups[1].ID != a.ID {
		t.Fatalf("unexpected order: %+v", groups)
	}
}

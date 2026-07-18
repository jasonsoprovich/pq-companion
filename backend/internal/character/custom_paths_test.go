package character

import "testing"

func TestCustomLevelingRecipes_AddListDelete(t *testing.T) {
	s, _ := openTestStore(t)

	if got, err := s.ListCustomLevelingRecipes(63); err != nil || len(got) != 0 {
		t.Fatalf("empty list: got %v err %v", got, err)
	}

	if err := s.AddCustomLevelingRecipe(63, 791); err != nil {
		t.Fatalf("add 791: %v", err)
	}
	if err := s.AddCustomLevelingRecipe(63, 100); err != nil {
		t.Fatalf("add 100: %v", err)
	}
	// Re-adding is idempotent.
	if err := s.AddCustomLevelingRecipe(63, 791); err != nil {
		t.Fatalf("re-add 791: %v", err)
	}
	// A different tradeskill's path is independent.
	if err := s.AddCustomLevelingRecipe(61, 555); err != nil {
		t.Fatalf("add 555 to tailoring: %v", err)
	}

	got, err := s.ListCustomLevelingRecipes(63)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := map[int]bool{791: true, 100: true}
	if len(got) != len(want) {
		t.Fatalf("list = %v, want keys of %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected recipe id %d in list %v", id, got)
		}
	}

	if got, err := s.ListCustomLevelingRecipes(61); err != nil || len(got) != 1 || got[0] != 555 {
		t.Fatalf("tailoring list = %v err %v, want [555]", got, err)
	}

	if err := s.DeleteCustomLevelingRecipe(63, 791); err != nil {
		t.Fatalf("delete 791: %v", err)
	}
	got, _ = s.ListCustomLevelingRecipes(63)
	if len(got) != 1 || got[0] != 100 {
		t.Errorf("after delete, list = %v, want [100]", got)
	}
}

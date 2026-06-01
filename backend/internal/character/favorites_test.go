package character

import "testing"

func TestFavoriteRecipes_AddListDelete(t *testing.T) {
	s, _ := openTestStore(t)

	if got, err := s.ListFavoriteRecipes(); err != nil || len(got) != 0 {
		t.Fatalf("empty list: got %v err %v", got, err)
	}

	if err := s.AddFavoriteRecipe(791); err != nil {
		t.Fatalf("add 791: %v", err)
	}
	if err := s.AddFavoriteRecipe(100); err != nil {
		t.Fatalf("add 100: %v", err)
	}
	// Re-adding is idempotent and must not reorder or duplicate.
	if err := s.AddFavoriteRecipe(791); err != nil {
		t.Fatalf("re-add 791: %v", err)
	}

	got, err := s.ListFavoriteRecipes()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []int{791, 100} // insertion order via sort_order
	if len(got) != len(want) {
		t.Fatalf("list = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("list[%d] = %d, want %d (full %v)", i, got[i], want[i], got)
		}
	}

	if err := s.DeleteFavoriteRecipe(791); err != nil {
		t.Fatalf("delete 791: %v", err)
	}
	got, _ = s.ListFavoriteRecipes()
	if len(got) != 1 || got[0] != 100 {
		t.Errorf("after delete, list = %v, want [100]", got)
	}
}

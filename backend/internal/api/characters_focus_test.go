package api

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func openFocusTestDB(t *testing.T) *db.DB {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	d, err := db.Open(filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db"))
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMissingFocusCategories(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	enchanter := character.Character{Class: 13, Race: 1, Level: 60}

	ids := func(m []missingFocusCategory) map[string]bool {
		out := map[string]bool{}
		for _, c := range m {
			out[c.ID] = true
		}
		return out
	}

	// Nothing worn: spell haste is available to an enchanter, so it's missing.
	none := ids(h.missingFocusCategories(enchanter, nil))
	if !none["spell_haste"] {
		t.Errorf("enchanter with no focus should be missing spell_haste, got %v", none)
	}

	// Wearing a spell-haste focus removes just that category.
	worn := []buffmod.Modifier{{SPA: buffmod.SPACastTime, Percent: 15, Category: "spell_haste"}}
	got := ids(h.missingFocusCategories(enchanter, worn))
	if got["spell_haste"] {
		t.Errorf("spell_haste should no longer be missing once worn")
	}
	if len(got) != len(none)-1 {
		t.Errorf("only spell_haste should drop out: before %d, after %d", len(none), len(got))
	}

	// Non-casters get no hints.
	warrior := character.Character{Class: 0, Race: 1, Level: 60}
	if m := h.missingFocusCategories(warrior, nil); len(m) != 0 {
		t.Errorf("warrior should get no missing-focus hints, got %v", m)
	}
}

func TestFocusCandidates_ClassAndLevelFiltered(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	lowLevel, err := h.focusCandidates(character.Character{Class: 13, Race: 1, Level: 10})
	if err != nil {
		t.Fatal(err)
	}
	high, err := h.focusCandidates(character.Character{Class: 13, Race: 1, Level: 60})
	if err != nil {
		t.Fatal(err)
	}
	count := func(m map[string][]focusItem) int {
		n := 0
		for _, v := range m {
			n += len(v)
		}
		return n
	}
	if count(high) <= count(lowLevel) {
		t.Errorf("level 60 should see more focus items than level 10: %d vs %d", count(high), count(lowLevel))
	}
	for cat, items := range high {
		for _, it := range items {
			if it.ReqLevel > 60 {
				t.Errorf("%s: %s requires level %d > 60", cat, it.Name, it.ReqLevel)
			}
		}
	}
}

// Focus level caps are hard limits: a level-65 caster gets nothing from a "Max
// Level 60" focus on their top spells, so Spell Haste IV (≤65) must count as
// covering them and III (≤60) must not — even though III's percent can be
// higher — while a level-60 caster is fully covered by III.
func TestFocusCandidates_CoversTop(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}

	coverage := func(level int) map[int]bool {
		cands, err := h.focusCandidates(character.Character{Class: 13, Race: 1, Level: level})
		if err != nil {
			t.Fatal(err)
		}
		out := map[int]bool{}
		for _, it := range cands["spell_haste"] {
			out[it.FocusSpellID] = it.CoversTop
		}
		return out
	}
	const spellHasteIII, spellHasteIV = 2341, 3525

	at65 := coverage(65)
	if covers, ok := at65[spellHasteIV]; !ok || !covers {
		t.Errorf("level 65: Spell Haste IV should be a covering candidate (present=%v covers=%v)", ok, covers)
	}
	if covers, ok := at65[spellHasteIII]; ok && covers {
		t.Errorf("level 65: Spell Haste III (max level 60) must not cover top spells")
	}
	if covers, ok := coverage(60)[spellHasteIII]; ok && !covers {
		t.Errorf("level 60: Spell Haste III should cover top spells")
	}
}

// Bash hate is tank gear, never a "missing" spell focus — even for a paladin,
// who isn't in the non-caster list.
func TestMissingFocusCategories_NoBashHate(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	paladin := character.Character{Class: 2, Race: 1, Level: 65}
	for _, c := range h.missingFocusCategories(paladin, nil) {
		if c.ID == "bash_hate" {
			t.Errorf("bash_hate must never be offered as a missing focus type")
		}
	}
}

func TestFocusCategorySummary_Equipped(t *testing.T) {
	h := &charactersHandler{db: openFocusTestDB(t)}
	enchanter := character.Character{Class: 13, Race: 1, Level: 60}
	worn := []buffmod.Modifier{
		{SPA: buffmod.SPACastTime, Percent: 10, Category: "spell_haste"},
		{SPA: buffmod.SPACastTime, Percent: 25, Category: "spell_haste"},
	}
	missing, equipped := h.focusCategorySummary(enchanter, worn)
	if len(equipped) != 1 || equipped[0].ID != "spell_haste" {
		t.Fatalf("want only spell_haste equipped, got %+v", equipped)
	}
	if equipped[0].Percent != 25 {
		t.Errorf("equipped percent should be the strongest worn (25), got %d", equipped[0].Percent)
	}
	if equipped[0].ItemCount == 0 {
		t.Errorf("spell_haste should list usable items for a level 60 enchanter")
	}
	for _, m := range missing {
		if m.ID == "spell_haste" {
			t.Errorf("spell_haste should not be both equipped and missing")
		}
	}

	// Non-casters still see what they wear, but get no missing hints.
	warrior := character.Character{Class: 0, Race: 1, Level: 60}
	missing, equipped = h.focusCategorySummary(warrior, []buffmod.Modifier{
		{SPA: buffmod.SPASpellHate, Percent: 5, Category: "bash_hate"},
	})
	if len(missing) != 0 || len(equipped) != 1 {
		t.Errorf("warrior: want 0 missing / 1 equipped, got %d / %d", len(missing), len(equipped))
	}
}

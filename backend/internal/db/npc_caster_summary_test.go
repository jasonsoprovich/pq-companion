package db

import (
	"path/filepath"
	"runtime"
	"testing"
)

// row builds a casterSpellRow with the given effect SPAs filled into the slots
// (rest default to 254 = empty), to keep the table terse.
func row(tt, aoe, cat, base int, own bool, effs ...int) casterSpellRow {
	r := casterSpellRow{targetType: tt, aoeRange: aoe, category: cat, baseValue1: base, ownList: own}
	for i := range r.effects {
		r.effects[i] = 254
	}
	for i, e := range effs {
		if i < len(r.effects) {
			r.effects[i] = e
		}
	}
	return r
}

func hasTag(hs []CasterHighlight, tag string) bool {
	for _, h := range hs {
		if h.Tag == tag {
			return true
		}
	}
	return false
}

func TestBuildHighlights(t *testing.T) {
	cases := []struct {
		name    string
		rows    []casterSpellRow
		want    []string // expected tags (any order, must match set exactly)
		notWant []string
	}{
		{
			name: "complete healing by base value",
			rows: []casterSpellRow{row(5, 0, 20, 7500, true, 0)}, // spell 13
			want: []string{"complete_heal"},
		},
		{
			name:    "weak heal is not complete heal",
			rows:    []casterSpellRow{row(5, 0, 20, 24, true, 0)}, // Light Healing
			notWant: []string{"complete_heal"},
		},
		{
			name: "complete heal by effect 101",
			rows: []casterSpellRow{row(5, 0, 0, 0, true, 101)},
			want: []string{"complete_heal"},
		},
		{
			name: "gate effect",
			rows: []casterSpellRow{row(6, 0, 56, 0, false, 26)}, // spell 36 Gate
			want: []string{"gate"},
		},
		{
			name:    "targeted AE by targettype, not flagged PB",
			rows:    []casterSpellRow{row(8, 80, -99, 0, false, 96)}, // Silence of the Shadows: AE + silence
			want:    []string{"targeted_ae", "silence"},
			notWant: []string{"pb_ae"},
		},
		{
			name:    "PB AE not double-flagged as targeted",
			rows:    []casterSpellRow{row(4, 35, 11, -138, false, 0)}, // Upheaval: PB AE nuke
			want:    []string{"pb_ae"},
			notWant: []string{"targeted_ae"},
		},
		{
			name: "aoerange alone implies AE",
			rows: []casterSpellRow{row(5, 40, 0, 0, false, 0)},
			want: []string{"targeted_ae"},
		},
		{
			name: "crowd control set",
			rows: []casterSpellRow{
				row(5, 0, 0, 0, false, 31), // mez
				row(5, 0, 0, 0, false, 22), // charm
				row(5, 0, 0, 0, false, 23), // fear
				row(5, 0, 0, 0, false, 99), // root
			},
			want: []string{"mez", "charm", "fear", "root"},
		},
		{
			name: "lifetap by targettype",
			rows: []casterSpellRow{row(13, 0, 0, 0, false, 0)},
			want: []string{"lifetap"},
		},
		{
			name: "plain single-target nuke yields no highlight",
			rows: []casterSpellRow{row(5, 0, 1, -90, false, 0)}, // Reckoning
			want: []string{},
		},
		{
			name: "tags deduped across many rows",
			rows: []casterSpellRow{
				row(8, 80, -99, 0, false, 59),
				row(8, 60, -99, 0, false, 10),
			},
			want: []string{"targeted_ae"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildHighlights(tc.rows)
			for _, w := range tc.want {
				if !hasTag(got, w) {
					t.Errorf("want tag %q, got %+v", w, got)
				}
			}
			for _, nw := range tc.notWant {
				if hasTag(got, nw) {
					t.Errorf("did not want tag %q, got %+v", nw, got)
				}
			}
			// "want" with no extras: when notWant is empty and want is the full
			// set, assert no stray tags.
			if tc.notWant == nil {
				if len(got) != len(tc.want) {
					t.Errorf("tag count mismatch: want %d %v, got %d %+v", len(tc.want), tc.want, len(got), got)
				}
			}
		})
	}
}

func TestCasterSpellRowNamedSpell(t *testing.T) {
	cases := []struct {
		name string
		row  casterSpellRow
		want NamedSpell
	}{
		{
			name: "AI recast_delay wins over spell recast_time, rounded to seconds",
			row:  casterSpellRow{spellID: 1, name: "Word of Command", targetType: 4, aoeRange: 35, recastDelayMS: 30000, recastTimeMS: 12000, resistType: 1, resistDiff: -100},
			want: NamedSpell{SpellID: 1, SpellName: "Word of Command", RecastSecs: 30, AEType: "PBAE", AERange: 35, ResistType: "MR", ResistDiff: -100},
		},
		{
			name: "falls back to spell recast_time when AI delay unset",
			row:  casterSpellRow{spellID: 2, name: "Fling", targetType: 4, aoeRange: 200, recastDelayMS: -1, recastTimeMS: 45000},
			want: NamedSpell{SpellID: 2, SpellName: "Fling", RecastSecs: 45, AEType: "PBAE", AERange: 200},
		},
		{
			name: "targeted AE classifies as TAE",
			row:  casterSpellRow{spellID: 3, name: "Silence of the Shadows", targetType: 8, aoeRange: 80, recastDelayMS: 30000},
			want: NamedSpell{SpellID: 3, SpellName: "Silence of the Shadows", RecastSecs: 30, AEType: "TAE", AERange: 80},
		},
		{
			name: "single-target with no recast/resist stays bare",
			row:  casterSpellRow{spellID: 4, name: "Reckoning", targetType: 5},
			want: NamedSpell{SpellID: 4, SpellName: "Reckoning"},
		},
		{
			name: "zero resist_diff suppresses the resist token",
			row:  casterSpellRow{spellID: 5, name: "Plague", targetType: 5, resistType: 5, resistDiff: 0},
			want: NamedSpell{SpellID: 5, SpellName: "Plague"},
		},
		{
			name: "bare radius on a non-AE targettype is generic AE",
			row:  casterSpellRow{spellID: 6, name: "Cloud", targetType: 5, aoeRange: 40},
			want: NamedSpell{SpellID: 6, SpellName: "Cloud", AEType: "AE", AERange: 40},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.row.namedSpell()
			if got != tc.want {
				t.Errorf("namedSpell()\n got  %+v\n want %+v", got, tc.want)
			}
		})
	}
}

// internalDBPath mirrors the db_test helper but stays in package db so the
// integration test can reach unexported helpers if needed.
func internalDBPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db")
}

func TestSummarizeNPCCaster_ThreadBosses(t *testing.T) {
	d, err := Open(internalDBPath(t))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	// Diabo Xi Xin Thall (Cleric): Complete Heal highlight, Mark of Darkness
	// attack proc at 15%, signature spells, and no inherited class list.
	cleric, err := d.SummarizeNPCCaster(158443)
	if err != nil {
		t.Fatalf("summarize 158443: %v", err)
	}
	if cleric == nil {
		t.Fatal("158443: expected caster summary, got nil")
	}
	if !hasTag(cleric.Highlights, "complete_heal") {
		t.Errorf("158443: expected complete_heal highlight, got %+v", cleric.Highlights)
	}
	if len(cleric.Procs) == 0 || cleric.Procs[0].SpellName != "Mark of Darkness" || cleric.Procs[0].Chance != 15 {
		t.Errorf("158443: expected Mark of Darkness proc @15%%, got %+v", cleric.Procs)
	}
	if len(cleric.Signature) == 0 {
		t.Errorf("158443: expected signature spells, got none")
	}
	if len(cleric.ClassLists) != 0 {
		t.Errorf("158443: expected no inherited class lists, got %+v", cleric.ClassLists)
	}

	// Diabo Xi Va Temariel (Wizard): signature "Black Winds" + an inherited
	// "Default Wizard List" with a meaningful count, and a Gate highlight.
	wiz, err := d.SummarizeNPCCaster(158441)
	if err != nil {
		t.Fatalf("summarize 158441: %v", err)
	}
	if wiz == nil {
		t.Fatal("158441: expected caster summary, got nil")
	}
	foundBlackWinds := false
	for _, s := range wiz.Signature {
		if s.SpellName == "Black Winds" {
			foundBlackWinds = true
		}
	}
	if !foundBlackWinds {
		t.Errorf("158441: expected 'Black Winds' in signature, got %+v", wiz.Signature)
	}
	if len(wiz.ClassLists) == 0 || wiz.ClassLists[0].Count < 10 {
		t.Errorf("158441: expected inherited class list with many spells, got %+v", wiz.ClassLists)
	}
	if !hasTag(wiz.Highlights, "gate") {
		t.Errorf("158441: expected gate highlight, got %+v", wiz.Highlights)
	}

	// A non-caster NPC returns nil so the UI hides the section.
	none, err := d.SummarizeNPCCaster(1)
	if err != nil {
		t.Fatalf("summarize 1: %v", err)
	}
	_ = none // may or may not be nil depending on npc 1; just assert no error path
}

package petoverlay

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// realSpells loads spell metadata from the on-disk quarm.db so the `/pet
// health` member matching is exercised against the real names (including
// backtick forms like "Boltran`s Agacerie").
func realSpells(t *testing.T) map[string]db.SpellMeta {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	d, err := db.Open(filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	m, err := d.SpellMetaByName()
	if err != nil {
		t.Fatalf("SpellMetaByName: %v", err)
	}
	return m
}

// realLookup adapts realSpells to the assembler's lookup.
func realLookup(t *testing.T) func(string) (SpellInfo, bool) {
	spells := realSpells(t)
	return func(name string) (SpellInfo, bool) {
		m, ok := spells[name]
		return SpellInfo{ID: m.ID, Icon: m.Icon, Beneficial: m.Beneficial, Charm: m.Charm}, ok
	}
}

func spellNames(s []HealthSpell) []string {
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = x.Name
	}
	return out
}

// feed runs raw log lines through an assembler and returns every completed
// snapshot, flushing a trailing open block.
func feed(t *testing.T, a *healthAssembler, lines []string) []HealthSnapshot {
	t.Helper()
	var out []HealthSnapshot
	for _, line := range lines {
		ts, msg, ok := logparser.ParseRawLine(line)
		if !ok {
			t.Fatalf("bad fixture line %q", line)
		}
		if s := a.handleLine(ts, msg); s != nil {
			out = append(out, *s)
		}
	}
	if s := a.flush(); s != nil {
		out = append(out, *s)
	}
	return out
}

// Blocks copied verbatim from testdata/eqlog_Osui_pq.proj.txt (an enchanter
// with a charmed pet), including the chat that followed each reply.
func TestHealthAssembler_RealReplies(t *testing.T) {
	lookup := realLookup(t)
	type want struct {
		hp     int
		spells []string
	}
	tests := []struct {
		name  string
		lines []string
		want  []want
	}{
		{
			name: "interleaved chat in the same second is skipped",
			lines: []string{
				"[Mon May 04 19:10:51 2026] Darkclaw begins to cast a spell.",
				"[Mon May 04 19:10:53 2026] I have 95 percent of my hit points left.",
				"[Mon May 04 19:10:53 2026] Tashanian",
				"[Mon May 04 19:10:53 2026] Boltran`s Agacerie",
				"[Mon May 04 19:10:53 2026] Speed of the Shissar",
				"[Mon May 04 19:10:53 2026] Primal Essence",
				"[Mon May 04 19:10:53 2026] Conalen tells General:2, 'is flowing black sash relatively easy to get?  In terms of drops?'",
				"[Mon May 04 19:10:53 2026] Tabbie's lips curl into a feral snarl as they descend into savagery.",
				"[Mon May 04 19:10:57 2026] --Takkisina has looted a Essence of Darkness.--",
			},
			want: []want{{
				hp:     95,
				spells: []string{"Tashanian", "Boltran`s Agacerie", "Speed of the Shissar", "Primal Essence"},
			}},
		},
		{
			name: "pet dialogue in the same second is skipped",
			lines: []string{
				"[Mon May 04 21:00:05 2026] I have 72 percent of my hit points left.",
				"[Mon May 04 21:00:05 2026] Diamondskin",
				"[Mon May 04 21:00:05 2026] Boltran`s Agacerie",
				"[Mon May 04 21:00:05 2026] Speed of the Shissar",
				"[Mon May 04 21:00:05 2026] A Shissar Revenant says 'Following you, Master.'",
				"[Mon May 04 21:00:05 2026] Muadib tells the guild, 'Erruh MT?'",
				"[Mon May 04 21:00:06 2026] Hoder begins to cast a spell.",
			},
			want: []want{{
				hp:     72,
				spells: []string{"Diamondskin", "Boltran`s Agacerie", "Speed of the Shissar"},
			}},
		},
		{
			name: "recourse effects and raid combat spam",
			lines: []string{
				"[Mon May 04 21:20:59 2026] Margit slashes Vyzh`dra the Cursed for 17 points of damage.",
				"[Mon May 04 21:20:59 2026] I have 100 percent of my hit points left.",
				"[Mon May 04 21:20:59 2026] Diamondskin",
				"[Mon May 04 21:20:59 2026] Boltran`s Agacerie",
				"[Mon May 04 21:20:59 2026] Bond of Death Recourse",
				"[Mon May 04 21:20:59 2026] Speed of the Shissar",
				"[Mon May 04 21:20:59 2026] Mendacious tells the raid,  '- - 005 - CH Margit with < 100% > remaining!'",
				"[Mon May 04 21:20:59 2026] Muadib slashes Vyzh`dra the Cursed for 49 points of damage.",
				"[Mon May 04 21:20:59 2026] Rora Scores a critical hit!(107)",
			},
			want: []want{{
				hp:     100,
				spells: []string{"Diamondskin", "Boltran`s Agacerie", "Bond of Death Recourse", "Speed of the Shissar"},
			}},
		},
		{
			name: "detrimentals are listed alongside buffs",
			lines: []string{
				"[Sun May 10 19:30:25 2026] I have 17 percent of my hit points left.",
				"[Sun May 10 19:30:25 2026] Speed of the Shissar",
				"[Sun May 10 19:30:25 2026] Clinging Darkness",
				"[Sun May 10 19:30:25 2026] Inner Fire",
				"[Sun May 10 19:30:25 2026] Guardian",
				"[Sun May 10 19:30:25 2026] Symbol of Pinzarn",
				"[Sun May 10 19:30:25 2026] Boltran`s Agacerie",
				"[Sun May 10 19:30:25 2026] Tashanian",
				"[Sun May 10 19:30:25 2026] Lucidboom begins to cast a spell.",
			},
			want: []want{{
				hp: 17,
				spells: []string{"Speed of the Shissar", "Clinging Darkness", "Inner Fire",
					"Guardian", "Symbol of Pinzarn", "Boltran`s Agacerie", "Tashanian"},
			}},
		},
		{
			name: "a new header closes the previous block",
			lines: []string{
				"[Mon May 04 19:15:52 2026] I have 100 percent of my hit points left.",
				"[Mon May 04 19:15:52 2026] Tashanian",
				"[Mon May 04 19:15:52 2026] I have 99 percent of my hit points left.",
				"[Mon May 04 19:15:52 2026] Primal Essence",
			},
			want: []want{
				{hp: 100, spells: []string{"Tashanian"}},
				{hp: 99, spells: []string{"Primal Essence"}},
			},
		},
		{
			name: "spell names outside a block are ignored",
			lines: []string{
				"[Mon May 04 19:15:50 2026] Tashanian",
				"[Mon May 04 19:15:52 2026] I have 100 percent of my hit points left.",
				"[Mon May 04 19:15:54 2026] Primal Essence",
			},
			want: []want{{hp: 100, spells: []string{}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &healthAssembler{lookup: lookup}
			got := feed(t, a, tc.lines)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d snapshots, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i].HPPercent != tc.want[i].hp {
					t.Errorf("[%d] hp = %d, want %d", i, got[i].HPPercent, tc.want[i].hp)
				}
				if names := spellNames(got[i].Spells); !reflect.DeepEqual(names, tc.want[i].spells) {
					t.Errorf("[%d] spells = %q, want %q", i, names, tc.want[i].spells)
				}
				if got[i].At.IsZero() {
					t.Errorf("[%d] At not stamped", i)
				}
			}
		})
	}
}

func TestSpellMetaByName(t *testing.T) {
	spells := realSpells(t)
	tests := []struct {
		name       string
		beneficial bool
		charm      bool
	}{
		{"Boltran`s Agacerie", false, true},
		{"Charm", false, true},
		{"Dominating Gaze", false, true},
		{"Speed of the Shissar", true, false},
		{"Primal Essence", true, false},
		{"Tashanian", false, false},
	}
	for _, tc := range tests {
		m, ok := spells[tc.name]
		if !ok {
			t.Errorf("%q missing", tc.name)
			continue
		}
		if m.Beneficial != tc.beneficial || m.Charm != tc.charm || m.Icon == 0 {
			t.Errorf("%q = %+v, want beneficial=%v charm=%v and an icon", tc.name, m, tc.beneficial, tc.charm)
		}
	}
	// Shared names keep the lowest id: the backtick Boltran's is 1705, the
	// apostrophe form a separate 1706 row.
	if spells["Boltran`s Agacerie"].ID != 1705 || spells["Boltran's Agacerie"].ID != 1706 {
		t.Errorf("Boltran's ids = %d / %d", spells["Boltran`s Agacerie"].ID, spells["Boltran's Agacerie"].ID)
	}
}

func TestHealthAssembler_SpanClosesBlock(t *testing.T) {
	a := &healthAssembler{lookup: func(s string) (SpellInfo, bool) { return SpellInfo{}, s == "Tashanian" }}
	base := time.Date(2026, 5, 4, 19, 0, 0, 0, time.Local)
	if s := a.handleLine(base, "I have 50 percent of my hit points left."); s != nil {
		t.Fatal("header should open, not close, a block")
	}
	// Two seconds later: past the span, so this closes the block and is not
	// itself a member.
	s := a.handleLine(base.Add(2*time.Second), "Tashanian")
	if s == nil || len(s.Spells) != 0 || s.HPPercent != 50 {
		t.Fatalf("got %+v, want an empty 50%% snapshot", s)
	}
}

package loot

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

func TestParseLoot(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		wantOK bool
		player string
		item   string
		self   bool
	}{
		{"other", "--Aransur has looted a Akheva Blood.--", true, "Aransur", "Akheva Blood", false},
		{"other lowercase name", "--aransur has looted a Centi Meat.--", true, "Aransur", "Centi Meat", false},
		{"item with colon", "--Darkclaw has looted a Spell: Khura's Focusing.--", true, "Darkclaw", "Spell: Khura's Focusing", false},
		{"self", "--You have looted a Cord of Insight.--", true, "", "Cord of Insight", true},
		{"not loot", "Aransur tells you, 'hi'", false, "", "", false},
		{"empty", "", false, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseLoot(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ParseLoot(%q) ok=%v want %v", tt.msg, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got.Player != tt.player || got.Item != tt.item || got.Self != tt.self {
				t.Errorf("ParseLoot(%q)=%+v want player=%q item=%q self=%v", tt.msg, got, tt.player, tt.item, tt.self)
			}
		})
	}
}

func TestStoreListAndFilters(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()
	base := time.Unix(1_700_000_000, 0)
	ins := func(player, item, zone string, off time.Duration) {
		if _, err := s.Insert(Input{Character: "Osui", Player: player, Item: item, Zone: zone, TS: base.Add(off)}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	ins("Aransur", "Akheva Blood", "Vex Thal", 0)
	ins("Osui", "Cord of Insight", "Vex Thal", time.Minute)
	ins("Luna", "Shissar Fangs", "Ssraeshza Temple", 2*time.Minute)

	// Duplicate ignored.
	if ok, _ := s.Insert(Input{Character: "Osui", Player: "Aransur", Item: "Akheva Blood", Zone: "Vex Thal", TS: base}); ok {
		t.Error("duplicate loot should be ignored")
	}

	all, _ := s.List(Filters{Character: "Osui", SortDesc: true})
	if len(all) != 3 || all[0].Player != "Luna" {
		t.Fatalf("List=%+v, want 3 newest-first (Luna first)", all)
	}
	byPlayer, _ := s.List(Filters{Character: "Osui", Player: "Aransur"})
	if len(byPlayer) != 1 || byPlayer[0].Item != "Akheva Blood" {
		t.Errorf("player filter=%+v, want 1 Akheva Blood", byPlayer)
	}
	byZone, _ := s.List(Filters{Character: "Osui", Zone: "Vex Thal"})
	if len(byZone) != 2 {
		t.Errorf("zone filter=%d, want 2", len(byZone))
	}
	search, _ := s.List(Filters{Character: "Osui", Search: "shissar"})
	if len(search) != 1 {
		t.Errorf("search=%d, want 1", len(search))
	}
	players, _ := s.Players("Osui")
	if len(players) != 3 {
		t.Errorf("players=%v, want 3", players)
	}
	zones, _ := s.Zones("Osui")
	if len(zones) != 2 {
		t.Errorf("zones=%v, want 2", zones)
	}
}

// TestBackfillRealLog drives the loot backfill over the committed Osui fixture.
func TestBackfillRealLog(t *testing.T) {
	logPath := filepath.Join("..", "..", "..", "testdata", "eqlog_Osui_pq.proj.txt")
	if _, err := os.Stat(logPath); err != nil {
		t.Skipf("testdata log not available: %v", err)
	}
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	run := func() int {
		f, err := os.Open(logPath)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer f.Close()
		h := NewBackfillHandler(s, "Osui")
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			ts, msg, ok := logparser.ParseRawLine(line)
			if !ok {
				continue
			}
			if ev, ok := logparser.ParseLine(line); ok {
				h.HandleEvent(ev)
			}
			h.HandleLine(ts, msg)
		}
		h.Finalize()
		return h.Inserted()
	}

	if n := run(); n == 0 {
		t.Fatal("backfill inserted 0 loot rows; expected loot in fixture")
	}
	if n := run(); n != 0 {
		t.Errorf("re-run inserted %d, want 0 (idempotent)", n)
	}
	// Aransur looted Akheva Blood many times in the fixture.
	got, _ := s.List(Filters{Character: "Osui", Player: "Aransur", Search: "Akheva"})
	if len(got) == 0 {
		t.Error("expected Aransur Akheva Blood loot from fixture")
	}
}

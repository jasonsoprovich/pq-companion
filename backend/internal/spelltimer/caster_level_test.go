package spelltimer

import (
	"strings"
	"time"

	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// levelEngine builds a DB-backed engine for an active character of the given
// class index and level (0 = unknown level).
func levelEngine(t *testing.T, class, level int) (*Engine, *db.DB) {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "", "Tester", class }
	e := NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil, nil, nil, nil)
	e.SetLevelProvider(func() int { return level })
	return e, database
}

func TestRescaleFormulaDuration(t *testing.T) {
	const (
		enchanter = 13
		warrior   = 0
		tashanian = 1702 // formula 9, base 140: 130 ticks at 60, 140 at 65
	)
	tests := []struct {
		name  string
		class int
		level int
		in    float64
		want  float64
	}{
		{"own cast at 65 extends level-60 pack value", enchanter, 65, 780, 840},
		{"own cast at 58 shortens it", enchanter, 58, 780, 756},
		{"unknown level assumes cap", enchanter, 0, 780, 840},
		{"someone else's cast assumes cap", warrior, 50, 780, 840},
		{"non-formula value is a user override", enchanter, 65, 700, 700},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, database := levelEngine(t, tt.class, tt.level)
			spell, err := database.GetSpell(tashanian)
			if err != nil || spell == nil {
				t.Fatalf("spell %d: %v", tashanian, err)
			}
			if got := e.rescaleFormulaDuration(spell, tt.in, "", time.Now()); got != tt.want {
				t.Errorf("got %g, want %g", got, tt.want)
			}
		})
	}
}

// A pack song timer saved before the extra song-window tick (18 s) is
// recognised as formula-derived and picks the tick up.
func TestRescaleSongWindowLegacyValue(t *testing.T) {
	const warsongOfZek = 3374
	e, database := levelEngine(t, 13, 60)
	spell, err := database.GetSpell(warsongOfZek)
	if err != nil || spell == nil {
		t.Fatalf("spell %d: %v", warsongOfZek, err)
	}
	for _, in := range []float64{18, 24} {
		if got := e.rescaleFormulaDuration(spell, in, "", time.Now()); got != 24 {
			t.Errorf("rescale(%g) = %g, want 24", in, got)
		}
	}
}

func TestOtherCasterLevel(t *testing.T) {
	const (
		enchanter   = 13
		necromancer = 10
		cleric      = 1
		archLich    = 1416 // self-only, necro 60
		tashanian   = 1702 // single target, 1s cast, enchanter 57
	)
	who := map[string][3]int{ // name → class, level, known(1/0)
		"Necro":   {necromancer, 62, 1},
		"Enchone": {enchanter, 63, 1},
		"Enchtwo": {enchanter, 65, 1},
		"Healer":  {cleric, 61, 1},
	}
	info := func(name string) (int, int, bool) {
		for k, v := range who {
			if strings.EqualFold(k, name) {
				return v[0], v[1], v[2] == 1
			}
		}
		return 0, 0, false
	}
	land := time.Date(2026, 10, 14, 20, 0, 10, 0, time.UTC)
	tests := []struct {
		name    string
		spellID int
		target  string
		casts   map[string]time.Duration // caster → seconds before land
		want    int
		wantOK  bool
	}{
		{"self-only: target is the caster", archLich, "Necro", nil, 62, true},
		{"self-only: unknown target", archLich, "Stranger", nil, 0, false},
		{"single matching caster", tashanian, "a gnoll", map[string]time.Duration{"Enchone": time.Second}, 63, true},
		{"other-class casters filtered out", tashanian, "a gnoll",
			map[string]time.Duration{"Enchone": time.Second, "Healer": time.Second}, 63, true},
		{"two enchanters at different levels is ambiguous", tashanian, "a gnoll",
			map[string]time.Duration{"Enchone": time.Second, "Enchtwo": time.Second}, 0, false},
		{"unknown caster in window is ambiguous", tashanian, "a gnoll",
			map[string]time.Duration{"Enchone": time.Second, "Stranger": time.Second}, 0, false},
		{"cast start outside the window ignored", tashanian, "a gnoll",
			map[string]time.Duration{"Enchone": time.Second, "Enchtwo": 12 * time.Second}, 63, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, database := levelEngine(t, enchanter, 60)
			e.SetPlayerInfoProvider(info)
			for name, before := range tt.casts {
				e.HandleLine(land.Add(-before), name+" begins to cast a spell.")
			}
			spell, err := database.GetSpell(tt.spellID)
			if err != nil || spell == nil {
				t.Fatalf("spell %d: %v", tt.spellID, err)
			}
			got, ok := e.otherCasterLevel(spell, tt.target, land)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("got (%d, %v), want (%d, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

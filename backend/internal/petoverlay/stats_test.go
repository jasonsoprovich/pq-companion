package petoverlay

import (
	"reflect"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestBuildStats(t *testing.T) {
	tests := []struct {
		name      string
		abilities string
		summary   *db.NPCCasterSummary
		level     int
		check     func(t *testing.T, s *Stats)
	}{
		{
			name:  "unknown level spans the spawn range",
			level: 0,
			check: func(t *testing.T, s *Stats) {
				// 10000 HP @50 → round(10000*54/50) @54; max hit +2/level.
				if s.LevelMin != 50 || s.LevelMax != 54 || s.Level != 0 {
					t.Errorf("levels = %d..%d (%d)", s.LevelMin, s.LevelMax, s.Level)
				}
				if s.HPMin != 10000 || s.HPMax != 10800 {
					t.Errorf("hp = %d..%d, want 10000..10800", s.HPMin, s.HPMax)
				}
				if s.MaxHitMin != 277 || s.MaxHitMax != 285 || s.MinHit != 85 {
					t.Errorf("hits = %d / %d..%d", s.MinHit, s.MaxHitMin, s.MaxHitMax)
				}
				// (85+277)/2 / 2.4s = 75.4
				if s.DPSMin != 75.4 {
					t.Errorf("dps min = %v, want 75.4", s.DPSMin)
				}
			},
		},
		{
			name:      "immunities, summon and offense flags",
			abilities: "1,1^5,1^13,1^16,1^21,1^50,1^51,1",
			level:     52,
			check: func(t *testing.T, s *Stats) {
				want := Immunities{Mez: true, SnareRoot: true, ReverseSlow: true, Haste: true}
				if s.Immunities != want {
					t.Errorf("immunities = %+v, want %+v", s.Immunities, want)
				}
				if !s.Summons {
					t.Error("summon (1) not flagged")
				}
				if !reflect.DeepEqual(s.Offense, []string{"Flurry"}) {
					t.Errorf("offense = %q, want [Flurry]", s.Offense)
				}
			},
		},
		{
			name:    "gate comes from the caster summary",
			summary: &db.NPCCasterSummary{Highlights: []db.CasterHighlight{{Tag: "gate"}}},
			check: func(t *testing.T, s *Stats) {
				if !s.Gates {
					t.Error("gate highlight not flagged")
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := BuildStats(revenant, db.ParseSpecialAbilities(tc.abilities), tc.summary, tc.level)
			if s == nil {
				t.Fatal("nil stats")
			}
			tc.check(t, s)
		})
	}
	if BuildStats(nil, nil, nil, 0) != nil {
		t.Error("nil npc should give nil stats")
	}
}

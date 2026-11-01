package petoverlay

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// Magician L33 elementals (quarm.db): same level/race/class, texture differs.
var elementals = []db.PetNPC{
	{NPCID: 560, Level: 33, Race: 75, Class: 1, Texture: 3}, // air
	{NPCID: 575, Level: 33, Race: 75, Class: 1, Texture: 0}, // earth
	{NPCID: 590, Level: 33, Race: 75, Class: 1, Texture: 1}, // fire
	{NPCID: 607, Level: 33, Race: 75, Class: 1, Texture: 2}, // water
}

func summonHarness() *harness {
	h := newHarness()
	h.tr.cfg.Summons = PetSummons{
		BySpell: map[string]int{"Lesser Conjuration: Earth": 575, "Lesser Conjuration: Water": 607},
		Rows:    elementals,
	}
	for _, r := range elementals {
		h.res.byNPC[r.NPCID] = overlay.Resolution{NPC: &db.NPC{
			ID: r.NPCID, Level: 33, HP: 1450, MinDmg: 8, MaxDmg: 40, AttackDelay: 28, Class: 1, Race: 75,
		}}
	}
	h.tr.SetPipeConnected(true)
	return h
}

func (h *harness) cast(spell string) {
	h.tr.Handle(logparser.LogEvent{
		Type:      logparser.EventSpellCast,
		Timestamp: h.now,
		Data:      logparser.SpellCastData{SpellName: spell},
	})
}

func TestSummon_CastBindsNewPet(t *testing.T) {
	h := summonHarness()
	h.cast("Lesser Conjuration: Earth")
	h.advance(8 * time.Second) // cast time + summon
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(9))

	st := h.state()
	if st.Kind != KindSummoned {
		t.Errorf("kind = %q, want summoned straight away", st.Kind)
	}
	if st.Stats == nil || st.Stats.NPCID != 575 || st.Stats.LevelMin != 33 {
		t.Fatalf("stats = %+v, want the earth elemental row", st.Stats)
	}
}

func TestSummon_NoBindWhenCastStaleOrInterrupted(t *testing.T) {
	tests := []struct {
		name  string
		setup func(h *harness)
	}{
		{"stale cast", func(h *harness) {
			h.cast("Lesser Conjuration: Earth")
			h.advance(summonBindWindow + time.Second)
		}},
		{"interrupted cast", func(h *harness) {
			h.cast("Lesser Conjuration: Earth")
			h.tr.Handle(logparser.LogEvent{Type: logparser.EventSpellInterrupt})
			h.advance(time.Second)
		}},
		{"not a pet spell", func(h *harness) {
			h.cast("Speed of the Shissar")
			h.advance(time.Second)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := summonHarness()
			tc.setup(h)
			h.tr.SetPipePetName("Gobeker")
			h.tr.SetPipePetID(ip(9))
			if st := h.state(); st.Stats != nil || st.Kind != KindUnknown {
				t.Fatalf("bound anyway: kind %q stats %+v", st.Kind, st.Stats)
			}
		})
	}
}

func TestSummon_CastConsumedByOnePet(t *testing.T) {
	h := summonHarness()
	h.cast("Lesser Conjuration: Earth")
	h.advance(5 * time.Second)
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(9))
	// The elemental dies and an enchanter-style charm follows within the
	// window: the new pet must not inherit the elemental's row.
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))
	if st := h.state(); st.Stats != nil && st.Stats.NPCID == 575 {
		t.Fatal("second pet inherited the summon binding")
	}
}

func TestSummon_CharmEvidenceDropsSummonStats(t *testing.T) {
	h := summonHarness()
	h.cast("Lesser Conjuration: Earth")
	h.advance(5 * time.Second)
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))
	if h.state().Stats == nil {
		t.Fatal("setup: no summon stats")
	}
	h.charm, h.charmTgt = "Boltran`s Agacerie", "a Shissar Revenant"
	h.tr.SetPipePetID(ip(812))
	st := h.state()
	if st.Kind != KindCharm {
		t.Fatalf("kind = %q, want charm", st.Kind)
	}
	if st.Stats != nil && st.Stats.NPCID == 575 {
		t.Fatal("kept the elemental's stats on a charmed pet")
	}
}

func TestSummon_TargetedPetMatchesAttributes(t *testing.T) {
	tests := []struct {
		name         string
		desc         zealpipe.TargetDescriptors
		wantNPC      int
		wantVariants int
	}{
		{"texture picks water", zealpipe.TargetDescriptors{Level: 33, Race: 75, Class: 1, HasAppearance: true, Texture: 2}, 607, 0},
		{"no appearance: ambiguous", zealpipe.TargetDescriptors{Level: 33, Race: 75, Class: 1}, 560, 4},
		{"no match", zealpipe.TargetDescriptors{Level: 34, Race: 75, Class: 1}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := summonHarness()
			h.tr.SetPipePetName("Gobeker")
			h.tr.SetPipePetID(ip(9))
			// A different target is ignored.
			d := tc.desc
			h.tr.SetPipeTargetSnapshot(ip(44), &d)
			if h.state().Stats != nil {
				t.Fatal("matched a target that isn't the pet")
			}
			h.tr.SetPipeTargetSnapshot(ip(9), &d)
			st := h.state()
			if tc.wantNPC == 0 {
				if st.Stats != nil {
					t.Fatalf("stats = %+v, want none", st.Stats)
				}
				return
			}
			if st.Stats == nil || st.Stats.NPCID != tc.wantNPC || st.Stats.Level != 33 || st.Variants != tc.wantVariants {
				t.Fatalf("stats = %+v variants %d, want npc %d variants %d", st.Stats, st.Variants, tc.wantNPC, tc.wantVariants)
			}
			if st.Kind != KindSummoned {
				t.Errorf("kind = %q, want summoned", st.Kind)
			}
		})
	}
}

func TestPetSummons_RealDB(t *testing.T) {
	bySpell, rows, err := openRealDB(t).PetSummons()
	if err != nil {
		t.Fatalf("PetSummons: %v", err)
	}
	for spell, want := range map[string]int{
		"Pendril's Animation":       500, // enchanter animation (SPA 33)
		"Lesser Conjuration: Earth": 575, // magician elemental (SPA 33)
		"Lesser Conjuration: Water": 607,
		"Animate Dead":              619, // necromancer (SPA 71)
		"Spirit of Sharik":          515, // beastlord warder (SPA 106)
	} {
		if got := bySpell[spell]; got != want {
			t.Errorf("%q → %d, want %d", spell, got, want)
		}
	}
	for _, spell := range []string{"Eye of Zomm", "Minor Familiar", "Wake the Dead"} {
		if _, ok := bySpell[spell]; ok {
			t.Errorf("%q should not count as the player's pet", spell)
		}
	}
	textures := map[int]int{}
	for _, r := range rows {
		if r.NPCID == 575 || r.NPCID == 607 {
			textures[r.NPCID] = r.Texture
		}
	}
	if textures[575] != 0 || textures[607] != 2 {
		t.Errorf("elemental textures = %v, want earth 0 / water 2", textures)
	}
}

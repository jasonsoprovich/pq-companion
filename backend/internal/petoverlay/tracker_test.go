package petoverlay

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
)

// fakeResolver stands in for the NPC overlay: byID mimics its spawn-id
// resolution cache, byName its zone lookup.
type fakeResolver struct {
	byID      map[int]overlay.Resolution
	byName    map[string]overlay.Resolution
	nameCalls int
}

func (f *fakeResolver) ResolvedForID(id int) (overlay.Resolution, bool) {
	r, ok := f.byID[id]
	return r, ok
}

func (f *fakeResolver) ResolveByName(name string) overlay.Resolution {
	f.nameCalls++
	return f.byName[name]
}

// revenant mirrors quarm.db npc_types 162197 (A_Shissar_Revenant, 50-54).
var revenant = &db.NPC{
	ID: 162197, Name: "A_Shissar_Revenant", Level: 50, MaxLevel: 54,
	HP: 10000, MinDmg: 85, MaxDmg: 277, AttackDelay: 24, MR: 50, Class: 11, Race: 217,
}

type harness struct {
	tr       *Tracker
	res      *fakeResolver
	now      time.Time
	charm    string // active charm timer spell ("" = none)
	sent     []State
	charmTgt string
}

func newHarness() *harness {
	h := &harness{
		res: &fakeResolver{byID: map[int]overlay.Resolution{}, byName: map[string]overlay.Resolution{}},
		now: time.Date(2026, 11, 1, 20, 0, 0, 0, time.Local),
	}
	h.tr = New(Config{
		Resolver:   h.res,
		Broadcast:  func(s State) { h.sent = append(h.sent, s) },
		ActiveChar: func() string { return "Osui" },
		CharmTimer: func() (string, string, bool) {
			return h.charm, h.charmTgt, h.charm != ""
		},
		IsSpell: func(s string) bool {
			return s == "Boltran`s Agacerie" || s == "Speed of the Shissar"
		},
		IsCharmSpell: func(s string) bool { return s == "Boltran`s Agacerie" },
	})
	h.tr.now = func() time.Time { return h.now }
	return h
}

func (h *harness) advance(d time.Duration) { h.now = h.now.Add(d) }

func ip(v int) *int { return &v }

func (h *harness) state() State { return h.tr.GetState() }

func TestTracker_ZealCharmFlow(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	// The pet was targeted while charming: its row is cached by spawn id
	// with the live level from the Zeal descriptors.
	h.res.byID[812] = overlay.Resolution{NPC: revenant, Level: 52}
	h.charm, h.charmTgt = "Boltran`s Agacerie", "a Shissar Revenant"

	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))
	h.tr.SetPipePetHPPercent("88")

	st := h.state()
	if st.Mode != ModePet || st.Kind != KindCharm || st.CharmSpell != "Boltran`s Agacerie" {
		t.Fatalf("mode/kind/charm = %v/%v/%q", st.Mode, st.Kind, st.CharmSpell)
	}
	if st.Name != "a Shissar Revenant" || st.PetID == nil || *st.PetID != 812 {
		t.Fatalf("name/id = %q/%v", st.Name, st.PetID)
	}
	if st.Stats == nil || st.Stats.Level != 52 || st.Stats.LevelMin != 52 || st.Stats.LevelMax != 52 {
		t.Fatalf("stats level = %+v", st.Stats)
	}
	// HP scales linearly from the base row: round(10000*52/50).
	if st.Stats.HPMin != 10400 || st.Stats.HPMax != 10400 {
		t.Errorf("hp = %d..%d, want 10400", st.Stats.HPMin, st.Stats.HPMax)
	}
	if st.HPPercent == nil || *st.HPPercent != 88 || st.HPSource != HPSourceZeal {
		t.Errorf("hp%% = %v source %q", st.HPPercent, st.HPSource)
	}
	if st.ZealLimited {
		t.Error("ZealLimited with the pipe connected")
	}
	if h.res.nameCalls != 0 {
		t.Errorf("by-name fallback ran %d times despite a spawn-id hit", h.res.nameCalls)
	}
}

func TestTracker_PetLossDebounceAndSwap(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.charm = "Boltran`s Agacerie"
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))

	h.tr.SetPipePetID(nil)
	h.tr.SetPipePetID(nil)
	if h.state().Mode != ModePet {
		t.Fatal("two dropped frames ended the pet")
	}
	h.tr.SetPipePetID(nil)
	st := h.state()
	if st.Mode != ModeBroken || st.Name != "a Shissar Revenant" || st.SwapAt == nil {
		t.Fatalf("after loss: mode %v name %q swapAt %v", st.Mode, st.Name, st.SwapAt)
	}
	if st.HPPercent != nil || st.PetID != nil {
		t.Error("broken pet kept live HP / id")
	}

	h.advance(DefaultSwapDelay - time.Second)
	h.tr.expireBroken()
	if h.state().Mode != ModeBroken {
		t.Fatal("swapped before the delay elapsed")
	}
	h.advance(time.Second)
	h.tr.expireBroken()
	if st := h.state(); st.Mode != ModeNone || st.Name != "" {
		t.Fatalf("after swap: mode %v name %q", st.Mode, st.Name)
	}
}

func TestTracker_LogCharmBreakIgnoresLingeringID(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.charm = "Boltran`s Agacerie"
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))

	h.charm = ""
	h.tr.Handle(logparser.LogEvent{Type: logparser.EventCharmBroken})
	if h.state().Mode != ModeBroken {
		t.Fatal("charm break line did not break the pet")
	}
	// Zeal still lists the old pet (and its name) for a few frames.
	h.tr.SetPipePetID(ip(812))
	h.tr.SetPipePetName("a Shissar Revenant")
	if st := h.state(); st.Mode != ModeBroken {
		t.Fatalf("lingering pet_id re-opened the pet: mode %v", st.Mode)
	}

	// A re-charm of a different spawn starts a new pet straight away.
	h.charm = "Boltran`s Agacerie"
	h.tr.SetPipePetName("a Shissar Arbiter")
	h.tr.SetPipePetID(ip(901))
	st := h.state()
	if st.Mode != ModePet || st.Name != "a Shissar Arbiter" || st.PetID == nil || *st.PetID != 901 {
		t.Fatalf("re-charm: mode %v name %q id %v", st.Mode, st.Name, st.PetID)
	}
	if st.BrokeAt != nil || st.SwapAt != nil {
		t.Error("new pet kept the broken pet's timestamps")
	}
}

func TestTracker_NewSpawnIDUsesLabelName(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(10))
	// Label 68 for the next pet lands a beat before its new pet_id.
	h.tr.SetPipePetName("Kabann")
	h.tr.SetPipePetID(ip(11))
	if st := h.state(); st.Name != "Kabann" || *st.PetID != 11 {
		t.Fatalf("name/id = %q/%v, want Kabann/11", st.Name, *st.PetID)
	}
}

func TestTracker_KindSettlesToSummoned(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(10))
	if k := h.state().Kind; k != KindUnknown {
		t.Fatalf("kind right after appearing = %q, want unknown", k)
	}
	h.advance(kindSettle)
	h.tr.SetPipePetID(ip(10))
	if k := h.state().Kind; k != KindSummoned {
		t.Fatalf("kind after settle = %q, want summoned", k)
	}
}

func TestTracker_PetHealthWithoutZeal(t *testing.T) {
	h := newHarness()
	ts := h.now
	h.tr.HandleLine(ts, "I have 64 percent of my hit points left.")
	h.tr.HandleLine(ts, "Boltran`s Agacerie")
	h.tr.HandleLine(ts, "Speed of the Shissar")
	h.tr.HandleLine(ts.Add(3*time.Second), "You feel quickened.")

	st := h.state()
	if st.Mode != ModePet || st.Kind != KindCharm {
		t.Fatalf("mode/kind = %v/%v, want pet/charm", st.Mode, st.Kind)
	}
	if st.HPPercent == nil || *st.HPPercent != 64 || st.HPSource != HPSourcePetHealth {
		t.Fatalf("hp = %v source %q", st.HPPercent, st.HPSource)
	}
	if st.Health == nil || len(st.Health.Spells) != 2 {
		t.Fatalf("health = %+v", st.Health)
	}
	if !st.ZealLimited {
		t.Error("ZealLimited false with no pipe")
	}
}

func TestTracker_FreshZealHPOutranksPetHealth(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(ip(812))
	h.tr.SetPipePetHPPercent("90")

	h.advance(500 * time.Millisecond)
	h.tr.HandleLine(h.now, "I have 64 percent of my hit points left.")
	h.tr.flushHealth()
	if st := h.state(); *st.HPPercent != 90 || st.HPSource != HPSourceZeal {
		t.Fatalf("hp = %d (%s), want the fresh Zeal 90", *st.HPPercent, st.HPSource)
	}

	h.advance(zealHPFresh + time.Second)
	h.tr.HandleLine(h.now, "I have 61 percent of my hit points left.")
	h.tr.flushHealth()
	if st := h.state(); *st.HPPercent != 61 || st.HPSource != HPSourcePetHealth {
		t.Fatalf("hp = %d (%s), want the newer /pet health 61", *st.HPPercent, st.HPSource)
	}
}

func TestTracker_ResolutionUpgrade(t *testing.T) {
	h := newHarness()
	other := *revenant
	other.ID = 162490
	// Logged pet with no id: the by-name fallback returns an ambiguous pick.
	h.res.byName["a Shissar Revenant"] = overlay.Resolution{NPC: &other, Variants: 2}
	h.tr.Handle(logparser.LogEvent{
		Type: logparser.EventCharmedPet,
		Data: logparser.CharmedPetData{Pet: "a Shissar Revenant"},
	})
	st := h.state()
	if st.Stats == nil || st.Stats.NPCID != 162490 || st.Variants != 2 {
		t.Fatalf("fallback stats = %+v variants %d", st.Stats, st.Variants)
	}
	if st.Stats.LevelMin != 50 || st.Stats.LevelMax != 54 || st.Stats.Level != 0 {
		t.Errorf("unknown level should span the row: %+v", st.Stats)
	}

	// The pipe connects and the player targets the pet: the spawn-id cache
	// has a unique pick with an exact level, which replaces the guess.
	h.tr.SetPipeConnected(true)
	h.res.byID[812] = overlay.Resolution{NPC: revenant, Level: 53}
	h.tr.SetPipePetID(ip(812))
	st = h.state()
	if st.Stats == nil || st.Stats.NPCID != 162197 || st.Stats.Level != 53 || st.Variants != 0 {
		t.Fatalf("upgraded stats = %+v variants %d", st.Stats, st.Variants)
	}
	if h.res.nameCalls != 1 {
		t.Errorf("by-name fallback ran %d times, want once", h.res.nameCalls)
	}
}

func TestTracker_NameFallbackWaitsForPetID(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.res.byName["a Shissar Revenant"] = overlay.Resolution{NPC: revenant}
	h.tr.SetPipePetName("a Shissar Revenant")
	h.tr.SetPipePetID(nil) // a Zeal build that never reports pet_id
	if h.res.nameCalls != 0 || h.state().Stats != nil {
		t.Fatal("by-name fallback ran before pet_id had a chance")
	}
	h.advance(nameFallbackDelay)
	h.tr.SetPipePetID(nil)
	if h.res.nameCalls != 1 || h.state().Stats == nil {
		t.Fatalf("fallback calls %d, stats %v; want it to run after the delay",
			h.res.nameCalls, h.state().Stats)
	}
}

func TestTracker_ZoneWithoutPipeDropsCharm(t *testing.T) {
	h := newHarness()
	h.tr.Handle(logparser.LogEvent{
		Type: logparser.EventCharmedPet,
		Data: logparser.CharmedPetData{Pet: "a Shissar Revenant"},
	})
	h.tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The Grey"},
	})
	if m := h.state().Mode; m != ModeNone {
		t.Fatalf("charm survived a zone with no pipe: mode %v", m)
	}
}

func TestTracker_PetOwnerOnlyForActiveChar(t *testing.T) {
	h := newHarness()
	h.tr.Handle(logparser.LogEvent{
		Type: logparser.EventPetOwner,
		Data: logparser.PetOwnerData{Pet: "Kebartik", Owner: "Kildrey"},
	})
	if m := h.state().Mode; m != ModeNone {
		t.Fatalf("another player's pet bound: mode %v", m)
	}
	h.tr.Handle(logparser.LogEvent{
		Type: logparser.EventPetOwner,
		Data: logparser.PetOwnerData{Pet: "Gobeker", Owner: "osui"},
	})
	if st := h.state(); st.Mode != ModePet || st.Name != "Gobeker" {
		t.Fatalf("own pet: mode %v name %q", st.Mode, st.Name)
	}
}

func TestTracker_HPBroadcastRateLimited(t *testing.T) {
	h := newHarness()
	h.tr.SetPipeConnected(true)
	h.tr.SetPipePetName("Gobeker")
	h.tr.SetPipePetID(ip(10))
	before := len(h.sent)
	h.tr.SetPipePetHPPercent("80")
	h.tr.SetPipePetHPPercent("79") // within the interval: held back
	h.tr.SetPipePetHPPercent("79")
	if n := len(h.sent) - before; n != 1 {
		t.Fatalf("broadcasts within interval = %d, want 1", n)
	}
	h.advance(hpBroadcastInterval)
	h.tr.SetPipePetHPPercent("79") // the settled value goes out once allowed
	if n := len(h.sent) - before; n != 2 || *h.sent[len(h.sent)-1].HPPercent != 79 {
		t.Fatalf("broadcasts = %d, last hp %v", n, h.sent[len(h.sent)-1].HPPercent)
	}
}

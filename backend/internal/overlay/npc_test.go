package overlay

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestTracker returns an NPCTracker with a real (unstarted) hub and no DB.
// The nil-DB guard in lookupNPC means NPC data always comes back nil, which
// is fine for behavioural tests that only care about target/zone state.
func newTestTracker() *NPCTracker {
	hub := ws.NewHub()
	return NewNPCTracker(hub, nil)
}

// newRealDBTracker returns an NPCTracker backed by the on-disk quarm.db so
// variant disambiguation tests can exercise the full lookup → filter path.
func newRealDBTracker(t *testing.T) *NPCTracker {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dbPath := filepath.Join(filepath.Dir(file), "..", "..", "data", "quarm.db")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	hub := ws.NewHub()
	return NewNPCTracker(hub, d)
}

func TestNPCTracker_ZoneEventClearsTarget(t *testing.T) {
	tr := newTestTracker()

	// Simulate a combat hit so the tracker has an active target.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "You", Skill: "slash", Target: "a gnoll", Damage: 50},
	})
	if !tr.GetState().HasTarget {
		t.Fatal("expected HasTarget=true after combat hit")
	}

	// A zone-change must clear the target and record the zone name.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The North Karana"},
	})

	st := tr.GetState()
	if st.HasTarget {
		t.Errorf("HasTarget = true after zone change, want false")
	}
	if st.CurrentZone != "The North Karana" {
		t.Errorf("CurrentZone = %q, want %q", st.CurrentZone, "The North Karana")
	}
}

func TestNPCTracker_ZoneNameNotSetAsTarget(t *testing.T) {
	tr := newTestTracker()

	// Enter a zone.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The North Karana"},
	})

	// Attempt to set the target to the zone name (simulates a false-positive
	// from the log parser where a zone-entry line is misidentified as a
	// consider event).  The tracker must silently reject this.
	tr.setTarget("The North Karana")

	st := tr.GetState()
	if st.HasTarget {
		t.Errorf("HasTarget = true after setting target to zone name, want false")
	}
}

func TestNPCTracker_ConsiderEventSetsTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "Crushbone"},
	})

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "an orc centurion"},
	})

	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false after consider event, want true")
	}
	if st.TargetName != "an orc centurion" {
		t.Errorf("TargetName = %q, want %q", st.TargetName, "an orc centurion")
	}
}

func TestNPCTracker_KillClearsMatchingTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "You", Target: "a gnoll"},
	})

	if tr.GetState().HasTarget {
		t.Error("HasTarget = true after killing target, want false")
	}
}

func TestNPCTracker_KillDoesNotClearUnrelatedTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	// A group member kills a different mob — our target should remain.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Guildmate", Target: "a kobold"},
	})

	st := tr.GetState()
	if !st.HasTarget {
		t.Error("HasTarget = false after unrelated kill, want true")
	}
	if st.TargetName != "a gnoll" {
		t.Errorf("TargetName = %q, want %q", st.TargetName, "a gnoll")
	}
}

func TestStripCorpseSuffix(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantIs   bool
	}{
		{"a gnoll", "a gnoll", false},
		{"a gnoll's corpse", "a gnoll", true},
		{"A Gnoll's Corpse", "A Gnoll", true},
		{"a_gnoll's_corpse", "a_gnoll", true},
		{"Diabo`Teka`Temariel's corpse", "Diabo`Teka`Temariel", true},
		{"corpse", "corpse", false},
	}
	for _, tc := range cases {
		gotName, gotIs := stripCorpseSuffix(tc.in)
		if gotName != tc.wantName || gotIs != tc.wantIs {
			t.Errorf("stripCorpseSuffix(%q) = (%q, %t), want (%q, %t)",
				tc.in, gotName, gotIs, tc.wantName, tc.wantIs)
		}
	}
}

func TestNPCTracker_PipeCorpseTargetFlags(t *testing.T) {
	tr := newTestTracker()
	tr.SetPipeTarget("a gnoll's corpse")
	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
	if st.TargetName != "a gnoll's corpse" {
		t.Errorf("TargetName = %q, want %q", st.TargetName, "a gnoll's corpse")
	}
	if !st.IsCorpse {
		t.Error("IsCorpse = false, want true")
	}
	if st.HPPercent != 0 {
		t.Errorf("HPPercent = %d, want 0", st.HPPercent)
	}
}

func TestNPCTracker_DeathClearsTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventConsidered,
		Data: logparser.ConsideredData{TargetName: "a gnoll"},
	})
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventDeath,
		Data: logparser.DeathData{SlainBy: "a gnoll"},
	})

	if tr.GetState().HasTarget {
		t.Error("HasTarget = true after player death, want false")
	}
}

// ─── Variant disambiguation ───────────────────────────────────────────────────

// nearestSpawnDistance returns +Inf for an empty spawn list — the filter
// drops zero-spawn variants when any sibling has spawns, but keeps them as
// the variant set when nothing has spawns.
func TestNearestSpawnDistance_NoSpawns(t *testing.T) {
	d := nearestSpawnDistance(nil, 0, 0)
	if !math.IsInf(d, 1) {
		t.Errorf("nearestSpawnDistance(nil) = %v, want +Inf", d)
	}
}

func TestNearestSpawnDistance_PicksClosest(t *testing.T) {
	spawns := []db.SpawnPoint{
		{X: 100, Y: 100},
		{X: 0, Y: 30}, // closest to player at (0,0)
		{X: -50, Y: -50},
	}
	got := nearestSpawnDistance(spawns, 0, 0)
	if math.Abs(got-30) > 1e-9 {
		t.Errorf("nearestSpawnDistance = %v, want 30", got)
	}
}

// Variants with distinct spawn points far apart resolve to a single winner
// when the player stands near one of them (Kaas Thox pattern).
func TestFilterVariantsByPlayerPosition_DistinctSpawnsPickOne(t *testing.T) {
	north := db.NPCVariant{
		NPC:         db.NPC{ID: 1},
		SpawnPoints: []db.SpawnPoint{{X: 141, Y: 318}},
	}
	south := db.NPCVariant{
		NPC:         db.NPC{ID: 2},
		SpawnPoints: []db.SpawnPoint{{X: 141, Y: -321}},
	}
	// Player at the north spawn — south is 639 yards away, far past the tie
	// tolerance, so only north survives.
	got := filterVariantsByPlayerPosition([]db.NPCVariant{north, south}, 141, 318)
	if len(got) != 1 {
		t.Fatalf("got %d variants, want 1", len(got))
	}
	if got[0].NPC.ID != 1 {
		t.Errorf("picked id %d, want 1", got[0].NPC.ID)
	}
}

// Variants that share spawn points (Quarm RNG-pair, like ssratemple's
// shissar revenant necro/SK) cannot be distinguished by position, so both
// survive the filter and the caller surfaces them as a variant set.
func TestFilterVariantsByPlayerPosition_SharedSpawnsKeepBoth(t *testing.T) {
	shared := []db.SpawnPoint{
		{X: 540, Y: -380, SpawngroupID: 162197},
		{X: 580, Y: -400, SpawngroupID: 162197},
	}
	necro := db.NPCVariant{NPC: db.NPC{ID: 162197, Class: 11}, SpawnPoints: shared}
	sk := db.NPCVariant{NPC: db.NPC{ID: 162490, Class: 5}, SpawnPoints: shared}
	got := filterVariantsByPlayerPosition([]db.NPCVariant{necro, sk}, 550, -390)
	if len(got) != 2 {
		t.Fatalf("got %d variants, want 2 (shared spawns must keep both)", len(got))
	}
}

// Same-name boss rows in Quarm (a raid boss plus low-HP siblings that all
// spawn in one zone) must headline the real boss. sortVariantsByStrength
// orders raid_target first, then HP, then id — so A Dracoliche's 175k raid row
// wins over its 32k siblings even though the 32k normal row has the lowest id.
func TestSortVariantsByStrength_RaidBossWins(t *testing.T) {
	// Mirrors quarm.db a_dracoliche rows that spawn in fearplane.
	in := []db.NPCVariant{
		{NPC: db.NPC{ID: 72006, HP: 32000, RaidTarget: 0}},  // normal, lowest id
		{NPC: db.NPC{ID: 72090, HP: 175000, RaidTarget: 1}}, // the raid boss
		{NPC: db.NPC{ID: 72590, HP: 32000, RaidTarget: 1}},  // low-HP raid sibling
	}
	sortVariantsByStrength(in)
	if in[0].NPC.ID != 72090 {
		t.Errorf("primary id = %d, want 72090 (175k raid boss)", in[0].NPC.ID)
	}
	// raid_target rows ahead of the normal one; HP breaks the raid tie.
	wantOrder := []int{72090, 72590, 72006}
	for i, want := range wantOrder {
		if in[i].NPC.ID != want {
			t.Errorf("position %d = id %d, want %d", i, in[i].NPC.ID, want)
		}
	}
}

// HP alone resolves ties when raid_target matches — Cazic Thule's 450k raid row
// outranks its 32k raid sibling regardless of id ordering.
func TestSortVariantsByStrength_HighestHPAmongRaid(t *testing.T) {
	in := []db.NPCVariant{
		{NPC: db.NPC{ID: 72500, HP: 32000, RaidTarget: 1}},
		{NPC: db.NPC{ID: 72003, HP: 450000, RaidTarget: 1}},
	}
	sortVariantsByStrength(in)
	if in[0].NPC.ID != 72003 {
		t.Errorf("primary id = %d, want 72003 (450k)", in[0].NPC.ID)
	}
}

// A variant with no spawn points is dropped when siblings have them — the
// no-spawn entry can't be position-matched and at least one sibling can.
func TestFilterVariantsByPlayerPosition_DropsNoSpawnWhenOthersExist(t *testing.T) {
	noSpawns := db.NPCVariant{NPC: db.NPC{ID: 1}}
	withSpawns := db.NPCVariant{NPC: db.NPC{ID: 2}, SpawnPoints: []db.SpawnPoint{{X: 10, Y: 10}}}
	got := filterVariantsByPlayerPosition([]db.NPCVariant{noSpawns, withSpawns}, 10, 10)
	if len(got) != 1 || got[0].NPC.ID != 2 {
		t.Errorf("filter result = %+v, want only id=2", got)
	}
}

// Integration: targeting "a Shissar Revenant" in ssratemple with no player
// position yet should yield a variant set of 2 (necro + SK). The shissar
// in The Grey is a third row globally, but it must NOT appear because the
// zone filter restricts to ssratemple.
func TestNPCTracker_VariantSetForSharedSpawngroupRNG(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(162 /* ssratemple zoneidnumber */, 0, 0, 0)
	// Player position is unknown — only zone is set — so all in-zone variants
	// come back as the set.
	tr.SetPipeTarget("A Shissar Revenant")
	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
	if len(st.Variants) != 2 {
		t.Fatalf("Variants len = %d, want 2 (necro + SK in ssratemple)", len(st.Variants))
	}
	classes := map[int]bool{}
	for _, v := range st.Variants {
		classes[v.NPC.Class] = true
	}
	if !classes[5] || !classes[11] {
		t.Errorf("Variant classes = %v, want both 5 (SK) and 11 (Necro)", classes)
	}
	// Primary must be a deterministic pick (lowest npc_id) so single-variant
	// consumers see something stable.
	if st.NPCData == nil {
		t.Fatal("NPCData = nil, want a primary pick")
	}
	if st.NPCData.ID != st.Variants[0].NPC.ID {
		t.Errorf("NPCData.ID = %d, want %d (Variants[0])", st.NPCData.ID, st.Variants[0].NPC.ID)
	}
}

// Integration: targeting Kaas Thox in Vex Thal — a raid boss — should always
// surface both variants regardless of player position. Raid bosses are
// routinely dragged far from their spawn2 coordinates before most of the raid
// targets them, so position-based filtering would silently hide whichever
// variant the player wasn't standing near, and with it that variant's loot
// table. Even standing right at the north spawn point, both must come back.
func TestNPCTracker_RaidBossKeepsBothVariantsRegardlessOfPosition(t *testing.T) {
	tr := newRealDBTracker(t)
	// Vex Thal zoneidnumber is 158 (verified in the DB earlier).
	tr.SetPipePlayerSnapshot(158, 141, 318, 130)
	tr.SetPipeTarget("Kaas Thox Xi Aten Ha Ra")
	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
	if len(st.Variants) != 2 {
		t.Errorf("Variants len = %d, want 2 (raid boss: position filtering must be skipped)", len(st.Variants))
	}
	if st.NPCData == nil {
		t.Fatal("NPCData = nil, want a primary pick")
	}
	// Both candidates tie on raid_target and HP, so the deterministic
	// lowest-id pick (158437, loottable 12519) headlines; 158464 (loottable
	// 96732) still rides along in Variants so its loot isn't hidden.
	if st.NPCData.ID != 158437 {
		t.Errorf("Picked npc_id %d, want 158437 (lowest id tiebreak)", st.NPCData.ID)
	}
}

// Integration: targeting Emperor Ssraeshza in his temple should headline the
// real encounter (npc 162491, "Emperor_Ssraeshza_", loottable 12791, HP
// 1,250,000) even though the player is standing right next to the decoy row
// (npc 162065, "#Emperor_Ssraeshza", loottable 0) — the real row is
// script-spawned with no spawn2 coordinates at all, so it can only surface
// via db.ScriptSpawnedNPCOverrides, and position filtering must not discard
// it just because the decoy has coordinates and it doesn't. See the reported
// bug: the overlay was showing the decoy's stats (no loot table) instead.
func TestNPCTracker_EmperorSsraeshzaSurfacesScriptSpawnedEncounter(t *testing.T) {
	tr := newRealDBTracker(t)
	// ssratemple zoneidnumber is 162; stand at the decoy's own spawn2 coords
	// (1000, -325) to make sure position filtering can't accidentally help.
	tr.SetPipePlayerSnapshot(162, 1000, -325, 421)
	tr.SetPipeTarget("Emperor Ssraeshza")
	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
	if st.NPCData == nil {
		t.Fatal("NPCData = nil, want a primary pick")
	}
	if st.NPCData.ID != 162491 {
		t.Errorf("Picked npc_id %d, want 162491 (real encounter, not the 162065 decoy)", st.NPCData.ID)
	}
	if st.NPCData.LootTableID == 0 {
		t.Errorf("Picked variant has loottable_id 0, want the real encounter's non-zero loot table")
	}
	foundDecoy := false
	for _, v := range st.Variants {
		if v.NPC.ID == 162065 {
			foundDecoy = true
		}
	}
	if !foundDecoy {
		t.Error("decoy npc 162065 missing from Variants, want it still listed as an alternative")
	}
}

// Integration: when no player position is available but zone is, the variant
// set still surfaces — we can't pick by position so the user sees the
// alternatives honestly. (The kaas thox case with zone-only.)
func TestNPCTracker_VariantSetWhenZoneKnownButPositionMissing(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(158, 0, 0, 0)
	// Manually clear pipePlayerKnown to simulate the rare "zone arrived,
	// position not yet" race.
	tr.mu.Lock()
	tr.pipePlayerKnown = false
	tr.mu.Unlock()
	tr.SetPipeTarget("Kaas Thox Xi Aten Ha Ra")
	st := tr.GetState()
	if !st.HasTarget {
		t.Fatal("HasTarget = false, want true")
	}
	if len(st.Variants) != 2 {
		t.Errorf("Variants len = %d, want 2 (no position → keep both)", len(st.Variants))
	}
}

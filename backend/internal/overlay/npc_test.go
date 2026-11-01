package overlay

import (
	"bytes"
	"encoding/json"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
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

func TestNPCTracker_PetHitSetsTarget(t *testing.T) {
	tr := newTestTracker()

	// The player's pet (a generated EQMac pet name, not "You") lands a hit —
	// this should be treated the same as the player landing a hit, since a
	// pet only ever attacks what its owner sent it at, or what just attacked
	// the owner.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "Vonanab", Skill: "bite", Target: "a gnoll", Damage: 30},
	})

	st := tr.GetState()
	if !st.HasTarget || st.TargetName != "a gnoll" {
		t.Fatalf("got HasTarget=%v TargetName=%q, want HasTarget=true TargetName=%q", st.HasTarget, st.TargetName, "a gnoll")
	}
}

func TestNPCTracker_VerifiedPlayerNotTreatedAsPet(t *testing.T) {
	tr := newTestTracker()

	// "Vonanab" fits the generated-pet syllable shape, but once it's been
	// verified as a real player (via chat), it must never be treated as the
	// player's pet — otherwise a party member's off-target melee swing would
	// hijack the tracked target.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventVerifiedPlayer,
		Data: logparser.VerifiedPlayerData{Name: "Vonanab"},
	})
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "Vonanab", Skill: "slash", Target: "a gnoll", Damage: 30},
	})

	if tr.GetState().HasTarget {
		t.Errorf("HasTarget = true after a verified player's hit, want false")
	}
}

func TestNPCTracker_NPCHitsPlayerSetsTarget(t *testing.T) {
	tr := newTestTracker()

	// An NPC landing a hit on the player implies engagement with that NPC —
	// this covers the case where an idle pet auto-engages whatever just
	// attacked its owner, and the general "getting hit means you're in a
	// fight with something" signal in log-only mode (no Zeal pipe).
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "a gnoll", Skill: "bite", Target: "You", Damage: 15},
	})

	st := tr.GetState()
	if !st.HasTarget || st.TargetName != "a gnoll" {
		t.Fatalf("got HasTarget=%v TargetName=%q, want HasTarget=true TargetName=%q", st.HasTarget, st.TargetName, "a gnoll")
	}
}

func TestNPCTracker_NPCMissesPlayerSetsTarget(t *testing.T) {
	tr := newTestTracker()

	tr.Handle(logparser.LogEvent{
		Type: logparser.EventCombatMiss,
		Data: logparser.CombatMissData{Actor: "a gnoll", Target: "You", MissType: "miss"},
	})

	st := tr.GetState()
	if !st.HasTarget || st.TargetName != "a gnoll" {
		t.Fatalf("got HasTarget=%v TargetName=%q, want HasTarget=true TargetName=%q", st.HasTarget, st.TargetName, "a gnoll")
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
	got := filterVariantsByPosition([]db.NPCVariant{north, south}, 141, 318, tieToleranceYards)
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
	got := filterVariantsByPosition([]db.NPCVariant{necro, sk}, 550, -390, tieToleranceYards)
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
	got := filterVariantsByPosition([]db.NPCVariant{noSpawns, withSpawns}, 10, 10, tieToleranceYards)
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

// Regression: a variant's SpecialAbilities must never marshal as JSON null.
// "a lizard page" in the Lost Temple of Cazic Thule resolves to three rows,
// two of which (48039, 48853) have an empty special_abilities string and no
// see-invis flags — ParseSpecialAbilities returns nil for those. The field
// carries no omitempty, so a nil slice shipped a literal null, and the
// frontend's variant renderer called .filter() on it. That throw unmounted
// the whole overlay window and painted it solid black until an app restart
// (reported 2026-08-24, screenshot taken in this exact zone).
func TestNPCTracker_VariantSpecialAbilitiesNeverNull(t *testing.T) {
	tr := newRealDBTracker(t)
	// cazicthule_old (Lost Temple of CazicThule) is zoneidnumber 1048. No
	// player position, so every same-name row survives as a variant — the
	// log-only case the reporter was in.
	tr.SetPipePlayerSnapshot(1048, 0, 0, 0)
	tr.mu.Lock()
	tr.pipePlayerKnown = false
	tr.mu.Unlock()
	tr.SetPipeTarget("a lizard page")

	st := tr.GetState()
	if len(st.Variants) < 2 {
		t.Fatalf("Variants len = %d, want >= 2 (same-name lizard page rows)", len(st.Variants))
	}
	sawEmpty := false
	for _, v := range st.Variants {
		if v.SpecialAbilities == nil {
			t.Errorf("npc %d (%s): SpecialAbilities is nil, want an empty slice", v.NPC.ID, v.NPC.Name)
		}
		if len(v.SpecialAbilities) == 0 {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatal("no ability-less variant in the set — this fixture no longer covers the bug")
	}

	blob, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if bytes.Contains(blob, []byte(`"special_abilities":null`)) {
		t.Error("payload contains special_abilities:null, want []")
	}
}

// ── Zeal v1.4.6 target_id: sticky variant resolution ─────────────────────────

func ptrInt(n int) *int { return &n }

func TestEqIntPtr(t *testing.T) {
	a, b := 5, 5
	c := 6
	cases := []struct {
		x, y *int
		want bool
	}{
		{nil, nil, true},
		{&a, nil, false},
		{nil, &a, false},
		{&a, &b, true},
		{&a, &c, false},
	}
	for _, tc := range cases {
		if got := eqIntPtr(tc.x, tc.y); got != tc.want {
			t.Errorf("eqIntPtr(%v,%v) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

// A seeded cache entry for the current spawn id must be used verbatim by
// setTarget instead of the (nil-DB) fresh lookup — proving the id-keyed result
// wins over re-running disambiguation.
func TestPipeTargetID_CacheHitUsedBySetTarget(t *testing.T) {
	tr := newTestTracker()

	seeded := resolvedTarget{
		name: "a sarnak conscript",
		npc:  &db.NPC{ID: 424242, Name: "a_sarnak_conscript"},
	}
	tr.mu.Lock()
	tr.lastPipeTargetID = ptrInt(9001)
	tr.variantCache[9001] = seeded
	tr.mu.Unlock()

	tr.SetPipeTarget("a sarnak conscript")

	st := tr.GetState()
	if st.NPCData == nil || st.NPCData.ID != 424242 {
		t.Fatalf("setTarget ignored the id-keyed cache: NPCData=%v", st.NPCData)
	}
}

// A cache entry only applies when its stored name still matches the live
// target — a recycled/stale id pointing at a different name must not leak.
func TestPipeTargetID_CacheRejectedOnNameMismatch(t *testing.T) {
	tr := newTestTracker()
	tr.mu.Lock()
	tr.lastPipeTargetID = ptrInt(9001)
	tr.variantCache[9001] = resolvedTarget{name: "a orc pawn", npc: &db.NPC{ID: 111}}
	tr.mu.Unlock()

	tr.SetPipeTarget("a sarnak conscript") // different name, same stale id

	if st := tr.GetState(); st.NPCData != nil {
		t.Fatalf("stale-id cache leaked across names: NPCData=%v", st.NPCData)
	}
}

func TestPipeTargetID_MissResolvesAndCaches(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(162, 1000, -325, 421) // ssratemple

	tr.SetPipeTarget("A Shissar Revenant")
	tr.SetPipeTargetID(ptrInt(7777))

	tr.mu.RLock()
	entry, ok := tr.variantCache[7777]
	tr.mu.RUnlock()
	if !ok {
		t.Fatal("SetPipeTargetID (cache miss) did not store a resolution")
	}
	if entry.name != "A Shissar Revenant" {
		t.Errorf("cached entry name = %q", entry.name)
	}
	if entry.npc == nil && len(entry.variants) == 0 {
		t.Error("cached a completely empty resolution for a known NPC")
	}

	// Re-targeting the same spawn id is a hit — no new work, same result.
	before := tr.GetState()
	tr.ClearPipeTarget()
	tr.SetPipeTargetID(nil)
	tr.SetPipeTarget("A Shissar Revenant")
	tr.SetPipeTargetID(ptrInt(7777))
	after := tr.GetState()
	if len(before.Variants) != len(after.Variants) {
		t.Errorf("re-pull changed the variant count: %d -> %d", len(before.Variants), len(after.Variants))
	}
}

func TestPipeTargetID_ZoneChangeFlushesCache(t *testing.T) {
	tr := newTestTracker()
	tr.mu.Lock()
	tr.lastPipeTargetID = ptrInt(1)
	tr.variantCache[1] = resolvedTarget{name: "x"}
	tr.variantCache[2] = resolvedTarget{name: "y"}
	tr.mu.Unlock()

	// Log-driven zone change goes through setZone.
	tr.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The Deep"},
	})

	tr.mu.RLock()
	n := len(tr.variantCache)
	id := tr.lastPipeTargetID
	tr.mu.RUnlock()
	if n != 0 || id != nil {
		t.Fatalf("zone change didn't flush: %d entries, id=%v", n, id)
	}
}

func TestPipeTargetID_PipeZoneChangeFlushesCache(t *testing.T) {
	tr := newTestTracker()
	tr.SetPipePlayerSnapshot(100, 0, 0, 0)
	tr.mu.Lock()
	tr.variantCache[5] = resolvedTarget{name: "z"}
	tr.lastPipeTargetID = ptrInt(5)
	tr.mu.Unlock()

	tr.SetPipePlayerSnapshot(101, 0, 0, 0) // zoned

	tr.mu.RLock()
	n := len(tr.variantCache)
	tr.mu.RUnlock()
	if n != 0 {
		t.Fatalf("pipe zone change didn't flush the variant cache: %d entries", n)
	}
}

func TestPipeTargetID_NilIsNoOp(t *testing.T) {
	tr := newTestTracker()
	tr.mu.Lock()
	tr.variantCache[3] = resolvedTarget{name: "keep me"}
	tr.mu.Unlock()

	tr.SetPipeTargetID(nil)
	tr.SetPipeTargetID(nil)

	tr.mu.RLock()
	_, kept := tr.variantCache[3]
	tr.mu.RUnlock()
	if !kept {
		t.Error("nil target id wiped the cache")
	}
}

func TestPipeTargetID_DisconnectFlushesCache(t *testing.T) {
	tr := newTestTracker()
	tr.mu.Lock()
	tr.variantCache[8] = resolvedTarget{name: "q"}
	tr.lastPipeTargetID = ptrInt(8)
	tr.mu.Unlock()

	tr.ResetPipeFields() // pipe disconnect

	tr.mu.RLock()
	n := len(tr.variantCache)
	id := tr.lastPipeTargetID
	tr.mu.RUnlock()
	if n != 0 || id != nil {
		t.Fatalf("disconnect didn't flush: %d entries, id=%v", n, id)
	}
}

// ── Zeal target descriptors (PR #239): live level/class/race/position ───────

// descAt builds a descriptor set for tests. loc=false models Zeal withholding
// target_loc (target 250+ units from the player).
func descAt(name string, level, class, race int, loc bool, x, y float64) *zealpipe.TargetDescriptors {
	return &zealpipe.TargetDescriptors{
		Name: name, Type: zealpipe.TargetTypeNPC, Level: level, Class: class, Race: race,
		LocKnown: loc, GameX: x, GameY: y,
	}
}

// targetWithDesc drives the production order: the name label first, then
// the MsgPlayer snapshot carrying the spawn id and descriptors.
func targetWithDesc(tr *NPCTracker, id int, d *zealpipe.TargetDescriptors) TargetState {
	tr.SetPipeTarget(d.Name)
	tr.SetPipeTargetSnapshot(ptrInt(id), d, 0, false)
	return tr.GetState()
}

func variantIDs(st TargetState) []int {
	if len(st.Variants) == 0 {
		if st.NPCData == nil {
			return nil
		}
		return []int{st.NPCData.ID}
	}
	ids := make([]int, 0, len(st.Variants))
	for _, v := range st.Variants {
		ids = append(ids, v.NPC.ID)
	}
	return ids
}

func sameIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[int]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func TestNPCTracker_TargetDescriptorsResolveVariants(t *testing.T) {
	const kaas = "Kaas Thox Xi Aten Ha Ra"
	tests := []struct {
		name string
		zone int
		// player position; Kaas Thox cases stand far from both spawns so the
		// result can only come from the descriptors.
		px, py float64
		desc   *zealpipe.TargetDescriptors
		want   []int
	}{
		// ssratemple: both rows share every spawn point, level range and
		// race — class is the only thing that tells them apart.
		{"shissar necro by class", 162, 550, -390,
			descAt("A Shissar Revenant", 52, 11, 217, true, 550, -390), []int{162197}},
		{"shissar SK by class", 162, 550, -390,
			descAt("A Shissar Revenant", 52, 5, 217, true, 550, -390), []int{162490}},
		// Vex Thal raid bosses: rows identical except spawn position (+318 vs -321).
		{"kaas thox at north spawn", 158, 0, 0, descAt(kaas, 66, 9, 145, true, 141, 318), []int{158437}},
		{"kaas thox at south spawn", 158, 0, 0, descAt(kaas, 66, 9, 145, true, 141, -321), []int{158464}},
		{"kaas thox pulled partway", 158, 0, 0, descAt(kaas, 66, 9, 145, true, 141, 170), []int{158437}},
		{"kaas thox at midpoint stays ambiguous", 158, 0, 0,
			descAt(kaas, 66, 9, 145, true, 141, 0), []int{158437, 158464}},
		{"kaas thox beyond 250 (no loc) stays ambiguous", 158, 0, 0,
			descAt(kaas, 66, 9, 145, false, 0, 0), []int{158437, 158464}},
		{"thall va xakra at south spawn", 158, 0, 0,
			descAt("Thall Va Xakra", 60, 1, 224, true, 142, -408), []int{158465}},
		// Non-raid pair with four spawn clusters each.
		{"va xakra inner cluster", 158, 0, 0, descAt("Va Xakra", 60, 5, 224, true, 140, 419), []int{158006}},
		{"va xakra outer cluster", 158, 0, 0, descAt("Va Xakra", 60, 5, 224, true, 1172, 336), []int{158086}},
		// Plane of Fear: raid row vs low-HP sibling differ by level.
		{"cazic thule raid version by level", 72, 0, 0,
			descAt("Cazic Thule", 70, 5, 95, false, 0, 0), []int{72003}},
		{"cazic thule 32k sibling by level", 72, 0, 0,
			descAt("Cazic Thule", 55, 5, 95, false, 0, 0), []int{72500}},
		{"a dracoliche raid version by level", 72, 0, 0,
			descAt("a dracoliche", 58, 11, 122, false, 0, 0), []int{72090}},
		// A descriptor set no row matches must not lose the NPC.
		{"mismatch keeps full set", 162, 0, 0,
			descAt("A Shissar Revenant", 52, 12, 217, false, 0, 0), []int{162197, 162490}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newRealDBTracker(t)
			tr.SetPipePlayerSnapshot(tt.zone, tt.px, tt.py, 0)
			st := targetWithDesc(tr, 4242, tt.desc)
			if got := variantIDs(st); !sameIDs(got, tt.want) {
				t.Errorf("resolved to %v, want %v", got, tt.want)
			}
			if st.LiveLevel == nil || *st.LiveLevel != tt.desc.Level {
				t.Errorf("LiveLevel = %v, want %d", st.LiveLevel, tt.desc.Level)
			}
			if st.LiveClass == nil || *st.LiveClass != tt.desc.Class {
				t.Errorf("LiveClass = %v, want %d", st.LiveClass, tt.desc.Class)
			}
		})
	}
}

// A PC target that happens to share an NPC's name must not show NPC data.
func TestNPCTracker_TargetDescriptorsPCSkipsNPCLookup(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(162, 0, 0, 0)
	d := descAt("A Shissar Revenant", 60, 11, 1, true, 0, 0)
	d.Type = zealpipe.TargetTypePC
	st := targetWithDesc(tr, 1, d)
	if st.NPCData != nil || len(st.Variants) != 0 {
		t.Errorf("PC target resolved to NPC data: %v / %d variants", st.NPCData, len(st.Variants))
	}
}

// Targeted from beyond 250 units (no loc) the boss is ambiguous; once the
// target comes into range its position narrows the same spawn id's entry.
// Afterwards, moving to the midpoint must not un-resolve it.
func TestNPCTracker_TargetDescriptorsRefineAmbiguousEntry(t *testing.T) {
	const kaas = "Kaas Thox Xi Aten Ha Ra"
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(158, 0, 0, 0)

	st := targetWithDesc(tr, 77, descAt(kaas, 66, 9, 145, false, 0, 0))
	if len(st.Variants) != 2 {
		t.Fatalf("far target: %d variants, want 2", len(st.Variants))
	}

	tr.SetPipeTargetSnapshot(ptrInt(77), descAt(kaas, 66, 9, 145, true, 141, -321), 200, true)
	st = tr.GetState()
	if got := variantIDs(st); !sameIDs(got, []int{158464}) {
		t.Fatalf("in-range target resolved to %v, want [158464]", got)
	}

	tr.SetPipeTargetSnapshot(ptrInt(77), descAt(kaas, 66, 9, 145, true, 141, 0), 100, true)
	if got := variantIDs(tr.GetState()); !sameIDs(got, []int{158464}) {
		t.Errorf("resolved entry un-resolved after moving to the midpoint: %v", got)
	}

	// A fresh re-target of the same spawn is served from the refined cache.
	tr.ClearPipeTarget()
	tr.SetPipeTargetSnapshot(nil, nil, 0, false)
	st = targetWithDesc(tr, 77, descAt(kaas, 66, 9, 145, false, 0, 0))
	if got := variantIDs(st); !sameIDs(got, []int{158464}) {
		t.Errorf("re-target of resolved spawn = %v, want [158464]", got)
	}
}

// Stock Zeal (target_id only): no live fields, and the old raid-boss rule
// (keep both) still applies.
func TestNPCTracker_NoDescriptorsKeepsLegacyBehaviour(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(158, 141, 318, 130)
	tr.SetPipeTarget("Kaas Thox Xi Aten Ha Ra")
	tr.SetPipeTargetID(ptrInt(5))
	st := tr.GetState()
	if len(st.Variants) != 2 {
		t.Errorf("Variants len = %d, want 2", len(st.Variants))
	}
	if st.LiveLevel != nil || st.LiveClass != nil || st.TargetType != nil {
		t.Errorf("live fields set without descriptors: %v %v %v", st.LiveLevel, st.LiveClass, st.TargetType)
	}
	if d := tr.GetDistance(); d.HasDescriptors || d.Distance != nil {
		t.Errorf("distance = %+v, want empty", d)
	}
}

// Descriptors from the previous target (name mismatch) must be ignored.
func TestDescFor(t *testing.T) {
	d := descAt("a gnoll", 10, 1, 39, false, 0, 0)
	if descFor(d, "a gnoll") != d {
		t.Error("matching name rejected")
	}
	if descFor(d, "A Gnoll") != d {
		t.Error("case-only difference rejected")
	}
	if descFor(d, "an orc pawn") != nil {
		t.Error("mismatched name accepted")
	}
	corpse := descAt("a gnoll's corpse", 10, 1, 39, false, 0, 0)
	if descFor(corpse, "a gnoll") != corpse {
		t.Error("corpse suffix not stripped")
	}
	if descFor(nil, "a gnoll") != nil {
		t.Error("nil desc")
	}
}

func TestNPCTracker_DistanceFeed(t *testing.T) {
	tr := newTestTracker()
	d := descAt("a gnoll", 10, 1, 39, true, 0, 0)

	tr.SetPipeTargetSnapshot(ptrInt(1), d, 142.6, true)
	if got := tr.GetDistance(); !got.HasDescriptors || got.Distance == nil || *got.Distance != 143 {
		t.Fatalf("distance = %+v, want 143", got)
	}
	// Beyond 250: descriptors but no position → null distance immediately.
	far := *d
	far.LocKnown = false
	tr.SetPipeTargetSnapshot(ptrInt(1), &far, 0, false)
	if got := tr.GetDistance(); !got.HasDescriptors || got.Distance != nil {
		t.Fatalf("distance = %+v, want has_descriptors with null distance", got)
	}
	// Disconnect clears it.
	tr.ResetPipeFields()
	if got := tr.GetDistance(); got.HasDescriptors || got.Distance != nil {
		t.Fatalf("distance after disconnect = %+v, want empty", got)
	}
}

// A Valiant Spirit (hohonora) is two rows with identical class/level/HP/size;
// only the 90/10 spawn weight, body texture and Summon differ. The overlay
// must carry enough for the UI to label them apart.
func TestNPCTracker_ValiantSpiritVariantsAreDistinguishable(t *testing.T) {
	tr := newRealDBTracker(t)
	tr.SetPipePlayerSnapshot(211 /* hohonora */, 0, 0, 0)
	tr.SetPipeTarget("A Valiant Spirit")
	st := tr.GetState()
	if len(st.Variants) != 2 {
		t.Fatalf("Variants len = %d, want 2", len(st.Variants))
	}
	byID := map[int]TargetVariant{}
	for _, v := range st.Variants {
		byID[v.NPC.ID] = v
	}
	plain, summoner := byID[211017], byID[211119]
	if plain.SpawnChance == nil || *plain.SpawnChance != 90 {
		t.Errorf("211017 SpawnChance = %v, want 90", plain.SpawnChance)
	}
	if summoner.SpawnChance == nil || *summoner.SpawnChance != 10 {
		t.Errorf("211119 SpawnChance = %v, want 10", summoner.SpawnChance)
	}
	contains := func(hs []string, want string) bool {
		for _, h := range hs {
			if h == want {
				return true
			}
		}
		return false
	}
	if !contains(summoner.Distinguish, "summons") || !contains(summoner.Distinguish, "texture 2") {
		t.Errorf("211119 Distinguish = %v, want summons + texture 2", summoner.Distinguish)
	}
	if !contains(plain.Distinguish, "no summon") || !contains(plain.Distinguish, "texture 0") {
		t.Errorf("211017 Distinguish = %v, want no summon + texture 0", plain.Distinguish)
	}
}

func descWithAppearance(name string, level, class, race, texture, gender int, size float64) *zealpipe.TargetDescriptors {
	d := descAt(name, level, class, race, false, 0, 0)
	d.HasAppearance, d.Texture, d.Gender, d.Size = true, texture, gender, size
	return d
}

// A Valiant Spirit rows match on every pre-appearance descriptor, so texture
// is the only thing that resolves them.
func TestNPCTracker_AppearanceResolvesValiantSpirit(t *testing.T) {
	const name = "A Valiant Spirit"
	tests := []struct {
		name string
		desc *zealpipe.TargetDescriptors
		want []int
	}{
		{"texture 0 is the non-summoner", descWithAppearance(name, 62, 1, 323, 0, 2, 12), []int{211017}},
		{"texture 2 is the summoner", descWithAppearance(name, 62, 1, 323, 2, 2, 12), []int{211119}},
		{"unmatched texture keeps both", descWithAppearance(name, 62, 1, 323, 7, 2, 12), []int{211017, 211119}},
		{"older Zeal without appearance keeps both", descAt(name, 62, 1, 323, false, 0, 0), []int{211017, 211119}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newRealDBTracker(t)
			tr.SetPipePlayerSnapshot(211, 0, 0, 0)
			st := targetWithDesc(tr, 4242, tt.desc)
			if got := variantIDs(st); !sameIDs(got, tt.want) {
				t.Errorf("resolved to %v, want %v", got, tt.want)
			}
		})
	}
}

// PreviewState builds a live-shaped target for any NPC id (Settings preview)
// without disturbing the tracker's own target.
func TestPreviewState(t *testing.T) {
	tr := newRealDBTracker(t)
	st, err := tr.PreviewState(32040) // Lord Nagafen
	if err != nil {
		t.Fatalf("PreviewState: %v", err)
	}
	if !st.HasTarget || st.NPCData == nil || st.NPCData.ID != 32040 {
		t.Fatalf("state = %+v, want Nagafen target", st)
	}
	if st.TargetName != "Lord Nagafen" {
		t.Errorf("target name = %q", st.TargetName)
	}
	if st.HPPercent != -1 {
		t.Errorf("hp percent = %d, want -1", st.HPPercent)
	}
	if len(st.SpecialAbilities) == 0 {
		t.Error("want parsed special abilities")
	}
	if tr.GetState().HasTarget {
		t.Error("preview changed the live target")
	}
	if _, err := tr.PreviewState(-1); err == nil {
		t.Error("want error for unknown id")
	}
}

// ── Pet overlay accessors ────────────────────────────────────────────────────

func TestResolvedForID_CarriesLiveLevel(t *testing.T) {
	tr := newRealDBTracker(t)
	if _, ok := tr.ResolvedForID(812); ok {
		t.Fatal("hit for a spawn id that was never targeted")
	}
	targetWithDesc(tr, 812, descAt("A Shissar Revenant", 52, 11, 217, false, 0, 0))
	r, ok := tr.ResolvedForID(812)
	if !ok || r.NPC == nil {
		t.Fatal("no resolution cached for the targeted spawn id")
	}
	if r.Level != 52 {
		t.Errorf("level = %d, want the live 52", r.Level)
	}
	if !strings.EqualFold(r.NPC.Name, "A_Shissar_Revenant") {
		t.Errorf("npc = %q", r.NPC.Name)
	}
}

func TestResolveByName_UnknownLevel(t *testing.T) {
	tr := newRealDBTracker(t)
	r := tr.ResolveByName("A Shissar Revenant")
	if r.NPC == nil || r.Level != 0 {
		t.Fatalf("got npc %v level %d, want a row with unknown level", r.NPC, r.Level)
	}
	if r := tr.ResolveByName("Gobeker"); r.NPC != nil {
		t.Errorf("generated pet name resolved to %q", r.NPC.Name)
	}
}

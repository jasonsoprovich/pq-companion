package zeal

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeInstall writes an EQ folder with a Zeal.asi reporting version and,
// when ini isn't "", a zeal.ini with that content.
func fakeInstall(t *testing.T, version, ini string) string {
	t.Helper()
	dir := t.TempDir()
	asi := []byte("MZ junk Zeal Version: \x00" + version + "\x00 more junk")
	if err := os.WriteFile(filepath.Join(dir, "Zeal.asi"), asi, 0o644); err != nil {
		t.Fatal(err)
	}
	if ini != "" {
		if err := os.WriteFile(filepath.Join(dir, "zeal.ini"), []byte(ini), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func rowsByID(rows []CapRow) map[CapID]CapRow {
	m := map[CapID]CapRow{}
	for _, r := range rows {
		m[r.ID] = r
	}
	return m
}

func TestReadZealINI(t *testing.T) {
	dir := fakeInstall(t, "1.5.0", "[Osui]\nPipeVerbose=TRUE\n\n[Zeal]\n ; comment\npipeverbose = true\nPipeSpells=2\nExportOnCamp=FALSE\n[Other]\nPipeConsider=TRUE\n")
	ini := ReadZealINI(dir)
	if !ini.Readable {
		t.Fatal("expected readable")
	}
	if v, ok := ini.Get("PipeVerbose"); !ok || v != "true" {
		t.Errorf("PipeVerbose = %q %v, want [Zeal] value", v, ok)
	}
	if v, _ := ini.Get("pipespells"); v != "2" {
		t.Errorf("PipeSpells = %q", v)
	}
	if _, ok := ini.Get("PipeConsider"); ok {
		t.Error("keys outside [Zeal] must be ignored")
	}
	if got := ReadZealINI(t.TempDir()); got.Readable {
		t.Error("missing file must be unreadable, not empty-but-readable")
	}
	if got := ReadZealINI(""); got.Readable {
		t.Error("empty path must be unreadable")
	}
}

func TestBuildCapRows_PendingHiddenOnReleasedZeal(t *testing.T) {
	dir := fakeInstall(t, "1.5.0", "[Zeal]\nPipeSpells=2\nPipeConsider=TRUE\n")
	rows := rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: ReadZealINI(dir)}))
	for _, id := range []CapID{CapPipeSpells, CapPipeConsider, CapRosterAppearance, CapTargetSpawnLoc} {
		if _, ok := rows[id]; ok {
			t.Errorf("%s listed without proof; a zeal.ini key alone must not count", id)
		}
	}
	for _, id := range []CapID{CapExportOnCamp, CapPopFlagsOnCamp, CapPipeVerbose} {
		if _, ok := rows[id]; !ok {
			t.Errorf("%s should be listed on Zeal 1.5.0", id)
		}
	}
	old := rowsByID(BuildCapRows(CapEvidence{Version: "1.4.8", INI: ReadZealINI(dir)}))
	if _, ok := old[CapPopFlagsOnCamp]; ok {
		t.Error("popflags_on_camp needs Zeal 1.5.0")
	}
}

func TestBuildCapRows_States(t *testing.T) {
	ini := ReadZealINI(fakeInstall(t, "1.5.0", "[Zeal]\nPipeVerbose=TRUE\nPipeSpells=1\nExportOnCamp=TRUE\n"))
	all := map[CapID]bool{CapPipeSpells: true, CapPipeConsider: true, CapRosterAppearance: true, CapTargetSpawnLoc: true}
	rows := rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: ini, Persisted: all}))

	want := map[CapID]string{
		CapExportOnCamp:     CapOn,
		CapPopFlagsOnCamp:   CapOff, // missing key = Zeal default FALSE
		CapPipeVerbose:      CapOn,
		CapPipeSpells:       CapOn,
		CapPipeConsider:     CapOff,
		CapRosterAppearance: CapOn, // follows PipeVerbose
		CapTargetSpawnLoc:   CapNotSeen,
	}
	for id, st := range want {
		if rows[id].State != st {
			t.Errorf("%s state = %q, want %q", id, rows[id].State, st)
		}
	}
	if rows[CapPipeSpells].Value != "detrimental" {
		t.Errorf("PipeSpells value = %q", rows[CapPipeSpells].Value)
	}
	if !rows[CapPipeSpells].Pending || rows[CapPipeVerbose].Pending {
		t.Error("pending flag wrong")
	}
	if rows[CapPipeVerbose].Hint == "" {
		t.Error("an enabled row waiting for data should carry a hint")
	}
}

func TestBuildCapRows_LiveProofBeatsINI(t *testing.T) {
	// zeal.ini says off (or is unreadable), but the pipe is carrying the data.
	off := ReadZealINI(fakeInstall(t, "1.5.0", "[Zeal]\nPipeVerbose=FALSE\nPipeSpells=0\n"))
	live := map[CapID]bool{CapPipeVerbose: true, CapPipeSpells: true, CapTargetSpawnLoc: true}
	rows := rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: off, Live: live}))
	for id := range live {
		if rows[id].State != CapWorking {
			t.Errorf("%s = %q, want working", id, rows[id].State)
		}
	}
	rows = rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: ZealINI{}, Live: live}))
	if rows[CapPipeVerbose].State != CapWorking {
		t.Error("live proof should beat an unreadable zeal.ini")
	}
	if rows[CapExportOnCamp].State != CapUnknown {
		t.Errorf("export_on_camp with unreadable ini = %q, want unknown", rows[CapExportOnCamp].State)
	}
}

func TestBuildCapRows_ExportNeverWorking(t *testing.T) {
	ini := ReadZealINI(fakeInstall(t, "1.5.0", "[Zeal]\nExportOnCamp=TRUE\n"))
	live := map[CapID]bool{CapExportOnCamp: true, CapPopFlagsOnCamp: true}
	rows := rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: ini, Live: live}))
	if rows[CapExportOnCamp].State != CapOn {
		t.Errorf("export rows have no pipe proof; got %q", rows[CapExportOnCamp].State)
	}
}

func TestBuildCapRows_AppearanceOffNamesVerbose(t *testing.T) {
	ini := ReadZealINI(fakeInstall(t, "1.5.0", "[Zeal]\n"))
	rows := rowsByID(BuildCapRows(CapEvidence{Version: "1.5.0", INI: ini, Persisted: map[CapID]bool{CapRosterAppearance: true}}))
	r := rows[CapRosterAppearance]
	if r.State != CapOff || r.Hint == "" {
		t.Errorf("appearance with verbose off = %q hint %q", r.State, r.Hint)
	}
}

// tracker with the pipe's game folder forced to dir and an in-memory store.
func testTracker(dir string, store map[string]string, changes *int32) *CapTracker {
	tr := NewCapTracker(
		func() map[string]string { return store },
		func(id CapID, identity string) { store[string(id)] = identity },
		func() { atomic.AddInt32(changes, 1) },
	)
	tr.exeDir = func(uint32) string { return dir }
	return tr
}

func TestCapTracker_LiveProofOnlyForMatchingInstall(t *testing.T) {
	a := fakeInstall(t, "1.5.0", "")
	b := fakeInstall(t, "1.5.0", "")
	var n int32
	tr := testTracker(a, map[string]string{}, &n)
	tr.Connected(1)
	tr.Saw(CapPipeSpells)

	if live, persisted, match := tr.Evidence(a); !match || !live[CapPipeSpells] || !persisted[CapPipeSpells] {
		t.Errorf("install A: match=%v live=%v persisted=%v", match, live, persisted)
	}
	if live, persisted, match := tr.Evidence(b); match || live[CapPipeSpells] || persisted[CapPipeSpells] {
		t.Errorf("install B must get no credit: match=%v live=%v persisted=%v", match, live, persisted)
	}
}

func TestCapTracker_UnresolvedProcessNeverCounts(t *testing.T) {
	a := fakeInstall(t, "1.5.0", "")
	store := map[string]string{}
	var n int32
	tr := testTracker("", store, &n)
	tr.Connected(1)
	tr.Saw(CapPipeSpells)
	if len(store) != 0 {
		t.Error("proof from an unknown game folder must not persist")
	}
	if live, persisted, _ := tr.Evidence(a); live[CapPipeSpells] || persisted[CapPipeSpells] {
		t.Error("proof from an unknown game folder must not show as seen")
	}
}

func TestCapTracker_ResetOnReconnectAndBroadcastOnce(t *testing.T) {
	a := fakeInstall(t, "1.5.0", "")
	var n int32
	tr := testTracker(a, map[string]string{}, &n)
	tr.Connected(1)
	base := atomic.LoadInt32(&n)
	for range 50 {
		tr.Saw(CapPipeConsider)
	}
	if got := atomic.LoadInt32(&n) - base; got != 1 {
		t.Errorf("repeated proof broadcast %d times, want 1", got)
	}
	tr.Disconnected()
	if live, _, match := tr.Evidence(a); match || live[CapPipeConsider] {
		t.Error("disconnect must clear live proof")
	}
	tr.Connected(2)
	if live, _, _ := tr.Evidence(a); live[CapPipeConsider] {
		t.Error("a new connection starts with no live proof")
	}
}

func TestCapTracker_PersistedProofTiedToBuild(t *testing.T) {
	a := fakeInstall(t, "1.5.0", "")
	store := map[string]string{}
	var n int32
	tr := testTracker(a, store, &n)
	tr.Connected(1)
	tr.Saw(CapPipeSpells)
	tr.Disconnected()

	// Restart: a fresh tracker reading the same saved config still lists it.
	tr2 := testTracker(a, store, &n)
	if _, persisted, _ := tr2.Evidence(a); !persisted[CapPipeSpells] {
		t.Fatal("proof should survive a restart on the same build")
	}

	// Zeal replaced by a build with the same version string (new size).
	asi := filepath.Join(a, "Zeal.asi")
	if err := os.WriteFile(asi, []byte("MZ Zeal Version: \x001.5.0\x00 a different build"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, persisted, _ := tr2.Evidence(a); persisted[CapPipeSpells] {
		t.Error("a replaced Zeal build must not inherit proof")
	}

	// Downgrade (different version) on the same folder.
	if err := os.WriteFile(asi, []byte("MZ Zeal Version: \x001.4.8\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, persisted, _ := tr2.Evidence(a); persisted[CapPipeSpells] {
		t.Error("a downgraded Zeal must not inherit proof")
	}
}

func TestCapTracker_DirChangeDuringConnection(t *testing.T) {
	a := fakeInstall(t, "1.5.0", "")
	b := fakeInstall(t, "1.5.0", "")
	var n int32
	tr := testTracker(a, map[string]string{}, &n)
	tr.Connected(1)
	tr.Saw(CapPipeVerbose)
	// The user points PQC at another folder while the game from A runs.
	if live, _, match := tr.Evidence(b); match || live[CapPipeVerbose] {
		t.Error("switching the EQ folder mid-connection must not carry A's live proof")
	}
}

func TestWatchINI_NoCharacterNeeded(t *testing.T) {
	dir := fakeInstall(t, "1.5.0", "[Zeal]\nPipeVerbose=FALSE\n")
	var n int32
	tr := testTracker(dir, map[string]string{}, &n)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go tr.WatchINI(ctx, func() string { return dir }, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	start := atomic.LoadInt32(&n)
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(filepath.Join(dir, "zeal.ini"), []byte("[Zeal]\nPipeVerbose=TRUE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(filepath.Join(dir, "zeal.ini"), future, future)
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&n) == start && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&n) == start {
		t.Error("a zeal.ini change should notify with no character configured")
	}
}

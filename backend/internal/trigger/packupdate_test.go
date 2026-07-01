package trigger

import (
	"testing"
)

// updTrigger builds a pack-definition trigger for update tests.
func updTrigger(name, pattern, packName string) Trigger {
	return Trigger{
		Name:     name,
		Enabled:  true,
		Pattern:  pattern,
		PackName: packName,
		Actions:  []Action{{Type: ActionTextToSpeech, Text: name, Volume: 1.0}},
	}
}

// installWithBaselines installs a test pack and snapshots its definitions as
// baselines — what insertPackTriggers does automatically for built-in packs,
// done by hand here because test packs aren't in AllPacks().
func installWithBaselines(t *testing.T, s *Store, pack TriggerPack) {
	t.Helper()
	if err := InstallPack(s, pack); err != nil {
		t.Fatalf("install %s: %v", pack.PackName, err)
	}
	// InstallPack stamps IDs into the caller's slice; re-derive pristine defs.
	for i := range pack.Triggers {
		def := pack.Triggers[i]
		def.ID = ""
		def.SourcePack = ""
		if err := s.UpsertPackBaseline(pack.PackName, &def); err != nil {
			t.Fatalf("baseline %s: %v", def.Name, err)
		}
		// Stamp pack_key on the installed row like insertPackTriggers does
		// for built-in packs.
		row, err := s.FindByPackAndName(def.PackName, def.Name)
		if err != nil || row == nil {
			t.Fatalf("find %s: %v", def.Name, err)
		}
		row.PackKey = packKeyOf(&def)
		if err := s.Update(row); err != nil {
			t.Fatalf("stamp pack_key %s: %v", def.Name, err)
		}
	}
}

func rowByKey(t *testing.T, s *Store, pack, key string) *Trigger {
	t.Helper()
	rows, err := s.ListBySourcePack(pack)
	if err != nil {
		t.Fatalf("ListBySourcePack: %v", err)
	}
	for _, r := range rows {
		if r.PackKey == key {
			return r
		}
	}
	return nil
}

func testPackV1() TriggerPack {
	return TriggerPack{
		PackName: "TestPack",
		Triggers: []Trigger{
			updTrigger("Alpha", `^alpha$`, "TestPack"),
			updTrigger("Beta", `^beta$`, "TestPack"),
		},
	}
}

func TestComputePackDiff_CleanInstallHasNoUpdates(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	diff, err := ComputePackDiff(s, testPackV1())
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff.HasUpdates() || len(diff.DeletedLocally) != 0 {
		t.Fatalf("clean install should have no updates: %+v", diff)
	}
	if diff.UpToDate != 2 {
		t.Fatalf("up_to_date = %d, want 2", diff.UpToDate)
	}
}

// A user customization alone must never read as an available update.
func TestComputePackDiff_UserEditIsNotAnUpdate(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	row := rowByKey(t, s, "TestPack", "Alpha")
	row.Actions = []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3", Volume: 1.0}}
	row.RefireCooldownSecs = 5
	if err := s.Update(row); err != nil {
		t.Fatalf("update: %v", err)
	}

	diff, err := ComputePackDiff(s, testPackV1())
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diff.HasUpdates() {
		t.Fatalf("user edit flagged as update: %+v", diff)
	}
}

func TestComputePackDiff_DevChangeShowsFieldDiff(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	// User customized actions on Alpha; dev then changed Alpha's pattern.
	row := rowByKey(t, s, "TestPack", "Alpha")
	row.Actions = []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3"}}
	if err := s.Update(row); err != nil {
		t.Fatalf("update: %v", err)
	}
	v2 := testPackV1()
	v2.Triggers[0].Pattern = `^alpha v2$`

	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].PackKey != "Alpha" {
		t.Fatalf("changed = %+v, want Alpha", diff.Changed)
	}
	fields := diff.Changed[0].Fields
	if len(fields) != 1 || fields[0].Field != "pattern" {
		t.Fatalf("fields = %+v, want single pattern diff", fields)
	}
	if fields[0].UserCustomized {
		t.Fatal("pattern was not user-customized")
	}
	if fields[0].Old != `^alpha$` || fields[0].New != `^alpha v2$` {
		t.Fatalf("old/new = %q/%q", fields[0].Old, fields[0].New)
	}
}

func TestComputePackDiff_AddedRemovedDeletedLocally(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	// User deletes Beta locally.
	beta := rowByKey(t, s, "TestPack", "Beta")
	if err := s.Delete(beta.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// v2: drops Alpha, keeps Beta, adds Gamma.
	v2 := TriggerPack{
		PackName: "TestPack",
		Triggers: []Trigger{
			updTrigger("Beta", `^beta$`, "TestPack"),
			updTrigger("Gamma", `^gamma$`, "TestPack"),
		},
	}
	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.Added) != 1 || diff.Added[0].PackKey != "Gamma" {
		t.Fatalf("added = %+v, want Gamma", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].PackKey != "Alpha" {
		t.Fatalf("removed = %+v, want Alpha", diff.Removed)
	}
	if len(diff.DeletedLocally) != 1 || diff.DeletedLocally[0].PackKey != "Beta" {
		t.Fatalf("deleted_locally = %+v, want Beta", diff.DeletedLocally)
	}
}

func TestApplyPackUpdate_PreserveKeepsUserFields(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	// User: custom actions + refire on Alpha, moved it to a custom category.
	row := rowByKey(t, s, "TestPack", "Alpha")
	custom := []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3"}}
	row.Actions = custom
	row.RefireCooldownSecs = 5
	row.PackName = "My Stuff"
	if err := s.Update(row); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Dev: new pattern, new actions, new refire on Alpha.
	v2 := testPackV1()
	v2.Triggers[0].Pattern = `^alpha v2$`
	v2.Triggers[0].Actions = []Action{{Type: ActionTextToSpeech, Text: "Alpha improved"}}
	v2.Triggers[0].RefireCooldownSecs = 2

	res, err := ApplyPackUpdate(s, v2, v2, UpdateModePreserve, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d, want 1", res.Updated)
	}

	got := rowByKey(t, s, "TestPack", "Alpha")
	if got.Pattern != `^alpha v2$` {
		t.Fatalf("pattern = %q, want dev's v2 value", got.Pattern)
	}
	if len(got.Actions) != 1 || got.Actions[0].SoundPath != "/tmp/ding.mp3" {
		t.Fatalf("actions = %+v, want user's sound kept", got.Actions)
	}
	if got.RefireCooldownSecs != 5 {
		t.Fatalf("refire = %v, want user's 5 kept", got.RefireCooldownSecs)
	}
	if got.PackName != "My Stuff" {
		t.Fatalf("category = %q, want user's move kept", got.PackName)
	}

	// Baseline advanced: the same v2 diff is now clean.
	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("re-diff: %v", err)
	}
	if diff.HasUpdates() {
		t.Fatalf("diff after apply should be clean: %+v", diff)
	}
}

func TestApplyPackUpdate_ResetRestoresDefaults(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	row := rowByKey(t, s, "TestPack", "Alpha")
	row.Actions = []Action{{Type: ActionPlaySound, SoundPath: "/tmp/ding.mp3"}}
	row.PackName = "My Stuff"
	if err := s.Update(row); err != nil {
		t.Fatalf("update: %v", err)
	}

	v2 := testPackV1()
	v2.Triggers[0].Pattern = `^alpha v2$`
	defaulted := testPackV1()
	defaulted.Triggers[0].Pattern = `^alpha v2$`
	defaulted.Triggers[0].Characters = []string{"Osui"}

	if _, err := ApplyPackUpdate(s, v2, defaulted, UpdateModeReset, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got := rowByKey(t, s, "TestPack", "Alpha")
	if got.Pattern != `^alpha v2$` || got.PackName != "TestPack" {
		t.Fatalf("reset kept user values: pattern=%q category=%q", got.Pattern, got.PackName)
	}
	if len(got.Actions) != 1 || got.Actions[0].Type != ActionTextToSpeech {
		t.Fatalf("actions = %+v, want pack default", got.Actions)
	}
	if len(got.Characters) != 1 || got.Characters[0] != "Osui" {
		t.Fatalf("characters = %+v, want defaulted list", got.Characters)
	}
	if got.ID != row.ID {
		t.Fatal("reset must keep the row ID")
	}
}

func TestApplyPackUpdate_SelectiveKeys(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	v2 := testPackV1()
	v2.Triggers[0].Pattern = `^alpha v2$`
	v2.Triggers[1].Pattern = `^beta v2$`

	res, err := ApplyPackUpdate(s, v2, v2, UpdateModePreserve, []string{"Alpha"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d, want 1", res.Updated)
	}
	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("re-diff: %v", err)
	}
	if len(diff.Changed) != 1 || diff.Changed[0].PackKey != "Beta" {
		t.Fatalf("Beta should stay pending: %+v", diff.Changed)
	}
}

func TestApplyPackUpdate_AddRemove(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	v2 := TriggerPack{
		PackName: "TestPack",
		Triggers: []Trigger{
			updTrigger("Alpha", `^alpha$`, "TestPack"),
			updTrigger("Gamma", `^gamma$`, "TestPack"),
		},
	}
	res, err := ApplyPackUpdate(s, v2, v2, UpdateModePreserve, nil)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Added != 1 || res.Removed != 1 {
		t.Fatalf("added/removed = %d/%d, want 1/1", res.Added, res.Removed)
	}
	if rowByKey(t, s, "TestPack", "Beta") != nil {
		t.Fatal("Beta should be deleted")
	}
	gamma := rowByKey(t, s, "TestPack", "Gamma")
	if gamma == nil || gamma.SourcePack != "TestPack" {
		t.Fatalf("Gamma not installed correctly: %+v", gamma)
	}
	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("re-diff: %v", err)
	}
	if diff.HasUpdates() {
		t.Fatalf("diff after apply should be clean: %+v", diff)
	}
}

func TestApplyPackUpdate_DeletedLocallyIsOptIn(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())
	beta := rowByKey(t, s, "TestPack", "Beta")
	if err := s.Delete(beta.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Blanket apply must not resurrect Beta.
	if _, err := ApplyPackUpdate(s, testPackV1(), testPackV1(), UpdateModePreserve, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if rowByKey(t, s, "TestPack", "Beta") != nil {
		t.Fatal("blanket apply resurrected a locally-deleted trigger")
	}

	// Explicit selection re-adds it.
	res, err := ApplyPackUpdate(s, testPackV1(), testPackV1(), UpdateModePreserve, []string{"Beta"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Added != 1 || rowByKey(t, s, "TestPack", "Beta") == nil {
		t.Fatal("explicit selection should re-add Beta")
	}
}

// A developer rename carries identity through an explicit PackKey: the trigger
// updates in place instead of showing as remove+add.
func TestPackUpdate_DevRenameViaPackKey(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	v2 := testPackV1()
	v2.Triggers[0].Name = "Alpha Prime"
	v2.Triggers[0].PackKey = "Alpha"

	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.Added) != 0 || len(diff.Removed) != 0 || len(diff.Changed) != 1 {
		t.Fatalf("rename should be a change, not add/remove: %+v", diff)
	}
	if diff.Changed[0].Fields[0].Field != "name" {
		t.Fatalf("fields = %+v, want name diff", diff.Changed[0].Fields)
	}
	if _, err := ApplyPackUpdate(s, v2, v2, UpdateModePreserve, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := rowByKey(t, s, "TestPack", "Alpha")
	if got == nil || got.Name != "Alpha Prime" {
		t.Fatalf("row = %+v, want renamed in place with stable key", got)
	}
}

// A user rename is a customization: preserve mode keeps the user's name while
// still applying the developer's other changes.
func TestPackUpdate_UserRenameKeptInPreserve(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	row := rowByKey(t, s, "TestPack", "Alpha")
	row.Name = "My Alpha"
	if err := s.Update(row); err != nil {
		t.Fatalf("update: %v", err)
	}

	v2 := testPackV1()
	v2.Triggers[0].Name = "Alpha Prime"
	v2.Triggers[0].PackKey = "Alpha"
	v2.Triggers[0].Pattern = `^alpha v2$`

	if _, err := ApplyPackUpdate(s, v2, v2, UpdateModePreserve, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := rowByKey(t, s, "TestPack", "Alpha")
	if got.Name != "My Alpha" {
		t.Fatalf("name = %q, want user rename kept", got.Name)
	}
	if got.Pattern != `^alpha v2$` {
		t.Fatalf("pattern = %q, want dev change applied", got.Pattern)
	}
}

// Added triggers whose dedup_key is owned by another installed pack are
// neither reported nor inserted.
func TestPackUpdate_AddedRespectsDedupOwnership(t *testing.T) {
	s := openTestStore(t)
	installWithBaselines(t, s, testPackV1())

	// Another pack owns shared_key.
	other := makePack("OtherPack", "shared_key")
	if err := InstallPack(s, other); err != nil {
		t.Fatalf("install other: %v", err)
	}

	v2 := testPackV1()
	shared := updTrigger("Shared Trigger", `^shared$`, "TestPack")
	shared.DedupKey = "shared_key"
	v2.Triggers = append(v2.Triggers, shared)

	diff, err := ComputePackDiff(s, v2)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.Added) != 0 {
		t.Fatalf("dedup-owned trigger reported as added: %+v", diff.Added)
	}
}

// The one-time seed stamps pack_key on pre-feature rows and snapshots
// baselines, so a real built-in pack installed before this feature diffs
// clean against the current build.
func TestBackfillPackKeysAndBaselines_Seed(t *testing.T) {
	s := openTestStore(t)

	// Simulate a pre-feature install of the Monk pack: rows exist with
	// source_pack set but no pack_key and no baselines.
	var monk TriggerPack
	for _, p := range AllPacks() {
		if p.PackName == "Monk" {
			monk = p
			break
		}
	}
	if err := InstallPack(s, monk); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE triggers SET pack_key = ''`); err != nil {
		t.Fatalf("strip pack_key: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM pack_baselines`); err != nil {
		t.Fatalf("strip baselines: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM pack_default_updates WHERE key = 'PackBaselines:Seed:v1'`); err != nil {
		t.Fatalf("reset ledger: %v", err)
	}

	if err := s.backfillPackKeysAndBaselines(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, p := range AllPacks() {
		if p.PackName != "Monk" {
			continue
		}
		diff, err := ComputePackDiff(s, p)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		if diff.HasUpdates() || len(diff.DeletedLocally) != 0 {
			t.Fatalf("seeded install should diff clean: %+v", diff)
		}
		if diff.UpToDate == 0 {
			t.Fatal("expected up-to-date triggers after seed")
		}
	}
}

package trigger

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// openTestStore creates a temporary user.db for testing.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// openTestEngine returns a Store and Engine backed by a temp database.
func openTestEngine(t *testing.T) (*Store, *Engine) {
	t.Helper()
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)
	return s, e
}

func TestEngine_MatchAndFire(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "test1",
		Name:    "Mez Wore Off",
		Enabled: true,
		Pattern: `Your .+ spell has worn off\.`,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "MEZ BROKE!", DurationSecs: 5},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Should fire.
	e.Handle(time.Now(), "Your Mesmerization spell has worn off.")
	hist := e.GetHistory()
	if len(hist) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(hist))
	}
	if hist[0].TriggerName != "Mez Wore Off" {
		t.Errorf("expected trigger name %q, got %q", "Mez Wore Off", hist[0].TriggerName)
	}
	if hist[0].MatchedLine != "Your Mesmerization spell has worn off." {
		t.Errorf("unexpected matched line: %q", hist[0].MatchedLine)
	}

	// Should not fire on unrelated line.
	e.Handle(time.Now(), "You slash a gnoll for 100 points of damage.")
	if len(e.GetHistory()) != 1 {
		t.Errorf("unrelated line should not fire trigger")
	}
}

func TestEngine_RefireCooldownSuppressesRepeat(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:                 "cd1",
		Name:               "Rampage",
		Enabled:            true,
		Pattern:            `goes on a RAMPAGE`,
		Actions:            []Action{{Type: ActionOverlayText, Text: "RAMP", DurationSecs: 5}},
		RefireCooldownSecs: 10,
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	// First fire is allowed.
	e.Handle(base, "A gnoll goes on a RAMPAGE!")
	// 3s later — within the 10s cooldown — suppressed.
	e.Handle(base.Add(3*time.Second), "A gnoll goes on a RAMPAGE!")
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("expected 1 fire during cooldown, got %d", got)
	}
	// 11s after the first fire — cooldown elapsed — fires again.
	e.Handle(base.Add(11*time.Second), "A gnoll goes on a RAMPAGE!")
	if got := len(e.GetHistory()); got != 2 {
		t.Fatalf("expected 2 fires after cooldown elapsed, got %d", got)
	}
}

func TestEngine_RefireCooldownSurvivesReload(t *testing.T) {
	s, e := openTestEngine(t)
	tr := &Trigger{
		ID:                 "cd2",
		Name:               "Spam",
		Enabled:            true,
		Pattern:            `ding`,
		Actions:            []Action{{Type: ActionOverlayText, Text: "DING"}},
		RefireCooldownSecs: 30,
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	e.Handle(base, "ding")
	e.Reload() // an unrelated CRUD edit recompiles triggers
	// Still within the cooldown after a reload — must stay suppressed.
	e.Handle(base.Add(5*time.Second), "ding")
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("cooldown should survive Reload; expected 1 fire, got %d", got)
	}
}

func TestEngine_DisabledTriggerDoesNotFire(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:        "disabled1",
		Name:      "Disabled Trigger",
		Enabled:   false,
		Pattern:   `You have been slain`,
		Actions:   []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "You have been slain by a gnoll.")
	if len(e.GetHistory()) != 0 {
		t.Errorf("disabled trigger should not fire")
	}
}

func TestEngine_ReloadUpdatesPatterns(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:        "upd1",
		Name:      "Tell Alert",
		Enabled:   true,
		Pattern:   `\w+ tells you,`,
		Actions:   []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Somedude tells you, 'hello'")
	if len(e.GetHistory()) != 1 {
		t.Fatalf("expected 1 entry after match")
	}

	// Disable and reload — should no longer fire.
	tr.Enabled = false
	if err := s.Update(tr); err != nil {
		t.Fatalf("Update: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Otherdude tells you, 'hi'")
	if len(e.GetHistory()) != 1 {
		t.Errorf("disabled trigger fired after reload")
	}
}

// ExcludePatterns suppress firing when any exclude regex matches the same
// line. The intended use is filtering pet/merchant lines out of a broad
// "incoming tell" pattern.
func TestEngine_ExcludePatternsSuppressMatch(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "ex1",
		Name:    "Incoming Tell",
		Enabled: true,
		Pattern: `\w+ tells you,`,
		Actions: []Action{},
		ExcludePatterns: []string{
			`\b[Mm]aster[.!]`,
			`tells you, '[Tt]hat'll be `,
			`tells you, '[Ii]'ll give you `,
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	cases := []struct {
		line     string
		wantFire bool
	}{
		{"Somedude tells you, 'hello'", true},
		{"Pyrtaark tells you, 'Attacking Soandso Master.'", false},
		{"Sandsplinter tells you, 'Following you, Master.'", false},
		{"Somepet tells you, 'By your command, master.'", false},
		{"Tarki tells you, 'That'll be 5 gold pieces for the Cloth Cap.'", false},
		{"Tarki tells you, 'I'll give you 5 silver pieces for the Cloth Cap.'", false},
		{"Friend tells you, 'group inv?'", true},
	}
	wantTotal := 0
	for _, c := range cases {
		e.Handle(time.Now(), c.line)
		if c.wantFire {
			wantTotal++
		}
	}
	got := len(e.GetHistory())
	if got != wantTotal {
		t.Fatalf("history: got %d entries, want %d", got, wantTotal)
	}
}

// Invalid exclude regex is logged and skipped — the trigger still fires on
// matching lines, the bad pattern is just ignored. Mirrors the lenient
// behaviour for invalid worn-off patterns.
func TestEngine_InvalidExcludePatternIsSkipped(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:              "ex2",
		Name:            "Tell",
		Enabled:         true,
		Pattern:         `\w+ tells you,`,
		Actions:         []Action{},
		ExcludePatterns: []string{`[invalid(regex`, `\bMaster`},
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Somedude tells you, 'hi'")
	e.Handle(time.Now(), "Pet tells you, 'Following you, Master.'")
	if got := len(e.GetHistory()); got != 1 {
		t.Errorf("expected the valid exclude (', Master') to suppress pet line; got %d firings", got)
	}
}

func TestEngine_HistoryRingBuffer(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:        "ring1",
		Name:      "Any Hit",
		Enabled:   true,
		Pattern:   `damage`,
		Actions:   []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	for i := 0; i < historyMaxSize+10; i++ {
		e.Handle(time.Now(), "You slash a gnoll for 100 points of damage.")
	}
	if len(e.GetHistory()) != historyMaxSize {
		t.Errorf("expected history capped at %d, got %d", historyMaxSize, len(e.GetHistory()))
	}
}

func TestStore_CRUD(t *testing.T) {
	s := openTestStore(t)

	// Insert.
	tr := &Trigger{
		ID:      "crud1",
		Name:    "Test Trigger",
		Enabled: true,
		Pattern: `test`,
		Actions: []Action{
			{Type: ActionOverlayText, Text: "Test!", DurationSecs: 3, Color: "#ffffff"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Get.
	got, err := s.Get("crud1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != tr.Name {
		t.Errorf("name mismatch: got %q want %q", got.Name, tr.Name)
	}
	if len(got.Actions) != 1 || got.Actions[0].Text != "Test!" {
		t.Errorf("actions not preserved: %+v", got.Actions)
	}

	// Update.
	got.Name = "Updated"
	got.Enabled = false
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := s.Get("crud1")
	if updated.Name != "Updated" || updated.Enabled {
		t.Errorf("update did not persist: %+v", updated)
	}

	// List.
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 trigger, got %d", len(list))
	}

	// Delete.
	if err := s.Delete("crud1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("crud1"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_ErrNotFound(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.Get("nonexistent"); err != ErrNotFound {
		t.Errorf("Get nonexistent: expected ErrNotFound, got %v", err)
	}
	if err := s.Delete("nonexistent"); err != ErrNotFound {
		t.Errorf("Delete nonexistent: expected ErrNotFound, got %v", err)
	}
	if err := s.Update(&Trigger{ID: "nonexistent", Actions: []Action{}}); err != ErrNotFound {
		t.Errorf("Update nonexistent: expected ErrNotFound, got %v", err)
	}
}

// ApplyDefaultUpdates appends new exclude patterns to an installed pack
// trigger without touching customizations. Re-running the same key is a
// no-op (idempotent). The user-modified pattern field stays as-is.
func TestApplyDefaultUpdates_AppendsAndIsIdempotent(t *testing.T) {
	s := openTestStore(t)

	// Simulate a user with the pack installed and a customized "Incoming
	// Tell" trigger: they've changed the pattern and added a personal
	// exclude. Both must survive the migration.
	tr := &Trigger{
		ID:              "ut1",
		Name:            "Incoming Tell",
		PackName:        "General Triggers",
		Enabled:         true,
		Pattern:         `\w+ tells you, '\['`,               // user-customized: only fire on item-link tells
		ExcludePatterns: []string{`^IDLEBOT \w+ tells you,`}, // user's personal entry
		Actions:         []Action{},
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	updates := []DefaultUpdate{
		{
			Key:                   "test:incoming-tell:v1",
			PackName:              "General Triggers",
			TriggerName:           "Incoming Tell",
			AppendExcludePatterns: []string{`\b[Mm]aster[.!]`, `tells you, '[Tt]hat'll be `},
		},
	}

	mutated, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates: %v", err)
	}
	if mutated != 1 {
		t.Errorf("mutated: got %d, want 1", mutated)
	}

	got, err := s.Get(tr.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Pattern != `\w+ tells you, '\['` {
		t.Errorf("user pattern was overwritten: %q", got.Pattern)
	}
	wantExcludes := map[string]bool{
		`^IDLEBOT \w+ tells you,`:    true,
		`\b[Mm]aster[.!]`:            true,
		`tells you, '[Tt]hat'll be `: true,
	}
	if len(got.ExcludePatterns) != len(wantExcludes) {
		t.Errorf("excludes count: got %d, want %d (%v)", len(got.ExcludePatterns), len(wantExcludes), got.ExcludePatterns)
	}
	for _, p := range got.ExcludePatterns {
		if !wantExcludes[p] {
			t.Errorf("unexpected exclude after merge: %q", p)
		}
	}

	// Second run: nothing should change (key already applied).
	mutated2, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates 2: %v", err)
	}
	if mutated2 != 0 {
		t.Errorf("second run mutated %d triggers; expected 0", mutated2)
	}

	// User removes one of our defaults — it must NOT come back. The key is
	// recorded as applied, so the migration is skipped entirely.
	got.ExcludePatterns = []string{`^IDLEBOT \w+ tells you,`, `\b[Mm]aster[.!]`}
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := ApplyDefaultUpdates(s, updates); err != nil {
		t.Fatalf("ApplyDefaultUpdates 3: %v", err)
	}
	got2, _ := s.Get(tr.ID)
	for _, p := range got2.ExcludePatterns {
		if p == `tells you, '[Tt]hat'll be ` {
			t.Errorf("removed default came back: user opt-out wasn't respected")
		}
	}
}

// When the target trigger doesn't exist (user uninstalled the pack), the
// update is silently skipped but still marked applied so we don't keep
// retrying on every startup.
func TestApplyDefaultUpdates_MissingTriggerSkipsAndMarks(t *testing.T) {
	s := openTestStore(t)

	updates := []DefaultUpdate{
		{
			Key:                   "test:missing:v1",
			PackName:              "General Triggers",
			TriggerName:           "Incoming Tell",
			AppendExcludePatterns: []string{`anything`},
		},
	}

	mutated, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates: %v", err)
	}
	if mutated != 0 {
		t.Errorf("mutated: got %d, want 0", mutated)
	}
	applied, err := s.IsDefaultUpdateApplied("test:missing:v1")
	if err != nil {
		t.Fatalf("IsDefaultUpdateApplied: %v", err)
	}
	if !applied {
		t.Errorf("update should be marked applied even when target trigger is absent")
	}
}

// ApplyDefaultUpdates adds a brand-new trigger to an already-installed pack,
// is idempotent, and never resurrects a pack the user doesn't have.
func TestApplyDefaultUpdates_InsertTrigger(t *testing.T) {
	s := openTestStore(t)

	insert := petSpellWornOff("Spell Breaks")
	updates := []DefaultUpdate{
		{
			Key:           "test:pet-worn-off:add-v1",
			PackName:      "Spell Breaks",
			InsertTrigger: &insert,
		},
	}

	// Pack not installed → skipped (don't resurrect it), but marked applied.
	mutated, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates (no pack): %v", err)
	}
	if mutated != 0 {
		t.Errorf("insert into absent pack mutated %d; want 0", mutated)
	}
	if got, _ := s.FindByPackAndName("Spell Breaks", "Pet Spell Worn Off"); got != nil {
		t.Fatal("trigger inserted into a pack the user never installed")
	}

	// Now the user has the pack installed (some other trigger present). A fresh
	// key must insert the pet trigger.
	owner := &Trigger{
		ID: "sb1", Name: "Spell Worn Off", PackName: "Spell Breaks",
		SourcePack: "Spell Breaks", Enabled: true, Pattern: `x`,
		Actions: []Action{}, CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(owner); err != nil {
		t.Fatalf("Insert owner: %v", err)
	}
	updates[0].Key = "test:pet-worn-off:add-v2" // fresh key (v1 already marked)
	mutated, err = ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates (installed): %v", err)
	}
	if mutated != 1 {
		t.Errorf("insert mutated %d; want 1", mutated)
	}
	got, err := s.FindByPackAndName("Spell Breaks", "Pet Spell Worn Off")
	if err != nil || got == nil {
		t.Fatalf("pet trigger not inserted: %v", err)
	}
	if got.SourcePack != "Spell Breaks" || got.Pattern != petWornOffPattern {
		t.Errorf("inserted trigger malformed: source=%q pattern=%q", got.SourcePack, got.Pattern)
	}

	// Re-running with the same key is a no-op; the trigger isn't duplicated.
	mutated, err = ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates (rerun): %v", err)
	}
	if mutated != 0 {
		t.Errorf("rerun mutated %d; want 0", mutated)
	}
}

// ApplyDefaultUpdates fills in a missing reuse cooldown (monk Feign Death
// gained a 9s CD after some installs were created). Idempotent on re-run.
func TestApplyDefaultUpdates_SetsCooldownWhenUnset(t *testing.T) {
	s := openTestStore(t)

	// A Feign Death trigger that predates the cooldown — CooldownSecs left 0.
	unset := &Trigger{
		ID:        "fd-unset",
		Name:      "Feign Death",
		PackName:  "Monk",
		Enabled:   true,
		Pattern:   `^You feign death\.$`,
		Actions:   []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(unset); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	updates := []DefaultUpdate{
		{
			Key:             "test:monk-fd-cd:v1",
			PackName:        "Monk",
			TriggerName:     "Feign Death",
			SetCooldownSecs: 9,
		},
	}

	mutated, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates: %v", err)
	}
	if mutated != 1 {
		t.Errorf("mutated: got %d, want 1", mutated)
	}
	got, err := s.Get(unset.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CooldownSecs != 9 {
		t.Errorf("CooldownSecs: got %v, want 9", got.CooldownSecs)
	}

	// Idempotent: the key is recorded as applied, so a second run is a no-op.
	mutated2, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates 2: %v", err)
	}
	if mutated2 != 0 {
		t.Errorf("second run mutated %d; want 0", mutated2)
	}
}

// A user who hand-tuned their Feign Death cooldown keeps it — the migration
// only fills in an unset (0) value, never overwrites.
func TestApplyDefaultUpdates_PreservesCustomCooldown(t *testing.T) {
	s := openTestStore(t)

	custom := &Trigger{
		ID:           "fd-custom",
		Name:         "Feign Death",
		PackName:     "Monk",
		Enabled:      true,
		Pattern:      `^You feign death\.$`,
		CooldownSecs: 20, // user hand-tuned
		Actions:      []Action{},
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.Insert(custom); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	updates := []DefaultUpdate{
		{
			Key:             "test:monk-fd-cd-preserve:v1",
			PackName:        "Monk",
			TriggerName:     "Feign Death",
			SetCooldownSecs: 9,
		},
	}

	mutated, err := ApplyDefaultUpdates(s, updates)
	if err != nil {
		t.Fatalf("ApplyDefaultUpdates: %v", err)
	}
	if mutated != 0 {
		t.Errorf("mutated: got %d, want 0 (custom value must be preserved)", mutated)
	}
	got, err := s.Get(custom.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CooldownSecs != 20 {
		t.Errorf("user cooldown was overwritten: got %v, want 20", got.CooldownSecs)
	}
}

func TestInstallPack(t *testing.T) {
	s := openTestStore(t)

	pack := EnchanterPack()
	if err := InstallPack(s, pack); err != nil {
		t.Fatalf("InstallPack: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != len(pack.Triggers) {
		t.Errorf("expected %d triggers after install, got %d", len(pack.Triggers), len(list))
	}

	// Re-installing should replace, not duplicate.
	if err := InstallPack(s, pack); err != nil {
		t.Fatalf("second InstallPack: %v", err)
	}
	list2, _ := s.List()
	if len(list2) != len(pack.Triggers) {
		t.Errorf("re-install duplicated triggers: expected %d, got %d", len(pack.Triggers), len(list2))
	}
}

// ── Stage E: pipe-source triggers ────────────────────────────────────────────

// insertPipeTrigger persists a pipe-source trigger and reloads the engine.
// Returns the trigger so callers can assert on history matches by ID.
func insertPipeTrigger(t *testing.T, s *Store, e *Engine, name string, cond PipeCondition) *Trigger {
	t.Helper()
	tr := &Trigger{
		ID:            "pipe-" + name,
		Name:          name,
		Enabled:       true,
		Source:        SourcePipe,
		PipeCondition: &cond,
		Actions:       []Action{{Type: ActionOverlayText, Text: name + " fired"}},
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert pipe trigger %s: %v", name, err)
	}
	e.Reload()
	return tr
}

func TestPipe_TargetHPBelowFiresOnceOnTransition(t *testing.T) {
	s, e := openTestEngine(t)
	insertPipeTrigger(t, s, e, "low-hp", PipeCondition{
		Kind: PipeKindTargetHPBelow, HPThreshold: 20,
	})

	now := time.Now()
	// Healthy target — no fire.
	e.HandlePipeTarget("Sandrian", 80, "", now)
	if got := len(e.GetHistory()); got != 0 {
		t.Errorf("healthy HP fired: %d", got)
	}
	// HP drops below threshold — fire once.
	e.HandlePipeTarget("Sandrian", 15, "", now.Add(time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("expected 1 fire on transition, got %d", got)
	}
	// Stays low — no refire.
	e.HandlePipeTarget("Sandrian", 10, "", now.Add(2*time.Second))
	e.HandlePipeTarget("Sandrian", 5, "", now.Add(3*time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Errorf("refired while staying below threshold: %d", got)
	}
	// HP rises above and falls again — fires again.
	e.HandlePipeTarget("Sandrian", 90, "", now.Add(4*time.Second))
	e.HandlePipeTarget("Sandrian", 15, "", now.Add(5*time.Second))
	if got := len(e.GetHistory()); got != 2 {
		t.Errorf("expected 2 fires after re-arm + drop, got %d", got)
	}
}

func TestPipe_TargetNameFiresOnceOnMatch(t *testing.T) {
	s, e := openTestEngine(t)
	insertPipeTrigger(t, s, e, "spotted", PipeCondition{
		Kind: PipeKindTargetName, TargetName: "Vulak`Aerr",
	})

	now := time.Now()
	e.HandlePipeTarget("a goblin pup", 100, "", now)
	if got := len(e.GetHistory()); got != 0 {
		t.Errorf("non-matching name fired: %d", got)
	}
	e.HandlePipeTarget("Vulak`Aerr", 100, "", now.Add(time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("expected 1 fire on name match, got %d", got)
	}
	// Same target on next tick — no refire (dedupe).
	e.HandlePipeTarget("Vulak`Aerr", 99, "", now.Add(2*time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Errorf("refired on identical target name: %d", got)
	}
}

func TestPipe_BuffLandedAndFaded(t *testing.T) {
	s, e := openTestEngine(t)
	insertPipeTrigger(t, s, e, "shield-on", PipeCondition{
		Kind: PipeKindBuffLanded, SpellName: "Shield of Words",
	})
	insertPipeTrigger(t, s, e, "shield-off", PipeCondition{
		Kind: PipeKindBuffFaded, SpellName: "Shield of Words",
	})

	now := time.Now()
	// Initial empty slots — nothing fires.
	e.HandlePipeBuffSlots(nil, "", now)
	if got := len(e.GetHistory()); got != 0 {
		t.Fatalf("empty slots fired: %d", got)
	}
	// Buff lands.
	e.HandlePipeBuffSlots([]string{"Shield of Words", "Clarity"}, "", now.Add(time.Second))
	hist := e.GetHistory()
	if len(hist) != 1 || hist[0].TriggerName != "shield-on" {
		t.Fatalf("expected shield-on, got %+v", hist)
	}
	// Same slots — no refire.
	e.HandlePipeBuffSlots([]string{"Shield of Words", "Clarity"}, "", now.Add(2*time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Errorf("buff_landed refired on stable slots: %d", got)
	}
	// Buff fades.
	e.HandlePipeBuffSlots([]string{"Clarity"}, "", now.Add(3*time.Second))
	hist = e.GetHistory()
	if len(hist) != 2 || hist[1].TriggerName != "shield-off" {
		t.Fatalf("expected shield-off, got %+v", hist)
	}
}

func TestPipe_PipeCommandExactMatch(t *testing.T) {
	s, e := openTestEngine(t)
	insertPipeTrigger(t, s, e, "pull-cmd", PipeCondition{
		Kind: PipeKindPipeCommand, Text: "pull",
	})

	now := time.Now()
	e.HandlePipeCommand("nope", "", now)
	if got := len(e.GetHistory()); got != 0 {
		t.Errorf("non-matching text fired: %d", got)
	}
	e.HandlePipeCommand("pull", "", now.Add(time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("expected 1 fire on exact pipe-command match, got %d", got)
	}
	// Repeat — fires again (one-shot per call, no edge dedupe).
	e.HandlePipeCommand("pull", "", now.Add(2*time.Second))
	if got := len(e.GetHistory()); got != 2 {
		t.Errorf("expected pipe_command to refire on repeat, got %d", got)
	}
}

func TestPipe_DisconnectResetsEdgeState(t *testing.T) {
	s, e := openTestEngine(t)
	insertPipeTrigger(t, s, e, "low-hp", PipeCondition{
		Kind: PipeKindTargetHPBelow, HPThreshold: 20,
	})

	now := time.Now()
	e.HandlePipeTarget("Sandrian", 80, "", now)
	e.HandlePipeTarget("Sandrian", 15, "", now.Add(time.Second))
	if got := len(e.GetHistory()); got != 1 {
		t.Fatalf("setup: expected 1 fire, got %d", got)
	}
	// Disconnect (e.g. Zeal exits) drops the prev state.
	e.HandlePipeReset()
	// Reconnect with a still-low value. The reset means "no prior reading",
	// which we treat as 100%, so this counts as a fresh transition and fires.
	e.HandlePipeTarget("Sandrian", 15, "", now.Add(2*time.Second))
	if got := len(e.GetHistory()); got != 2 {
		t.Errorf("expected refire after reset, got %d", got)
	}
}

// TestPipe_LogTriggersUntouchedByReload confirms that the addition of
// pipe-source segregation doesn't break the existing log-source path.
func TestPipe_LogTriggersUntouchedByReload(t *testing.T) {
	s, e := openTestEngine(t)
	logTrig := &Trigger{
		ID: "log-1", Name: "Log Trigger", Enabled: true,
		Pattern:   `You have entered`,
		Actions:   []Action{{Type: ActionOverlayText, Text: "x"}},
		CreatedAt: time.Now(),
	}
	if err := s.Insert(logTrig); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	insertPipeTrigger(t, s, e, "pipe-1", PipeCondition{
		Kind: PipeKindPipeCommand, Text: "ignored",
	})

	// Log trigger still fires via Handle.
	e.Handle(time.Now(), "You have entered The North Karana.")
	hist := e.GetHistory()
	if len(hist) != 1 || hist[0].TriggerName != "Log Trigger" {
		t.Errorf("log trigger didn't fire after pipe segregation; history=%+v", hist)
	}
}

func TestSubstituteCaptures(t *testing.T) {
	re := regexp.MustCompile(`^(.+) tells you, '(.+)'$`)
	line := "Bob tells you, 'can you rez me?'"
	m := re.FindStringSubmatch(line)
	names := re.SubexpNames()

	cases := []struct{ in, want string }{
		{"Tell from {1}: {2}", "Tell from Bob: can you rez me?"},
		{"Tell from $1: $2", "Tell from Bob: can you rez me?"},
		{"whole: {0}", "whole: " + line},
		{"no refs here", "no refs here"},
		{"unknown {9} and {nope} stay", "unknown {9} and {nope} stay"},
	}
	for _, c := range cases {
		if got := substituteCaptures(c.in, m, names, nil); got != c.want {
			t.Errorf("substituteCaptures(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Named groups.
	reNamed := regexp.MustCompile(`^(?P<sender>.+) tells you, '(?P<message>.+)'$`)
	mn := reNamed.FindStringSubmatch(line)
	if got := substituteCaptures("Tell from {sender}: {message}", mn, reNamed.SubexpNames(), nil); got != "Tell from Bob: can you rez me?" {
		t.Errorf("named-group substitution = %q", got)
	}

	// No match → template returned unchanged.
	if got := substituteCaptures("Tell from {1}", nil, nil, nil); got != "Tell from {1}" {
		t.Errorf("no-match should pass through, got %q", got)
	}
}

func TestSubstituteCaptures_GINAAliasesAndBuiltins(t *testing.T) {
	re := regexp.MustCompile(`^(.+) tells you, '(.+)'$`)
	line := "Bob tells you, 'can you rez me?'"
	m := re.FindStringSubmatch(line)
	names := re.SubexpNames()
	builtins := map[string]string{
		"c": "Vortikai", "char": "Vortikai", "self": "Vortikai",
		"target": "a gnoll", "t": "a gnoll",
	}

	cases := []struct{ in, want string }{
		// GINA {S}/{SN} aliases for plain numbered groups.
		{"{S} said {S2}", "Bob said can you rez me?"},
		{"{s1} whispers", "Bob whispers"},
		{"{S3} missing", "{S3} missing"}, // group 3 doesn't exist
		// Built-ins, case-insensitive.
		{"{c} kill {target}!", "Vortikai kill a gnoll!"},
		{"{C} / {CHAR} / {self} / {T}", "Vortikai / Vortikai / Vortikai / a gnoll"},
		// Unknown tokens untouched.
		{"{spell} stays", "{spell} stays"},
	}
	for _, c := range cases {
		if got := substituteCaptures(c.in, m, names, builtins); got != c.want {
			t.Errorf("substituteCaptures(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Builtins substitute even with no regex match (pipe triggers).
	if got := substituteCaptures("{c} ready", nil, nil, builtins); got != "Vortikai ready" {
		t.Errorf("builtin-only substitution = %q", got)
	}

	// An explicit named group shadows the same-named built-in.
	reNamed := regexp.MustCompile(`^(?P<target>.+) hits you`)
	mn := reNamed.FindStringSubmatch("a willowisp hits you for 10.")
	if got := substituteCaptures("{target}", mn, reNamed.SubexpNames(), builtins); got != "a willowisp" {
		t.Errorf("named group should shadow built-in, got %q", got)
	}
}

func TestNormalizePattern(t *testing.T) {
	cases := []struct{ in, char, want string }{
		// Character tokens.
		{`^{c} has been slain`, "Vortikai", `^Vortikai has been slain`},
		{`^{C} begins`, "Vortikai", `^Vortikai begins`},
		{`{char} {self}`, "Vortikai", `Vortikai Vortikai`},
		{`^{c} stays`, "", `^{c} stays`}, // no character yet → literal
		// GINA wildcards.
		{`{S} tells you, '{S2}'`, "", `(?P<S>.+) tells you, '(?P<S2>.+)'`},
		{`hits for {N} points`, "", `hits for (?P<N>[0-9]+) points`},
		{`{N1} and {n2}`, "", `(?P<N1>[0-9]+) and (?P<N2>[0-9]+)`},
		// .NET named groups → Go syntax.
		{`(?<sender>.+) tells you`, "", `(?P<sender>.+) tells you`},
		// Repetition syntax + unknown tokens untouched.
		{`\d{2} {S10} {spell}`, "", `\d{2} {S10} {spell}`},
	}
	for _, c := range cases {
		if got := normalizePattern(c.in, c.char); got != c.want {
			t.Errorf("normalizePattern(%q, %q) = %q, want %q", c.in, c.char, got, c.want)
		}
		if _, err := regexp.Compile(normalizePattern(c.in, c.char)); err != nil {
			t.Errorf("normalized %q doesn't compile: %v", c.in, err)
		}
	}
}

func TestParseDurationText(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"400", 400},
		{"1.5", 1.5},
		{"6:40", 400},
		{"6:40.5", 400.5},
		{"1:02:03", 3723},
		{"6m40s", 400},
		{"6M40S", 400},
		{"2h", 7200},
		{"90s", 90},
		{"5m", 300},
		{"1h2m3s", 3723},
		{" 300 ", 300},
		{"", 0},
		{"abc", 0},
		{"-5", 0},
		{"6:xx", 0},
	}
	for _, c := range cases {
		if got := ParseDurationText(c.in); got != c.want {
			t.Errorf("ParseDurationText(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// captureSink records StartExternal/StopExternal calls for asserting timer
// dispatch. The scalar fields hold the most recent StartExternal call.
type captureSink struct {
	name     string
	category string
	duration float64
	spellID  int
	target   string
	barColor string
	calls    int

	stopName    string
	stopSpellID int
	stops       int
}

func (s *captureSink) StartExternal(name, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string) {
	s.name, s.category, s.duration, s.spellID, s.target, s.barColor = name, category, durationSecs, spellID, targetName, barColor
	s.calls++
}
func (s *captureSink) StopExternal(name string, spellID int) {
	s.stopName, s.stopSpellID = name, spellID
	s.stops++
}

// TestEngine_CustomTimerWithCaptureDuration verifies timer_type "custom"
// dispatches to the custom category and that timer_duration_capture parses
// the captured text into the timer duration, falling back to the fixed
// duration when the capture doesn't parse.
func TestEngine_CustomTimerWithCaptureDuration(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "ct-1", Name: "Manual Timer", Enabled: true,
		Pattern:              `^(.+) sets a (.+) timer\.$`,
		TimerType:            TimerTypeCustom,
		TimerDurationSecs:    60, // fallback
		TimerDurationCapture: "2",
		Actions:              []Action{{Type: ActionOverlayText, Text: "timer set"}},
		CreatedAt:            time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Vortikai sets a 6m40s timer.")
	if sink.calls != 1 || sink.category != "custom" || sink.duration != 400 {
		t.Errorf("capture duration dispatch = %+v, want custom/400", sink)
	}

	// Unparseable capture falls back to the fixed duration.
	e.Handle(time.Now(), "Vortikai sets a banana timer.")
	if sink.calls != 2 || sink.duration != 60 {
		t.Errorf("fallback dispatch = %+v, want duration 60", sink)
	}

	// timer_duration_capture round-trips through the store.
	stored, err := s.Get("ct-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.TimerDurationCapture != "2" || stored.TimerType != TimerTypeCustom {
		t.Errorf("persisted trigger = capture %q type %q", stored.TimerDurationCapture, stored.TimerType)
	}
}

// TestEngine_FractionalTimerDuration verifies a sub-second timer duration
// (e.g. 1.5s, common in EQNag/EQLogParser imports) flows through the engine to
// the timer sink without truncation, both as a fixed value and from a captured
// decimal duration.
func TestEngine_FractionalTimerDuration(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "frac-1", Name: "Quick Pulse", Enabled: true,
		Pattern:              `^(.+) pulses for (.+)\.$`,
		TimerType:            TimerTypeCustom,
		TimerDurationSecs:    1.5, // fallback
		TimerDurationCapture: "2",
		Actions:              []Action{{Type: ActionOverlayText, Text: "pulse"}},
		CreatedAt:            time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Decimal capture parses to a fractional duration.
	e.Handle(time.Now(), "Aura pulses for 2.5.")
	if sink.calls != 1 || sink.duration != 2.5 {
		t.Errorf("captured fractional dispatch = %+v, want duration 2.5", sink)
	}

	// Unparseable capture falls back to the fixed fractional duration.
	e.Handle(time.Now(), "Aura pulses for nothing.")
	if sink.calls != 2 || sink.duration != 1.5 {
		t.Errorf("fallback dispatch = %+v, want duration 1.5", sink)
	}

	// Fractional duration round-trips through the store.
	stored, err := s.Get("frac-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.TimerDurationSecs != 1.5 {
		t.Errorf("persisted TimerDurationSecs = %v, want 1.5", stored.TimerDurationSecs)
	}
}

// TestEngine_BarColorDispatched verifies a trigger's per-trigger BarColor is
// passed through to the timer sink (and round-trips through the store), so the
// overlay can color-code that trigger's bar.
func TestEngine_BarColorDispatched(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "bc-1", Name: "Pet Haste", Enabled: true,
		Pattern:           `^Your pet begins to move faster\.$`,
		TimerType:         TimerTypeBuff,
		TimerDurationSecs: 600,
		BarColor:          "#22c55e",
		Actions:           []Action{{Type: ActionOverlayText, Text: "haste"}},
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Your pet begins to move faster.")
	if sink.calls != 1 || sink.barColor != "#22c55e" {
		t.Errorf("bar color dispatch = %+v, want barColor #22c55e", sink)
	}

	stored, err := s.Get("bc-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.BarColor != "#22c55e" {
		t.Errorf("persisted bar_color = %q, want #22c55e", stored.BarColor)
	}
}

// TestEngine_TimerTargetCapture verifies a buff trigger captures the target
// name into the timer (the "on <target>" overlay suffix) when its "lands on
// other" branch matches, and leaves the target empty for the self-cast branch
// of the same alternation. Mirrors how the built-in Visions of Grandeur pack
// trigger is shaped.
func TestEngine_TimerTargetCapture(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "vog-1", Name: "Visions of Grandeur", Enabled: true,
		Pattern: `^(?:You experience visions of grandeur\.|` +
			`(?P<target>[A-Z][a-zA-Z']{2,14}) experiences visions of grandeur\.)$`,
		TimerType:          TimerTypeBuff,
		TimerDurationSecs:  2520,
		TimerTargetCapture: "target",
		Actions:            []Action{},
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Cast on another player — the named group captures them as the target.
	e.Handle(time.Now(), "Soandso experiences visions of grandeur.")
	if sink.calls != 1 || sink.target != "Soandso" {
		t.Errorf("cast-on-other dispatch = %+v, want target Soandso", sink)
	}

	// Self-cast branch leaves the target group empty → no target suffix.
	e.Handle(time.Now(), "You experience visions of grandeur.")
	if sink.calls != 2 || sink.target != "" {
		t.Errorf("self-cast dispatch = %+v, want empty target", sink)
	}

	// timer_target_capture round-trips through the store.
	stored, err := s.Get("vog-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.TimerTargetCapture != "target" {
		t.Errorf("persisted target capture = %q, want \"target\"", stored.TimerTargetCapture)
	}
}

// TestEngine_ExtraPatterns verifies any-pattern semantics: a trigger fires
// when the primary OR any enabled extra pattern matches, the matching
// pattern's captures feed the actions, disabled extras are ignored, and
// excludes suppress extra-pattern matches too.
func TestEngine_ExtraPatterns(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "multi1",
		Name:    "Tracking",
		Enabled: true,
		Pattern: `^(.+) is ahead and to the left\.$`,
		ExtraPatterns: []ExtraPattern{
			{Pattern: `^(.+) is behind you\.$`, Enabled: true},
			{Pattern: `^(.+) is straight ahead\.$`, Enabled: true},
			{Pattern: `^(.+) is to the right\.$`, Enabled: false}, // toggled off
		},
		ExcludePatterns: []string{`a skeleton`},
		Actions: []Action{
			{Type: ActionOverlayText, Text: "Track: {1}", DurationSecs: 5},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "a gnoll is ahead and to the left.")  // primary
	e.Handle(time.Now(), "a wolf is behind you.")              // extra 1
	e.Handle(time.Now(), "a bear is straight ahead.")          // extra 2
	e.Handle(time.Now(), "a lion is to the right.")            // disabled extra → no fire
	e.Handle(time.Now(), "a skeleton is behind you.")          // excluded → no fire
	e.Handle(time.Now(), "You have entered The North Karana.") // no match

	hist := e.GetHistory()
	if len(hist) != 3 {
		t.Fatalf("expected 3 fires, got %d: %+v", len(hist), hist)
	}
	want := []string{"Track: a gnoll", "Track: a wolf", "Track: a bear"}
	for i, w := range want {
		if got := hist[i].Actions[0].Text; got != w {
			t.Errorf("fire %d action text = %q, want %q", i, got, w)
		}
	}

	// Round-trip: extra patterns survive store persistence.
	stored, err := s.Get("multi1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(stored.ExtraPatterns) != 3 || stored.ExtraPatterns[2].Enabled {
		t.Errorf("extra patterns not persisted faithfully: %+v", stored.ExtraPatterns)
	}
}

// TestEngine_CharacterTokenPattern verifies the end-to-end path: a pattern
// using {c} compiles against the active character and matches a real line,
// and {target}/{c} resolve in action text via the engine's providers.
func TestEngine_CharacterTokenPattern(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, func() string { return "Vortikai" })
	e.SetTargetProvider(func() string { return "a gnoll pup" })

	tr := &Trigger{
		ID:      "tok-1",
		Name:    "Slain",
		Enabled: true,
		Pattern: `^{c} has been slain by (.+)!$`,
		Actions: []Action{
			{Type: ActionTextToSpeech, Text: "{c} died to {S1} while fighting {target}"},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Vortikai has been slain by a gnoll guard!")
	e.Handle(time.Now(), "Someoneelse has been slain by a gnoll guard!") // must NOT fire

	hist := e.GetHistory()
	if len(hist) != 1 {
		t.Fatalf("expected exactly 1 fire, got %d", len(hist))
	}
	want := "Vortikai died to a gnoll guard while fighting a gnoll pup"
	if got := hist[0].Actions[0].Text; got != want {
		t.Errorf("action text = %q, want %q", got, want)
	}
}

func TestEngine_CaptureSubstitutionInActions(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "tell1",
		Name:    "Incoming Tell",
		Enabled: true,
		Pattern: `^(.+) tells you, '(.+)'$`,
		Actions: []Action{
			{Type: ActionTextToSpeech, Text: "Tell from {1}: {2}"},
			{Type: ActionOverlayText, Text: "$1 says hi", DurationSecs: 5},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Bob tells you, 'can you rez me?'")
	hist := e.GetHistory()
	if len(hist) != 1 {
		t.Fatalf("expected 1 fired event, got %d", len(hist))
	}
	if got := hist[0].Actions[0].Text; got != "Tell from Bob: can you rez me?" {
		t.Errorf("TTS text not substituted: %q", got)
	}
	if got := hist[0].Actions[1].Text; got != "Bob says hi" {
		t.Errorf("overlay text not substituted: %q", got)
	}

	// The stored trigger's action text must NOT be mutated by firing.
	stored, _ := s.Get("tell1")
	if stored.Actions[0].Text != "Tell from {1}: {2}" {
		t.Errorf("stored trigger action text was mutated: %q", stored.Actions[0].Text)
	}
}

// TestEngine_MergedTriggerPerPatternTimers verifies a merged spell-line
// trigger: one trigger with one pattern per spell, per-pattern duration and
// spell-id overrides, and timer_key_capture keying the timer by the captured
// spell name so each spell runs an independent countdown and the worn-off
// pattern clears the right one.
func TestEngine_MergedTriggerPerPatternTimers(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "mez-1", Name: "Mez", Enabled: true,
		Pattern:           `^You begin casting (Mesmerize)\.$`,
		TimerType:         TimerTypeDetrimental,
		TimerDurationSecs: 24, // primary pattern's spell
		SpellID:           292,
		TimerKeyCapture:   "1",
		WornOffPattern:    `^Your (Mesmerize|Dazzle|Enthrall) spell has worn off\.$`,
		ExtraPatterns: []ExtraPattern{
			{Pattern: `^You begin casting (Dazzle)\.$`, Enabled: true, TimerDurationSecs: 96, SpellID: 4954},
			{Pattern: `^You begin casting (Enthrall)\.$`, Enabled: true, TimerDurationSecs: 144, SpellID: 367},
		},
		Actions:   []Action{},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	// Primary pattern: trigger-level duration/spell, key from capture.
	e.Handle(time.Now(), "You begin casting Mesmerize.")
	if sink.calls != 1 || sink.name != "Mesmerize" || sink.duration != 24 || sink.spellID != 292 {
		t.Errorf("primary dispatch = %+v, want Mesmerize/24/292", sink)
	}

	// Extra pattern: per-row duration and spell id override the trigger's.
	e.Handle(time.Now(), "You begin casting Dazzle.")
	if sink.calls != 2 || sink.name != "Dazzle" || sink.duration != 96 || sink.spellID != 4954 {
		t.Errorf("extra dispatch = %+v, want Dazzle/96/4954", sink)
	}

	e.Handle(time.Now(), "You begin casting Enthrall.")
	if sink.calls != 3 || sink.name != "Enthrall" || sink.duration != 144 || sink.spellID != 367 {
		t.Errorf("extra dispatch = %+v, want Enthrall/144/367", sink)
	}

	// Worn-off resolves the same capture into the stop key, and passes
	// spellID 0 — the trigger-level SpellID belongs to the primary spell
	// and must not remove a sibling's timer by ID.
	e.Handle(time.Now(), "Your Dazzle spell has worn off.")
	if sink.stops != 1 || sink.stopName != "Dazzle" || sink.stopSpellID != 0 {
		t.Errorf("worn-off stop = %+v, want Dazzle/0", sink)
	}

	// Per-row overrides and the key capture round-trip through the store.
	stored, err := s.Get("mez-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.TimerKeyCapture != "1" {
		t.Errorf("TimerKeyCapture = %q, want \"1\"", stored.TimerKeyCapture)
	}
	if len(stored.ExtraPatterns) != 2 ||
		stored.ExtraPatterns[0].TimerDurationSecs != 96 || stored.ExtraPatterns[0].SpellID != 4954 ||
		stored.ExtraPatterns[1].TimerDurationSecs != 144 || stored.ExtraPatterns[1].SpellID != 367 {
		t.Errorf("ExtraPatterns round-trip = %+v", stored.ExtraPatterns)
	}
}

// TestEngine_WornOffWithoutKeyCapture preserves the legacy clear path: no
// timer_key_capture means the worn-off stops the trigger-name key with the
// trigger's SpellID (which also clears merged spell-landed timers by ID).
func TestEngine_WornOffWithoutKeyCapture(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &captureSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID: "haste-1", Name: "Speed of the Shissar", Enabled: true,
		Pattern:           `^You feel fast\.$`,
		TimerType:         TimerTypeBuff,
		TimerDurationSecs: 600,
		SpellID:           1709,
		WornOffPattern:    `^Your body slows\.$`,
		Actions:           []Action{},
		CreatedAt:         time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	e.Handle(time.Now(), "Your body slows.")
	if sink.stops != 1 || sink.stopName != "Speed of the Shissar" || sink.stopSpellID != 1709 {
		t.Errorf("legacy worn-off stop = %+v, want Speed of the Shissar/1709", sink)
	}
}

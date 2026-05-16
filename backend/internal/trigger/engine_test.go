package trigger

import (
	"path/filepath"
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

func TestEngine_DisabledTriggerDoesNotFire(t *testing.T) {
	s, e := openTestEngine(t)

	tr := &Trigger{
		ID:      "disabled1",
		Name:    "Disabled Trigger",
		Enabled: false,
		Pattern: `You have been slain`,
		Actions: []Action{},
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
		ID:      "upd1",
		Name:    "Tell Alert",
		Enabled: true,
		Pattern: `\w+ tells you,`,
		Actions: []Action{},
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
		ID:      "ring1",
		Name:    "Any Hit",
		Enabled: true,
		Pattern: `damage`,
		Actions: []Action{},
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
		PackName:        "Group Awareness",
		Enabled:         true,
		Pattern:         `\w+ tells you, '\['`, // user-customized: only fire on item-link tells
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
			PackName:              "Group Awareness",
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
			PackName:              "Group Awareness",
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


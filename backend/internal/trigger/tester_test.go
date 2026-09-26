package trigger

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// fakeSink is a minimal TimerSink recorder used to assert whether test runs
// touch the sink only when fire effects are on.
type fakeSink struct {
	started []string
	stopped []string
}

func (f *fakeSink) StartExternal(name, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string, pinned bool, customGroup string, targetIsCaster bool, stack ...bool) {
	f.started = append(f.started, name)
}
func (f *fakeSink) StopExternal(name string, spellID int) {
	f.stopped = append(f.stopped, name)
}

func TestRunTest_CharacterTokenExpansion(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, func() string { return "Cothgrok" })

	tr := &Trigger{
		ID:      "t1",
		Name:    "Self Cast",
		Enabled: true,
		Pattern: `{c} begins to cast a spell\.`,
		Actions: []Action{{Type: ActionOverlayText, Text: "casting!"}},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	report := e.RunTest(TestRequest{Lines: "Cothgrok begins to cast a spell."})
	if report.Matched != 1 {
		t.Fatalf("expected 1 match, got %d (report: %+v)", report.Matched, report)
	}
	if len(report.Lines) != 1 || len(report.Lines[0].Matches) != 1 {
		t.Fatalf("expected 1 line with 1 match, got %+v", report.Lines)
	}
	if got := report.Lines[0].Matches[0].Status; got != LineStatusMatched {
		t.Errorf("expected matched status, got %q", got)
	}

	// Live history must stay untouched by a test run.
	if len(e.GetHistory()) != 0 {
		t.Errorf("test run must not write to trigger history")
	}
}

func TestRunTest_WrongCharacterFiltered(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	tr := &Trigger{
		ID:         "t2",
		Name:       "Alt Only",
		Enabled:    true,
		Pattern:    `Rampage`,
		Actions:    []Action{{Type: ActionOverlayText, Text: "RAMPAGE"}},
		Characters: []string{"SomeoneElse"},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	report := e.RunTest(TestRequest{Lines: "A gnoll goes on a Rampage!", Character: "Cothgrok"})
	if report.Matched != 0 {
		t.Fatalf("expected 0 matches (wrong character), got %d", report.Matched)
	}
	if len(report.Lines[0].Matches) != 1 {
		t.Fatalf("expected the trigger to still be reported, got %+v", report.Lines[0].Matches)
	}
	if got := report.Lines[0].Matches[0].Status; got != LineStatusWrongCharacter {
		t.Errorf("expected wrong_character status, got %q", got)
	}
}

func TestRunTest_ExcludePattern(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	tr := &Trigger{
		ID:              "t3",
		Name:            "Incoming Tell",
		Enabled:         true,
		Pattern:         `tells you,`,
		Actions:         []Action{{Type: ActionOverlayText, Text: "tell!"}},
		ExcludePatterns: []string{`Merchant`},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	report := e.RunTest(TestRequest{Lines: "A Merchant tells you, 'Welcome!'"})
	if report.Excluded != 1 || report.Matched != 0 {
		t.Fatalf("expected 1 excluded / 0 matched, got %+v", report)
	}
	if got := report.Lines[0].Matches[0].Status; got != LineStatusExcluded {
		t.Errorf("expected excluded status, got %q", got)
	}
}

func TestRunTest_RefireCooldownAcrossPastedLines(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	tr := &Trigger{
		ID:                 "t4",
		Name:               "Rampage",
		Enabled:            true,
		Pattern:            `goes on a RAMPAGE`,
		Actions:            []Action{{Type: ActionOverlayText, Text: "RAMP"}},
		RefireCooldownSecs: 10,
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	lines := "[Thu Jun 26 12:00:00 2026] A gnoll goes on a RAMPAGE!\n" +
		"[Thu Jun 26 12:00:03 2026] A gnoll goes on a RAMPAGE!\n" +
		"[Thu Jun 26 12:00:11 2026] A gnoll goes on a RAMPAGE!\n"

	report := e.RunTest(TestRequest{Lines: lines})
	if report.Matched != 2 {
		t.Fatalf("expected 2 matched (first + after cooldown), got %d (report: %+v)", report.Matched, report)
	}
	if report.Cooldowns != 1 {
		t.Fatalf("expected 1 cooldown-suppressed, got %d", report.Cooldowns)
	}

	// A test run must never perturb the live engine's own refire-cooldown
	// state: a real line arriving right after should still fire.
	e.Reload()
	e.Handle(time.Date(2026, 6, 26, 12, 0, 12, 0, time.UTC), "A gnoll goes on a RAMPAGE!")
	if len(e.GetHistory()) != 1 {
		t.Errorf("live engine's refire cooldown must be independent of test-run state, got history %+v", e.GetHistory())
	}
}

func TestRunTest_ExtraPatternMatch(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	tr := &Trigger{
		ID:      "t5",
		Name:    "Mez",
		Enabled: true,
		Pattern: `will never match this literal string`,
		Actions: []Action{{Type: ActionOverlayText, Text: "mezzed"}},
		ExtraPatterns: []ExtraPattern{
			{Pattern: `(?P<target>[A-Za-z]+) is mesmerized\.`, Enabled: true},
		},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	report := e.RunTest(TestRequest{Lines: "Grokii is mesmerized."})
	if report.Matched != 1 {
		t.Fatalf("expected 1 match via extra pattern, got %d (report: %+v)", report.Matched, report)
	}
	m := report.Lines[0].Matches[0]
	if m.PatternLabel != "extra 1" {
		t.Errorf("expected pattern_label %q, got %q", "extra 1", m.PatternLabel)
	}
	if m.Captures["target"] != "Grokii" {
		t.Errorf("expected captured target %q, got %+v", "Grokii", m.Captures)
	}
}

func TestRunTest_DraftTriggerInvalidPattern(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	e := NewEngine(s, hub, nil, nil)

	draft := &Trigger{
		ID:      "draft1",
		Name:    "Draft",
		Enabled: false,
		Pattern: `(unclosed`,
		Actions: []Action{{Type: ActionOverlayText, Text: "x"}},
	}

	report := e.RunTest(TestRequest{Lines: "anything", Trigger: draft})
	if len(report.Errors) == 0 {
		t.Fatalf("expected a pattern-compile error, got none: %+v", report)
	}
	if len(report.Lines) != 0 {
		t.Errorf("expected no line results for an uncompilable draft, got %+v", report.Lines)
	}
}

func TestRunTest_FireEffectsOffDoesNotTouchSink(t *testing.T) {
	s := openTestStore(t)
	hub := ws.NewHub()
	sink := &fakeSink{}
	e := NewEngine(s, hub, sink, nil)

	tr := &Trigger{
		ID:                "t6",
		Name:              "Slow Timer",
		Enabled:           true,
		Pattern:           `is slowed\.`,
		Actions:           []Action{{Type: ActionOverlayText, Text: "slowed"}},
		TimerType:         TimerTypeDetrimental,
		TimerDurationSecs: 30,
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	report := e.RunTest(TestRequest{Lines: "A gnoll is slowed.", FireEffects: false})
	if report.Matched != 1 || report.Fired != 0 {
		t.Fatalf("expected 1 matched / 0 fired, got %+v", report)
	}
	if len(sink.started) != 0 {
		t.Errorf("fire effects off must not start a real timer, got %+v", sink.started)
	}
	if report.Lines[0].Matches[0].Timer == nil {
		t.Errorf("expected the timer info to still be reported even with fire effects off")
	}

	report2 := e.RunTest(TestRequest{Lines: "A gnoll is slowed.", FireEffects: true})
	if report2.Fired != 1 {
		t.Fatalf("expected 1 fired with fire effects on, got %+v", report2)
	}
	if len(sink.started) != 1 {
		t.Errorf("expected fire effects on to start a real timer, got %+v", sink.started)
	}
	// Still must never write to history or post webhooks.
	if len(e.GetHistory()) != 0 {
		t.Errorf("test fires must never write to trigger history, even with fire effects on")
	}
}

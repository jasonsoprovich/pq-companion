package trigger

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompilePattern(t *testing.T) {
	tests := []struct {
		name      string
		expr      string
		line      string
		backtrack bool
		want      []string // nil = no match
		names     []string
	}{
		{"RE2 stays on RE2", `^(\w+) tells you, '(.+)'$`, "Soandso tells you, 'hi'",
			false, []string{"Soandso tells you, 'hi'", "Soandso", "hi"}, []string{"", "", ""}},
		{"negative lookahead", `^(\w+) tells you, '(?!Master)(.+)'$`, "Soandso tells you, 'hi'",
			true, []string{"Soandso tells you, 'hi'", "Soandso", "hi"}, []string{"", "", ""}},
		{"negative lookahead rejects", `^(\w+) tells you, '(?!Master)(.+)'$`, "Pet tells you, 'Master, I attack'",
			true, nil, []string{"", "", ""}},
		{"lookbehind", `(?<=slain by )(\w+)`, "You have been slain by Vox!",
			true, []string{"Vox", "Vox"}, []string{"", ""}},
		{"backreference", `^(\w+) hits \1\b`, "Bob hits Bob for 5 points",
			true, []string{"Bob hits Bob", "Bob"}, []string{"", ""}},
		// Named and unnamed groups keep RE2's left-to-right numbering.
		{"named group order", `^(?P<who>\w+) (\w+) (?!x)(?P<what>\w+)`, "a b c",
			true, []string{"a b c", "a", "b", "c"}, []string{"", "who", "", "what"}},
		{"group not taking part", `^(?=a)(a)(b)?`, "a",
			true, []string{"a", "a", ""}, []string{"", "", ""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := compilePattern(tc.expr)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if p.Backtracking() != tc.backtrack {
				t.Fatalf("Backtracking = %v, want %v", p.Backtracking(), tc.backtrack)
			}
			if got := p.FindStringSubmatch(tc.line); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("match = %q, want %q", got, tc.want)
			}
			if got := p.MatchString(tc.line); got != (tc.want != nil) {
				t.Fatalf("MatchString = %v", got)
			}
			if got := p.SubexpNames(); !reflect.DeepEqual(got, tc.names) {
				t.Fatalf("names = %q, want %q", got, tc.names)
			}
		})
	}
}

func TestCompilePatternRejectsBroken(t *testing.T) {
	if _, err := compilePattern(`(unclosed`); err == nil {
		t.Fatal("expected an error")
	}
	if validatePattern(`(unclosed`) {
		t.Fatal("validatePattern accepted a broken pattern")
	}
	if !validatePattern(`(?<!pet )tells you`) {
		t.Fatal("validatePattern rejected a lookbehind pattern")
	}
}

func TestPatternTimeoutSwitchesOff(t *testing.T) {
	// The backreference forces the fallback engine; (a+)+ backtracks
	// exponentially once the line fails to end in a repeat.
	p, err := compilePattern(`^(a+)+\1$`)
	if err != nil {
		t.Fatal(err)
	}
	p.bt.MatchTimeout = time.Millisecond
	line := strings.Repeat("a", 40) + "!"
	for i := 0; i < backtrackMaxTimeouts; i++ {
		p.MatchString(line)
	}
	if !p.disabled() {
		t.Fatalf("timeouts = %d, want %d", p.timeouts.Load(), backtrackMaxTimeouts)
	}
	start := time.Now()
	if p.MatchString(line) || p.FindStringSubmatch(line) != nil || time.Since(start) > 5*time.Millisecond {
		t.Fatal("switched-off pattern still ran")
	}
}

func TestEngineFiresOnLookaroundPattern(t *testing.T) {
	s, e := openTestEngine(t)
	tr := &Trigger{
		ID:        "tell",
		Name:      "tell",
		Enabled:   true,
		Pattern:   `^(?<who>\w+) tells you, '(?!Master)`,
		Actions:   []Action{{Type: ActionOverlayText, Text: "{who} sent a tell"}},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Insert(tr); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.Handle(time.Now(), "Pet tells you, 'Master, I attack'")
	e.Handle(time.Now(), "Soandso tells you, 'inv pls'")
	hist := e.GetHistory()
	if len(hist) != 1 || hist[0].Actions[0].Text != "Soandso sent a tell" {
		t.Fatalf("history = %+v", hist)
	}
}

func TestMigrateEnableBacktrackingPatterns(t *testing.T) {
	s := openTestStore(t)
	add := func(id, pattern string, enabled bool) {
		t.Helper()
		if err := s.Insert(&Trigger{ID: id, Name: id, Enabled: enabled, Pattern: pattern,
			Actions: []Action{}, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	add("lookaround", `tells you, '(?!Master)`, false)
	add("plain-off", `tells you`, false)   // user switched it off
	add("broken", `(unclosed(?!x)`, false) // still doesn't compile
	n, err := s.MigrateEnableBacktrackingPatterns()
	if err != nil || n != 1 {
		t.Fatalf("enabled %d, err %v", n, err)
	}
	all, _ := s.List()
	for _, tr := range all {
		if tr.Enabled != (tr.ID == "lookaround") {
			t.Errorf("%s enabled = %v", tr.ID, tr.Enabled)
		}
	}
	// Runs once.
	if err := s.Update(&Trigger{ID: "lookaround", Name: "lookaround", Pattern: `tells you, '(?!Master)`,
		Actions: []Action{}}); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.MigrateEnableBacktrackingPatterns(); n != 0 {
		t.Fatalf("second run enabled %d", n)
	}
}

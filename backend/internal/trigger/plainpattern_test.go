package trigger

import (
	"strings"
	"testing"
)

func TestPlainToRegex(t *testing.T) {
	tests := []struct {
		plain, want, err string
	}{
		{plain: "You have been slain", want: `You have been slain`},
		{plain: "Cast it. (now?)", want: `Cast it\. \(now\?\)`},
		{plain: "{name} tells you, {msg}", want: `(?P<name>.+?) tells you, (?P<msg>.+)`},
		{plain: "{who} begins to cast a spell.", want: `(?P<who>.+?) begins to cast a spell\.`},
		{plain: "{C} has fallen", want: `{C} has fallen`},
		{plain: "{S} hits {char} for {N} points", want: `{S} hits {char} for {N} points`},
		{plain: "a {{brace}} here", want: `a \{brace\} here`},
		{plain: "", err: "empty"},
		{plain: "{who} and {who}", err: "more than once"},
		{plain: "open {name", err: "unclosed"},
		{plain: "stray } brace", err: "unmatched"},
		{plain: "{1bad}", err: "valid name"},
		{plain: "{}", err: "valid name"},
	}
	for _, tc := range tests {
		got, err := PlainToRegex(tc.plain)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("PlainToRegex(%q) err = %v, want containing %q", tc.plain, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("PlainToRegex(%q) = %q, %v; want %q", tc.plain, got, err, tc.want)
		}
	}
}

func TestRegexToPlainRoundTrip(t *testing.T) {
	ok := []string{
		"You have been slain",
		"Cast it. (now?)",
		"{name} tells you, {msg}",
		"{C} has fallen to the ground.",
		"{S} hits {char} for {N} points",
		"a {{brace}} here",
		"Your $5 item [rare] costs ^much^ | maybe",
	}
	for _, plain := range ok {
		re, err := PlainToRegex(plain)
		if err != nil {
			t.Fatalf("PlainToRegex(%q): %v", plain, err)
		}
		back, err := RegexToPlain(re)
		if err != nil || back != plain {
			t.Errorf("RegexToPlain(%q) = %q, %v; want %q", re, back, err, plain)
		}
	}

	notPlain := []string{
		`^You have been slain`,
		`\w+ has been slain by`,
		`(.+) tells you`,
		`You (hit|slash) it`,
		`\d{2} points`,
		`[Aa]bc`,
		`(?P<who>.+) tells you, (?P<msg>.+)`, // greedy mid-pattern capture: re-emits as lazy
		`(?P<x>[0-9]+) points`,
		`trailing\`,
	}
	for _, re := range notPlain {
		if got, err := RegexToPlain(re); err == nil {
			t.Errorf("RegexToPlain(%q) = %q, want error", re, got)
		}
	}
}

// Generated patterns compile through the engine's path and match, and named
// captures feed {name}/{target} substitution.
func TestPlainPatternMatchesAndSubstitutes(t *testing.T) {
	cases := []struct {
		plain, line string
		captures    map[string]string
	}{
		{
			plain:    "{target} has been mesmerized.",
			line:     "A gnoll pup has been mesmerized.",
			captures: map[string]string{"target": "A gnoll pup"},
		},
		{
			plain:    "{name} tells you, {msg}",
			line:     "Soandso tells you, 'need a port, 50pp?'",
			captures: map[string]string{"name": "Soandso", "msg": "'need a port, 50pp?'"},
		},
		{
			plain:    "Your {spell} spell has worn off.",
			line:     "Your Tashanian spell has worn off.",
			captures: map[string]string{"spell": "Tashanian"},
		},
	}
	for _, tc := range cases {
		re, err := PlainToRegex(tc.plain)
		if err != nil {
			t.Fatalf("PlainToRegex(%q): %v", tc.plain, err)
		}
		p, err := compilePattern(normalizePattern(re, "Osui"))
		if err != nil {
			t.Fatalf("compilePattern(%q): %v", re, err)
		}
		m := p.FindStringSubmatch(tc.line)
		if m == nil {
			t.Fatalf("%q didn't match %q", re, tc.line)
		}
		names := p.SubexpNames()
		for name, want := range tc.captures {
			got := substituteCaptures("{"+name+"}", m, names, map[string]string{"target": "builtin"})
			if got != want {
				t.Errorf("%q: {%s} = %q, want %q", tc.plain, name, got, want)
			}
		}
	}

	// Reserved tokens still expand: {C} becomes the character name.
	re, _ := PlainToRegex("{C} has fallen to the ground.")
	p, err := compilePattern(normalizePattern(re, "Osui"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.MatchString("Osui has fallen to the ground.") || p.MatchString("Bob has fallen to the ground.") {
		t.Errorf("{C} didn't expand to the character name: %q", re)
	}
}

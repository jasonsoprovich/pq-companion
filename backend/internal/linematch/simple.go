// Package linematch holds the log-line matching pieces shared by features
// that let users write their own line rules (Custom Overlays, Chat Windows):
// the plain-text "Simple" pattern builder, the chat-line classifier, and the
// active-character name token.
package linematch

import (
	"fmt"
	"regexp"
	"strings"
)

// Simple match kinds (Simple.Kind).
const (
	SimpleStarts   = "starts"   // the message starts with the text
	SimpleContains = "contains" // the text appears anywhere
	SimpleExact    = "exact"    // the whole message is the text
)

// MaxPatternLen bounds a pattern or a Simple match's text.
const MaxPatternLen = 1000

// Simple is the beginner-friendly way to write a rule's pattern: plain text
// with {placeholders}, e.g. "SOLD! {item} to {winner} for {amount:number}".
// It is turned into a regex, so engines only ever see a regex, and editors
// round-trip what the author typed.
//
// Matching is case-insensitive and any run of spaces matches any run of
// whitespace. A placeholder captures as a named group. Types: text (default)
// is any characters, number is digits with optional thousands commas, word is
// one run of non-spaces.
type Simple struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Options are opt-in extensions to the Simple syntax. Custom Overlays use
// none of them, so their existing rules keep their meaning.
type Options struct {
	// Wildcards makes * match any run of characters.
	Wildcards bool
	// SelfToken makes an untyped {c}, {char} or {self} match the active
	// character's name (Self) instead of capturing. With no Self it matches
	// nothing, so a "mentions me" rule can't match every line.
	SelfToken bool
	Self      string
}

var (
	placeholderRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?::([A-Za-z]+))?\}`)
	spacesRe      = regexp.MustCompile(` +`)
)

// matchNothing is an RE2 expression that never matches (RE2 has no (?!)).
const matchNothing = `[^\s\S]`

// literal escapes plain text for a regex, letting spaces stretch and, with
// wildcards, * match anything.
func literal(s string, opts Options) string {
	if !opts.Wildcards {
		return spacesRe.ReplaceAllString(regexp.QuoteMeta(s), `\s+`)
	}
	parts := strings.Split(s, "*")
	for i, p := range parts {
		parts[i] = spacesRe.ReplaceAllString(regexp.QuoteMeta(p), `\s+`)
	}
	return strings.Join(parts, `.*?`)
}

// IsSelfToken reports whether a placeholder name is the active-character
// token.
func IsSelfToken(name string) bool {
	switch strings.ToLower(name) {
	case "c", "char", "self":
		return true
	}
	return false
}

// SimplePattern builds the regex for m with no extensions.
func SimplePattern(m Simple) (string, error) {
	return SimplePatternWith(m, Options{})
}

// SimplePatternWith builds the regex for m.
func SimplePatternWith(m Simple, opts Options) (string, error) {
	switch m.Kind {
	case SimpleStarts, SimpleContains, SimpleExact:
	default:
		return "", fmt.Errorf("unknown match kind %q", m.Kind)
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return "", fmt.Errorf("match text is required")
	}
	if len(text) > MaxPatternLen {
		return "", fmt.Errorf("match text is longer than %d characters", MaxPatternLen)
	}
	var b strings.Builder
	b.WriteString("(?i)")
	if m.Kind != SimpleContains {
		b.WriteString("^")
	}
	locs := placeholderRe.FindAllStringSubmatchIndex(text, -1)
	seen := map[string]bool{}
	last := 0
	for i, loc := range locs {
		b.WriteString(literal(text[last:loc[0]], opts))
		last = loc[1]
		name := text[loc[2]:loc[3]]
		if opts.SelfToken && loc[4] < 0 && IsSelfToken(name) {
			if opts.Self == "" {
				b.WriteString(matchNothing)
			} else {
				b.WriteString(regexp.QuoteMeta(opts.Self))
			}
			continue
		}
		typ := "text"
		if loc[4] >= 0 {
			typ = strings.ToLower(text[loc[4]:loc[5]])
		}
		if seen[strings.ToLower(name)] {
			return "", fmt.Errorf("placeholder {%s} is used twice", name)
		}
		seen[strings.ToLower(name)] = true
		var expr string
		switch typ {
		case "number":
			expr = `\d[\d,]*`
		case "word":
			expr = `\S+`
		case "text":
			// A trailing text placeholder takes the rest of the line; any
			// other is lazy so the literal after it decides where it stops.
			if i == len(locs)-1 && loc[1] == len(text) {
				expr = `.+`
			} else {
				expr = `.+?`
			}
		default:
			return "", fmt.Errorf("placeholder {%s:%s}: type must be text, number or word", name, typ)
		}
		fmt.Fprintf(&b, "(?P<%s>%s)", name, expr)
	}
	b.WriteString(literal(text[last:], opts))
	if m.Kind == SimpleExact {
		b.WriteString("$")
	}
	return b.String(), nil
}

// selfTokenRe finds {c}, {char} and {self} in a regex, case-insensitively.
var selfTokenRe = regexp.MustCompile(`(?i)\{(?:c|char|self)\}`)

// ExpandSelf replaces {c}, {char} and {self} in a regex with the active
// character's name. With no name they match nothing.
func ExpandSelf(pattern, self string) string {
	repl := matchNothing
	if self != "" {
		repl = regexp.QuoteMeta(self)
	}
	return selfTokenRe.ReplaceAllLiteralString(pattern, repl)
}

package customoverlay

import (
	"fmt"
	"regexp"
	"strings"
)

// Simple-mode match kinds (SimpleMatch.Kind).
const (
	SimpleStarts   = "starts"   // the message starts with the text
	SimpleContains = "contains" // the text appears anywhere
	SimpleExact    = "exact"    // the whole message is the text
)

// SimpleMatch is the beginner-friendly way to write a rule's pattern: plain
// text with {placeholders}, e.g. "SOLD! {item} to {winner} for
// {amount:number}". Validate turns it into Rule.Pattern, so the engine only
// ever sees a regex, and the editor round-trips what the author typed.
//
// Matching is case-insensitive and any run of spaces matches any run of
// whitespace. A placeholder captures as a named group usable in actions as
// {name}. Types: text (default) is any characters, number is digits with
// optional thousands commas, word is one run of non-spaces.
type SimpleMatch struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

var (
	placeholderRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?::([A-Za-z]+))?\}`)
	spacesRe      = regexp.MustCompile(` +`)
)

// literal escapes plain text for a regex, letting spaces stretch.
func literal(s string) string {
	return spacesRe.ReplaceAllString(regexp.QuoteMeta(s), `\s+`)
}

// simplePattern builds the regex for a SimpleMatch.
func simplePattern(m SimpleMatch) (string, error) {
	switch m.Kind {
	case SimpleStarts, SimpleContains, SimpleExact:
	default:
		return "", fmt.Errorf("unknown match kind %q", m.Kind)
	}
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return "", fmt.Errorf("match text is required")
	}
	if len(text) > maxPatternLen {
		return "", fmt.Errorf("match text is longer than %d characters", maxPatternLen)
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
		b.WriteString(literal(text[last:loc[0]]))
		name := text[loc[2]:loc[3]]
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
		last = loc[1]
	}
	b.WriteString(literal(text[last:]))
	if m.Kind == SimpleExact {
		b.WriteString("$")
	}
	return b.String(), nil
}

package trigger

import (
	"fmt"
	"regexp"
	"strings"
)

// Plain-text trigger patterns: a friendlier way to write a pattern without
// regex. The trigger always stores the regex; plain text is only an editing
// view, converted by PlainToRegex and (when exactly reversible) back by
// RegexToPlain.
//
// Grammar:
//
//	literal text      matched exactly (regexp.QuoteMeta)
//	{name}            captures any text as a named group. The last capture
//	                  is greedy, (?P<name>.+), so it runs to the last match
//	                  of the text after it ("{name} tells you, '{msg}'"
//	                  keeps apostrophes inside msg); earlier captures are
//	                  lazy, (?P<name>.+?). name is [A-Za-z_][A-Za-z0-9_]*
//	                  and may appear only once.
//	{C} {char} {self} the active character's name   } passed through as-is;
//	{groupmember}     a player in your group         }
//	{S} {S1}…{S9}     any text                       } normalizePattern
//	{N} {N1}…{N9}     a number                       } expands them
//	{{  }}            literal { and }
//
// Patterns aren't anchored, the same as regex triggers.

var plainNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// isReservedToken reports whether name is one of the GINA-style tokens
// normalizePattern expands (case-insensitive), which plain mode passes
// through instead of turning into a capture.
func isReservedToken(name string) bool {
	switch strings.ToLower(name) {
	case "c", "char", "self", groupMemberToken:
		return true
	}
	up := strings.ToUpper(name)
	if len(up) == 0 || len(up) > 2 || (up[0] != 'S' && up[0] != 'N') {
		return false
	}
	return len(up) == 1 || (up[1] >= '1' && up[1] <= '9')
}

type plainPart struct {
	literal string // set for literal runs
	token   string // set for {name} tokens
}

func parsePlain(plain string) ([]plainPart, error) {
	var parts []plainPart
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			parts = append(parts, plainPart{literal: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(plain); i++ {
		c := plain[i]
		switch c {
		case '{':
			if i+1 < len(plain) && plain[i+1] == '{' {
				lit.WriteByte('{')
				i++
				continue
			}
			end := strings.IndexByte(plain[i+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("unclosed { at position %d (use {{ for a literal brace)", i+1)
			}
			name := plain[i+1 : i+1+end]
			if !plainNameRe.MatchString(name) {
				return nil, fmt.Errorf("{%s} isn't a valid name: use letters, digits and _ (use {{ for a literal brace)", name)
			}
			flush()
			parts = append(parts, plainPart{token: name})
			i += end + 1
		case '}':
			if i+1 < len(plain) && plain[i+1] == '}' {
				lit.WriteByte('}')
				i++
				continue
			}
			return nil, fmt.Errorf("unmatched } at position %d (use }} for a literal brace)", i+1)
		default:
			lit.WriteByte(c)
		}
	}
	flush()
	return parts, nil
}

// PlainToRegex converts a plain-text pattern to the regex stored on the
// trigger.
func PlainToRegex(plain string) (string, error) {
	if strings.TrimSpace(plain) == "" {
		return "", fmt.Errorf("pattern is empty")
	}
	parts, err := parsePlain(plain)
	if err != nil {
		return "", err
	}
	lastCapture := -1
	for i, p := range parts {
		if p.token != "" && !isReservedToken(p.token) {
			lastCapture = i
		}
	}
	seen := map[string]bool{}
	var b strings.Builder
	for i, p := range parts {
		if p.token == "" {
			b.WriteString(regexp.QuoteMeta(p.literal))
			continue
		}
		if isReservedToken(p.token) {
			b.WriteString("{" + p.token + "}")
			continue
		}
		if seen[p.token] {
			return "", fmt.Errorf("{%s} is used more than once", p.token)
		}
		seen[p.token] = true
		if i == lastCapture {
			b.WriteString("(?P<" + p.token + ">.+)")
		} else {
			b.WriteString("(?P<" + p.token + ">.+?)")
		}
	}
	return b.String(), nil
}

// regexMeta is the set of characters regexp.QuoteMeta escapes.
const regexMeta = `\.+*?()|[]{}^$`

var captureAtRe = regexp.MustCompile(`^\(\?P<([A-Za-z_][A-Za-z0-9_]*)>\.\+\??\)`)
var reservedAtRe = regexp.MustCompile(`^\{([A-Za-z][A-Za-z0-9_]*)\}`)

// RegexToPlain converts a regex back to plain text, but only when the
// result converts back to exactly the same regex. Anything else (character
// classes, alternation, anchors, numbered groups…) returns an error naming
// why, and the regex must be edited as regex.
func RegexToPlain(re string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(re); {
		c := re[i]
		switch {
		case c == '\\':
			if i+1 >= len(re) || !strings.ContainsRune(regexMeta, rune(re[i+1])) {
				return "", fmt.Errorf("uses a regex escape (%s)", safeSlice(re, i, 2))
			}
			switch re[i+1] {
			case '{':
				b.WriteString("{{")
			case '}':
				b.WriteString("}}")
			default:
				b.WriteByte(re[i+1])
			}
			i += 2
		case c == '(':
			m := captureAtRe.FindStringSubmatch(re[i:])
			if m == nil {
				return "", fmt.Errorf("uses a regex group")
			}
			b.WriteString("{" + m[1] + "}")
			i += len(m[0])
		case c == '{':
			m := reservedAtRe.FindStringSubmatch(re[i:])
			if m == nil || !isReservedToken(m[1]) {
				return "", fmt.Errorf("uses regex repetition or braces")
			}
			b.WriteString(m[0])
			i += len(m[0])
		case strings.ContainsRune(regexMeta, rune(c)):
			return "", fmt.Errorf("uses regex syntax (%c)", c)
		default:
			b.WriteByte(c)
			i++
		}
	}
	plain := b.String()
	back, err := PlainToRegex(plain)
	if err != nil {
		return "", err
	}
	if back != re {
		return "", fmt.Errorf("can't be shown as plain text without changing it")
	}
	return plain, nil
}

func safeSlice(s string, i, n int) string {
	if i+n > len(s) {
		return s[i:]
	}
	return s[i : i+n]
}

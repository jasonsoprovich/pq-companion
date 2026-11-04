package trigger

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dlclark/regexp2"
)

const (
	// backtrackMatchTimeout caps one match of a fallback (backtracking)
	// pattern against one log line. A badly written lookaround pattern can
	// backtrack for seconds on a long line; this keeps one bad trigger from
	// stalling the log pipeline.
	backtrackMatchTimeout = 25 * time.Millisecond

	// backtrackMaxTimeouts is how many timed-out matches a fallback pattern
	// gets before it is switched off until the next reload.
	backtrackMaxTimeouts = 5

	// backtrackClockPeriod is how often regexp2 checks its timeout clock.
	// The default (100 ms) lets a match overrun its timeout by up to two
	// periods; 10 ms keeps a runaway match near backtrackMatchTimeout.
	backtrackClockPeriod = 10 * time.Millisecond
)

func init() {
	regexp2.SetTimeoutCheckPeriod(backtrackClockPeriod)
}

// pattern is a compiled trigger regex. Patterns compile with Go's RE2 engine
// when they can. Patterns RE2 rejects (lookahead, lookbehind,
// backreferences, all common in GINA and EQLogParser exports, which use .NET
// regex) fall back to regexp2, a backtracking engine, in its RE2
// compatibility mode, with groups numbered left to right as RE2 does (see
// nameCaptureGroups).
type pattern struct {
	src   string
	re    *regexp.Regexp
	bt    *regexp2.Regexp
	nums  []int    // bt only: regexp2 group number for each RE2-order index
	names []string // SubexpNames-style: index -> name, "" for unnamed

	timeouts atomic.Int32
}

// compilePattern compiles expr with RE2, falling back to regexp2 when RE2
// rejects it. The returned error is RE2's when both engines reject expr,
// since that's the message users have seen before.
func compilePattern(expr string) (*pattern, error) {
	re, err := regexp.Compile(expr)
	if err == nil {
		return &pattern{src: expr, re: re, names: re.SubexpNames()}, nil
	}
	named, groups := nameCaptureGroups(expr)
	bt, btErr := regexp2.Compile(named, regexp2.RE2)
	if btErr != nil {
		return nil, err
	}
	bt.MatchTimeout = backtrackMatchTimeout
	p := &pattern{src: expr, bt: bt}
	p.nums = make([]int, len(groups)+1) // nums[0] = 0, the whole match
	p.names = make([]string, len(groups)+1)
	for i, name := range groups {
		p.nums[i+1] = bt.GroupNumberFromName(name)
		if !strings.HasPrefix(name, autoGroupPrefix) {
			p.names[i+1] = name
		}
	}
	return p, nil
}

// autoGroupPrefix names the unnamed groups nameCaptureGroups rewrites.
const autoGroupPrefix = "__g"

// nameCaptureGroups gives every unnamed capture group in expr a generated
// name and returns the rewritten pattern plus every capture group's name in
// the order the groups open. regexp2 numbers unnamed groups before named
// ones (.NET order) even in RE2 mode, so {1}-style references would point
// at a different group than under RE2; looking groups up by name in opening
// order restores RE2's left-to-right numbering.
func nameCaptureGroups(expr string) (string, []string) {
	var b strings.Builder
	var groups []string
	inClass := false
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case c == '\\':
			b.WriteByte(c)
			if i+1 < len(expr) {
				i++
				b.WriteByte(expr[i])
			}
			continue
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
			b.WriteByte(c)
			// A ']' first in the class (after an optional '^') is literal.
			if i+1 < len(expr) && expr[i+1] == '^' {
				i++
				b.WriteByte(expr[i])
			}
			if i+1 < len(expr) && expr[i+1] == ']' {
				i++
				b.WriteByte(expr[i])
			}
			continue
		case c == '(' && strings.HasPrefix(expr[i:], "(?"):
			if name, ok := groupName(expr[i:]); ok {
				groups = append(groups, name)
			}
		case c == '(':
			name := autoGroupPrefix + strconv.Itoa(len(groups)+1)
			groups = append(groups, name)
			b.WriteString("(?P<" + name + ">")
			continue
		}
		b.WriteByte(c)
	}
	return b.String(), groups
}

// groupName returns the name of a named group opening at the start of s:
// (?P<name>, (?<name> or (?'name'. Lookbehinds ((?<= and (?<!) aren't
// names.
func groupName(s string) (string, bool) {
	var rest string
	var end byte
	switch {
	case strings.HasPrefix(s, "(?P<"):
		rest, end = s[4:], '>'
	case strings.HasPrefix(s, "(?<") && !strings.HasPrefix(s, "(?<=") && !strings.HasPrefix(s, "(?<!"):
		rest, end = s[3:], '>'
	case strings.HasPrefix(s, "(?'"):
		rest, end = s[3:], '\''
	default:
		return "", false
	}
	j := strings.IndexByte(rest, end)
	if j <= 0 {
		return "", false
	}
	return rest[:j], true
}

// Backtracking reports whether the pattern runs on the fallback engine.
func (p *pattern) Backtracking() bool { return p.bt != nil }

// String returns the source text of the pattern.
func (p *pattern) String() string { return p.src }

// SubexpNames mirrors regexp.Regexp.SubexpNames.
func (p *pattern) SubexpNames() []string { return p.names }

// MatchString mirrors regexp.Regexp.MatchString.
func (p *pattern) MatchString(s string) bool {
	if p.re != nil {
		return p.re.MatchString(s)
	}
	if p.disabled() {
		return false
	}
	ok, err := p.bt.MatchString(s)
	if err != nil {
		p.noteTimeout(err)
		return false
	}
	return ok
}

// FindStringSubmatch mirrors regexp.Regexp.FindStringSubmatch: nil for no
// match, otherwise the whole match at index 0 and each group at its number,
// with "" for a group that didn't take part.
func (p *pattern) FindStringSubmatch(s string) []string {
	if p.re != nil {
		return p.re.FindStringSubmatch(s)
	}
	if p.disabled() {
		return nil
	}
	m, err := p.bt.FindStringMatch(s)
	if err != nil {
		p.noteTimeout(err)
		return nil
	}
	if m == nil {
		return nil
	}
	out := make([]string, len(p.names))
	for i, n := range p.nums {
		if g := m.GroupByNumber(n); g != nil && len(g.Captures) > 0 {
			out[i] = g.String()
		}
	}
	return out
}

func (p *pattern) disabled() bool {
	return p.timeouts.Load() >= backtrackMaxTimeouts
}

func (p *pattern) noteTimeout(err error) {
	n := p.timeouts.Add(1)
	if n == backtrackMaxTimeouts {
		slog.Warn("trigger: pattern keeps timing out, switched off until the next reload",
			"pattern", p.src, "timeout", backtrackMatchTimeout, "err", err)
	}
}

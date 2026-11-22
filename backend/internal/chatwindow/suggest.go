package chatwindow

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/linematch"
)

// Suggest modes.
const (
	SuggestHide = "hide"
	SuggestKeep = "keep"
)

// defaultKeepColor highlights a kept line until the user picks a color.
const defaultKeepColor = "#facc15"

// tokenRe finds the parts of a line that change between otherwise identical
// lines: numbers, and words (which may be player names).
var tokenRe = regexp.MustCompile(`\d[\d,]*|[A-Za-z][A-Za-z` + "`" + `']*`)

// notNames are capitalized words that start EQ lines but are never a player.
var notNames = map[string]bool{
	"you": true, "your": true, "a": true, "an": true, "the": true, "it": true,
	"they": true, "he": true, "she": true, "players": true, "there": true,
}

// SetNameSource gives Suggest a lookup for known player names (seen
// players, the raid roster). Optional; called only while building a
// suggestion, never per log line.
func (e *Engine) SetNameSource(fn func(name string) bool) {
	e.mu.Lock()
	e.names = fn
	e.mu.Unlock()
}

// Suggest builds a keep or hide rule from one of a window's stored lines
// (by seq, from its buffer or pins), ignoring the parts that change: numbers
// and player names become placeholders. A chat line gets a rule on its
// channel matching the message; any other line, a rule on the whole line.
// It reports how many of the window's lines the rule matches.
func (e *Engine) Suggest(id string, seq int64, mode string) (Suggestion, error) {
	if mode != SuggestHide && mode != SuggestKeep {
		return Suggestion{}, fmt.Errorf("mode must be hide or keep")
	}
	e.mu.Lock()
	w := e.findLocked(id)
	if w == nil {
		e.mu.Unlock()
		return Suggestion{}, ErrNotFound
	}
	var line *Line
	for i := range w.State.Lines {
		if w.State.Lines[i].Seq == seq {
			l := w.State.Lines[i]
			line = &l
			break
		}
	}
	if line == nil {
		for _, p := range w.State.Pins {
			if p.Seq == seq {
				l := p.Line
				line = &l
				break
			}
		}
	}
	lines := cloneWindow(w).State.Lines // what the window shows
	namesFn := e.names
	self := e.compSelf
	e.mu.Unlock()
	if line == nil {
		return Suggestion{}, ErrNotFound
	}

	names := map[string]bool{}
	for _, n := range []string{line.Speaker, line.Peer, line.Character, self} {
		if n != "" {
			names[strings.ToLower(n)] = true
		}
	}

	known := func(n string) bool {
		if names[n] {
			return true
		}
		if namesFn == nil {
			return false
		}
		ok := namesFn(n)
		names[n] = ok
		return ok
	}
	rule := suggestRule(*line, known, self)
	if mode == SuggestKeep {
		rule.Color = defaultKeepColor
	}
	rule.Name = suggestName(mode, *line)
	rule.Enabled = true

	s := Suggestion{Rule: rule}
	if cr, err := compileRule(rule, self); err == nil {
		for i := range lines {
			if cr.match(&lines[i]) {
				s.Matches++
			}
		}
	}
	return s, nil
}

func suggestName(mode string, l Line) string {
	verb := "Hide"
	if mode == SuggestKeep {
		verb = "Keep"
	}
	text := l.Text
	if l.isChat() {
		text = l.Message
	}
	if len(text) > 40 {
		text = text[:40] + "…"
	}
	return verb + ": " + text
}

// suggestRule builds the rule for one line and checks that it matches the
// line it came from, falling back to the line's exact text if not.
func suggestRule(l Line, known func(string) bool, self string) Rule {
	target := l.Text
	r := Rule{Mode: MatchSimple, Kind: linematch.SimpleExact}
	if l.isChat() {
		target = l.Message
		r.Channel = l.Channel
	}
	// Simple text can't hold a literal * or braces; use a regex instead.
	regex := strings.ContainsAny(target, "*{}")
	r.Text = templatize(target, known, !l.isChat(), regex)
	if regex {
		r.Mode = MatchRegex
		r.Kind = ""
	}
	if cr, err := compileRule(r, self); err != nil || !cr.match(&l) {
		r.Mode, r.Kind = MatchRegex, ""
		r.Text = "(?i)^" + regexp.QuoteMeta(target) + "$"
	}
	return r
}

// templatize turns the changing parts of text into placeholders (Simple) or
// expressions (regex): numbers, known player names, and — for a non-chat line
// — a leading capitalized word followed by a lowercase one ("Soandso begins
// to cast…"), which is almost always a player or a named mob.
func templatize(text string, known func(string) bool, leadingName, regex bool) string {
	var b strings.Builder
	nums, words := 0, 0
	last := 0
	locs := tokenRe.FindAllStringIndex(text, -1)
	lit := func(s string) {
		if regex {
			b.WriteString(regexp.QuoteMeta(s))
		} else {
			b.WriteString(s)
		}
	}
	for i, loc := range locs {
		tok := text[loc[0]:loc[1]]
		var repl string
		switch {
		case tok[0] >= '0' && tok[0] <= '9':
			nums++
			if regex {
				repl = `\d[\d,]*`
			} else {
				repl = fmt.Sprintf("{n%d:number}", nums)
			}
		case isName(tok, known) || (leadingName && i == 0 && leadsName(text, tok, locs)):
			words++
			if regex {
				repl = `\S+`
			} else {
				repl = fmt.Sprintf("{name%d:word}", words)
			}
		default:
			continue
		}
		lit(text[last:loc[0]])
		b.WriteString(repl)
		last = loc[1]
	}
	lit(text[last:])
	if regex {
		return "(?i)^" + b.String() + "$"
	}
	return b.String()
}

// isName reports whether a token is a known player name (lowercased lookup;
// a possessive "Bob's" counts as Bob). known caches its own misses.
func isName(tok string, known func(string) bool) bool {
	t := strings.ToLower(strings.TrimRight(tok, "`'"))
	if strings.HasSuffix(t, "`s") || strings.HasSuffix(t, "'s") {
		t = t[:len(t)-2]
	}
	return len(t) >= 3 && known(t)
}

// leadsName reports whether the first token of a non-chat line looks like a
// name: capitalized, not a common sentence opener, at the very start, and
// followed by a lowercase word.
func leadsName(text, tok string, locs [][]int) bool {
	if locs[0][0] != 0 || len(tok) < 3 || tok[0] < 'A' || tok[0] > 'Z' || notNames[strings.ToLower(tok)] {
		return false
	}
	if len(locs) < 2 {
		return false
	}
	next := text[locs[1][0]:locs[1][1]]
	return next[0] >= 'a' && next[0] <= 'z'
}

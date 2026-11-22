package chatwindow

import (
	"regexp"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/linematch"
)

// compiledRule is a Rule ready to run.
type compiledRule struct {
	rule     Rule
	re       *regexp.Regexp // nil = every line the channel/speakers allow
	speakers map[string]bool
}

// compiledWindow is a window's sources and rules, compiled for one active
// character (the {c} token). Rebuilt on save and on a character change, never
// per line.
type compiledWindow struct {
	channels   map[string]bool
	allNamed   bool
	other      bool
	characters map[string]bool // nil = any character
	keep       []compiledRule
	hide       []compiledRule
	self       string
}

// standardChannels are the channels that aren't named channels.
var standardChannels = map[string]bool{
	ChannelTell: true, ChannelGuild: true, ChannelGroup: true, ChannelRaid: true,
	ChannelSay: true, ChannelOOC: true, ChannelShout: true, ChannelAuction: true,
}

// compileRule compiles one validated rule for the active character self.
func compileRule(r Rule, self string) (compiledRule, error) {
	cr := compiledRule{rule: r}
	if len(r.Speakers) > 0 {
		cr.speakers = make(map[string]bool, len(r.Speakers))
		for _, s := range r.Speakers {
			cr.speakers[strings.ToLower(s)] = true
		}
	}
	if r.Text == "" {
		return cr, nil
	}
	var pattern string
	if r.Mode == MatchSimple {
		p, err := linematch.SimplePatternWith(linematch.Simple{Kind: r.Kind, Text: r.Text},
			linematch.Options{Wildcards: true, SelfToken: true, Self: self})
		if err != nil {
			return cr, err
		}
		pattern = p
	} else {
		pattern = linematch.ExpandSelf(r.Text, self)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return cr, err
	}
	cr.re = re
	return cr, nil
}

// compileWindow compiles a validated definition. Rules that fail to compile
// (only possible for a stored definition Validate would now reject) are
// skipped.
func compileWindow(def WindowDef, self string) *compiledWindow {
	cw := &compiledWindow{
		channels: make(map[string]bool, len(def.Sources.Channels)),
		allNamed: def.Sources.AllNamed,
		other:    def.Sources.Other,
		self:     self,
	}
	for _, c := range def.Sources.Channels {
		cw.channels[c] = true
	}
	if len(def.Characters) > 0 {
		cw.characters = make(map[string]bool, len(def.Characters))
		for _, c := range def.Characters {
			cw.characters[strings.ToLower(c)] = true
		}
	}
	add := func(rules []Rule) []compiledRule {
		var out []compiledRule
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			if cr, err := compileRule(r, self); err == nil {
				out = append(out, cr)
			}
		}
		return out
	}
	cw.keep = add(def.Keep)
	cw.hide = add(def.Hide)
	return cw
}

// wants reports whether the line is one of the window's sources.
func (cw *compiledWindow) wants(l *Line) bool {
	if cw.characters != nil && !cw.characters[strings.ToLower(l.Character)] {
		return false
	}
	if !l.isChat() {
		return cw.other
	}
	if cw.channels[l.Channel] {
		return true
	}
	return cw.allNamed && !standardChannels[l.Channel]
}

// match runs one rule against a line.
func (cr *compiledRule) match(l *Line) bool {
	r := &cr.rule
	if r.Channel != "" && l.Channel != r.Channel {
		return false
	}
	if cr.speakers != nil && (!l.isChat() || !cr.speakers[strings.ToLower(l.Speaker)]) {
		return false
	}
	if cr.re == nil {
		return true
	}
	if r.Channel != "" {
		return cr.re.MatchString(l.Message)
	}
	return cr.re.MatchString(l.Text)
}

// verdict is what a window does with one line.
type verdict struct {
	result string // VerdictKept | VerdictShown | VerdictHidden | VerdictFiltered
	rule   *Rule  // the deciding keep or hide rule
	color  string
	pin    int // minutes
}

func (v verdict) shown() bool { return v.result == VerdictKept || v.result == VerdictShown }

// evaluate decides one line: sources first, then keep rules (the first match
// sets color and pin, and protects the line from every hide rule), then hide
// rules. Live lines, the editor's test box and re-filtering a buffer after a
// rule change all go through here, so they always agree.
func (cw *compiledWindow) evaluate(l *Line) verdict {
	if !cw.wants(l) {
		return verdict{result: VerdictFiltered}
	}
	for i := range cw.keep {
		if cw.keep[i].match(l) {
			r := &cw.keep[i].rule
			return verdict{result: VerdictKept, rule: r, color: r.Color, pin: r.PinMinutes}
		}
	}
	for i := range cw.hide {
		if cw.hide[i].match(l) {
			return verdict{result: VerdictHidden, rule: &cw.hide[i].rule}
		}
	}
	return verdict{result: VerdictShown}
}

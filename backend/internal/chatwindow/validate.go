package chatwindow

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/linematch"
)

var (
	keyRe     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	channelRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	colorRe   = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Validate checks a definition and returns it normalized: names trimmed,
// missing rule IDs generated, channels lowercased, limits defaulted and
// clamped. Every pattern must compile with RE2. It is the gate for both the
// editor and imported JSON, so it never trusts sizes.
func Validate(def WindowDef) (WindowDef, error) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return def, fmt.Errorf("name is required")
	}
	if len(def.Name) > MaxNameLen {
		return def, fmt.Errorf("name is longer than %d characters", MaxNameLen)
	}

	chars := def.Characters[:0:0]
	for _, c := range def.Characters {
		if c = strings.TrimSpace(c); c != "" {
			chars = append(chars, c)
		}
	}
	def.Characters = chars

	seen := map[string]bool{}
	channels := []string{}
	for _, c := range def.Sources.Channels {
		c = normChannel(c)
		if c == "" || seen[c] {
			continue
		}
		if !channelRe.MatchString(c) {
			return def, fmt.Errorf("invalid channel %q", c)
		}
		seen[c] = true
		channels = append(channels, c)
	}
	def.Sources.Channels = channels
	if len(channels) == 0 && !def.Sources.AllNamed && !def.Sources.Other {
		return def, fmt.Errorf("pick at least one channel or other log lines")
	}

	if len(def.Keep) > MaxRules || len(def.Hide) > MaxRules {
		return def, fmt.Errorf("at most %d keep rules and %d hide rules", MaxRules, MaxRules)
	}
	ids := map[string]bool{}
	var err error
	if def.Keep, err = validateRules(def.Keep, true, ids); err != nil {
		return def, err
	}
	if def.Hide, err = validateRules(def.Hide, false, ids); err != nil {
		return def, err
	}

	a := &def.Appearance
	if a.FontScale == 0 {
		a.FontScale = 1
	}
	a.FontScale = clamp(a.FontScale, MinFontScale, MaxFontScale)
	if a.Opacity == 0 {
		a.Opacity = 0.85
	}
	a.Opacity = clamp(a.Opacity, 0.1, 1)
	colors := map[string]string{}
	for k, v := range a.ChannelColors {
		k = normChannel(k)
		if k != ChannelOther && !channelRe.MatchString(k) {
			continue
		}
		if colorRe.MatchString(v) {
			colors[k] = v
		}
	}
	a.ChannelColors = nil
	if len(colors) > 0 {
		a.ChannelColors = colors
	}

	if def.MaxLines <= 0 {
		def.MaxLines = DefaultMaxLines
	}
	if def.MaxLines > LimitMaxLines {
		def.MaxLines = LimitMaxLines
	}
	return def, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// normChannel lowercases a channel key and drops a named channel's slot
// number ("General:1" → "general").
func normChannel(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if i := strings.IndexByte(c, ':'); i >= 0 {
		c = c[:i]
	}
	return c
}

func validateRules(rules []Rule, keep bool, ids map[string]bool) ([]Rule, error) {
	kind := "hide"
	if keep {
		kind = "keep"
	}
	out := make([]Rule, len(rules))
	for i, r := range rules {
		if r.ID == "" {
			r.ID = newID()
		}
		if !keyRe.MatchString(r.ID) || ids[r.ID] {
			return nil, fmt.Errorf("%s rule %d: invalid or duplicate id %q", kind, i+1, r.ID)
		}
		ids[r.ID] = true
		if err := validateRule(&r, keep); err != nil {
			return nil, fmt.Errorf("%s rule %d (%s): %w", kind, i+1, ruleLabel(r), err)
		}
		out[i] = r
	}
	return out, nil
}

func ruleLabel(r Rule) string {
	if r.Name != "" {
		return r.Name
	}
	if len(r.Text) > 30 {
		return r.Text[:30] + "…"
	}
	return r.Text
}

func validateRule(r *Rule, keep bool) error {
	r.Name = strings.TrimSpace(r.Name)
	if len(r.Name) > MaxNameLen {
		return fmt.Errorf("name is longer than %d characters", MaxNameLen)
	}
	r.Channel = normChannel(r.Channel)
	if r.Channel != "" && !channelRe.MatchString(r.Channel) {
		return fmt.Errorf("invalid channel %q", r.Channel)
	}
	speakers := r.Speakers[:0:0]
	for _, s := range r.Speakers {
		if s = strings.TrimSpace(s); s != "" {
			speakers = append(speakers, s)
		}
	}
	if len(speakers) > MaxSpeakers {
		return fmt.Errorf("at most %d speakers", MaxSpeakers)
	}
	r.Speakers = speakers
	r.Text = strings.TrimSpace(r.Text)
	if len(r.Text) > linematch.MaxPatternLen {
		return fmt.Errorf("pattern is longer than %d characters", linematch.MaxPatternLen)
	}
	switch r.Mode {
	case "", MatchSimple:
		r.Mode = MatchSimple
		if r.Kind == "" {
			r.Kind = linematch.SimpleContains
		}
	case MatchRegex:
		r.Kind = ""
	default:
		return fmt.Errorf("unknown match mode %q", r.Mode)
	}
	if r.Text == "" && r.Channel == "" && len(r.Speakers) == 0 {
		return fmt.Errorf("needs a pattern, a channel or speakers")
	}
	// Compile with a stand-in name so a pattern error surfaces on save.
	if _, err := compileRule(*r, "Validate"); err != nil {
		return err
	}
	if keep {
		if r.Color != "" && !colorRe.MatchString(r.Color) {
			return fmt.Errorf("color must be #RRGGBB")
		}
		if r.PinMinutes < 0 || r.PinMinutes > MaxPinMinutes {
			return fmt.Errorf("pin minutes must be 0–%d", MaxPinMinutes)
		}
	} else {
		r.Color = ""
		r.PinMinutes = 0
	}
	return nil
}

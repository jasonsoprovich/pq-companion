// Package chchain watches raid chat for Complete-Heal-chain call lines and
// turns each into a countdown timer in the spell-timer engine (category
// "ch_chain", or "ch_chain_2" for the optional secondary ramp/split chain),
// which the dedicated CH Chain overlay renders. The matcher is driven by
// user-configurable regexes + cadence so it adapts to different guild
// chain-call formats without code changes.
package chchain

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// Sink is the subset of the spell-timer engine the matcher needs. It matches
// (*spelltimer.Engine).StartExternal so the engine satisfies it directly.
type Sink interface {
	StartExternal(name string, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string, pinned bool, customGroup string)
}

// categoryCHChain / categoryCHChain2 mirror spelltimer.CategoryCHChain /
// CategoryCHChain2. Duplicated as strings to avoid importing the spelltimer
// package just for the constants.
const (
	categoryCHChain  = "ch_chain"
	categoryCHChain2 = "ch_chain_2"
)

// cachedRegex compiles a pattern on demand, recompiling only when the source
// changes. A bad pattern is logged once per distinct source and reported as
// not-ok so a typo in settings can't spam the log or panic.
type cachedRegex struct {
	src   string
	re    *regexp.Regexp
	names []string
}

func (c *cachedRegex) compile(src string) (*regexp.Regexp, []string, bool) {
	if src == c.src {
		return c.re, c.names, c.re != nil
	}
	c.src = src
	re, err := regexp.Compile(src)
	if err != nil {
		slog.Warn("chchain: invalid pattern, matcher disabled until fixed", "pattern", src, "err", err)
		c.re, c.names = nil, nil
		return nil, nil, false
	}
	c.re = re
	c.names = re.SubexpNames()
	return c.re, c.names, true
}

// Matcher compiles the configured CH-chain regexes on demand (recompiling
// when a pattern changes) and creates a ch_chain / ch_chain_2 timer per
// matched chain call.
type Matcher struct {
	sink Sink
	cfg  func() config.CHChainSettings

	mu        sync.Mutex
	primary   cachedRegex
	secondary cachedRegex

	callsMu     sync.Mutex
	recentCalls map[string]recentCall
}

// recentCall records one chain callout's target and the time it was seen, so
// CastWatcher can later look up which target a caster's "begins to cast"
// line should confirm.
type recentCall struct {
	target string
	at     time.Time
}

// recentCallWindow bounds how long a chain callout stays eligible for
// cast-begin correlation. Complete Healing's cast time is 10s and a caster's
// "begins to cast" line normally follows the callout within a second or two,
// so this comfortably covers that while staying well short of a full chain
// cycle (chainSize × delay), which is when the same caster's NEXT callout for
// this position would otherwise arrive and could be confused for this one.
const recentCallWindow = config.CHCastSecs*time.Second + 2*time.Second

// New constructs a Matcher reading live settings via cfg and emitting timers
// through sink.
func New(sink Sink, cfg func() config.CHChainSettings) *Matcher {
	return &Matcher{sink: sink, cfg: cfg, recentCalls: make(map[string]recentCall)}
}

// TargetForCaster returns the target of caster's most recent chain callout,
// if one was recorded within recentCallWindow of now. Used by CastWatcher to
// resolve a "begins to cast" bystander line (which carries a caster name but
// no target) back to the chain timer it should confirm.
func (m *Matcher) TargetForCaster(caster string, now time.Time) (string, bool) {
	if caster == "" {
		return "", false
	}
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	rc, ok := m.recentCalls[caster]
	if !ok || now.Sub(rc.at) > recentCallWindow {
		return "", false
	}
	return rc.target, true
}

// HandleLine matches one raw log line against the configured pattern(s) and,
// on a hit, starts a countdown timer for that chain position. When the
// secondary chain is enabled its pattern is tried FIRST: the primary
// catch-all default also matches letter markers, so letter calls must be
// claimed by the secondary chain before the primary gets a look.
func (m *Matcher) HandleLine(ts time.Time, msg string) {
	settings := m.cfg()
	if !settings.Enabled {
		return
	}

	if settings.SecondaryEnabled {
		pattern := settings.SecondaryPattern
		if pattern == "" {
			pattern = config.DefaultCHChainSecondaryPattern
		}
		if m.matchAndStart(ts, msg, pattern, &m.secondary, categoryCHChain2) {
			return
		}
	}

	pattern := settings.Pattern
	if pattern == "" {
		pattern = config.DefaultCHChainPattern
	}
	m.matchAndStart(ts, msg, pattern, &m.primary, categoryCHChain)
}

// matchAndStart applies one pattern to msg and, on a hit, emits a timer in
// the given category. Returns true when the line matched (even if it was
// dropped for having no target) so the caller can stop trying patterns.
func (m *Matcher) matchAndStart(ts time.Time, msg, pattern string, cache *cachedRegex, category string) bool {
	m.mu.Lock()
	re, names, ok := cache.compile(pattern)
	m.mu.Unlock()
	if !ok {
		return false
	}
	match := re.FindStringSubmatch(msg)
	if match == nil {
		return false
	}

	caster, target := "", ""
	chainnum := 0
	for i, name := range names {
		if i >= len(match) {
			break
		}
		switch name {
		case "caster":
			caster = match[i]
		case "target":
			target = match[i]
		case "chainnum":
			chainnum = parseChainNum(match[i])
		}
	}
	if target == "" {
		return true // a chain call with no target isn't actionable
	}

	// Record the callout for cast-begin correlation (see TargetForCaster) so
	// CastWatcher can later confirm this exact timer when the caster's
	// "begins to cast" line arrives, regardless of whether the regex
	// captured a caster name (a pattern without a `caster` group just never
	// records here, and possible-miss detection quietly finds nothing).
	if caster != "" {
		m.callsMu.Lock()
		m.recentCalls[caster] = recentCall{target: target, at: ts}
		m.callsMu.Unlock()
	}

	// The label doubles as the timer key. Encoding the position as a leading
	// "#N" lets the overlay sort by chain order; including target keeps each
	// position's bar distinct so concurrent calls don't dedup into one.
	label := fmt.Sprintf("#%d  %s", chainnum, target)
	if caster != "" {
		label += "  ← " + caster // "← caster"
	}

	// The bar runs the CH cast time, so it counts down to when this cleric's
	// heal lands (a callout fires at cast-start). The live spacing between
	// casts is measured in the overlay from successive callout timestamps, so
	// the bar length is the fixed cast duration rather than the cadence.
	//
	// target is passed through as the timer's TargetName (previously left
	// empty — the overlay only ever parsed the target back out of the label
	// text) so CastWatcher can correlate a cast-begin line to the right
	// timer via Engine.ConfirmHeal.
	m.sink.StartExternal(label, category, config.CHCastSecs, 0, ts, nil, 0, target, "", false, "")
	return true
}

// parseChainNum turns a chain marker into a position. Numeric markers ("001",
// "002") parse directly; letter markers ("AAA", "bbb") map their first letter
// to A=1, B=2, … so letter chains get real positions for overlay sorting and
// the metronome's watch-position logic. Anything else is position 0.
func parseChainNum(s string) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if s == "" {
		return 0
	}
	c := s[0]
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 1
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 1
	}
	return 0
}

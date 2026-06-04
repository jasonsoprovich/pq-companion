// Package chchain watches raid chat for Complete-Heal-chain call lines and
// turns each into a countdown timer in the spell-timer engine (category
// "ch_chain"), which the dedicated CH Chain overlay renders. The matcher is
// driven by a user-configurable regex + cadence so it adapts to different
// guild chain-call formats without code changes.
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
	StartExternal(name string, category string, durationSecs, displayThresholdSecs int, startedAt time.Time, alerts json.RawMessage, spellID int)
}

// categoryCHChain mirrors spelltimer.CategoryCHChain. Duplicated as a string
// to avoid importing the spelltimer package just for the constant.
const categoryCHChain = "ch_chain"

// Matcher compiles the configured CH-chain regex on demand (recompiling when
// the pattern changes) and creates a ch_chain timer per matched chain call.
type Matcher struct {
	sink Sink
	cfg  func() config.CHChainSettings

	mu          sync.Mutex
	cachedSrc   string
	cachedRe    *regexp.Regexp
	cachedNames []string
}

// New constructs a Matcher reading live settings via cfg and emitting timers
// through sink.
func New(sink Sink, cfg func() config.CHChainSettings) *Matcher {
	return &Matcher{sink: sink, cfg: cfg}
}

// HandleLine matches one raw log line against the configured pattern and, on a
// hit, starts a ch_chain countdown timer for that chain position.
func (m *Matcher) HandleLine(ts time.Time, msg string) {
	settings := m.cfg()
	if !settings.Enabled {
		return
	}
	pattern := settings.Pattern
	if pattern == "" {
		pattern = config.DefaultCHChainPattern
	}

	re, names, ok := m.compile(pattern)
	if !ok {
		return
	}
	match := re.FindStringSubmatch(msg)
	if match == nil {
		return
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
			chainnum, _ = strconv.Atoi(match[i])
		}
	}
	if target == "" {
		return // a chain call with no target isn't actionable
	}

	interval := settings.IntervalSecs
	if interval <= 0 {
		interval = config.DefaultCHChainIntervalSecs
	}

	// The label doubles as the timer key. Encoding the position as a leading
	// "#N" lets the overlay sort by chain order; including target keeps each
	// position's bar distinct so concurrent calls don't dedup into one.
	label := fmt.Sprintf("#%d  %s", chainnum, target)
	if caster != "" {
		label += "  ← " + caster // "← caster"
	}

	m.sink.StartExternal(label, categoryCHChain, interval, 0, ts, nil, 0)
}

// compile returns the regex + capture-group names for src, recompiling only
// when src changes. A bad pattern is logged once per distinct source and
// reported as not-ok so a typo in settings can't spam the log or panic.
func (m *Matcher) compile(src string) (*regexp.Regexp, []string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if src == m.cachedSrc {
		return m.cachedRe, m.cachedNames, m.cachedRe != nil
	}
	m.cachedSrc = src
	re, err := regexp.Compile(src)
	if err != nil {
		slog.Warn("chchain: invalid pattern, matcher disabled until fixed", "pattern", src, "err", err)
		m.cachedRe, m.cachedNames = nil, nil
		return nil, nil, false
	}
	m.cachedRe = re
	m.cachedNames = re.SubexpNames()
	return m.cachedRe, m.cachedNames, true
}

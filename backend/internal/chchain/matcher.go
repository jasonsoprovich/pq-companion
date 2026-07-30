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
// (*spelltimer.Engine).StartExternal / .ConfirmCast / .SetCasterMana so the
// engine satisfies it directly.
type Sink interface {
	StartExternal(name string, category string, durationSecs, displayThresholdSecs float64, startedAt time.Time, alerts json.RawMessage, spellID int, targetName, barColor string, pinned bool, customGroup string)
	ConfirmCast(name, targetName string)
	// UnconfirmCast reverses ConfirmCast for the chain timer whose caster is
	// caster, if one is currently confirmed and still within its cast
	// window at ts — see InterruptWatcher / Matcher.NoteCastInterrupted.
	UnconfirmCast(caster string, ts time.Time)
	// SetCasterMana updates the (name, targetName)-keyed timer with the
	// caster's self-reported remaining mana percentage, without touching its
	// identity, duration, or countdown. Called right after StartExternal for
	// the same (name, targetName) pair when the callout's trailing text
	// included one — kept as a side-channel update rather than folded into
	// name/label because the label doubles as the timer's map key: baking in
	// a value that changes every cast (mana drops as the fight goes on) would
	// make each callout mint a brand-new key instead of restarting the same
	// position's existing bar, producing a duplicate row for the ~4s tail of
	// the old bar's countdown until it expired on its own.
	SetCasterMana(name, targetName string, pct int)
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

	callsMu      sync.Mutex
	recentCalls  map[string]recentCall
	pendingCasts map[string]time.Time
}

// recentCall records one chain callout's timer identity (label + target — the
// pair that uniquely names the timer the matcher created for it) and the time
// it was seen, so a later "begins to cast" line from the same caster can
// confirm that exact timer.
//
// Identifying the timer by label rather than by target alone is essential: in
// a real CH chain every cleric heals the SAME tank, so a target-keyed lookup
// cannot tell whose callout a cast-begin line belongs to. The label embeds the
// chain position and the caster ("#2  Larzek  ← Eruna"), which does.
type recentCall struct {
	label  string
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

// earlyCastWindow bounds how long a cast-begin line that arrived with no
// matching callout yet stays eligible to confirm the NEXT callout from that
// caster. Chain macros usually shout before they cast, but the two land on the
// same or adjacent log seconds and the order isn't guaranteed — measured
// against real raid logs, ~8% of callouts have their cast-begin line one
// second AHEAD of the shout. Without this those would all be flagged possible
// misses despite the cast plainly happening.
const earlyCastWindow = 3 * time.Second

// New constructs a Matcher reading live settings via cfg and emitting timers
// through sink.
func New(sink Sink, cfg func() config.CHChainSettings) *Matcher {
	return &Matcher{
		sink:         sink,
		cfg:          cfg,
		recentCalls:  make(map[string]recentCall),
		pendingCasts: make(map[string]time.Time),
	}
}

// NoteCastBegin records that caster was observed starting a cast at ts (see
// CastWatcher). If that caster made a chain callout within recentCallWindow,
// the timer created for it is confirmed immediately so pruneExpired won't flag
// it a possible miss. Otherwise the cast is held as pending, so a callout
// arriving in the next earlyCastWindow can claim it — covering macros whose
// cast line beats their shout into the log.
//
// Correlation is one-shot in both directions: a consumed callout is dropped so
// a caster's later, unrelated cast (a heal outside the chain, a rez, a buff)
// can't confirm the same callout twice or leak onto the next one.
func (m *Matcher) NoteCastBegin(caster string, ts time.Time) {
	if caster == "" {
		return
	}
	m.callsMu.Lock()
	rc, ok := m.recentCalls[caster]
	if ok && !ts.Before(rc.at) && ts.Sub(rc.at) <= recentCallWindow {
		delete(m.recentCalls, caster)
		m.callsMu.Unlock()
		m.sink.ConfirmCast(rc.label, rc.target)
		return
	}
	m.pendingCasts[caster] = ts
	m.callsMu.Unlock()
}

// NoteCastInterrupted records that caster's cast was interrupted at ts (see
// InterruptWatcher). Unlike NoteCastBegin, this needs no callout bookkeeping
// of its own — the sink already has enough identity (the chain timer's
// caster-suffixed label) to find and un-confirm the right timer directly.
func (m *Matcher) NoteCastInterrupted(caster string, ts time.Time) {
	if caster == "" {
		return
	}
	m.sink.UnconfirmCast(caster, ts)
}

// noteCall records a fresh callout for cast-begin correlation and reports
// whether a cast-begin line for it was already seen (the early-cast case), in
// which case the caller confirms the new timer right away.
func (m *Matcher) noteCall(caster, label, target string, ts time.Time) (confirmed bool) {
	m.callsMu.Lock()
	defer m.callsMu.Unlock()
	if at, ok := m.pendingCasts[caster]; ok {
		delete(m.pendingCasts, caster)
		if !ts.Before(at) && ts.Sub(at) <= earlyCastWindow {
			return true
		}
	}
	m.recentCalls[caster] = recentCall{label: label, target: target, at: ts}
	return false
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
	// Indices (not just substrings) are needed so the mana lookup below can
	// scan only the text after the target name, rather than the whole line.
	loc := re.FindStringSubmatchIndex(msg)
	if loc == nil {
		return false
	}

	caster, target := "", ""
	chainnum := 0
	targetEnd := -1
	for i, name := range names {
		start, end := loc[2*i], loc[2*i+1]
		if start < 0 {
			continue // group didn't participate in the match (e.g. an alternation branch)
		}
		switch name {
		case "caster":
			caster = msg[start:end]
		case "target":
			target = msg[start:end]
			targetEnd = end
		case "chainnum":
			chainnum = parseChainNum(msg[start:end])
		}
	}
	if target == "" {
		return true // a chain call with no target isn't actionable
	}

	// The label doubles as the timer key. Encoding the position as a leading
	// "#N" lets the overlay sort by chain order; including target keeps each
	// position's bar distinct so concurrent calls don't dedup into one, and
	// including caster is what makes (label, target) identify one caster's
	// callout rather than "somebody's heal on the tank" — see recentCall.
	label := fmt.Sprintf("#%d  %s", chainnum, target)
	if caster != "" {
		label += "  ← " + caster // "← caster"
	}

	// The caster's self-reported remaining mana, if the callout's trailing
	// text included one — applied via SetCasterMana below (not baked into
	// label; see the Sink doc comment for why).
	manaPct := -1
	if targetEnd >= 0 {
		if mm := manaPercentRe.FindStringSubmatch(msg[targetEnd:]); mm != nil {
			if pct, err := strconv.Atoi(mm[1]); err == nil && pct <= 100 {
				manaPct = pct
			}
		}
	}

	// Record the callout for cast-begin correlation (see NoteCastBegin) so a
	// later "begins to cast" line from this caster confirms this exact timer,
	// regardless of whether the regex captured a caster name (a pattern
	// without a `caster` group just never records here, and possible-miss
	// detection quietly finds nothing rather than guessing).
	earlyCast := false
	if caster != "" {
		earlyCast = m.noteCall(caster, label, target, ts)
	}

	// The bar runs the CH cast time, so it counts down to when this cleric's
	// heal lands (a callout fires at cast-start). The live spacing between
	// casts is measured in the overlay from successive callout timestamps, so
	// the bar length is the fixed cast duration rather than the cadence.
	//
	// target is passed through as the timer's TargetName (previously left
	// empty — the overlay only ever parsed the target back out of the label
	// text) so it can be paired with the label to address exactly this timer
	// in Engine.ConfirmCast.
	m.sink.StartExternal(label, category, config.CHCastSecs, 0, ts, nil, 0, target, "", false, "")
	if manaPct >= 0 {
		m.sink.SetCasterMana(label, target, manaPct)
	}
	if earlyCast {
		m.sink.ConfirmCast(label, target)
	}
	return true
}

// manaPercentRe finds a percentage in the free-form text after a chain
// callout's target name — healers commonly tack their remaining mana onto the
// callout ("CH Tank, 94% remaining", "CH Tank, 90% mana", "<< 100% Mana >>").
// Requires the digits to be immediately followed by "%" so it never matches
// the chain marker or other stray numbers in the line.
var manaPercentRe = regexp.MustCompile(`(\d{1,3})\s*%`)

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

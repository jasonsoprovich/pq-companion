package chchain

import (
	"regexp"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// HealSink is the subset of the spell-timer engine CastWatcher needs. It
// matches (*spelltimer.Engine).ConfirmHeal so the engine satisfies it
// directly.
type HealSink interface {
	ConfirmHeal(targetName string)
}

// CasterLookup resolves a caster name back to the target of their most
// recent chain callout. (*Matcher).TargetForCaster satisfies this directly.
type CasterLookup interface {
	TargetForCaster(caster string, now time.Time) (target string, ok bool)
}

// reBeginCastOther matches "<Name> begins to cast a spell." — the generic
// bystander message EQ shows to anyone nearby when another player starts
// casting any multi-tick spell. It never reveals which spell, only that a
// cast attempt began, which is enough here: the caster name plus the callout
// recorded in Matcher.recentCalls is what identifies which chain timer this
// confirms.
var reBeginCastOther = regexp.MustCompile(`^([A-Z][a-z]{2,14}) begins to cast a spell\.$`)

// reBeginCastSelf matches "You begin casting <SpellName>." — the caster's own
// version of the same message, used when the watching player themselves
// occupies the chain slot being confirmed. Unlike reBeginCastOther this does
// reveal the spell name, but it isn't needed: the caller is always "You",
// which is exactly the caster value Matcher records for the player's own
// chain-call lines (matcher.go's chChainPatternPrefix names the caster group
// "You" for second-person verb forms).
var reBeginCastSelf = regexp.MustCompile(`^You begin casting .+\.$`)

// CastWatcher watches raw log lines for a caster starting to cast (their own
// or a nearby bystander's) and confirms the matching CH-chain timer via
// HealSink.ConfirmHeal, so Engine.pruneExpired won't flag it a possible miss.
// Purely additive: it never creates, modifies, or removes a chain timer's
// identity — only whether it gets flagged.
//
// This supersedes an earlier design that correlated off the heal actually
// landing on the target (a bystander line range-limited to the TARGET's
// position, and one that raced pruneExpired's 10s expiry). Cast-begin
// correlation is range-limited to the CASTER's position instead, which is
// reliable since CH-chain casters cluster together, and it resolves within
// ~0-2s of the callout rather than at the cast-window boundary. It's also
// class-agnostic: a "begins to cast" line never reveals which spell, so the
// same watcher covers Cleric Complete Healing, Druid Tunare's/Karana's
// Renewal, or anything else a chain macro casts, with no per-spell regex.
type CastWatcher struct {
	sink   HealSink
	lookup CasterLookup
	cfg    func() config.CHChainSettings
}

// NewCastWatcher constructs a CastWatcher reading live settings via cfg,
// resolving caster→target via lookup, and confirming heals through sink.
func NewCastWatcher(sink HealSink, lookup CasterLookup, cfg func() config.CHChainSettings) *CastWatcher {
	return &CastWatcher{sink: sink, lookup: lookup, cfg: cfg}
}

// HandleLine checks one raw log line against the watched cast-begin patterns
// and, on a hit, resolves the caster to their most recent chain callout's
// target and confirms it.
func (w *CastWatcher) HandleLine(ts time.Time, msg string) {
	settings := w.cfg()
	if !settings.Enabled || !settings.PossibleMissEnabled {
		return
	}

	var caster string
	if reBeginCastSelf.MatchString(msg) {
		caster = "You"
	} else if m := reBeginCastOther.FindStringSubmatch(msg); m != nil {
		caster = m[1]
	} else {
		return
	}

	if target, ok := w.lookup.TargetForCaster(caster, ts); ok {
		w.sink.ConfirmHeal(target)
	}
}

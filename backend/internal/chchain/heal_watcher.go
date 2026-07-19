package chchain

import (
	"regexp"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// HealSink is the subset of the spell-timer engine HealWatcher needs. It
// matches (*spelltimer.Engine).ConfirmHeal so the engine satisfies it
// directly.
type HealSink interface {
	ConfirmHeal(targetName string)
}

// reHealLanded matches "<Target> is completely healed." — the bystander
// message EQ shows to anyone nearby when Complete Healing (spells_new id 13,
// the Cleric spell CH-chain macros actually cast) lands on a target,
// regardless of who cast it. Verified against quarm.db to be the ONLY
// player-castable spell using this exact text (two other rows share it —
// "Healing Touch" id 842 and "Healing Complete" id 1469 — but both are
// uncastable by every class, i.e. NPC/unused entries).
//
// Deliberately NOT watching Superior Healing's "<Target> feels much
// better." (the Druid DCH spell, spells_new id 9): that text is shared by
// over a dozen unrelated heal spells across multiple classes (Healing,
// Greater Healing, Word of Health, Nature's Touch, …), so any off-chain
// healer topping off the same target would false-confirm a chain slot that
// actually missed. Reliable correlation needs an unambiguous bystander
// message; only Complete Healing has one.
var reHealLanded = regexp.MustCompile(`^([A-Z][a-z]{3,14}) is completely healed\.$`)

// HealWatcher watches raw log lines for the Complete Healing landed-on-other
// message and confirms the matching CH-chain timer via HealSink.ConfirmHeal,
// so Engine.pruneExpired won't flag it a possible miss. Purely additive: it
// never creates, modifies, or removes a chain timer's identity — only
// whether it gets flagged.
type HealWatcher struct {
	sink HealSink
	cfg  func() config.CHChainSettings
}

// NewHealWatcher constructs a HealWatcher reading live settings via cfg and
// confirming heals through sink.
func NewHealWatcher(sink HealSink, cfg func() config.CHChainSettings) *HealWatcher {
	return &HealWatcher{sink: sink, cfg: cfg}
}

// HandleLine checks one raw log line against reHealLanded and, on a hit,
// confirms the heal for the captured target name.
func (w *HealWatcher) HandleLine(msg string) {
	settings := w.cfg()
	if !settings.Enabled || !settings.PossibleMissEnabled {
		return
	}
	m := reHealLanded.FindStringSubmatch(msg)
	if m == nil {
		return
	}
	w.sink.ConfirmHeal(m[1])
}

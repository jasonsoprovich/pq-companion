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

// reCompleteHealingLanded matches "<Target> is completely healed." — the
// bystander message EQ shows to anyone nearby when Complete Healing
// (spells_new id 13, the Cleric spell CH-chain macros actually cast) lands
// on a target, regardless of who cast it. Verified against quarm.db to be
// the ONLY player-castable spell using this exact text (two other rows
// share it — "Healing Touch" id 842 and "Healing Complete" id 1469 — but
// both are uncastable by every class, i.e. NPC/unused entries). Always
// watched whenever possible-miss detection is on.
var reCompleteHealingLanded = regexp.MustCompile(`^([A-Z][a-z]{3,14}) is completely healed\.$`)

// reSuperiorHealingLanded matches "<Target> feels much better." — the
// bystander message for Superior Healing (spells_new id 9, the Druid's
// equivalent "DCH" — the only class that can cast it). Unlike Complete
// Healing's text, this exact string is shared by over a dozen unrelated heal
// spells across multiple classes (Healing, Greater Healing, Word of Health,
// Nature's Touch, …), so any healer's filler/spot heal on the same target
// would false-confirm a chain slot that actually missed. Watching it is
// therefore opt-in (CHChainSettings.PossibleMissIncludeDruid, default off) —
// reliable for raids that rarely spot-heal the CH-chain tank outside the
// chain itself, noisy otherwise.
var reSuperiorHealingLanded = regexp.MustCompile(`^([A-Z][a-z]{3,14}) feels much better\.$`)

// HealWatcher watches raw log lines for heal-landed-on-other messages and
// confirms the matching CH-chain timer via HealSink.ConfirmHeal, so
// Engine.pruneExpired won't flag it a possible miss. Purely additive: it
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

// HandleLine checks one raw log line against the watched heal-landed
// patterns and, on a hit, confirms the heal for the captured target name.
// Complete Healing is always checked; Superior Healing only when the user
// has opted into its noisier correlation.
func (w *HealWatcher) HandleLine(msg string) {
	settings := w.cfg()
	if !settings.Enabled || !settings.PossibleMissEnabled {
		return
	}
	if m := reCompleteHealingLanded.FindStringSubmatch(msg); m != nil {
		w.sink.ConfirmHeal(m[1])
		return
	}
	if settings.PossibleMissIncludeDruid {
		if m := reSuperiorHealingLanded.FindStringSubmatch(msg); m != nil {
			w.sink.ConfirmHeal(m[1])
		}
	}
}

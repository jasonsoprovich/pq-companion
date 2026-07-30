package chchain

import (
	"regexp"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// InterruptObserver receives each observed interrupted cast, keyed by
// caster, so a chain timer that CastObserver already confirmed can be
// un-confirmed and re-flagged a possible miss when that confirmed cast never
// actually lands. (*Matcher).NoteCastInterrupted satisfies this directly.
type InterruptObserver interface {
	NoteCastInterrupted(caster string, ts time.Time)
}

// reInterruptOther matches "<Name>'s casting is interrupted!" — the
// bystander message EQ shows to anyone nearby when another player's cast is
// broken (e.g. by a melee hit or stun) before it completes. Same visibility
// rules as reBeginCastOther: only seen by players near the caster.
var reInterruptOther = regexp.MustCompile(`^([A-Z][a-z]{2,14})'s casting is interrupted!$`)

// reInterruptSelf matches the local player's own interrupted-cast message,
// which comes in both a named and an unnamed form:
//
//	"Your spell is interrupted."
//	"Your Complete Healing spell is interrupted."
var reInterruptSelf = regexp.MustCompile(`^Your (?:.+ )?spell is interrupted\.$`)

// InterruptWatcher watches raw log lines for a caster's cast being
// interrupted (their own or a nearby bystander's) and reports the caster to
// an InterruptObserver, which un-confirms the matching chain timer.
//
// Gated behind CHChainSettings.InterruptDetectionEnabled, off by default, on
// top of the existing PossibleMissEnabled gate: this only refines that
// feature's output, so guilds happy with the current confirm-on-cast-begin
// behavior are entirely unaffected unless they opt in. Purely additive,
// mirroring CastWatcher: it never creates, modifies, or removes a chain
// timer's identity, only whether it's flagged.
type InterruptWatcher struct {
	obs InterruptObserver
	cfg func() config.CHChainSettings
}

// NewInterruptWatcher constructs an InterruptWatcher reading live settings
// via cfg and reporting observed interrupts to obs.
func NewInterruptWatcher(obs InterruptObserver, cfg func() config.CHChainSettings) *InterruptWatcher {
	return &InterruptWatcher{obs: obs, cfg: cfg}
}

// HandleLine checks one raw log line against the watched interrupt patterns
// and, on a hit, reports the caster so their already-confirmed chain timer
// (if any) can be un-confirmed.
func (w *InterruptWatcher) HandleLine(ts time.Time, msg string) {
	settings := w.cfg()
	if !settings.Enabled || !settings.PossibleMissEnabled || !settings.InterruptDetectionEnabled {
		return
	}

	var caster string
	if reInterruptSelf.MatchString(msg) {
		caster = "You"
	} else if m := reInterruptOther.FindStringSubmatch(msg); m != nil {
		caster = m[1]
	} else {
		return
	}

	w.obs.NoteCastInterrupted(caster, ts)
}

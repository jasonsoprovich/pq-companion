package spelltimer

import (
	"log/slog"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// StripKind says whose spell was stripped.
type StripKind string

const (
	// StripBuff is one of the active player's own buffs removed early
	// (dispelled, or clicked off: the pipe can't tell the two apart).
	StripBuff StripKind = "buff"
	// StripDebuff is one of the active player's detrimentals dispelled off
	// a mob.
	StripDebuff StripKind = "debuff"
)

// StripEvent is a buff or debuff that ended early enough to be worth a
// recast/rebuff callout.
type StripEvent struct {
	Kind   StripKind
	Spell  string
	Target string // the mob for StripDebuff, the active player for StripBuff
	At     time.Time
}

const (
	// stripMinRemaining is how much time a spell must still have had for
	// its early end to count as a strip. Two ticks: anything closer is
	// within the timer's own error and likely a natural fade.
	stripMinRemaining = 2 * eqTickDuration

	// stripDispelWindow pairs a dispel spell landing on a mob with the
	// worn-off lines it causes. The server's order of the land message and
	// the fade messages isn't fixed, so the window applies both ways.
	stripDispelWindow = 2500 * time.Millisecond

	// stripPairWindow is how close two early fades on the same mob must be
	// to count as one dispel without a visible dispel land line (the
	// dispel was cast out of log range, or the mob dispelled itself
	// silently). A single early fade is an overwrite and is ignored.
	stripPairWindow = time.Second

	// seCancelMagic is SPA 27, the dispel effect.
	seCancelMagic = 27
)

// earlyFade is one of the active player's detrimentals that wore off with
// at least stripMinRemaining left, waiting for evidence that it was a
// dispel.
type earlyFade struct {
	spell  string
	target string // as displayed on the timer
	norm   string // normalizeNPCName(target)
	at     time.Time
	mez    bool
	done   bool
}

// SetStripHandler wires the receiver for strip callouts. nil disables them.
// fn is called without the engine lock held.
func (e *Engine) SetStripHandler(fn func(StripEvent)) {
	e.mu.Lock()
	e.stripFn = fn
	e.mu.Unlock()
}

// emitStrips sends events to the strip handler. e.mu must NOT be held.
func (e *Engine) emitStrips(events []StripEvent) {
	if len(events) == 0 {
		return
	}
	e.mu.Lock()
	fn := e.stripFn
	e.mu.Unlock()
	for _, ev := range events {
		slog.Info("spelltimer: spell stripped",
			"kind", ev.Kind, "spell", ev.Spell, "target", ev.Target)
		if fn != nil {
			fn(ev)
		}
	}
}

// noteEarlyFadeLocked records a detrimental that wore off early and returns
// any strips the new evidence completes. e.mu must be held.
func (e *Engine) noteEarlyFadeLocked(t *ActiveTimer, at time.Time) []StripEvent {
	e.trimStripEvidenceLocked(at)
	f := earlyFade{
		spell:  t.SpellName,
		target: t.TargetName,
		norm:   normalizeNPCName(t.TargetName),
		at:     at,
		mez:    t.Category == CategoryMez,
	}
	if d, ok := e.dispelLands[f.norm]; ok && absDuration(at.Sub(d)) <= stripDispelWindow {
		f.done = true
		e.earlyFades = append(e.earlyFades, f)
		return []StripEvent{f.event()}
	}
	var out []StripEvent
	// Mez also breaks on damage, and one AE can break two at once, so a
	// pair of mez fades is not evidence of a dispel.
	if !f.mez {
		for i := range e.earlyFades {
			p := &e.earlyFades[i]
			if p.mez || p.norm != f.norm || at.Sub(p.at) > stripPairWindow {
				continue
			}
			if !p.done {
				p.done = true
				out = append(out, p.event())
			}
			f.done = true
		}
		if f.done {
			out = append(out, f.event())
		}
	}
	e.earlyFades = append(e.earlyFades, f)
	return out
}

// noteDispelLandLocked records a dispel spell landing on target and returns
// the early fades on that mob it explains. e.mu must be held.
func (e *Engine) noteDispelLandLocked(target string, at time.Time) []StripEvent {
	e.trimStripEvidenceLocked(at)
	norm := normalizeNPCName(target)
	if e.dispelLands == nil {
		e.dispelLands = make(map[string]time.Time)
	}
	e.dispelLands[norm] = at
	var out []StripEvent
	for i := range e.earlyFades {
		p := &e.earlyFades[i]
		if p.done || p.norm != norm || absDuration(at.Sub(p.at)) > stripDispelWindow {
			continue
		}
		p.done = true
		out = append(out, p.event())
	}
	return out
}

// trimStripEvidenceLocked drops evidence too old to pair with anything
// arriving at now. e.mu must be held.
func (e *Engine) trimStripEvidenceLocked(now time.Time) {
	kept := e.earlyFades[:0]
	for _, f := range e.earlyFades {
		if now.Sub(f.at) <= stripDispelWindow {
			kept = append(kept, f)
		}
	}
	e.earlyFades = kept
	for k, at := range e.dispelLands {
		if now.Sub(at) > stripDispelWindow {
			delete(e.dispelLands, k)
		}
	}
}

func (f earlyFade) event() StripEvent {
	return StripEvent{Kind: StripDebuff, Spell: f.spell, Target: f.target, At: f.at}
}

// noteSpellLandedForStrip checks a landed spell for a dispel on a mob.
// Called for every landed line, before any filtering, because whose dispel
// it was doesn't matter.
func (e *Engine) noteSpellLandedForStrip(at time.Time, data logparser.SpellLandedData) {
	if data.Kind == logparser.SpellLandedKindYou || data.TargetName == "" {
		return
	}
	if normalizeNPCName(data.TargetName) == normalizeNPCName(e.activePlayerName()) {
		return
	}
	ids := make([]int, 0, len(data.Candidates)+1)
	if data.SpellID > 0 {
		ids = append(ids, data.SpellID)
	}
	for _, c := range data.Candidates {
		ids = append(ids, c.SpellID)
	}
	dispel := false
	for _, id := range ids {
		if e.isDispelSpell(id) {
			dispel = true
			break
		}
	}
	if !dispel {
		return
	}
	e.mu.Lock()
	events := e.noteDispelLandLocked(data.TargetName, at)
	e.mu.Unlock()
	e.emitStrips(events)
}

// isDispelSpell reports whether spell id has a Cancel Magic effect. Results
// are cached; the game database never changes at runtime.
func (e *Engine) isDispelSpell(id int) bool {
	if id <= 0 {
		return false
	}
	e.mu.Lock()
	v, ok := e.dispelSpells[id]
	e.mu.Unlock()
	if ok || e.db == nil {
		return v
	}
	sp, err := e.db.GetSpell(id)
	v = false
	if err == nil {
		for _, eff := range sp.EffectIDs {
			if eff == seCancelMagic {
				v = true
				break
			}
		}
	}
	e.mu.Lock()
	if e.dispelSpells == nil {
		e.dispelSpells = make(map[int]bool)
	}
	e.dispelSpells[id] = v
	e.mu.Unlock()
	return v
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

package spelltimer

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// Native charm timers.
//
// A charm never logs a land line for its caster: every charm spell's
// cast_on_other is empty, so the spell-landed pipeline can't see one take
// hold. Until now the only charm timer came from the Enchanter trigger pack,
// so a player without the pack had no charm countdown at all (and the pet
// overlay's charm bar sat on "no timer running").
//
// The engine now treats "You begin casting <charm>." as a pending charm and
// starts the timer once the cast time has passed with no resist, interrupt
// or did-not-take-hold line. With a live Zeal pet feed it also waits for a
// pet id to appear, so a charm that failed silently (target too high, already
// have a pet) doesn't start a phantom timer. A pack charm timer started for
// the same cast wins: the native one is skipped rather than duplicated.

// charmConfirmMargin is how long past a charm's cast time the engine waits
// for a resist/interrupt line before taking the charm as landed.
const charmConfirmMargin = 1500 * time.Millisecond

// charmPetWait bounds how much longer, with a live Zeal pet feed, the engine
// waits for the new pet's id before deciding the charm didn't take.
const charmPetWait = 3 * time.Second

// charmPetPoll is the recheck interval while waiting for that pet id.
const charmPetPoll = 500 * time.Millisecond

// pipePetFeedFresh is how recently SetPipePetID must have run for its "no
// pet" to count as evidence (it runs ~10 Hz while the pipe is connected).
const pipePetFeedFresh = 2 * time.Second

// pendingCharm is an in-flight charm cast.
type pendingCharm struct {
	seq      uint64
	spell    *db.Spell
	castAt   time.Time // log time of "You begin casting"
	target   string    // pipe target at cast time ("" without Zeal)
	targetID int
	// petDeadline is set on the first confirm that finds no pet id yet.
	petDeadline time.Time
}

// lookupCharmSpell resolves a cast line's spell name to its DB row when it is
// a charm. The log and quarm.db disagree on backtick vs apostrophe for some
// names ("Boltran`s Agacerie"), so both spellings are tried.
func (e *Engine) lookupCharmSpell(name string) *db.Spell {
	if e.db == nil || name == "" {
		return nil
	}
	names := []string{name}
	if alt := strings.ReplaceAll(name, "`", "'"); alt != name {
		names = append(names, alt)
	} else if alt := strings.ReplaceAll(name, "'", "`"); alt != name {
		names = append(names, alt)
	}
	for _, n := range names {
		spell, err := e.db.GetSpellByExactName(n)
		if err != nil {
			slog.Warn("spelltimer: DB error looking up charm spell", "name", n, "err", err)
			return nil
		}
		if spell != nil {
			if isCharmSpell(spell) {
				return spell
			}
			return nil
		}
	}
	return nil
}

// armCharm records a charm cast and schedules its confirmation. Non-charm
// casts are ignored: a charm that landed must not be cancelled by the next
// spell the player starts.
func (e *Engine) armCharm(castAt time.Time, spellName, target string, targetID int) {
	if e.trackingMode() == modeTriggersOnly {
		return
	}
	spell := e.lookupCharmSpell(spellName)
	if spell == nil {
		return
	}
	e.mu.Lock()
	e.charmSeq++
	seq := e.charmSeq
	e.pendingCharm = &pendingCharm{
		seq:      seq,
		spell:    spell,
		castAt:   castAt,
		target:   target,
		targetID: targetID,
	}
	e.mu.Unlock()
	delay := time.Duration(spell.CastTime)*time.Millisecond + charmConfirmMargin
	time.AfterFunc(delay, func() { e.confirmCharm(seq) })
}

// confirmCharm starts the timer for pending charm seq unless it was cancelled
// or superseded, or a live pet feed shows no pet arrived.
func (e *Engine) confirmCharm(seq uint64) {
	now := time.Now()
	e.mu.Lock()
	p := e.pendingCharm
	if p == nil || p.seq != seq {
		e.mu.Unlock()
		return
	}
	feedLive := !e.pipePetFeedAt.IsZero() && now.Sub(e.pipePetFeedAt) < pipePetFeedFresh
	if feedLive && e.pipePetID == nil {
		if p.petDeadline.IsZero() {
			p.petDeadline = now.Add(charmPetWait)
		}
		if now.Before(p.petDeadline) {
			e.mu.Unlock()
			time.AfterFunc(charmPetPoll, func() { e.confirmCharm(seq) })
			return
		}
		e.pendingCharm = nil
		e.mu.Unlock()
		slog.Debug("timer-debug: charm cast produced no pet; no timer", "spell", p.spell.Name)
		return
	}
	e.pendingCharm = nil
	// A pack trigger keyed on the same cast line already started a charm
	// timer for this cast.
	for _, t := range e.timers {
		if t.IsCharm && !t.CastAt.Before(p.castAt.Add(-dedupGraceWindow)) {
			e.mu.Unlock()
			return
		}
	}
	e.mu.Unlock()

	spell := p.spell
	landedAt := p.castAt.Add(time.Duration(spell.CastTime) * time.Millisecond)
	ticks := SpellDurationTicks(spell, e.casterLevelFor(spell, true, p.target, landedAt))
	if ticks <= 0 {
		return
	}
	durationSeconds := e.applyDurationModifiers(spell, float64(ticks)*eqTickSeconds)
	caster := e.activePlayerName()

	e.mu.Lock()
	key := timerKey(spell.Name, p.target)
	if p.target != "" && p.targetID != 0 {
		key = timerKey(spell.Name, p.target+targetIDKeySep+strconv.Itoa(p.targetID))
	}
	e.timers[key] = &ActiveTimer{
		ID:              key,
		SpellName:       spell.Name,
		SpellID:         spell.ID,
		Icon:            spell.NewIcon,
		TargetName:      p.target,
		Category:        categorize(spell),
		CastAt:          landedAt,
		StartsAt:        landedAt,
		ExpiresAt:       landedAt.Add(time.Duration(durationSeconds * float64(time.Second))),
		DurationSeconds: durationSeconds,
		IsCharm:         true,
		CasterCharacter: caster,
	}
	snap := e.snapshot(time.Now())
	e.mu.Unlock()

	slog.Debug("timer-debug: native charm timer started",
		"spell", spell.Name, "target", p.target, "duration_secs", durationSeconds)
	e.hub.Broadcast(ws.Event{Type: WSEventTimers, Data: snap})
}

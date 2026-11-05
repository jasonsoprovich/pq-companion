// Package selfstate tracks the parts of the player's own state that change
// how an NPC cons them: invisibility (and hide), illusions, faction buffs
// like Alliance, feign death, and being engaged with the NPC.
//
// The server's consider reply already accounts for all of these
// (Client::GetFactionLevel): an NPC that can't see the player, or a feigned
// player, reads indifferent whatever the real standing, and an NPC already
// fighting the player reads threatening. So a reading is only good while the
// player's state stays as it was when it was taken (the NPC overlay drops it
// on a change, see OnChange), and only a reading taken with none of them
// active says anything about the real standing (the faction tracker flags
// the rest as suspect, see State.Reason and EngagedWith).
//
// Buffs come from the Zeal pipe's buff-slot labels when it's connected. The
// log fills in what no buff shows (feign death, hide, Alliance's instant
// faction bonus, combat) and, without Zeal, signals that a relevant buff was
// cast or faded so a reading can be dropped.
package selfstate

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// SPAs that change how NPCs perceive the player.
const (
	spaInvisibility  = 12
	spaFaction       = 19 // SE_AddFaction: Alliance, Collaboration, Guide Alliance
	spaInvisUndead   = 28
	spaInvisAnimals  = 29
	spaIllusion      = 58
	spaFeignDeathSpa = 74 // the Feign Death spell (necromancer / SK)
)

// How long a log-only state is assumed to last. EQ logs feigning and hiding
// but not standing up or a hide broken by moving, so these bound how long a
// state the log can't see end is believed.
const (
	feignTTL = 2 * time.Minute
	hideTTL  = time.Minute
)

// buffLossConfirm is how long a buff must stay missing from the pipe's buff
// slots before it counts as gone, riding out a dropped or partial frame (the
// spell timer engine does the same, see spelltimer.pipeFadeConfirm).
const buffLossConfirm = 2 * time.Second

// engagedWindow is how long after the last melee exchange with an NPC the
// player still counts as engaged with it.
const engagedWindow = 15 * time.Second

var (
	reHidden   = regexp.MustCompile(`^You (?:have hidden|hide) yourself from view\.$`)
	reUnhidden = regexp.MustCompile(`^You are no longer hidden`)
	reAppear   = regexp.MustCompile(`^You feel yourself starting to appear\.$`)
)

// State is the player's con-affecting state at a moment.
type State struct {
	Invisible   bool // an invisibility buff of any kind (invis, IVU, IVA)
	Hidden      bool // rogue Hide or a hide effect, from the log
	Illusioned  bool // an illusion buff (pipe buff slots only)
	FactionBuff bool // a faction buff, or Alliance-type cast this zone
	Feigned     bool // feign death, from the log
}

// Reason names the first state that keeps a consider from being a clean read
// of the player's standing, or "" when there is none.
func (s State) Reason() string {
	switch {
	case s.Feigned:
		return "feigned"
	case s.Invisible:
		return "invisible"
	case s.Hidden:
		return "hidden"
	case s.Illusioned:
		return "illusioned"
	case s.FactionBuff:
		return "faction buff"
	}
	return ""
}

// EffectsResolver returns the SPA ids of the spell with the given name, best
// effort. ok is false when the name isn't a known spell.
type EffectsResolver func(spellName string) (effectIDs []int, ok bool)

// Tracker follows the player's con-affecting state. Safe for concurrent use.
type Tracker struct {
	resolve  EffectsResolver
	now      func() time.Time
	after    func(time.Duration, func())
	onChange func()

	mu      sync.Mutex
	effects map[string][]int // lowercased spell name → SPAs; nil entry = unknown

	buffKey                              string
	buffInvis, buffIllusion, buffFaction debounced

	feignedAt   time.Time // zero = not feigned
	hiddenAt    time.Time // zero = not hidden
	factionCast bool      // an Alliance-type spell was cast this zone

	engaged          map[string]time.Time // lowercased NPC name → last melee
	autoAttack       bool
	autoAttackTarget int // spawn id; 0 = none

	last State // state as of the last change notification
}

// New returns a Tracker resolving buff names with resolve (may be nil, which
// leaves every buff unrecognised).
func New(resolve EffectsResolver) *Tracker {
	return &Tracker{
		resolve: resolve,
		now:     time.Now,
		after:   func(d time.Duration, fn func()) { time.AfterFunc(d, fn) },
		effects: make(map[string][]int),
		engaged: make(map[string]time.Time),
	}
}

// OnChange registers fn to be called (outside the lock) whenever the state
// changes, or the log shows a spell that changes it being cast or fading.
// Engagement changes don't notify. Set it before feeding events.
func (t *Tracker) OnChange(fn func()) {
	t.mu.Lock()
	t.onChange = fn
	t.mu.Unlock()
}

// State returns the current state.
func (t *Tracker) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateLocked(t.now())
}

// EngagedWith reports whether the player is fighting the NPC: auto attack on
// with it targeted (pipe, by spawn id), or a melee exchange with an NPC of
// that name in the last few seconds (log).
func (t *Tracker) EngagedWith(name string, spawnID int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if spawnID > 0 && t.autoAttack && t.autoAttackTarget == spawnID {
		return true
	}
	at, ok := t.engaged[strings.ToLower(name)]
	return ok && t.now().Sub(at) < engagedWindow
}

// SetPipeBuffs records the player's current buff names from Zeal's buff-slot
// labels. Cheap to call on every label envelope: an unchanged set is a no-op.
func (t *Tracker) SetPipeBuffs(names []string) {
	key := strings.ToLower(strings.Join(names, "\x00"))
	t.mu.Lock()
	if key == t.buffKey {
		t.mu.Unlock()
		return
	}
	t.buffKey = key
	var invis, illusion, faction bool
	for _, n := range names {
		for _, e := range t.effectsLocked(n) {
			switch e {
			case spaInvisibility, spaInvisUndead, spaInvisAnimals:
				invis = true
			case spaIllusion:
				illusion = true
			case spaFaction:
				faction = true
			}
		}
	}
	now := t.now()
	lost := t.buffInvis.set(invis, now)
	lost = t.buffIllusion.set(illusion, now) || lost
	lost = t.buffFaction.set(faction, now) || lost
	if lost {
		t.after(buffLossConfirm, t.recheck)
	}
	fn := t.changedLocked(false)
	t.mu.Unlock()
	call(fn)
}

// SetPipeAutoAttack records the player's auto attack flag and current target
// spawn id (nil = none) from Zeal's player snapshot.
func (t *Tracker) SetPipeAutoAttack(on bool, targetID *int) {
	t.mu.Lock()
	t.autoAttack = on
	t.autoAttackTarget = 0
	if targetID != nil {
		t.autoAttackTarget = *targetID
	}
	t.mu.Unlock()
}

// ResetPipe drops the pipe-derived state when Zeal disconnects.
func (t *Tracker) ResetPipe() {
	t.mu.Lock()
	t.buffKey = ""
	t.buffInvis, t.buffIllusion, t.buffFaction = debounced{}, debounced{}, debounced{}
	t.autoAttack, t.autoAttackTarget = false, 0
	fn := t.changedLocked(false)
	t.mu.Unlock()
	call(fn)
}

// Handle processes a parsed log event.
func (t *Tracker) Handle(ev logparser.LogEvent) {
	t.mu.Lock()
	now := t.now()
	pulse := false
	switch ev.Type {
	case logparser.EventFeignDeath:
		t.feignedAt = now
		t.after(feignTTL, t.recheck)
	case logparser.EventCombatHit:
		if d, ok := ev.Data.(logparser.CombatHitData); ok {
			// A DoT or spell tick doesn't stand a feigned player up.
			melee := d.SpellName == "" && d.Skill != "spell" && d.Skill != "dot"
			t.noteCombatLocked(d.Actor, d.Target, melee, now)
		}
	case logparser.EventCombatMiss:
		if d, ok := ev.Data.(logparser.CombatMissData); ok {
			t.noteCombatLocked(d.Actor, d.Target, true, now)
		}
	case logparser.EventSpellCast:
		// Casting stands a feigned player up and breaks hide.
		t.feignedAt, t.hiddenAt = time.Time{}, time.Time{}
		if d, ok := ev.Data.(logparser.SpellCastData); ok {
			effects := t.effectsLocked(d.SpellName)
			if hasAny(effects, spaFaction) {
				t.factionCast = true
			}
			pulse = hasAny(effects, spaInvisibility, spaInvisUndead, spaInvisAnimals, spaIllusion)
			if hasAny(effects, spaFeignDeathSpa) {
				t.feignedAt = now
				t.after(feignTTL, t.recheck)
			}
		}
	case logparser.EventSpellFade:
		if d, ok := ev.Data.(logparser.SpellFadeData); ok {
			pulse = hasAny(t.effectsLocked(d.SpellName),
				spaInvisibility, spaInvisUndead, spaInvisAnimals, spaIllusion, spaFaction)
		}
	case logparser.EventIllusionFade:
		pulse = true
	case logparser.EventZone, logparser.EventDeath:
		t.feignedAt, t.hiddenAt = time.Time{}, time.Time{}
		t.factionCast = false
		t.engaged = make(map[string]time.Time)
	case logparser.EventKill:
		if d, ok := ev.Data.(logparser.KillData); ok {
			delete(t.engaged, strings.ToLower(d.Target))
		}
	default:
		t.mu.Unlock()
		return
	}
	fn := t.changedLocked(pulse)
	t.mu.Unlock()
	call(fn)
}

// HandleLine processes a raw log message for the lines no parsed event
// covers: hiding and invisibility starting to fade.
func (t *Tracker) HandleLine(msg string) {
	t.mu.Lock()
	pulse := false
	switch {
	case reHidden.MatchString(msg):
		t.hiddenAt = t.now()
		t.after(hideTTL, t.recheck)
	case reUnhidden.MatchString(msg):
		t.hiddenAt = time.Time{}
	case reAppear.MatchString(msg):
		pulse = true
	default:
		t.mu.Unlock()
		return
	}
	fn := t.changedLocked(pulse)
	t.mu.Unlock()
	call(fn)
}

// recheck re-evaluates the state once a log-only state may have timed out.
func (t *Tracker) recheck() {
	t.mu.Lock()
	fn := t.changedLocked(false)
	t.mu.Unlock()
	call(fn)
}

func (t *Tracker) noteCombatLocked(actor, target string, melee bool, now time.Time) {
	switch {
	case actor == "You" && target != "" && target != "You":
		t.engaged[strings.ToLower(target)] = now
		// Attacking stands a feigned player up and breaks hide.
		if melee {
			t.feignedAt, t.hiddenAt = time.Time{}, time.Time{}
		}
	case target == "You" && actor != "" && actor != "You":
		t.engaged[strings.ToLower(actor)] = now
	}
}

func (t *Tracker) stateLocked(now time.Time) State {
	return State{
		Invisible:   t.buffInvis.on(now),
		Hidden:      !t.hiddenAt.IsZero() && now.Sub(t.hiddenAt) < hideTTL,
		Illusioned:  t.buffIllusion.on(now),
		FactionBuff: t.buffFaction.on(now) || t.factionCast,
		Feigned:     !t.feignedAt.IsZero() && now.Sub(t.feignedAt) < feignTTL,
	}
}

// changedLocked records the current state and returns the change callback to
// run once the lock is released, or nil when nothing changed and pulse is
// false.
func (t *Tracker) changedLocked(pulse bool) func() {
	s := t.stateLocked(t.now())
	if s == t.last && !pulse {
		return nil
	}
	t.last = s
	return t.onChange
}

// effectsLocked returns the SPAs of the named spell, resolving each name once.
func (t *Tracker) effectsLocked(name string) []int {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil
	}
	if e, ok := t.effects[key]; ok {
		return e
	}
	var e []int
	if t.resolve != nil {
		e, _ = t.resolve(name)
	}
	t.effects[key] = e
	return e
}

// debounced is a buff flag from the pipe that turns on at once but only off
// once it has been missing for buffLossConfirm.
type debounced struct {
	cur    bool
	lostAt time.Time
}

// set records the latest snapshot's value, reporting whether it just went
// missing (so the caller can schedule a recheck).
func (d *debounced) set(v bool, now time.Time) (lost bool) {
	if v {
		d.cur, d.lostAt = true, time.Time{}
		return false
	}
	if d.cur {
		d.cur, d.lostAt = false, now
		return true
	}
	return false
}

func (d debounced) on(now time.Time) bool {
	return d.cur || (!d.lostAt.IsZero() && now.Sub(d.lostAt) < buffLossConfirm)
}

func hasAny(effects []int, spas ...int) bool {
	for _, e := range effects {
		for _, s := range spas {
			if e == s {
				return true
			}
		}
	}
	return false
}

func call(fn func()) {
	if fn != nil {
		fn()
	}
}
